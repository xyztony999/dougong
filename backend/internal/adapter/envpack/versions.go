package envpack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// 包版本白名单（R-08：进入 argv 前收窄，防把用户输入拼进命令）。
var pkgVersionRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+:~_-]{0,63}$`)

// ValidPkgVersion 报告版本字符串是否可安全作为包管理器版本参数。
func ValidPkgVersion(v string) bool {
	return pkgVersionRe.MatchString(v)
}

// ListVersions 列出仓库中该包的可安装版本（去重保序）。通道不可用如实错误。
func (g *Gateway) ListVersions(ctx context.Context, family, pkgName string) ([]string, error) {
	if family == "" {
		return nil, errors.New("envpack: channel unavailable")
	}
	if pkgName == "" {
		return nil, errors.New("envpack: empty package name")
	}
	switch family {
	case "apt":
		out, err := g.run(ctx, "apt-cache", "madison", pkgName)
		if err != nil {
			return nil, err
		}
		return parseAptMadison(out), nil
	case "dnf", "yum":
		out, err := g.run(ctx, family, "list", "--showduplicates", pkgName)
		if err != nil {
			return nil, err
		}
		return parseRpmList(out, pkgName), nil
	}
	return nil, errors.New("envpack: channel unavailable")
}

// InstallAtVersion 安装指定版本（version 空则等同 Install 最新）。
// apt 用 pkg=ver 单参数；rpm 系用 pkg-ver。
func (g *Gateway) InstallAtVersion(ctx context.Context, family, pkgName, version string) (string, error) {
	if version == "" {
		return g.Install(ctx, family, pkgName)
	}
	if !ValidPkgVersion(version) {
		return "", fmt.Errorf("envpack: invalid version %q", version)
	}
	switch family {
	case "apt":
		return g.runEnv(ctx, map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
			"apt-get", "-y",
			"-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold",
			"install", pkgName+"="+version)
	case "dnf":
		return g.run(ctx, "dnf", "install", "-y", pkgName+"-"+version)
	case "yum":
		return g.run(ctx, "yum", "install", "-y", pkgName+"-"+version)
	}
	return "", errors.New("envpack: channel unavailable")
}

func parseAptMadison(out string) []string {
	seen := map[string]struct{}{}
	var vers []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		v := strings.TrimSpace(parts[1])
		if v == "" || !ValidPkgVersion(v) {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		vers = append(vers, v)
	}
	return vers
}

func parseRpmList(out, pkgName string) []string {
	seen := map[string]struct{}{}
	var vers []string
	prefix := pkgName + "."
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		nameArch := fields[0]
		if !strings.HasPrefix(nameArch, prefix) && nameArch != pkgName {
			continue
		}
		v := fields[1]
		if i := strings.IndexByte(v, ':'); i >= 0 && i+1 < len(v) {
			v = v[i+1:] // 去掉 epoch
		}
		if v == "" || !ValidPkgVersion(v) {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		vers = append(vers, v)
	}
	return vers
}
