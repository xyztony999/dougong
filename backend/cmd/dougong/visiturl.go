package main

import (
	"log/slog"
	"net"
)

// logVisitURLs 打印可直接访问的候选 URL。listen 日志的 addr=0.0.0.0:port 是
// 绑定地址、不可直接当访问地址用——枚举本机非环回 IPv4（至多 4 个防刷屏）
// 帮助首访用户定位入口；绑定到具体地址时仅列该地址；枚举失败回退环回。
func logVisitURLs(scheme, addr string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	var hosts []string
	switch host {
	case "", "0.0.0.0", "::":
		ifaces, ierr := net.Interfaces()
		if ierr == nil {
			for _, ifc := range ifaces {
				if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
					continue
				}
				addrs, aerr := ifc.Addrs()
				if aerr != nil {
					continue
				}
				for _, a := range addrs {
					ipnet, ok := a.(*net.IPNet)
					if !ok {
						continue
					}
					ip4 := ipnet.IP.To4()
					if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
						continue
					}
					hosts = append(hosts, ip4.String())
				}
			}
		}
		if len(hosts) == 0 {
			hosts = append(hosts, "127.0.0.1")
		}
	default:
		hosts = []string{host}
	}
	urls := make([]string, 0, len(hosts))
	for i, h := range hosts {
		if i >= 4 {
			break
		}
		urls = append(urls, scheme+"://"+net.JoinHostPort(h, port)+"/")
	}
	slog.Info("panel ready — open in browser", "urls", urls)
}
