//go:build !windows

package sysuser

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
)

// ListLoginShellUsers /etc/passwd 直读 + 非交互 shell 过滤（§3.4 排除集）
// + name 字典序升序（D-02）。
func (g *Gateway) ListLoginShellUsers(ctx context.Context) ([]terminalsvc.SystemUser, error) {
	entries, err := readPasswd()
	if err != nil {
		return nil, fmt.Errorf("sysuser read passwd: %w", err)
	}
	out := make([]terminalsvc.SystemUser, 0, len(entries))
	for _, u := range entries {
		if terminalsvc.IsInteractiveShell(u.Shell) {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get 按名称查找原始条目（TERM_002/003 判定在 service 层）。
func (g *Gateway) Get(ctx context.Context, name string) (terminalsvc.SystemUser, bool) {
	entries, err := readPasswd()
	if err != nil {
		return terminalsvc.SystemUser{}, false
	}
	for _, u := range entries {
		if u.Name == name {
			return u, true
		}
	}
	return terminalsvc.SystemUser{}, false
}

// readPasswd 解析 /etc/passwd（name:passwd:uid:gid:gecos:home:shell；
// 异常行跳过）。
func readPasswd() ([]terminalsvc.SystemUser, error) {
	f, err := os.Open("/etc/passwd") //nolint:gosec // 静态系统文件路径
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []terminalsvc.SystemUser
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 7 || fields[0] == "" {
			continue
		}
		uid, err := strconv.ParseUint(fields[2], 10, 32)
		if err != nil {
			continue
		}
		out = append(out, terminalsvc.SystemUser{
			Name: fields[0], UID: uint32(uid), Shell: fields[6]})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
