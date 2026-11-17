// Package dbmysql 是 MySQL/MariaDB 引擎适配器（api-database-mgmt.md §4.1.5
// DbGateway 契约 MySQL 族方法承载；idea.md §5.3 dbmysql 落位）。通道=本机
// mysql / mysqldump CLI 客户端：全部外部命令经 exec.CommandContext 字面量
// 参数数组执行（R-08/D-06：无 shell 无拼接），二进制路径构造期 exec.LookPath
// 解析（backend-03 §4）；密码经 MYSQL_PWD 环境变量注入子进程（不进 argv，
// G-03 纪律——ps 不可见）。
//
// 引擎方言与错误分类（引擎协议概念止步于本包）：认证被拒/连接失败/对象
// 实况冲突归一为哨兵错误（ErrAuthRejected/ErrConnFailed/ErrDatabaseExists
// ...），由 service 层翻译 DBM_ 语义；SQL 组装侧标识符一律白名单+引用、
// 自由文本走 quoteString 转义（A-DB3）。
package dbmysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/dougong/dougong/internal/adapter/dbconn"
	"os/exec"
	"strings"
	"time"
)

// 哨兵错误（service 层翻译为 DBM_ 错误码语义）。
var (
	// ErrAuthRejected 认证被拒（用户名/密码错误）→ DBM_005。
	ErrAuthRejected = errors.New("dbmysql: authentication rejected")
	// ErrConnFailed 网络/socket 层连接失败 → DBM_006 / DBM_003。
	ErrConnFailed = errors.New("dbmysql: connection failed")
	// ErrUnavailable 客户端二进制未检出/无法执行（通道不可用）。
	ErrUnavailable = errors.New("dbmysql: client binary unavailable")
	// ErrDatabaseExists 同名库已存在（实况）→ DBM_008。
	ErrDatabaseExists = errors.New("dbmysql: database already exists")
	// ErrDatabaseNotFound 库不存在（实况）→ DBM_007。
	ErrDatabaseNotFound = errors.New("dbmysql: database not found")
	// ErrUserExists 同名同 host 用户已存在（实况）→ DBM_010。
	ErrUserExists = errors.New("dbmysql: user already exists")
	// ErrUserNotFound 用户不存在（实况）→ DBM_009。
	ErrUserNotFound = errors.New("dbmysql: user not found")
	// ErrGrantMissing revoke 目标授权关系不存在（实况）→ DBM_011。
	ErrGrantMissing = errors.New("dbmysql: grant not found")
	// ErrQueryFailed 语句执行失败（其它未分类引擎错误；文本携带原因）。
	ErrQueryFailed = errors.New("dbmysql: statement failed")
)

// queryTimeout 单次 mysql 客户端查询/命令的执行上限（通道级防御；mysqldump
// 导出与导入为长操作，受调用方 ctx 控制另计）。
const queryTimeout = 30 * time.Second

// Gateway 是 MySQL/MariaDB CLI 网关（mysqlBin/dumpBin 通常为 "mysql"/
// "mysqldump"，LookPath 解析；含路径分隔符时视为显式路径直接校验——
// nginxx 同法）。
type Gateway struct {
	mysqlBin  string
	dumpBin   string
	lookupErr error
}

// NewGateway 构造网关；任一二进制缺席时 lookupErr 承载原因（调用方呈现）。
func NewGateway(mysqlBin, dumpBin string) *Gateway {
	g := &Gateway{mysqlBin: mysqlBin, dumpBin: dumpBin}
	if p, err := resolveBinary(mysqlBin); err != nil {
		g.lookupErr = err
		return g
	} else {
		g.mysqlBin = p
	}
	if p, err := resolveBinary(dumpBin); err != nil {
		g.lookupErr = err
		return g
	} else {
		g.dumpBin = p
	}
	return g
}

// Available 返回通道可用性（二进制未检出时携带原因）。
func (g *Gateway) Available() error { return g.lookupErr }

// DatabaseInfo 是实况库清单条目（§4.2.1 投影字段 {name, charset}）。
type DatabaseInfo struct {
	Name    string
	Charset string
}

// GrantLine 是 SHOW GRANTS 单行解析结果（DB=""* 表示全局 *.*；DB="" 表示
// 未识别形态——raw 保留供呈现，D-10「未识别不呈现不回收」判据）。
type GrantLine struct {
	DB  string
	Raw string
}

