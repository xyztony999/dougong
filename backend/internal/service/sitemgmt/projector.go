package sitemgmt

import (
	"context"
	"sort"

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

// projectItem 组装 list 项（§4.2.1 投影条目）。
func (s *Service) projectItem(e *SiteEntry, idx *certPathIndex) SiteItem {
	item := SiteItem{
		SiteID:      e.SiteID,
		ServerNames: nonEmpty(e.Block.ServerNames),
		Listen:      nonEmpty(e.Block.Listen),
		Root:        e.Block.Root,
		Enabled:     e.Enabled(),
		SourceFile:  e.File,
	}
	if e.Block.SSLCert != "" {
		ref := e.Block.SSLCert
		item.CertRef = &ref
	}
	if row, ok := idx.matchCert(e.Block.SSLCert); ok {
		id := row.ID
		status := s.computeStatus(row, s.clock())
		item.CertID = &id
		item.CertStatus = &status
	}
	if p := e.Block.Proxy(); p != nil {
		item.Proxy = &ProxyDTO{Upstream: p.Upstream, HostHeader: p.HostHeader, Websocket: p.Websocket}
	}
	return item
}

// projectDetail 组装 detail 项（list 项全字段 + locations/rawBlocks）。
func (s *Service) projectDetail(e *SiteEntry, idx *certPathIndex) SiteDetailData {
	d := SiteDetailData{SiteItem: s.projectItem(e, idx)}
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
	for i := range kept {
		if enabledFilter != "" && kept[i].Enabled() != enabledFilter {
			continue
		}
		data.Items = append(data.Items, s.projectItem(&kept[i], idx))
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
