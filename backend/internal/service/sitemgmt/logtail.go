package sitemgmt

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// LogTailService（§4.2.1，D-07）：路径解析链 ⚠️A15（server 块 access_log/
// error_log 指令 → nginx 默认路径回退）→ 仍缺失 SITE_011（如实提示不虚构）；
// 只读当前主日志（轮转归档不跟踪）；尾部 lines 行（默认 100 上限 1000）。

// nginxErrorLogTS nginx error log 行首时间戳形态（2026/11/14 10:00:00）。
var nginxErrorLogTS = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})\s*(.*)$`)

// TailLog GET /sites/log/tail（admin+readonly）。
func (s *Service) TailLog(ctx context.Context, siteID, logType, linesParam string) (*LogTailData, error) {
	switch logType {
	case LogTypeAccess, LogTypeError:
	case "":
		return nil, fieldError("type", "type 必填（access|error）")
	default:
		return nil, fieldError("type", "type 非法（access|error）")
	}
	lines := LogTailDefault
	if strings.TrimSpace(linesParam) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(linesParam))
		if err != nil || n < 1 {
			return nil, fieldError("lines", "lines 须为正整数")
		}
		if n > LogTailMax {
			n = LogTailMax
		}
		lines = n
	}
	e, err := s.resolveSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	logPath := resolveLogPath(e, logType)
	if logPath == "" || !s.gw.Exists(logPath) {
		return nil, errcode.ErrLogNotFound.WithData(ReasonData{
			Reason: fmt.Sprintf("日志路径缺失（%s）", logPath),
		})
	}
	ls, truncated, rerr := s.tailLines(logPath, lines)
	if rerr != nil {
		return nil, errcode.ErrLogNotFound.WithData(ReasonData{Reason: rerr.Error()})
	}
	return &LogTailData{SiteID: siteID, Type: logType, LogPath: logPath, Lines: ls, Truncated: truncated}, nil
}

// resolveLogPath ⚠️A15 路径解析链：server 块指令 → nginx 默认路径回退。
func resolveLogPath(e *SiteEntry, logType string) string {
	switch logType {
	case LogTypeAccess:
		if e.Block.AccessLog != "" && e.Block.AccessLog != "off" {
			return e.Block.AccessLog
		}
		return NginxDefaultAccessLog
	case LogTypeError:
		if e.Block.ErrorLog != "" {
			return e.Block.ErrorLog
		}
		return NginxDefaultErrorLog
	}
	return ""
}

// tailLines 从文件尾部读取最后 n 行（块状回探，避免整文件进内存；日志文件
// 可能很大）。truncated=文件实际行数多于返回行数。
func (s *Service) tailLines(path string, n int) ([]LogLine, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("site-mgmt: open log %s: %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("site-mgmt: stat log %s: %w", path, err)
	}
	const chunk = 16 * 1024
	size := st.Size()
	var sb strings.Builder
	offset := size
	for offset > 0 && sb.Len() < 2*chunk { // 只需略多于尾部 n 行的窗口
		step := int64(chunk)
		if offset < step {
			step = offset
		}
		offset -= step
		buf := make([]byte, step)
		if _, rerr := f.ReadAt(buf, offset); rerr != nil {
			return nil, false, fmt.Errorf("site-mgmt: read log %s: %w", path, rerr)
		}
		sb.Write(buf)
		if countNewlines(sb.String()) > n+1 {
			break
		}
	}
	raw := strings.ReplaceAll(sb.String(), "\r\n", "\n")
	raw = strings.TrimPrefix(raw, "\n") // 窗口起点可能截断行首——首残行丢弃
	all := strings.Split(raw, "\n")
	if offset > 0 && len(all) > 0 {
		all = all[1:]
	}
	all = trimTrailingEmpty(all)
	if len(all) == 0 {
		return []LogLine{}, false, nil
	}
	truncated := len(all) > n
	start := 0
	if truncated {
		start = len(all) - n
	}
	out := make([]LogLine, 0, n)
	for _, ln := range all[start:] {
		out = append(out, parseLogLine(ln))
	}
	return out, truncated, nil
}

// parseLogLine 拆解行首时间戳（error log 形态；access log 多无时间戳——
// ts 省略原样呈现，D-07「无时间戳行原样呈现」）。
func parseLogLine(ln string) LogLine {
	if m := nginxErrorLogTS.FindStringSubmatch(ln); m != nil {
		ts := m[1]
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", ts, time.Local); err == nil {
			formatted := t.UTC().Format(time.RFC3339)
			return LogLine{TS: &formatted, Text: m[2]}
		}
	}
	return LogLine{Text: ln}
}

func countNewlines(s string) int { return strings.Count(s, "\n") }

// trimTrailingEmpty 去除尾部空行（文件以换行结尾产生的空元素）。
func trimTrailingEmpty(ss []string) []string {
	for len(ss) > 0 && strings.TrimSpace(ss[len(ss)-1]) == "" {
		ss = ss[:len(ss)-1]
	}
	return ss
}

// NginxReload POST /sites/nginx/reload（D-05）：用户主动平滑 reload；失败
// SITE_004（配置保留）——失败亦记审计 result=failure（api §4.2.4 明文）。
func (s *Service) NginxReload(ctx context.Context, actor *authsvc.Identity) (*ReloadData, error) {
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	out, err := s.gw.Reload(ctx)
	if err != nil {
		if isChannelErr(err) {
			return nil, errcode.ErrNginxUnavailable.WithData(ReasonData{Reason: err.Error()})
		}
		s.auditAction(ctx, actor, AuditActionNginxReloaded, ObjectTypeSite, "",
			map[string]any{"output": out}, false)
		return nil, errcode.ErrReloadFailed.WithData(SiteErrorData{Output: out})
	}
	s.auditAction(ctx, actor, AuditActionNginxReloaded, ObjectTypeSite, "", nil, true)
	return &ReloadData{Reloaded: true}, nil
}

// ListPhpPools GET /sites/php-pools/list（D-14 实时读取；无 pool 空数组）。
func (s *Service) ListPhpPools(ctx context.Context) (*PhpPoolsData, error) {
	pools, err := s.phpfpm.ListPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: list php pools: %w", err)
	}
	out := make([]PhpPoolDTO, 0, len(pools))
	for _, p := range pools {
		out = append(out, PhpPoolDTO{Name: p.Name, Socket: p.Socket})
	}
	return &PhpPoolsData{Pools: out}, nil
}