// UserGrants 是实况用户清单条目（§4.3.1 投影 {username, host, grants}）。
type UserGrants struct {
	Username string
	Host     string
	Grants   []GrantLine
}

// ProcessRow 是 SHOW PROCESSLIST 单行（§4.1.4 processlist items；db/state/
// info 可为 NULL → nil）。
type ProcessRow struct {
	ID      int64
	User    string
	Host    string
	DB      *string
	Command string
	TimeSec int64
	State   *string
	Info    *string
}

// connArgs 组装连接参数（不含二进制名；字面量参数数组 R-08）。tcp 模式显式
// --protocol=TCP——mysql 客户端对 -h localhost 特例走编译默认 Unix socket
// 并忽略 -P，会将 tcp 语义静默改道（v0.1.4 修复轮：容器/远端 TCP 实例必须
// 强制 TCP，localhost 字面主机不再歧义）。
func connArgs(t dbconn.Target) []string {
	args := make([]string, 0, 8)
	if t.ConnMode == "socket" {
		args = append(args, "--socket", t.SocketPath)
	} else {
		args = append(args, "-h", t.Host, "-P", fmt.Sprintf("%d", t.Port), "--protocol=TCP")
	}
	return args
}

// Ping 连接验证（§4.1.5：认证/网络错误归一为 DBM_005/006 语义哨兵）。
// 以 SELECT 1 建立完整协议往返（认证+库选择语义俱全）。
func (g *Gateway) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	_, err := g.query(ctx, t, c, "SELECT 1")
	return err
}

// ServerVersion 返回 SELECT VERSION() 实况版本串（credentials/rescan 编排
// 刷新档案 version 用，D-04）。
func (g *Gateway) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	out, err := g.query(ctx, t, c, "SELECT VERSION()")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", fmt.Errorf("dbmysql: empty version result")
	}
	return strings.TrimSpace(lines[0]), nil
}

// ListDatabases 实况库清单（I-04 每请求实时；含系统库如实返回）。
func (g *Gateway) ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]DatabaseInfo, error) {
	out, err := g.query(ctx, t, c,
		"SELECT schema_name, default_character_set_name FROM information_schema.SCHEMATA ORDER BY schema_name")
	if err != nil {
		return nil, err
	}
	var out2 []DatabaseInfo
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		out2 = append(out2, DatabaseInfo{
			Name:    unescapeTSV(cols[0]),
			Charset: unescapeTSV(cols[1]),
		})
	}
	return out2, nil
}

// DatabaseExists 报告库实况存在性（DBM_007/008 前置校验）。
func (g *Gateway) DatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) (bool, error) {
	dbs, err := g.ListDatabases(ctx, t, c)
	if err != nil {
		return false, err
	}
	for _, db := range dbs {
		if db.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// CreateDatabase 建库（name/charset 已过 service 白名单校验 R-14；重名 →
// ErrDatabaseExists 实况哨兵）。
func (g *Gateway) CreateDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name, charset string) error {
	_, err := g.query(ctx, t, c,
		"CREATE DATABASE "+quoteIdent(name)+" CHARACTER SET "+quoteIdent(charset))
	return classifyDDL(err, "database exists", ErrDatabaseExists)
}

// DropDatabase 删库（不存在 → ErrDatabaseNotFound；不静默成功）。
func (g *Gateway) DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) error {
	_, err := g.query(ctx, t, c, "DROP DATABASE "+quoteIdent(name))
	return classifyDDL(err, "doesn't exist", ErrDatabaseNotFound)
}

// ShowCharsets 字符集实况候选（D-11 开放集合）。
func (g *Gateway) ShowCharsets(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]string, error) {
	out, err := g.query(ctx, t, c, "SHOW CHARACTER SET")
	if err != nil {
		return nil, err
	}
	var out2 []string
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) >= 1 && cols[0] != "" {
			out2 = append(out2, unescapeTSV(cols[0]))
		}
	}
	return out2, nil
}

