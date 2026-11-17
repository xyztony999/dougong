package dbredis

import (
	"context"
	"fmt"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// BootstrapRequirepass 是 RedisBootstrap 引导窄接口实现（api-database-mgmt.md
// §4.6/D-20，安装后配置就绪增量，Q4 拍板 CONFIG SET+REWRITE）：免认证连
// localhost:port（fresh install 无 requirepass）→ CONFIG GET requirepass 判已设
// → alreadySet=true 保守跳过（不覆盖用户密码，⚠️A13）；否则 CONFIG SET
// requirepass 随机密码 + CONFIG REWRITE 持久化（新密码认证）。bind 经 CONFIG GET
// 校验本机（fresh install 默认 127.0.0.1 ::1；bind 非运行时可改参数，仅校验告警）。
//
// 返回 (alreadySet, bindLoopback, err)：alreadySet=已设 requirepass 跳过；
// bindLoopback=bind 仅含 loopback 地址（service 侧非 loopback 时告警注记）。
// 密码经 REDISCLI_AUTH env 注入不进 argv（G-03）；命令字面量参数数组（R-08）。
func (g *Gateway) BootstrapRequirepass(ctx context.Context, host string, port int, newPassword string) (bool, bool, error) {
	t := dbconn.Target{Engine: "redis", ConnMode: "tcp", Host: host, Port: port}
	noAuth := dbconn.Creds{}
	out, err := g.run(ctx, t, noAuth, "CONFIG", "GET", "requirepass")
	if err != nil {
		return false, false, err
	}
	bindOut, err := g.run(ctx, t, noAuth, "CONFIG", "GET", "bind")
	if err != nil {
		return false, false, err
	}
	loopback := isLoopbackBind(configGetValue(bindOut, "bind"))
	if configGetValue(out, "requirepass") != "" {
		return true, loopback, nil // 已设置 requirepass，保守跳过
	}
	// CONFIG SET requirepass（免认证连接仍有效——requirepass 尚未设置）。
	setOut, err := g.run(ctx, t, noAuth, "CONFIG", "SET", "requirepass", newPassword)
	if err != nil {
		return false, false, err
	}
	if strings.TrimSpace(setOut) != "OK" {
		return false, false, fmt.Errorf("%w: CONFIG SET requirepass reply %q", ErrCommandFailed, firstLine(setOut))
	}
	// CONFIG REWRITE 持久化（requirepass 已设，后续命令须新密码认证）。
	newCreds := dbconn.Creds{Password: newPassword}
	if _, err := g.run(ctx, t, newCreds, "CONFIG", "REWRITE"); err != nil {
		return false, false, err
	}
	return false, loopback, nil
}

// configGetValue 解析 CONFIG GET 输出（--raw 形态 field/value 交替行）取指定
// 参数值（纯函数可测）。
func configGetValue(out, field string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == field {
			return strings.TrimSpace(lines[i+1])
		}
	}
	return ""
}

// isLoopbackBind 判定 bind 值是否仅含 loopback 地址（127.0.0.1/::1/localhost）；
// 空值（监听全部接口）或含 0.0.0.0/* 等判非本机（纯函数可测）。
func isLoopbackBind(bind string) bool {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return false
	}
	for _, tok := range strings.Fields(bind) {
		switch tok {
		case "127.0.0.1", "::1", "localhost":
		default:
			return false
		}
	}
	return true
}
