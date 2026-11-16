package sitemgmt

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// SiteEntry 是扫描出的一个站点（server 块）及其写回锚点（D-02 定位）。
type SiteEntry struct {
	SiteID string
	File   string // 来源文件（写回落点映射，⚠️A13）
	Dir    string // 所属惯例目录
	Index  int    // 文件内序号（含 dormant，A-SM3）
	Block  *nginxx.ServerBlock
	Tree   *nginxx.Tree // 来源文件解析树（active 块的写回载体）
}

// Enabled 站点启停投影（D-16：dormant=disabled）。
func (e *SiteEntry) Enabled() string {
	if e.Block.Dormant {
		return SiteDisabled
	}
	return SiteEnabled
}

// ScanResult 是一次实时扫描的产物（D-01：每请求即时扫描，无缓存无回退）。
type ScanResult struct {
	Entries     []SiteEntry // 按目录优先级+文件名+序号的扫描序
	GeneratedAt time.Time
}

// Find 按 siteId 定位站点条目。
func (r *ScanResult) Find(siteID string) *SiteEntry {
	for i := range r.Entries {
		if r.Entries[i].SiteID == siteID {
			return &r.Entries[i]
		}
	}
	return nil
}

// scanSites 执行导入管线：ScanDirs → Parse → ExtractSites（FR-04~06）+ 尾部
// 存量证书幂等纳管（A-SM5，FR-25）。纯文件实况，不经 nginx 二进制（I-12：
// GET 读路径不前置通道可用性）。
func (s *Service) scanSites(ctx context.Context) (*ScanResult, error) {
	files, err := s.gw.ScanConfDirs()
	if err != nil {
		return nil, err
	}
	res := &ScanResult{GeneratedAt: s.clock()}
	for _, f := range files {
		tree := nginxx.Parse(f.Content)
		servers := nginxx.ExtractServers(tree)
		for _, blk := range servers {
			res.Entries = append(res.Entries, SiteEntry{
				SiteID: buildSiteID(f.Path, blk.Index),
				File:   f.Path, Dir: f.Dir, Index: blk.Index,
				Block: blk, Tree: tree,
			})
		}
	}
	s.importExistingCerts(ctx, res)
	return res, nil
}

// ---- D-03 多目录消重（A-SM17）----

// siteSig 是消重签名（serverNames+listen；大小写与顺序归一）。
type siteSig struct {
	names  string
	listen string
}

func sigOf(e *SiteEntry) siteSig {
	names := make([]string, 0, len(e.Block.ServerNames))
	for _, n := range e.Block.ServerNames {
		names = append(names, strings.ToLower(n))
	}
	sort.Strings(names)
	listen := append([]string(nil), e.Block.Listen...)
	sort.Strings(listen)
	return siteSig{names: strings.Join(names, ","), listen: strings.Join(listen, ",")}
}

// dirPriority 目录优先级序（0 最高；未知目录排尾）。
func (s *Service) dirPriority(dir string) int {
	for i, d := range s.gw.PriorityDirs() {
		if d == dir {
			return i
		}
	}
	return len(ScanDirNames)
}

// dedupe 消重（D-03/A-SM17）：同签名跨文件并存 → items 仅保留最高优先级
// 目录（同目录则以扫描序首个）一处，其 siteId 即唯一写回落点；同文件内
// 重复定义两 siteId 均保留。返回保留集与重复组。
func (s *Service) dedupe(entries []SiteEntry) (kept []SiteEntry, dups []Duplicate) {
	groups := map[siteSig][]int{} // sig → entries 下标（扫描序）
	var order []siteSig
	for i := range entries {
		sig := sigOf(&entries[i])
		if _, ok := groups[sig]; !ok {
			order = append(order, sig)
		}
		groups[sig] = append(groups[sig], i)
	}
	keep := make([]bool, len(entries))
	for _, sig := range order {
		idxs := groups[sig]
		if len(idxs) == 1 {
			keep[idxs[0]] = true
			continue
		}
		// 重复组：确定各文件保留者（每文件保留扫描序首个——同文件内重复定义
		// 两 siteId 均列出 A-SM17；跨文件仅最高优先级目录的保留者进 items）。
		files := map[string]bool{}
		var rep int = idxs[0]
		bestPriority := s.dirPriority(entries[idxs[0]].Dir)
		for _, i := range idxs {
			files[entries[i].File] = true
			if p := s.dirPriority(entries[i].Dir); p < bestPriority {
				bestPriority, rep = p, i
			}
		}
		for _, i := range idxs {
			if entries[i].File == entries[rep].File {
				keep[i] = true // 同文件（含最高优先级目录文件）内均列出
			}
		}
		fileList := make([]string, 0, len(files))
		for f := range files {
			fileList = append(fileList, f)
		}
		sort.Strings(fileList)
		dups = append(dups, Duplicate{
			ServerNames: entries[idxs[0]].Block.ServerNames,
			Listen:      entries[idxs[0]].Block.Listen,
			Files:       fileList,
		})
	}
	for i := range entries {
		if keep[i] {
			kept = append(kept, entries[i])
		}
	}
	return kept, dups
}

// ---- 存量证书自动纳管（A-SM5/FR-25）----

// importExistingCerts 在扫描链尾部幂等纳管存量证书：site 的 ssl_cert 绝对
// 路径可读且 x509 可解析且无同路径 imported 行 → INSERT imported 行
// （status=valid、元数据 NULL 读时刷新 D-08）。机制无审计（PRD FR-25 明文）；
// 不可读/不可解析引用保持 certId=null 如实呈现。
func (s *Service) importExistingCerts(ctx context.Context, scan *ScanResult) {
	seen := map[string]bool{}
	rows, err := s.store.Cert().List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "site-mgmt: import existing certs: list failed", "err", err)
		return
	}
	for _, r := range rows {
		if r.Source == storesitemgmt.SourceImported && r.OriginPath != nil {
			seen[filepath.Clean(*r.OriginPath)] = true
		}
	}
	now := s.clock()
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if e.Block.SSLCert == "" {
			continue
		}
		path := filepath.Clean(e.Block.SSLCert)
		if seen[path] || !s.gw.Exists(path) {
			continue
		}
		if _, err := parseCertFile(s.gw, path); err != nil {
			continue // 不可解析：不纳管，引用保持 null（如实）
		}
		primary := primaryOf(e.Block.ServerNames)
		if _, err := s.store.Cert().Create(ctx, storesitemgmt.Certificate{
			PrimaryDomain: primary,
			Domains:       domainsOf(e),
			Source:        storesitemgmt.SourceImported,
			Status:        storesitemgmt.StatusValid,
			OriginPath:    &path,
		}); err != nil {
			slog.WarnContext(ctx, "site-mgmt: import existing cert failed",
				"path", path, "siteId", e.SiteID, "err", err)
			continue
		}
		seen[path] = true
		slog.InfoContext(ctx, "site-mgmt: existing certificate imported",
			"path", path, "siteId", e.SiteID, "primaryDomain", primary, "at", formatTS(now))
	}
}

// primaryOf 主域名（serverNames 首个；缺省空串如实）。
func primaryOf(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// domainsOf 站点 SAN 候选=serverNames 全集（无 server_name 站点返回空集）。
func domainsOf(e *SiteEntry) []string {
	if len(e.Block.ServerNames) == 0 {
		return []string{}
	}
	return append([]string(nil), e.Block.ServerNames...)
}
