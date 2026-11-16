// Package netlisten 是监听端口实况取数适配器（api-firewall.md §4.1.5
// ListenPortSource 契约承载；D1 口径-3「与系统监控采集统一取数来源」——
// monitoring 同源 gopsutil 纯库直读，单一只读通道，无外部命令 R-08/R-09）。
//
// 口径：tcp/tcp6 取 Status=="LISTEN" 的套接字；udp/udp6 无 LISTEN 态（数据
// 报协议），已绑定本地端口的套接字即监听语义（呈现口径，非精确仿真）；
// 监听进程 pid/名称尽力解析，取不到置零值（D-08 不虚构——service 层映射
// 为 process=null / 字段 null）。
package netlisten

import (
	"context"
	"fmt"
	"strings"
	"syscall"

	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// ListenPort 是监听套接字实况条目（api §4.1.1 监听端口对照条目数据源；
// Proto 透传 tcp/tcp6/udp/udp6）。
type ListenPort struct {
	Proto string // tcp | tcp6 | udp | udp6
	Addr  string // 原样监听地址（IP 文本）
	Port  int
	Pid   int32  // 0=取不到
	Name  string // ""=取不到
}

// Source 是监听端口取数通道（gopsutil net.Connections 纯库实现）。
type Source struct{}

// New 构造取数通道。
func New() *Source { return &Source{} }

// List 即时读取监听端口实况（I-01 无缓存；每次调用直读内核套接字表）。
func (s *Source) List(ctx context.Context) ([]ListenPort, error) {
	conns, err := gnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("netlisten: list inet connections: %w", err)
	}
	out := make([]ListenPort, 0, len(conns))
	for _, c := range conns {
		proto := protoOf(c)
		if proto == "" || c.Laddr.Port <= 0 {
			continue // 非 tcp/udp 族或未绑定端口
		}
		if c.Type == syscall.SOCK_STREAM && c.Status != "LISTEN" {
			continue // tcp 仅取 LISTEN
		}
		lp := ListenPort{Proto: proto, Addr: c.Laddr.IP, Port: int(c.Laddr.Port), Pid: c.Pid}
		if c.Pid > 0 { // 进程名尽力解析（失败置空——不虚构，D-08）
			if p, perr := process.NewProcessWithContext(ctx, c.Pid); perr == nil {
				if n, nerr := p.NameWithContext(ctx); nerr == nil {
					lp.Name = n
				}
			}
		}
		out = append(out, lp)
	}
	return out, nil
}

// protoOf 由套接字类型与本地地址族推导协议串（Stream→tcp / Dgram→udp；
// IPv6 地址文本含冒号→追加 6；其它族返回空跳过）。
func protoOf(c gnet.ConnectionStat) string {
	var base string
	switch c.Type {
	case syscall.SOCK_STREAM:
		base = "tcp"
	case syscall.SOCK_DGRAM:
		base = "udp"
	default:
		return ""
	}
	if strings.Contains(c.Laddr.IP, ":") {
		return base + "6"
	}
	return base
}
