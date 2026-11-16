// matrix.go 承载状态映射动作矩阵（BA 附录B.2 唯一口径；D-02/D-04）：
// supportedActions 服务端权威动作面（list 响应与 DK_004 data 同源计算），
// 过渡态（restarting/removing）为空数组恒拒。
package dockermgmt

import "github.com/dougong/dougong/internal/adapter/dockerx"

// SupportedActions 计算当前态的权威动作面（六动作子集，containerActionOrder
// 序）：start←stopped；stop/restart←running|paused；pause←running；
// unpause←paused；delete←stopped；过渡态恒空（D-02/D-04）。
func SupportedActions(state string) []string {
	var allowed map[string]bool
	switch state {
	case dockerx.StatusRunning:
		allowed = map[string]bool{
			dockerx.ActionStop: true, dockerx.ActionRestart: true, dockerx.ActionPause: true}
	case dockerx.StatusPaused:
		allowed = map[string]bool{
			dockerx.ActionStop: true, dockerx.ActionRestart: true, dockerx.ActionUnpause: true}
	case dockerx.StatusStopped:
		allowed = map[string]bool{
			dockerx.ActionStart: true, dockerx.ActionDelete: true}
	default:
		return []string{} // 过渡态（restarting/removing）：空数组
	}
	actions := make([]string, 0, len(allowed))
	for _, a := range containerActionOrder {
		if allowed[a] {
			actions = append(actions, a)
		}
	}
	return actions
}

// ActionAllowed 判定动作是否在当前态动作面内（DK_004 守卫依据，D-04）。
func ActionAllowed(state, action string) bool {
	for _, a := range SupportedActions(state) {
		if a == action {
			return true
		}
	}
	return false
}
