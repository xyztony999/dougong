// Package dbmongo 是 MongoDB 引擎适配器（api-database-mgmt.md §4.5.1
// MongoGateway 契约承载；v0.1.4 反馈第 3 项增量，用户拍板 Q2=A）。
// 通道=进程内官方 mongo-driver v2（go.mongodb.org/mongo-driver/v2，纯 Go——
// 无外部二进制依赖，根除「宿主机客户端缺席致通道不可用」缺陷类；R-09 零外呼
// 维持：仅连本机实例；凭据经 options.Credential 结构化传递，无 URI 转义面）。
//
// 生命期语义（D-15）：每操作独立建连即断（不驻留连接池长连——与 dbmysql/
// dbredis 每次 exec 的通道语义对齐）；连接超时 5s/命令超时 30s 通道级防御。
//
// 错误分类（引擎协议概念止步于本包）：认证被拒（code 18/SCRAM 失败）/
// 操作被拒（code 13 Unauthorized）/连接层（ServerSelection/ConnError/超时）/
// 用户实况冲突（code 11 UserNotFound、51003 UserAlreadyExists）归一哨兵，
// service 层翻译 DBM_ 语义；其余 CommandError → ErrQueryFailed（文本携带原因）。
package dbmongo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// 哨兵错误（service 层翻译为 DBM 错误码语义；与 dbmysql/dbredis 同构命名）。
var (
	// ErrAuthRejected 认证被拒（SCRAM 失败/code 18）→ DBM_005。
	ErrAuthRejected = errors.New("dbmongo: authentication rejected")
	// ErrConnFailed 连接层失败（拨号/ServerSelection/超时）→ DBM_006 / DBM_003。
	ErrConnFailed = errors.New("dbmongo: connection failed")
	// ErrUnauthorized 操作被拒（code 13，凭据账户权限不足）→ DBM_022。
	ErrUnauthorized = errors.New("dbmongo: unauthorized")
	// ErrUserNotFound 用户不存在（code 11）→ DBM_009。
	ErrUserNotFound = errors.New("dbmongo: user not found")
	// ErrUserExists 同名用户已存在（code 51003）→ DBM_010。
	ErrUserExists = errors.New("dbmongo: user already exists")
	// ErrQueryFailed 命令执行失败（其它未分类引擎错误；文本携带原因）。
	ErrQueryFailed = errors.New("dbmongo: command failed")
)

// 通道级超时（D-15）。
const (
	connectTimeout = 5 * time.Second
	commandTimeout = 30 * time.Second
	// AuthSourceFixed 凭据验证与建连固定 admin 库（D-16）。
	AuthSourceFixed = "admin"
	// RoleRead/RoleReadWrite/RoleDBAdmin 是 mongo_role 三档原生角色名（D-19）。
	RoleRead      = "read"
	RoleReadWrite = "readWrite"
	RoleDBAdmin   = "dbAdmin"
)

// Gateway 是 MongoDB mongo-driver 网关（无二进制依赖——Available 恒 nil，
// D-15：通道不可用哨兵不适用于本通道）。
type Gateway struct{}

// NewGateway 构造网关（进程内库通道，无构造期探测）。
func NewGateway() *Gateway { return &Gateway{} }

// Available 通道可用性（进程内库恒可用）。
func (g *Gateway) Available() error { return nil }

// DatabaseInfo 是库清单条目（§4.5.2 mdb/list items {name, sizeBytes, collections}）。
type DatabaseInfo struct {
	Name        string
	SizeBytes   int64
	Collections int64
}

// UserItem 是用户清单条目（§4.5.2 muser/list items；密码/凭证零出口 I-02）。
type UserItem struct {
	Username   string
	Mechanisms []string
	Roles      []UserRole
}

// UserRole 是用户持有的库级角色（{db, role}）。
type UserRole struct {
	DB   string
	Role string
}

// StatusSummary 是 serverStatus 白名单摘要（§4.5.2 status data；D-08 同构 ≤10 项）。
type StatusSummary struct {
	Version              string
	UptimeSec            int64
	ConnectionsCurrent   int64
	ConnectionsAvailable int64
	MemResidentMb        int64
	OpInsert             int64
	OpQuery              int64
	OpUpdate             int64
	OpDelete             int64
}

// MongoRoleValid 校验 mongo_role 取值（D-19 三档；service 白名单前置后双保险）。
func MongoRoleValid(role string) bool {
	return role == RoleRead || role == RoleReadWrite || role == RoleDBAdmin
}

