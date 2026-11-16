package sitemgmt

import (
	"context"
	"sort"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// SiteProjector（§4.2.1）：扫描条目 → list/detail 投影 + certRef 匹配纳管证书
// （D-11 实时推导：ssl_cert 值 ↔ storage_path/origin_path）。

// certPathIndex 证书路径索引（path → 最优行：not_after 最新者，缺省 id 最大）。
type certPathIndex struct {
	byPath map[string]storesitemgmt.Certificate
}

// buildCertIndex 读全表构建路径索引（同路径多行取有效期最新——重复申请历史
// 的淘汰语义；imported origin_path 与 acme storage_path 同域并入）。
func (s *Service) buildCertIndex(ctx context.Context) *certPathIndex {
	idx := &certPathIndex{byPath: map[string]storesitemgmt.Certificate{}}
	rows, err := s.store.Cert().List(ctx)
	if err != nil {
		return idx // 索引失败 → 全部 certId null（如实降级，不阻断列表）
	}
	for _, r := range rows {
		var path string
		switch r.Source {
		case storesitemgmt.SourceAcme:
			if r.StoragePath == nil {
				continue
			}
			path = *r.StoragePath
		case storesitemgmt.SourceImported:
			if r.OriginPath == nil {
				continue
			}
			path = *r.OriginPath
		default:
			continue
		}
		cur, ok := idx.byPath[path]
		if !ok || betterCert(r, cur) {
			idx.byPath[path] = r
		}
	}
	return idx
}

// betterCert r 是否优于 cur（not_after 更晚者优先；空值次之；平局 id 大者）。
func betterCert(r, cur storesitemgmt.Certificate) bool {
	switch {
	case r.NotAfter == nil && cur.NotAfter != nil:
		return false
	case r.NotAfter != nil && cur.NotAfter == nil:
		return true
	case r.NotAfter != nil && cur.NotAfter != nil:
		if !r.NotAfter.Equal(*cur.NotAfter) {
			return r.NotAfter.After(*cur.NotAfter)
		}
	}
	return r.ID > cur.ID
}

// matchCert 站点 certRef 匹配纳管证书（未纳管引用返回零值 false）。
func (idx *certPathIndex) matchCert(certRef string) (storesitemgmt.Certificate, bool) {
	if certRef == "" {
		return storesitemgmt.Certificate{}, false
	}
	r, ok := idx.byPath[certRef]
	return r, ok
}

// projectBlock 组装聚合条目的块级子项（v2.0 D-17：寻址键 siteId 保留在块级）。
func (s *Service) projectBlock(e *SiteEntry, idx *certPathIndex) SiteBlock {
	b := SiteBlock{
		SiteID:  e.SiteID,
		Listen:  nonEmpty(e.Block.Listen),
		Enabled: e.Enabled(),
	}
	if e.Block.SSLCert != "" {
		ref := e.Block.SSLCert
		b.CertRef = &ref
	}
	if row, ok := idx.matchCert(e.Block.SSLCert); ok {
		id := row.ID
		status := s.computeStatus(row, s.clock())
		b.CertID = &id
		b.CertStatus = &status
	}
	return b
}

// certStatusRank 组内最优排序权重（valid > expiring_soon > renew_failed >
// expired > 无；D-17——风险态语义顺序，数值小者优先）。
func certStatusRank(status string, has bool) int {
	if !has {
		return 5
	}
	switch status {
	case storesitemgmt.StatusValid:
		return 0
	case storesitemgmt.StatusExpiringSoon:
		return 1
	case storesitemgmt.StatusRenewFailed:
		return 2
	case storesitemgmt.StatusExpired:
		return 3
	}
	return 4 // pending 等其余
}

// isSSLBlock 块是否携带 ssl 声明（443/ssl 块在证书列推导中优先，D-17）。
func isSSLBlock(e *SiteEntry) bool {
	for _, l := range e.Block.Listen {
		if strings.Contains(l, "ssl") {
			return true
		}
	}
	return false
}

// aggregateGroup v2.0 D-17 聚合投影：同 sourceFile 且 server_name 全集相同
// （排序比较）的块合并为一个 SiteItem（单块条目 blocks 为 1 元素数组）。
func (s *Service) aggregateGroup(entries []*SiteEntry, idx *certPathIndex) SiteItem {
	item := SiteItem{
		ServerNames: nonEmpty(entries[0].Block.ServerNames),
		SourceFile:  entries[0].File,
		Blocks:      make([]SiteBlock, 0, len(entries)),
		Listen:      []string{},
	}
	seenListen := map[string]bool{}
	certRank := certStatusRank("", false)
	var certRef *string
	var certID *int64
	var certStatus *string
	var sslPreferred bool
	for _, e := range entries {
		b := s.projectBlock(e, idx)
		item.Blocks = append(item.Blocks, b)
		for _, l := range b.Listen {
			if !seenListen[l] {
				seenListen[l] = true
				item.Listen = append(item.Listen, l)
			}
		}
		if e.Enabled() == SiteEnabled {
			item.Enabled = SiteEnabled
		} else if item.Enabled == "" {
			item.Enabled = SiteDisabled
		}
		if item.Root == "" {
			item.Root = e.Block.Root
		}
		if item.Proxy == nil {
			if p := e.Block.Proxy(); p != nil {
				item.Proxy = &ProxyDTO{Upstream: p.Upstream, HostHeader: p.HostHeader, Websocket: p.Websocket}
			}
		}
		// 证书列=组内最优：同 rank 时 ssl 块优先。
		ssl := isSSLBlock(e)
		if b.CertStatus != nil {
			r := certStatusRank(*b.CertStatus, true)
			if r < certRank || (r == certRank && ssl && !sslPreferred) {
				certRank = r
				certRef, certID, certStatus = b.CertRef, b.CertID, b.CertStatus
				sslPreferred = ssl
			}
		} else if b.CertRef != nil && certRef == nil {
			certRef = b.CertRef // 未纳管引用兜底呈现（无状态）
		}
	}
	if item.Enabled == "" {
		item.Enabled = SiteDisabled
	}
	item.CertRef, item.CertID, item.CertStatus = certRef, certID, certStatus
	return item
}

// groupEntries 按聚合键分组（保持扫描顺序稳定：组间按首块出现序）。
func groupEntries(entries []SiteEntry) [][]*SiteEntry {
	type key struct {
		file  string
		names string
	}
	order := []key{}
	groups := map[key][]*SiteEntry{}
	for i := range entries {
		names := append([]string(nil), entries[i].Block.ServerNames...)
		sort.Strings(names)
		k := key{file: entries[i].File, names: strings.Join(names, "\x00")}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], &entries[i])
	}
	out := make([][]*SiteEntry, 0, len(order))
	for _, k := range order {
		out = append(out, groups[k])
	}
	return out
}