// ListUsers 实况用户清单（含逐用户 SHOW GRANTS 投影；零台账 I-04）。
func (g *Gateway) ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]UserGrants, error) {
	out, err := g.query(ctx, t, c,
		"SELECT user, host FROM mysql.user ORDER BY user, host")
	if err != nil {
		return nil, err
	}
	var users []UserGrants
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		users = append(users, UserGrants{
			Username: unescapeTSV(cols[0]),
			Host:     unescapeTSV(cols[1]),
		})
	}
	for i := range users {
		lines, gerr := g.ShowGrants(ctx, t, c, users[i].Username, users[i].Host)
		if gerr != nil {
			return nil, gerr
		}
		users[i].Grants = lines
	}
	return users, nil
}

// ShowGrants 返回指定用户的授权行（D-10 反向映射数据源；用户不存在 →
// ErrUserNotFound）。
func (g *Gateway) ShowGrants(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) ([]GrantLine, error) {
	out, err := g.query(ctx, t, c,
		"SHOW GRANTS FOR "+quoteUser(username, host))
	if err != nil {
		if errors.Is(err, ErrQueryFailed) && strings.Contains(err.Error(), "does not exist") {
			return nil, fmt.Errorf("%w: %s@%s", ErrUserNotFound, username, host)
		}
		return nil, err
	}
	var lines []GrantLine
	for _, line := range splitTSV(out) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, parseGrantLine(unescapeTSV(line)))
	}
	return lines, nil
}

// CreateUser 建用户（password 止步于本包——组入转义 SQL 后即弃，I-02；
// 同名同 host → ErrUserExists）。
func (g *Gateway) CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, password string) error {
	_, err := g.query(ctx, t, c,
		"CREATE USER "+quoteUser(username, host)+" IDENTIFIED BY "+quoteString(password))
	if err != nil && errors.Is(err, ErrQueryFailed) &&
		(strings.Contains(err.Error(), "Operation CREATE USER failed") ||
			strings.Contains(err.Error(), "already exists")) {
		return fmt.Errorf("%w: %s@%s", ErrUserExists, username, host)
	}
	return err
}

// DropUser 删用户（引擎语义连带回收授权；不存在 → ErrUserNotFound）。
func (g *Gateway) DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) error {
	_, err := g.query(ctx, t, c, "DROP USER "+quoteUser(username, host))
	if err != nil && errors.Is(err, ErrQueryFailed) &&
		(strings.Contains(err.Error(), "Operation DROP USER failed") ||
			strings.Contains(err.Error(), "does not exist")) {
		return fmt.Errorf("%w: %s@%s", ErrUserNotFound, username, host)
	}
	return err
}

// GrantLevel 权限档位常量（D-10 正向映射：grant_level → 引擎权限串）。
const (
	LevelReadonly = "readonly"
	LevelWrite    = "write"
	LevelAll      = "all"
)

// levelPrivileges 是 D-10 常量表（库级 db.* 语义）。
var levelPrivileges = map[string]string{
	LevelReadonly: "SELECT",
	LevelWrite:    "SELECT, INSERT, UPDATE, DELETE",
	LevelAll:      "ALL PRIVILEGES",
}

// Grant 库级授权（level ∈ readonly|write|all，D-10 三档；幂等——引擎 GRANT
// 重复执行结果一致）。
func (g *Gateway) Grant(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db, level string) error {
	privs, ok := levelPrivileges[level]
	if !ok {
		return fmt.Errorf("dbmysql: unknown grant level %q", level)
	}
	_, err := g.query(ctx, t, c,
		"GRANT "+privs+" ON "+quoteIdent(db)+".* TO "+quoteUser(username, host))
	return err
}

// Revoke 按库回收三档权限（授权关系不存在 → ErrGrantMissing，DBM_011）。
func (g *Gateway) Revoke(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db string) error {
	_, err := g.query(ctx, t, c,
		"REVOKE ALL PRIVILEGES ON "+quoteIdent(db)+".* FROM "+quoteUser(username, host))
	if err != nil && errors.Is(err, ErrQueryFailed) &&
		strings.Contains(err.Error(), "no such grant") {
		return fmt.Errorf("%w: %s@%s on %s", ErrGrantMissing, username, host, db)
	}
	return err
}