// connectURI 组装连接 URI：tcp=mongodb://host:port；socket=mongodb://<percent-
// 编码路径>（驱动约定——unix socket 路径需整体转义；凭据不进 URI）。
func connectURI(t dbconn.Target) string {
	if t.ConnMode == "socket" {
		return "mongodb://" + url.PathEscape(t.SocketPath)
	}
	return "mongodb://" + net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))
}

// dial 建立一次性客户端（每操作独立建连即断，D-15）。无凭据形态用于探测层
// 传输可达判定外的引擎操作场景（面板路径恒带凭据）。
func (g *Gateway) dial(t dbconn.Target, c dbconn.Creds) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(connectURI(t)).
		SetConnectTimeout(connectTimeout).
		SetTimeout(commandTimeout).
		SetRetryWrites(false) // 单次命令语义，不做驱动层重试（通道防御自持）
	if c.Username != "" {
		opts.SetAuth(options.Credential{
			AuthSource:  AuthSourceFixed,
			Username:    c.Username,
			Password:    c.Password,
			PasswordSet: c.Password != "",
		})
	}
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, classify(err)
	}
	return client, nil
}

// withClient 执行单命令编排：建连 → op → 断连（defer Disconnect；op 错误经
// classify 归一哨兵）。
func (g *Gateway) withClient(t dbconn.Target, c dbconn.Creds,
	op func(ctx context.Context, cl *mongo.Client) error) error {
	cl, err := g.dial(t, c)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Disconnect(context.Background()) }()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	return classify(op(ctx, cl))
}

// Ping 连接验证（credentials 编排，D-16：SCRAM + admin 库；ping 往返即可）。
func (g *Gateway) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Ping(ctx, nil)
	})
}

// ServerVersion 返回 buildInfo.version（档案 version 刷新，D-04）。
func (g *Gateway) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	var ver string
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		var res struct {
			Version string `bson:"version"`
		}
		if err := cl.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&res); err != nil {
			return err
		}
		ver = res.Version
		return nil
	})
	return ver, err
}

// ListDatabases 库清单只读投影（ListDatabases + 逐库 dbStats 集合数；D-18：
// 系统库 admin/config/local 如实返回）。
func (g *Gateway) ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]DatabaseInfo, error) {
	var out []DatabaseInfo
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		res, err := cl.ListDatabases(ctx, bson.D{})
		if err != nil {
			return err
		}
		for _, spec := range res.Databases {
			info := DatabaseInfo{Name: spec.Name, SizeBytes: spec.SizeOnDisk}
			var stats struct {
				Collections int64 `bson:"collections"`
			}
			if serr := cl.Database(spec.Name).RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}, {Key: "scale", Value: 1}}).Decode(&stats); serr == nil {
				info.Collections = stats.Collections
			} // dbStats 失败不阻断清单（集合数置 0 如实降级）
			out = append(out, info)
		}
		return nil
	})
	return out, err
}

// StatusSummary serverStatus 白名单摘要（§4.5.2； Unauthorized（code 13）经
// classify → ErrUnauthorized → DBM_022——凭据账户权限不足如实呈现）。
func (g *Gateway) StatusSummary(ctx context.Context, t dbconn.Target, c dbconn.Creds) (*StatusSummary, error) {
	var sum *StatusSummary
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		var res struct {
			Version     string `bson:"version"`
			Uptime      int64  `bson:"uptime"`
			Connections struct {
				Current   int64 `bson:"current"`
				Available int64 `bson:"available"`
			} `bson:"connections"`
			Mem struct {
				Resident int64 `bson:"resident"`
			} `bson:"mem"`
			Opcounters struct {
				Insert int64 `bson:"insert"`
				Query  int64 `bson:"query"`
				Update int64 `bson:"update"`
				Delete int64 `bson:"delete"`
			} `bson:"opcounters"`
		}
		if err := cl.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&res); err != nil {
			return err
		}
		sum = &StatusSummary{
			Version:              res.Version,
			UptimeSec:            res.Uptime,
			ConnectionsCurrent:   res.Connections.Current,
			ConnectionsAvailable: res.Connections.Available,
			MemResidentMb:        res.Mem.Resident,
			OpInsert:             res.Opcounters.Insert,
			OpQuery:              res.Opcounters.Query,
			OpUpdate:             res.Opcounters.Update,
			OpDelete:             res.Opcounters.Delete,
		}
		return nil
	})
	return sum, err
}

