package dbmysql

import (
	"context"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// BootstrapAdminUser 是 MySQLBootstrap 引导窄接口实现（api-database-mgmt.md
// §4.6/D-20，安装后配置就绪增量）：经 unix socket 以 root auth_socket 免密建连
// （引导通道，区别于 TCP+密码治理通道——面板以 root 运行，mysql 客户端子进程
// 亦 root，auth_socket 匹配 OS 用户免密；MYSQL_PWD 空注入 auth_socket 忽略）。
//
// 幂等保守：dougong@localhost 已存在 → exists=true（不创建、不重置密码，
// ⚠️A13）；否则 CREATE USER + GRANT ALL PRIVILEGES ON *.* WITH GRANT OPTION
// （Q1/Q2 拍板：dougong@localhost 单账户全权，支撑 db-mgmt 三档授权 all 档）。
// 标识符/密码经 quoteUser/quoteString 转义（A-DB3）；命令字面量参数数组（R-08）。
func (g *Gateway) BootstrapAdminUser(ctx context.Context, socketPath, username, password string) (bool, error) {
	t := dbconn.Target{Engine: "mysql", ConnMode: "socket", SocketPath: socketPath}
	root := dbconn.Creds{Username: "root", Password: ""} // auth_socket 免密引导通道
	out, err := g.query(ctx, t, root,
		"SELECT COUNT(*) FROM mysql.user WHERE user="+quoteString(username)+" AND host="+quoteString("localhost"))
	if err != nil {
		return false, err
	}
	if n := strings.TrimSpace(out); n != "" && n != "0" {
		return true, nil // 账户已存在，保守跳过（不覆盖用户已有配置）
	}
	userHost := quoteUser(username, "localhost")
	if _, err := g.query(ctx, t, root,
		"CREATE USER "+userHost+" IDENTIFIED BY "+quoteString(password)); err != nil {
		return false, err
	}
	if _, err := g.query(ctx, t, root,
		"GRANT ALL PRIVILEGES ON *.* TO "+userHost+" WITH GRANT OPTION"); err != nil {
		return false, err
	}
	if _, err := g.query(ctx, t, root, "FLUSH PRIVILEGES"); err != nil {
		return false, err
	}
	return false, nil
}
