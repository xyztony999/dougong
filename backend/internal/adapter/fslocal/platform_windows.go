//go:build windows

// Windows 开发机退化实现（A-F7）：无 /etc/passwd 属主语义与 chown/EXDEV
// 系统调用。目标环境 Linux 功能完整（platform_unix.go）；本文件仅保证
// 开发机自验（build/vet/test）与纯 fs 逻辑测试可用。
package fslocal

import (
	"context"
	"fmt"
	"io/fs"

	filemansvc "github.com/dougong/dougong/internal/service/fileman"
)

type ownerMaps struct {
	user  map[int]string
	group map[int]string
}

type ownerPair struct {
	uid, gid int
}

// statOwner Windows 无 Stat_t 属主语义：恒无（属主字段置空）。
func statOwner(info fs.FileInfo) (ownerPair, bool) {
	return ownerPair{}, false
}

// loadOwners Windows 无系统账号文件：空映射。
func loadOwners() ownerMaps {
	return ownerMaps{user: map[int]string{}, group: map[int]string{}}
}

// ListSystemAccounts 开发机无 /etc/passwd 系统账号语义：明确报不支持
// （service 归一 FILE_013 unsupported_platform）。
func (g *Gateway) ListSystemAccounts(ctx context.Context) (filemansvc.SystemAccounts, error) {
	return filemansvc.SystemAccounts{}, filemansvc.ErrUnsupportedPlatform
}

// Chown Windows 退化：不支持（归 FILE_013 unsupported_platform）。
func (g *Gateway) Chown(ctx context.Context, realPath string, uid, gid int) error {
	return fmt.Errorf("fslocal chown %s: %w", realPath, filemansvc.ErrUnsupportedPlatform)
}

// isCrossDevice Windows 退化：rename 不产生 EXDEV 语义。
func isCrossDevice(err error) bool {
	return false
}

// isNotADirectory Windows 退化：无 ENOTDIR 哨兵（错误经通用路径归一）。
func isNotADirectory(err error) bool {
	return false
}
