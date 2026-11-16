package fileman

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/dougong/dougong/internal/errcode"
)

// ScopeConfig 是受管根白名单配置的进程内副本（D-02：fs_path_scope 单行表
// 加载；修改提交后 Refresh 即予生效）。
type ScopeConfig struct {
	RootPath     string
	ExcludedDirs []string
}

// PathScopeGuard 是 R-04 两步校验器（D-01 全接口强制前置，无白名单旁路）：
// ①EvalSymlinks 解析真实路径 → ②受管根前缀校验 → ③排除目录校验；
// 校验通过后以真实路径执行。越界拒绝返回 FILE_002（data.reason 注明两类
// 原因之一，A-F10：越界拒绝不记审计）。
//
// 实现为 service 层前置（A-F2 裁定）：guard 产出「校验后的真实路径」回传
// 执行链，gin middleware 无法向 handler 回传数据。
type PathScopeGuard struct {
	cfg atomic.Pointer[ScopeConfig]
}

// NewGuard 以初始副本构造（main 启动时自 fs_path_scope 加载注入）。
func NewGuard(cfg ScopeConfig) *PathScopeGuard {
	g := &PathScopeGuard{}
	g.cfg.Store(&cfg)
	return g
}

// Snapshot 返回当前配置副本（path-scope/get 数据源）。
func (g *PathScopeGuard) Snapshot() ScopeConfig {
	c := g.cfg.Load()
	return ScopeConfig{RootPath: c.RootPath, ExcludedDirs: append([]string(nil), c.ExcludedDirs...)}
}

// Refresh 事务提交后刷新进程内副本（D-02：后续请求即予生效）。
func (g *PathScopeGuard) Refresh(cfg ScopeConfig) {
	g.cfg.Store(&cfg)
}

// Resolve 解析已存在条目的真实路径（EvalSymlinks 全链解析）并过两步校验。
// 词汇层域校验先行（D-01 前置 + 不泄露受管根外路径的存在性——越界一律
// FILE_002，无论目标是否存在）；解析成功后二次核域（symlink 逃逸拦截）。
// 条目不存在时返回包装的 fs.ErrNotExist（调用方按接口语义映射 FILE_003/FILE_012）。
func (g *PathScopeGuard) Resolve(path string) (string, error) {
	clean, err := validatePathForm(path)
	if err != nil {
		return "", err
	}
	if err := g.scopeCheck(clean); err != nil {
		return "", err // 词汇层越界：不触达文件系统（零探测面）
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("guard resolve %s: %w", clean, err)
	}
	if err := g.scopeCheck(real); err != nil {
		return "", err // 真实路径越界（symlink 逃逸，M2 验收硬项）
	}
	return real, nil
}

// ResolveParent 解析「目标条目可不存在」场景：解析已存在的父目录真实路径
// 并过两步校验，返回（父目录真实路径, 条目名）。name 的合法性由调用方按
// FILE_007 校验；join 后路径必在受管根内（父已校验 + name 无分隔符）。
func (g *PathScopeGuard) ResolveParent(path string) (string, string, error) {
	clean, err := validatePathForm(path)
	if err != nil {
		return "", "", err
	}
	if err := g.scopeCheck(clean); err != nil {
		return "", "", err // 词汇层越界先行
	}
	dir, name := filepath.Split(clean)
	dir = filepath.Clean(dir)
	if dir == "" {
		dir = string(filepath.Separator)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", fmt.Errorf("guard resolve parent %s: %w", dir, err)
	}
	if err := g.scopeCheck(realDir); err != nil {
		return "", "", err
	}
	return realDir, name, nil
}

// ResolveForWrite 写路径统一解析（A-F2 补充语义）：目标已存在（可能是符号
// 链接）→ 全链解析真实路径（防止经 symlink 逃逸写出受管根）；不存在 →
// 解析父目录返回拼合路径。
func (g *PathScopeGuard) ResolveForWrite(path string) (real string, existed bool, err error) {
	clean, ferr := validatePathForm(path)
	if ferr != nil {
		return "", false, ferr
	}
	if serr := g.scopeCheck(clean); serr != nil {
		return "", false, serr // 词汇层越界先行
	}
	if real, rerr := filepath.EvalSymlinks(clean); rerr == nil {
		if serr := g.scopeCheck(real); serr != nil {
			return "", false, serr
		}
		return real, true, nil
	}
	dir, name, perr := g.ResolveParent(clean)
	if perr != nil {
		return "", false, perr
	}
	return filepath.Join(dir, name), false, nil
}