// ListUsers 用户清单（usersInfo:1 全实例用户实况投影；密码/凭证零出口 I-02）。
func (g *Gateway) ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]UserItem, error) {
	var out []UserItem
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		var res struct {
			Users []struct {
				Username   string `bson:"user"`
				DB         string `bson:"db"`
				Mechanisms []struct {
					Mechanism string `bson:"mechanism"`
				} `bson:"mechanisms"`
				Roles []struct {
					DB   string `bson:"db"`
					Role string `bson:"role"`
				} `bson:"roles"`
			} `bson:"users"`
		}
		if err := cl.Database(AuthSourceFixed).RunCommand(ctx,
			bson.D{{Key: "usersInfo", Value: 1}, {Key: "showCredentials", Value: false}}).Decode(&res); err != nil {
			return err
		}
		for _, u := range res.Users {
			item := UserItem{Username: u.Username, Roles: []UserRole{}}
			for _, m := range u.Mechanisms {
				item.Mechanisms = append(item.Mechanisms, m.Mechanism)
			}
			for _, r := range u.Roles {
				item.Roles = append(item.Roles, UserRole{DB: r.DB, Role: r.Role})
			}
			out = append(out, item)
		}
		return nil
	})
	return out, err
}

// CreateUser 建用户（admin 库 db.createUser + 首库首角色；D-19；重名 →
// ErrUserExists（code 51003）——MongoDB 用户名唯一域=authSource 库，面板
// 固定 admin（D-16））。
func (g *Gateway) CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, password, db, role string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(AuthSourceFixed).RunCommand(ctx, bson.D{
			{Key: "createUser", Value: username},
			{Key: "pwd", Value: password},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: db}}}},
		}).Err()
	})
}

// DropUser 删用户（连带回收其全部角色，引擎语义；不存在 → ErrUserNotFound）。
func (g *Gateway) DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(AuthSourceFixed).RunCommand(ctx, bson.D{
			{Key: "dropUser", Value: username},
		}).Err()
	})
}

// GrantRole 授予库级角色（grantRolesToUser；幂等——重复授予结果一致，D-19）。
func (g *Gateway) GrantRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(AuthSourceFixed).RunCommand(ctx, bson.D{
			{Key: "grantRolesToUser", Value: username},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: db}}}},
		}).Err()
	})
}

// RevokeRole 回收库级角色（revokeRolesFromUser）。⚠️ 授权关系不存在 → 引擎
// 不必然报错（部分版本静默成功）——DBM_011 语义由 service 层经 ListUsers 前置
// 校验承载（区别于 MySQL 侧引擎报错分类路径）。
func (g *Gateway) RevokeRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(AuthSourceFixed).RunCommand(ctx, bson.D{
			{Key: "revokeRolesFromUser", Value: username},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: db}}}},
		}).Err()
	})
}

// classify 将 mongo-driver 错误归一哨兵（错误码优先，文本兜底——
// ServerSelectionError 对认证失败可能包裹为选服失败，需文本穿透）。
func classify(err error) error {
	if err == nil {
		return nil
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		switch cmdErr.Code {
		case 18: // AuthenticationFailed
			return fmt.Errorf("%w: %s", ErrAuthRejected, firstLine(cmdErr.Message))
		case 13: // Unauthorized（权限不足——身份对但无权执行）
			return fmt.Errorf("%w: %s", ErrUnauthorized, firstLine(cmdErr.Message))
		case 11: // UserNotFound
			return fmt.Errorf("%w: %s", ErrUserNotFound, firstLine(cmdErr.Message))
		case 51003: // UserAlreadyExists
			return fmt.Errorf("%w: %s", ErrUserExists, firstLine(cmdErr.Message))
		}
		return fmt.Errorf("%w: %s", ErrQueryFailed, firstLine(cmdErr.Message))
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "auth error"), strings.Contains(text, "AuthenticationFailed"):
		return fmt.Errorf("%w: %s", ErrAuthRejected, firstLine(text))
	}
	// 连接层：驱动网络错误助手 + ServerSelection/ConnectionError 文本形态 +
	// net.Error/超时（v2 顶层包不导出拓扑错误类型，文本判定免 x/ 内部包依赖）。
	if mongo.IsNetworkError(err) ||
		strings.Contains(text, "server selection error") ||
		strings.Contains(text, "connection error") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no servers found") ||
		strings.Contains(text, "timed out") {
		return fmt.Errorf("%w: %s", ErrConnFailed, firstLine(text))
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s", ErrConnFailed, firstLine(text))
	}
	return fmt.Errorf("%w: %s", ErrQueryFailed, firstLine(text))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