// projectDetail 组装 detail 项（单块寻址：块级字段直装 + locations/rawBlocks；
// v2.0 SiteItem 聚合语义下 detail 保持单块粒度——组级上下文由 list 呈现）。
func (s *Service) projectDetail(e *SiteEntry, idx *certPathIndex) SiteDetailData {
	b := s.projectBlock(e, idx)
	d := SiteDetailData{SiteItem: SiteItem{
		ServerNames: nonEmpty(e.Block.ServerNames),
		Listen:      nonEmpty(e.Block.Listen),
		Root:        e.Block.Root,
		Enabled:     e.Enabled(),
		CertRef:     b.CertRef, CertID: b.CertID, CertStatus: b.CertStatus,
		SourceFile: e.File,
		Blocks:     []SiteBlock{b},
	}}
	if p := e.Block.Proxy(); p != nil {
		d.Proxy = &ProxyDTO{Upstream: p.Upstream, HostHeader: p.HostHeader, Websocket: p.Websocket}
	}
	for _, loc := range e.Block.Locations {
		d.Locations = append(d.Locations, LocationDTO{
			Path: loc.Path, ProxyPass: loc.ProxyPass, FastCGIPass: loc.FastCGIPass,
		})
	}
	for _, rb := range e.Block.RawBlocks {
		d.RawBlocks = append(d.RawBlocks, RawBlockDTO{Line: rb.Line, Text: rb.Text})
	}
	if d.Locations == nil {
		d.Locations = []LocationDTO{}
	}
	if d.RawBlocks == nil {
		d.RawBlocks = []RawBlockDTO{}
	}
	return d
}

// nonEmpty 空集转空切片（JSON 呈现为 [] 而非 null）。
func nonEmpty(ss []string) []string {
	if len(ss) == 0 {
		return []string{}
	}
	return ss
}

// ListSites GET /sites/site/list（§4.2.4）：即时扫描 → 消重（D-03）→ enabled
// 筛选 → certRef 匹配。nginx 缺席：空列表 + nginxAvailable:false（I-06 空态
// 非错误）。
func (s *Service) ListSites(ctx context.Context, enabledFilter string) (*SiteListData, error) {
	if enabledFilter != "" && enabledFilter != SiteEnabled && enabledFilter != SiteDisabled {
		return nil, bindFieldError("enabled", "enabled 筛选值非法（enabled|disabled）")
	}
	data := &SiteListData{
		NginxAvailable: s.gw.Available() == nil,
		Items:          []SiteItem{},
		Duplicates:     []Duplicate{},
		GeneratedAt:    "",
	}
	if !data.NginxAvailable {
		data.GeneratedAt = formatTS(s.clock())
		return data, nil
	}
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, err
	}
	kept, dups := s.dedupe(scan.Entries)
	idx := s.buildCertIndex(ctx)
	for _, grp := range groupEntries(kept) {
		item := s.aggregateGroup(grp, idx)
		if enabledFilter != "" && item.Enabled != enabledFilter {
			continue
		}
		data.Items = append(data.Items, item)
	}
	data.Duplicates = dups
	if data.Duplicates == nil {
		data.Duplicates = []Duplicate{}
	}
	data.GeneratedAt = formatTS(scan.GeneratedAt)
	return data, nil
}

// SiteDetail GET /sites/site/detail（§4.2.4）：寻址前置链 I-08
// （SITE_001 形态 → SITE_002 实况存在）。
func (s *Service) SiteDetail(ctx context.Context, siteID string) (*SiteDetailData, error) {
	e, err := s.resolveSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	idx := s.buildCertIndex(ctx)
	d := s.projectDetail(e, idx)
	return &d, nil
}

// resolveSite 寻址前置链（I-08）：SITE_001 → 扫描实况 → SITE_002。
func (s *Service) resolveSite(ctx context.Context, siteID string) (*SiteEntry, error) {
	file, index, err := parseSiteID(siteID)
	if err != nil {
		return nil, err
	}
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, err
	}
	e := scan.Find(buildSiteID(file, index))
	if e == nil {
		return nil, errcode.ErrSiteNotFound
	}
	return e, nil
}

// sortCertItems 清单排序（not_after 升序临期在前，NULL 排尾；cert/list 用）。
func sortCertItems(items []CertItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].NotAfter, items[j].NotAfter
		switch {
		case a == nil && b == nil:
			return items[i].ID < items[j].ID
		case a == nil:
			return false
		case b == nil:
			return true
		}
		return *a < *b
	})
}