// TrashDir 返回回收站目录真实路径（受管根下 .dougong-trash，R-17/ADR-021）。
// 受管根本身必须存在（由调用方保证；不存在时 EvalSymlinks 报错如实上抛）。
func (g *PathScopeGuard) TrashDir() (string, error) {
	c := g.cfg.Load()
	root, err := filepath.EvalSymlinks(c.RootPath)
	if err != nil {
		return "", fmt.Errorf("guard trash dir: resolve root %s: %w", c.RootPath, err)
	}
	return filepath.Join(root, TrashDirName), nil
}

// ValidateScopeUpdate 校验 path-scope/update 入参（R-14/§4.2.4）：
// rootPath 绝对路径形态（FILE_001）；excludedDirs 每项绝对路径（FILE_001）
// 且均不等于/不包含 rootPath（排除受管根=全禁无意义，FILE_002 语义）。
// 通过后返回规范化（Clean）配置。
func ValidateScopeUpdate(rootPath string, excludedDirs []string) (ScopeConfig, error) {
	root, err := validatePathForm(rootPath)
	if err != nil {
		return ScopeConfig{}, err
	}
	cfg := ScopeConfig{RootPath: root, ExcludedDirs: make([]string, 0, len(excludedDirs))}
	for _, ex := range excludedDirs {
		cleanEx, err := validatePathForm(ex)
		if err != nil {
			return ScopeConfig{}, err
		}
		if cleanEx == root || isUnder(root, cleanEx) {
			return ScopeConfig{}, errcode.ErrFilePathOutOfScope.WithData(ReasonData{
				Reason: ReasonExcluded, // 排除项等于/包含受管根——全禁无意义
			})
		}
		cfg.ExcludedDirs = append(cfg.ExcludedDirs, cleanEx)
	}
	return cfg, nil
}

// CheckStatic 对「即将创建、尚不存在」的路径做形态+受管根+排除静态校验
// （解压逐条目预检用：条目声明路径在落盘前无法 EvalSymlinks，D-07①；
// 目标目录已解析为真实路径，条目名不含 .. 后拼合路径必为其后代）。
func (g *PathScopeGuard) CheckStatic(path string) error {
	clean, err := validatePathForm(path)
	if err != nil {
		return err
	}
	return g.scopeCheck(clean)
}

// scopeCheck 两步校验的第二三步：受管根前缀 + 排除清单（real 为已解析真实路径）。
func (g *PathScopeGuard) scopeCheck(real string) error {
	c := g.cfg.Load()
	if !isUnder(real, c.RootPath) {
		return errcode.ErrFilePathOutOfScope.WithData(ReasonData{Reason: ReasonNotUnderRoot})
	}
	for _, ex := range c.ExcludedDirs {
		if real == ex || isUnder(real, ex) {
			return errcode.ErrFilePathOutOfScope.WithData(ReasonData{Reason: ReasonExcluded})
		}
	}
	return nil
}

// isUnder 判断 path 是否位于 ancestor 之内（含相等；ancestor="/" 时全部绝对路径通过）。
func isUnder(path, ancestor string) bool {
	anc := filepath.Clean(ancestor)
	if anc == string(filepath.Separator) || anc == "" {
		return true
	}
	return path == anc || strings.HasPrefix(path, anc+string(filepath.Separator))
}

// validatePathForm 路径形态校验（R-14，FILE_001）：非空、绝对、无 NUL；
// 返回 Clean 后形态（Clean 折叠冗余段与尾部分隔符）。
func validatePathForm(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", errcode.ErrFileInvalidPath
	}
	if !filepath.IsAbs(path) {
		return "", errcode.ErrFileInvalidPath
	}
	return filepath.Clean(path), nil
}

// ValidateName 条目名校验（FILE_007，R-14）：非空、非保留名（./..）、
// 不含路径分隔符与 NUL。
func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return errcode.ErrFileInvalidName
	}
	return nil
}
