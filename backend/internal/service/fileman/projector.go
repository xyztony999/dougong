package fileman

import (
	"cmp"
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
)

// ListDir GET /files/entry/list 编排（I-08 校验前置 → I-06 即时投影 → D-03
// 服务端排序）。查看类不审计（A-F10）。
//
// A-F20（Gate 2 E1 修复）：path 缺省（空串/未携带）=受管根——前端首屏
// 不带 path 浏览（frontend/tasks.md 实现决策 1 联调口径）；readonly 无
// path-scope/get 权、无从预知受管根，缺省浏览只能由服务端解析。显式 path
// 语义不变（§4.2.4）。
func (s *Service) ListDir(ctx context.Context, path, sortBy, order string) (*ListData, error) {
	if sortBy == "" {
		sortBy = SortFieldName // 默认 name（D-03）
	}
	if order == "" {
		order = OrderAsc // 默认 asc
	}
	if sortBy != SortFieldName && sortBy != SortFieldSize && sortBy != SortFieldMtime {
		return nil, errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "sort", Message: "取值域 name|size|mtime",
		})
	}
	if order != OrderAsc && order != OrderDesc {
		return nil, errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "order", Message: "取值域 asc|desc",
		})
	}
	if strings.TrimSpace(path) == "" {
		path = s.guard.Snapshot().RootPath // A-F20：缺省=受管根
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		// 不存在与非目录合并 FILE_012（A-F16：list 校验链只列 FILE_012）。
		return nil, s.mapResolveErr(err, errcode.ErrFileNotADirectory)
	}
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	if snap.EntryType != TypeDirectory {
		return nil, errcode.ErrFileNotADirectory
	}
	entries, err := s.gw.ListDir(ctx, real)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	items := make([]EntryItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, EntryItem{
			Name:       e.Name,
			Path:       filepath.Join(real, e.Name),
			EntryType:  e.EntryType,
			Size:       e.Size,
			Mode:       e.Mode,
			OwnerUser:  e.OwnerUser,
			OwnerGroup: e.OwnerGroup,
			Mtime:      formatTS(e.Mtime),
		})
	}
	sortEntries(items, sortBy, order)
	return &ListData{
		Path:        real,
		Items:       items,
		Total:       len(items),
		GeneratedAt: formatTS(s.clock()),
	}, nil
}

// sortEntries 服务端排序（D-03：name|size|mtime × asc|desc；size/mtime 同值
// 回退名称字典序升序，保证确定性）。
func sortEntries(items []EntryItem, sortBy, order string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if sortBy == SortFieldName {
			if order == OrderDesc {
				return a.Name > b.Name
			}
			return a.Name < b.Name
		}
		var c int
		if sortBy == SortFieldSize {
			c = cmp.Compare(a.Size, b.Size)
		} else {
			c = strings.Compare(a.Mtime, b.Mtime) // RFC3339 UTC 字典序即时间序
		}
		if order == OrderDesc {
			if c != 0 {
				return c > 0
			}
			return a.Name < b.Name
		}
		if c != 0 {
			return c < 0
		}
		return a.Name < b.Name
	})
}

func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }
