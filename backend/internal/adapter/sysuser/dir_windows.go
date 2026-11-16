//go:build windows

// Windows 开发机退化实现（A-F7 同口径）：无 /etc/passwd 系统用户语义。
// ListLoginShellUsers 明确报不支持（service 归一 TERM_007 不返回虚构数据）；
// Get 恒未命中（service 归一 TERM_002）。
package sysuser

import (
	"context"
	"fmt"

	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
)

// ListLoginShellUsers Windows 无 /etc/passwd 实况语义：明确报不支持。
func (g *Gateway) ListLoginShellUsers(ctx context.Context) ([]terminalsvc.SystemUser, error) {
	return nil, fmt.Errorf("sysuser list: %w", terminalsvc.ErrUnsupportedPlatform)
}

// Get Windows 无系统用户实况：恒未命中。
func (g *Gateway) Get(ctx context.Context, name string) (terminalsvc.SystemUser, bool) {
	return terminalsvc.SystemUser{}, false
}
