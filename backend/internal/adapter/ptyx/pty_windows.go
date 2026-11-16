//go:build windows

// Windows 开发机退化实现（A-F7 同口径）：无 PTY 系统调用语义。目标环境
// Linux 功能完整（pty_unix.go）；本文件仅保证开发机自验（build/vet/test）。
package ptyx

import (
	"context"
	"fmt"

	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
)

// Spawn Windows 无 PTY 语义：明确报不支持（service 归一 TERM_004）。
func (g *Gateway) Spawn(ctx context.Context, identity terminalsvc.RunIdentity) (terminalsvc.PtyHandle, error) {
	return nil, fmt.Errorf("ptyx spawn %s: %w", identity.Source, terminalsvc.ErrUnsupportedPlatform)
}
