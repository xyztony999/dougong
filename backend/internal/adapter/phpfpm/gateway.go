// Package phpfpm 是 php-fpm pool 实况读取适配器（api-site-mgmt.md §4.2.5
// PhpFpmGateway 契约：ListPools 每次请求实时解析本机 php-fpm 配置的 pool
// 名与 socket，无缓存、不落库——D-14；候选清单为建站下拉选项过程数据 P4，
// 不做 pool 管理（OUT-7）。
//
// 解析面：INI 风格 php-fpm 配置（[pool] 段 + listen 指令），覆盖 Debian
// （/etc/php/*/fpm/pool.d/*.conf 与 php-fpm.conf include）与 RHEL（/etc/php-fpm.conf
// + /etc/php-fpm.d/*.conf）两种惯例布局；include 指令按 glob 跟随（一层展开，
// 相对路径相对引入文件目录解析）。纯文本解析无外部命令（R-08 by-absence）。
package phpfpm

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Pool 是一个 php-fpm pool 的实况投影（api §4.2.4 php-pools/list data.pools 项）。
type Pool struct {
	Name   string `json:"name"`
	Socket string `json:"socket"`
}

// Gateway 是 PhpFpmGateway 适配器（confRoot 列表为待解析配置文件/目录
// 惯例根；默认 DefaultConfRoots）。
type Gateway struct {
	roots []string
}

// DefaultConfRoots 是默认解析根（Debian + RHEL 惯例布局；glob 模式由
// Gateway 内部展开）。
var DefaultConfRoots = []string{
	"/etc/php-fpm.conf",           // RHEL 主配置
	"/etc/php-fpm.d",              // RHEL pool 目录
	"/etc/php/*/fpm/php-fpm.conf", // Debian 主配置（含 include pool.d）
	"/etc/php/*/fpm/pool.d",       // Debian pool 目录
}

// NewGateway 构造 pool 读取网关；roots 为空时用 DefaultConfRoots。
func NewGateway(roots ...string) *Gateway {
	if len(roots) == 0 {
		roots = DefaultConfRoots
	}
	return &Gateway{roots: roots}
}

// ListPools 实时解析本机 php-fpm pool（D-14：每次请求重读，无缓存）。
// 无 php-fpm 安装时返回空切片（FR-15 如实提示口径：无 pool 返回空数组）。
// 同名 pool 多处定义以首次出现为准（php-fpm 自身行为同判）。
func (g *Gateway) ListPools(ctx context.Context) ([]Pool, error) {
	pools := map[string]string{}
	visited := map[string]bool{}
	for _, root := range g.roots {
		files, err := expandRoot(root)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("phpfpm: list pools canceled: %w", err)
			}
			if visited[f] {
				continue // 根展开与 include 双路径命中同一文件：首次已解析
			}
			visited[f] = true
			if err := parsePoolFile(f, pools, visited); err != nil {
				return nil, err
			}
		}
	}
	names := make([]string, 0, len(pools))
	for n := range pools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Pool, 0, len(names))
	for _, n := range names {
		out = append(out, Pool{Name: n, Socket: pools[n]})
	}
	return out, nil
}

// expandRoot 展开单个根（glob 模式 / 目录下 *.conf / 单文件）；不做去重
// （由 ListPools 统一以 visited 判定）。根不存在返回空。
func expandRoot(root string) ([]string, error) {
	if strings.ContainsAny(root, "*?[") {
		m, err := filepath.Glob(root)
		if err != nil {
			return nil, fmt.Errorf("phpfpm: glob %s: %w", root, err)
		}
		var out []string
		for _, p := range m {
			if isDir(p) {
				sub, err := confFiles(p)
				if err != nil {
					return nil, err
				}
				out = append(out, sub...)
				continue
			}
			if strings.HasSuffix(p, ".conf") {
				out = append(out, p)
			}
		}
		return out, nil
	}
	switch {
	case isDir(root):
		return confFiles(root)
	case fileExists(root):
		return []string{root}, nil
	default:
		return nil, nil // 根不存在：该布局缺席，如实跳过
	}
}

// confFiles 收集目录下全部 .conf 文件（不递归；文件名升序）。
func confFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("phpfpm: read dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		p := filepath.Join(dir, n)
		out = append(out, p)
	}
	return out, nil
}

// parsePoolFile 解析单个 php-fpm 配置文件：[name] 段开启 pool，listen 指令
// 登记 socket；include 指令展开（相对引入文件目录解析，visited 防环）。
func parsePoolFile(path string, pools map[string]string, visited map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("phpfpm: open %s: %w", path, err)
	}
	defer f.Close()
	current := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if current != "" && current != "global" {
				if _, ok := pools[current]; !ok {
					pools[current] = "" // 先占位：出现即存在，listen 随后回填
				}
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "listen":
			if current != "" && current != "global" && pools[current] == "" {
				pools[current] = val
			}
		case "include":
			inc := val
			if !filepath.IsAbs(inc) {
				inc = filepath.Join(filepath.Dir(path), inc)
			}
			if m, gerr := filepath.Glob(inc); gerr == nil {
				for _, p := range m {
					if visited[p] || isDir(p) {
						continue
					}
					visited[p] = true
					if perr := parsePoolFile(p, pools, visited); perr != nil {
						return perr
					}
				}
			}
		}
	}
	return sc.Err()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