// AlterRootPassword 修改当前连接账户密码（ALTER USER USER()——A-DB8：改的
// 就是面板建连所用账户，与 D-05 两步同步语义精确对齐；newPassword 组入
// 转义 SQL 后即弃）。
func (g *Gateway) AlterRootPassword(ctx context.Context, t dbconn.Target, c dbconn.Creds, newPassword string) error {
	_, err := g.query(ctx, t, c,
		"ALTER USER USER() IDENTIFIED BY "+quoteString(newPassword))
	return err
}

// StatusVariables 全量 SHOW VARIABLES + SELECT VERSION()（D-08 白名单过滤
// 在 service 层——本包不感知呈现口径；I-04 实时读取不落库）。
func (g *Gateway) StatusVariables(ctx context.Context, t dbconn.Target, c dbconn.Creds) (version string, vars map[string]string, err error) {
	ver, err := g.ServerVersion(ctx, t, c)
	if err != nil {
		return "", nil, err
	}
	out, err := g.query(ctx, t, c, "SHOW VARIABLES")
	if err != nil {
		return "", nil, err
	}
	vars = map[string]string{}
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		vars[unescapeTSV(cols[0])] = unescapeTSV(cols[1])
	}
	return ver, vars, nil
}

// ProcessList 进程列表实时快照（全量返回——连接数量级小，D-09）。
func (g *Gateway) ProcessList(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]ProcessRow, error) {
	out, err := g.query(ctx, t, c, "SHOW PROCESSLIST")
	if err != nil {
		return nil, err
	}
	var rows []ProcessRow
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 7 {
			continue
		}
		row := ProcessRow{
			User:    unescapeTSV(cols[1]),
			Host:    unescapeTSV(cols[2]),
			Command: unescapeTSV(cols[4]),
			State:   tsvStringPtr(cols[6]),
		}
		fmt.Sscanf(cols[0], "%d", &row.ID)
		fmt.Sscanf(cols[5], "%d", &row.TimeSec)
		row.DB = tsvStringPtr(cols[3])
		if len(cols) >= 8 {
			row.Info = tsvStringPtr(cols[7])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// query 执行单条语句并返回 stdout（mysql --batch -N -e <sql>；字面量参数
// 数组 R-08；密码经 MYSQL_PWD env 注入）。错误分类：Access denied →
// ErrAuthRejected；连接层 → ErrConnFailed；退出码非 0 其余 → ErrQueryFailed
// （文本携带引擎错误原因）。
func (g *Gateway) query(ctx context.Context, t dbconn.Target, c dbconn.Creds, stmt string) (string, error) {
	if g.lookupErr != nil {
		return "", g.lookupErr
	}
	cctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	args := append(connArgs(t),
		"-u", c.Username, "--batch", "-N", "-e", stmt) // 字面量参数数组，无拼接
	cmd := exec.CommandContext(cctx, g.mysqlBin, args...)
	cmd.Env = append(cmdEnv(), "MYSQL_PWD="+c.Password) // 凭据进 env 不进 argv
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", classifyRun(err, stderr.String(), stdout.String())
	}
	return stdout.String(), nil
}

// classifyRun 将子进程执行结果归一为哨兵错误。
func classifyRun(runErr error, stderr, stdout string) error {
	detail := stderr + stdout
	switch {
	case strings.Contains(detail, "Access denied") || strings.Contains(detail, "using password: NO"):
		return fmt.Errorf("%w: %s", ErrAuthRejected, firstLine(detail))
	case strings.Contains(detail, "Can't connect") ||
		strings.Contains(detail, "Can't connect to MySQL server") ||
		strings.Contains(detail, "Lost connection") ||
		strings.Contains(detail, "Connection refused"):
		return fmt.Errorf("%w: %s", ErrConnFailed, firstLine(detail))
	default:
		return fmt.Errorf("%w: %s: %v", ErrQueryFailed, firstLine(detail), runErr)
	}
}

// classifyDDL 在语句失败文本含 marker 时替换为实况哨兵（DDL 存在性冲突）。
func classifyDDL(err error, marker string, sent error) error {
	if err != nil && errors.Is(err, ErrQueryFailed) && strings.Contains(err.Error(), marker) {
		return sent
	}
	return err
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

// resolveBinary 解析二进制路径（名字 → LookPath；含分隔符 → 显式路径校验）。
func resolveBinary(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty binary name", ErrUnavailable)
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %q not found in PATH: %v", ErrUnavailable, name, err)
	}
	return p, nil
}
