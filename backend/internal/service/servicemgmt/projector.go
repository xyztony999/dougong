package servicemgmt

import (
	"sort"
	"strings"

	"github.com/dougong/dougong/internal/adapter/sysd"
)

// ServiceProjector（api §4.1.1 领域服务）：清单投影组装 + 服务端状态过滤/
// 名称搜索（D-02/D-03，R-14——过滤搜索在服务端执行）。

// projectList 全量投影 → 过滤（state 多值命中）→ 搜索（名称子串不区分大小写）
// → 名称字典序升序（D-02）。canOperate/canToggleAutostart 为服务端权威计算
// 字段（I-07），随条目下发。
func projectList(units []sysd.UnitSnapshot, states map[string]bool, search string) []UnitListItem {
	q := strings.ToLower(search)
	items := make([]UnitListItem, 0, len(units))
	for _, u := range units {
		if states != nil && !states[u.ActiveState] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(u.Name), q) {
			continue
		}
		items = append(items, UnitListItem{
			Name:               u.Name,
			UnitType:           u.UnitType,
			ActiveState:        u.ActiveState,
			SubState:           u.SubState,
			EnabledState:       u.EnabledState,
			CanOperate:         canOperate(u.UnitType),
			CanToggleAutostart: canToggleAutostart(u.UnitType, u.EnabledState),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// projectDetail 单 unit 属性快照 → detail DTO（description/loadState 空值
// 落 null，§4.1.4 detail 响应）。
func projectDetail(d sysd.UnitDetail) *DetailData {
	data := &DetailData{
		Name:               d.Name,
		UnitType:           d.UnitType,
		ActiveState:        d.ActiveState,
		SubState:           d.SubState,
		EnabledState:       d.EnabledState,
		CanOperate:         canOperate(d.UnitType),
		CanToggleAutostart: canToggleAutostart(d.UnitType, d.EnabledState),
	}
	if d.Description != "" {
		s := d.Description
		data.Description = &s
	}
	if d.LoadState != "" {
		s := d.LoadState
		data.LoadState = &s
	}
	return data
}
