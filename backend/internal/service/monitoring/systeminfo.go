package monitoring

import (
	"context"
	"os"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

// readSystemInfo 是 SystemInfoReader 领域服务（api §4.2.1）：每次请求实时读取
// 本机组装响应（I-07 不缓存）；部分失败失败字段 null + errors[]（I-08/D-08 不
// 虚构默认值）；全部字段失败仍返回结构体（读取行为本身不视为服务错误，§4.2.4）。
func readSystemInfo(ctx context.Context) SystemInfoData {
	var data SystemInfoData
	var errs []InfoFieldError

	// distro/kernel/uptime 同源于一次 host.Info 读取；失败一并登记。
	if info, ok := safeRead(ctx, "host.info", func() (*host.InfoStat, error) {
		return host.InfoWithContext(ctx)
	}); ok {
		if info.Platform != "" {
			d := strings.TrimSpace(info.Platform + " " + info.PlatformVersion)
			data.Distro = &d
		}
		if info.KernelVersion != "" {
			k := info.KernelVersion
			data.Kernel = &k
		}
		u := info.Uptime
		data.UptimeSeconds = &u
	} else {
		errs = append(errs,
			InfoFieldError{Field: "distro", Reason: "主机信息读取失败"},
			InfoFieldError{Field: "kernel", Reason: "主机信息读取失败"},
			InfoFieldError{Field: "uptimeSeconds", Reason: "主机信息读取失败"})
	}

	if hn, err := os.Hostname(); err == nil && hn != "" {
		data.Hostname = &hn
	} else {
		errs = append(errs, InfoFieldError{Field: "hostname", Reason: "主机名读取失败"})
	}

	if len(errs) > 0 {
		data.Errors = errs
	}
	return data
}
