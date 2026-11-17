package dbmongo

import (
	"context"
	"errors"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// BootstrapAdminUser 是 MongoBootstrap 引导窄接口实现（api-database-mgmt.md
// §4.6/D-20，安装后配置就绪增量）：免认证连 localhost:port（localhost exception
// ——无用户时可经 localhost 免认证建首个 admin 用户）→ admin 库 createUser
// username root 角色随机密码。
//
// 幂等保守：已有用户/已开鉴权（createUser 回 Unauthorized code 13 / 认证失败
// code 18）或同名用户已存在（code 51003）→ exists=true 保守跳过（不覆盖，
// ⚠️A13）；建连/命令其它失败 → err（service 侧 bootstrap_status=failed）。
// 凭据经 options.Credential 结构化（此处免认证无凭据）；root 角色为 MongoDB
// 内置超级用户角色（admin 库）。
func (g *Gateway) BootstrapAdminUser(ctx context.Context, host string, port int, username, password string) (bool, error) {
	t := dbconn.Target{Engine: "mongodb", ConnMode: "tcp", Host: host, Port: port}
	noAuth := dbconn.Creds{} // localhost exception 免认证引导通道
	err := g.withClient(t, noAuth, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(AuthSourceFixed).RunCommand(ctx, bson.D{
			{Key: "createUser", Value: username},
			{Key: "pwd", Value: password},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: AuthSourceFixed}}}},
		}).Err()
	})
	if err == nil {
		return false, nil // 创建成功
	}
	if errors.Is(err, ErrUserExists) || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrAuthRejected) {
		return true, nil // 已有用户/已开鉴权，保守跳过
	}
	return false, err
}
