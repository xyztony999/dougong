package servicemgmt

import (
	"strings"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
)

// UnitActionGuard 统一校验链（api §4.1.4 跨接口复用规则，I-04）：unit 名称
// 形态（SVC_001）→ 存在性（SVC_003，实况经 Gateway.GetUnit）→ 类型边界
// （SVC_004）→ 两态主轴（SVC_005）；逐级返回，D-09 校验顺序。纯函数部分
// 在本文件，存在性在 service/action 编排中执行。

// guardUnitName 校验 unit 名称形态（SVC_001：官方后缀集正则，空名/畸形名
// 均非法——R-14 全部服务端校验）。
func guardUnitName(name string) *errcode.Error {
	if !unitNameRe.MatchString(name) {
		return errcode.ErrSvcInvalidUnitName
	}
	return nil
}

// guardUnitType 校验动作类型边界（SVC_004：仅 .service 可操作，I-04）。
// 须在 guardUnitName 之后调用（形态合法保证后缀在官方全集内，类型由后缀
// 归一——§4.1.1 unitType 口径）。
func guardUnitType(name string) *errcode.Error {
	if sysd.UnitTypeOf(name) != TypeService {
		return errcode.ErrSvcUnitTypeNotActionable
	}
	return nil
}

// guardToggleable 校验自启两态主轴（SVC_005：enabledState 非 enabled/disabled
// 精确两态即不可切换，⚠️A14/D-05 权威口径；A-S4）。
func guardToggleable(enabledState string) *errcode.Error {
	if enabledState != EnabledStateEnabled && enabledState != EnabledStateDisabled {
		return errcode.ErrSvcAutostartNotToggleable
	}
	return nil
}

// canOperate 服务端权威计算操作入口（I-07/D-05：unitType==service）。
func canOperate(unitType string) bool { return unitType == TypeService }

// canToggleAutostart 服务端权威计算自启切换入口（⚠️A14：canOperate ∧
// enabledState∈两态主轴；前端不自行推断）。
func canToggleAutostart(unitType, enabledState string) bool {
	return canOperate(unitType) && guardToggleable(enabledState) == nil
}

// parseStateFilter 解析 list 的 state 过滤参数（SVC_002：逗号分隔
// service_active_state 多值，如 active,failed；空串=不过滤；任一值非法整体
// 拒绝——严格匹配不做空白宽容，R-14）。
func parseStateFilter(raw string) (map[string]bool, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	allowed := map[string]bool{
		ActiveStateActive: true, ActiveStateInactive: true, ActiveStateFailed: true,
	}
	out := make(map[string]bool, len(parts))
	for _, p := range parts {
		if !allowed[p] {
			return nil, errcode.ErrSvcInvalidStateFilter
		}
		out[p] = true
	}
	return out, nil
}
