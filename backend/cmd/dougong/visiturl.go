package main

import (
	"log/slog"
	"net"
	"strings"
)

// visitURLs 由 scheme 与绑定地址枚举可直接访问的候选 URL（纯函数，v2.1
// D-P17：服务启动日志与 CLI status 访问 URL 列举共用同一结论）。绑定
// 地址 0.0.0.0/:: 不可直接访问——枚举本机非环回 IPv4（至多 4 个防刷屏）
// 帮助首访用户定位入口；绑定到具体地址时仅列该地址；枚举失败回退环回。
// 返回空切片=地址不可解析（调用方自行决定呈现方式）。
func visitURLs(scheme, addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
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
	return urls
}

// accessURLs 访问地址纯函数：绑定域名时仅返回域名 URL——HostGuard（ADR-028）
// 在绑定后拦截一切非绑定 Host，列出 IP 只会误导；未绑定时回退本机非环回 IPv4
// 枚举（visitURLs）。非默认端口显式携带（https 默认 443 / http 默认 80 省略）；
// entryPath 非空统一追加前缀（D-P03）。scheme/addr/域名/入口路径来自面板配置，
// 调用方负责读取（status/visit-urls/启动日志三处共用同一结论）。
func accessURLs(scheme, addr, domain, entryPath string) []string {
	var urls []string
	if domain != "" {
		u := scheme + "://" + domain
		if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
			if !(scheme == "https" && port == "443") && !(scheme == "http" && port == "80") {
				u += ":" + port
			}
		}
		urls = []string{u + "/"}
	} else {
		urls = visitURLs(scheme, addr)
	}
	if entryPath != "" {
		for i, u := range urls {
			urls[i] = strings.TrimSuffix(u, "/") + "/" + entryPath
		}
	}
	return urls
}

// logVisitURLs 服务启动时打印可直接访问的候选 URL（accessURLs 的日志消费方）。
// 绑定域名时仅列域名地址并附注（IP 已被 HostGuard 拦截）。
func logVisitURLs(scheme, addr, domain, entryPath string) {
	urls := accessURLs(scheme, addr, domain, entryPath)
	if len(urls) == 0 {
		return
	}
	if domain != "" {
		slog.Info("panel ready — open in browser", "urls", urls, "note", "已绑定域名，仅此地址可访问（IP 直连已被拦截）")
		return
	}
	slog.Info("panel ready — open in browser", "urls", urls)
}
