package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
)

// UpdateCheck POST /panel/system/update-check（admin）：双重门控单次外呼
// （I-P05：update_check=0 → PANEL_004；=1 且显式触发才产生一次外呼）；
// 失败不自动重试（D-P09/R-09）；结果为过程数据不落库、触发不落审计
// （PRD FR-16 口径：仅开关变更记审计）。
func (s *Service) UpdateCheck(ctx context.Context) (*UpdateCheckResult, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("panel: update check get config: %w", err)
	}
	if !cfg.UpdateCheck {
		return nil, errcode.ErrUpdateCheckDisabled // PANEL_004（I-P05）
	}
	if s.source == nil {
		// 版本源未装配（prd 待对齐-1：源标识与清单格式未锁定；A-P8 裁定：
		// 触发即 PANEL_005，ACL 缝合到位，待源定稿接入适配器零契约变更）。
		slog.InfoContext(ctx, "panel: update check triggered without version source")
		return nil, errcode.ErrUpdateCheckFailed
	}
	// 单次触发单次请求（无自动重试）；超时上限为编译期常量（N1 待校准）。
	fctx, cancel := context.WithTimeout(ctx, UpdateCheckTimeout)
	defer cancel()
	latest, err := s.source.LatestVersion(fctx)
	if err != nil {
		slog.WarnContext(ctx, "panel: update check fetch", "err", err)
		return nil, errcode.ErrUpdateCheckFailed // PANEL_005
	}
	if strings.TrimSpace(latest) == "" {
		slog.WarnContext(ctx, "panel: update check empty latest version")
		return nil, errcode.ErrUpdateCheckFailed // 清单格式不符
	}
	current := GetVersionInfo().Version
	// 当前版本为开发构建（不可解析）时视作 0.0.0（恒提示有更新）；
	// latest 不可解析 → PANEL_005（清单格式不符）。
	if _, err := parseSemver(current); err != nil {
		current = "0.0.0"
	}
	cmp, err := CompareSemver(latest, current)
	if err != nil {
		slog.WarnContext(ctx, "panel: update check parse version", "latest", latest, "err", err)
		return nil, errcode.ErrUpdateCheckFailed
	}
	return &UpdateCheckResult{
		CurrentVersion:  GetVersionInfo().Version,
		LatestVersion:   &latest,
		UpdateAvailable: cmp > 0,
		CheckedAt:       s.now(),
	}, nil
}

// HTTPVersionSource 是经 HTTP 拉取版本清单的默认适配器（D-P09 ACL 隔离：
// 清单格式为适配器内部实现——{"latestVersion":"v1.2.3"}；源定稿后仅改
// 本适配器，接口契约不变。Endpoint 为空即未配置，调用返回错误→PANEL_005）。
type HTTPVersionSource struct {
	Endpoint string
	Client   *http.Client
}

// LatestVersion 实现 VersionSource。
func (h *HTTPVersionSource) LatestVersion(ctx context.Context) (string, error) {
	if strings.TrimSpace(h.Endpoint) == "" {
		return "", fmt.Errorf("version source endpoint not configured")
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: UpdateCheckTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch manifest: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "panel: update check close body", "err", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest status %d", resp.StatusCode)
	}
	var manifest struct {
		LatestVersion string `json:"latestVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("decode manifest: %w", err)
	}
	return manifest.LatestVersion, nil
}

// DefaultUpdateRepo 是默认版本源仓库（斗拱开源发布仓；D-P09 源定稿落地）。
const DefaultUpdateRepo = "xyztony999/dougong"

// GitHubReleaseSource 经 GitHub Releases API 取最新 release 的 tag_name（D-P09）。
// 此前版本源未装配（A-P8 延期），检查更新恒报 PANEL_005「版本源不可达」；本适配器
// 接线后检查更新可用。BaseURL/Repo/Client 可注入（测试用 httptest）；缺省指向
// api.github.com 的 DefaultUpdateRepo。GitHub API 要求 User-Agent，匿名限流 60 次/时
// 对单次手动检查足够。
type GitHubReleaseSource struct {
	BaseURL string // 缺省 https://api.github.com
	Repo    string // 缺省 DefaultUpdateRepo（owner/name）
	Client  *http.Client
}

// LatestVersion 实现 VersionSource：GET /repos/{repo}/releases/latest → tag_name。
func (g *GitHubReleaseSource) LatestVersion(ctx context.Context) (string, error) {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	repo := g.Repo
	if repo == "" {
		repo = DefaultUpdateRepo
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: UpdateCheckTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dougong-panel-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "panel: update check close body", "err", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release status %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("decode release: %w", err)
	}
	return rel.TagName, nil
}

// ---- 语义化版本比较（semver 2.0.0 核心规则：MAJOR.MINOR.PATCH，可选先行版；
// build metadata 忽略；无先行版 > 有先行版；数字标识 < 字母数字标识）----

type semver struct {
	major, minor, patch uint64
	pre                 []string
}

// CompareSemver 比较语义化版本 a 与 b：a>b → 1、a<b → -1、相等 → 0。
// 前缀 v 可选；缺省 minor/patch 补 0；解析失败返回错误。
func CompareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	if c := cmpUint(pa.major, pb.major); c != 0 {
		return c, nil
	}
	if c := cmpUint(pa.minor, pb.minor); c != 0 {
		return c, nil
	}
	if c := cmpUint(pa.patch, pb.patch); c != 0 {
		return c, nil
	}
	if len(pa.pre) == 0 && len(pb.pre) == 0 {
		return 0, nil
	}
	if len(pa.pre) == 0 {
		return 1, nil // 1.0.0 > 1.0.0-rc.1
	}
	if len(pb.pre) == 0 {
		return -1, nil
	}
	for i := 0; i < len(pa.pre) && i < len(pb.pre); i++ {
		x, y := pa.pre[i], pb.pre[i]
		xNum, yNum := allDigits(x), allDigits(y)
		switch {
		case xNum && yNum:
			xi, _ := strconv.ParseUint(x, 10, 64)
			yi, _ := strconv.ParseUint(y, 10, 64)
			if c := cmpUint(xi, yi); c != 0 {
				return c, nil
			}
		case xNum:
			return -1, nil // 数字标识排序低于字母数字标识
		case yNum:
			return 1, nil
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c, nil
			}
		}
	}
	return cmpUint(uint64(len(pa.pre)), uint64(len(pb.pre))), nil
}

func parseSemver(v string) (semver, error) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if s == "" {
		return semver{}, fmt.Errorf("semver: empty version %q", v)
	}
	if i := strings.IndexByte(s, '+'); i >= 0 { // build metadata 忽略
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return semver{}, fmt.Errorf("semver: %q", v)
	}
	var nums [3]uint64
	for i, p := range parts {
		if !allDigits(p) {
			return semver{}, fmt.Errorf("semver: %q", v)
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return semver{}, fmt.Errorf("semver %q: %w", v, err)
		}
		nums[i] = n
	}
	sv := semver{major: nums[0], minor: nums[1], patch: nums[2]}
	if pre != "" {
		sv.pre = strings.Split(pre, ".")
		for _, id := range sv.pre {
			if id == "" {
				return semver{}, fmt.Errorf("semver: %q", v)
			}
		}
	}
	return sv, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func cmpUint(a, b uint64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}
