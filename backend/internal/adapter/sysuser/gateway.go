// Package sysuser 是系统用户实况只读网关适配器（api-terminal.md §4.1.5
// SysUserDirectory 契约：/etc/passwd 语义直读，D-02——与 monitoring
// gopsutil / service-mgmt dbus 同口径的本机资源直读，不复用 file-manager
// 跨模块 HTTP 自调用）。Windows 开发机为退化实现（dir_windows.go）。
package sysuser

// Gateway 实现 terminal.SysUserDirectory。
type Gateway struct{}

// New 构造网关（无状态：每次调用实时读取系统实况，无缓存）。
func New() *Gateway { return &Gateway{} }
