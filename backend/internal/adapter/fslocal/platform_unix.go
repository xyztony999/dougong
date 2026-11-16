//go:build !windows

package fslocal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	filemansvc "github.com/dougong/dougong/internal/service/fileman"
)

// ownerMaps 是 uid/gid → 名称映射（每次调用实时读取 /etc/passwd、/etc/group
// ——系统实况，D-14 同口径；读取失败降级空映射，属主字段置空）。
type ownerMaps struct {
	user  map[int]string
	group map[int]string
}

// ownerPair 是 Lstat 附带的属主数值对。
type ownerPair struct {
	uid, gid int
}

// statOwner 从 FileInfo 提取 uid/gid（unix Stat_t）。
func statOwner(info fs.FileInfo) (ownerPair, bool) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return ownerPair{uid: int(st.Uid), gid: int(st.Gid)}, true
	}
	return ownerPair{}, false
}

// loadOwners 实时读取系统账号名称映射。
func loadOwners() ownerMaps {
	return ownerMaps{user: readNameIDMap("/etc/passwd", 2), group: readNameIDMap("/etc/group", 2)}
}

// readNameIDMap 解析 name:x:id:... 形态文件 → id → name 映射（异常行跳过）。
func readNameIDMap(path string, idCol int) map[int]string {
	out := make(map[int]string)
	f, err := os.Open(path) //nolint:gosec // 静态系统文件路径
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) <= idCol {
			continue
		}
		id, err := strconv.Atoi(fields[idCol])
		if err != nil || fields[0] == "" {
			continue
		}
		out[id] = fields[0]
	}
	return out
}

// ListSystemAccounts 候选清单实时读取（D-14；仅 name/uid、name/gid）。
func (g *Gateway) ListSystemAccounts(ctx context.Context) (filemansvc.SystemAccounts, error) {
	out := filemansvc.SystemAccounts{}
	if err := readColonFile("/etc/passwd", 2, func(fields []string) {
		if uid, err := strconv.Atoi(fields[2]); err == nil {
			out.Users = append(out.Users, filemansvc.SysAccountUser{Name: fields[0], UID: uid})
		}
	}); err != nil {
		return filemansvc.SystemAccounts{}, fmt.Errorf("fslocal read passwd: %w", err)
	}
	if err := readColonFile("/etc/group", 2, func(fields []string) {
		if gid, err := strconv.Atoi(fields[2]); err == nil {
			out.Groups = append(out.Groups, filemansvc.SysAccountGroup{Name: fields[0], GID: gid})
		}
	}); err != nil {
		return filemansvc.SystemAccounts{}, fmt.Errorf("fslocal read group: %w", err)
	}
	return out, nil
}

func readColonFile(path string, minFields int, fn func([]string)) error {
	f, err := os.Open(path) //nolint:gosec // 静态系统文件路径
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) > minFields && fields[0] != "" {
			fn(fields)
		}
	}
	return sc.Err()
}

// Chown 属主修改（uid/gid -1 = 不变；仅目标自身不递归，D-06）。
func (g *Gateway) Chown(ctx context.Context, realPath string, uid, gid int) error {
	if err := os.Chown(realPath, uid, gid); err != nil {
		return fmt.Errorf("fslocal chown %s: %w", realPath, err)
	}
	return nil
}

// isCrossDevice EXDEV 判定（调用方降级 D-15）。
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// isNotADirectory ENOTDIR 判定。
func isNotADirectory(err error) bool {
	return errors.Is(err, syscall.ENOTDIR)
}
