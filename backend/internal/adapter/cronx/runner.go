package cronx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNoLogSource 三级来源均不可用（journalctl 缺失 + /var/log/syslog 与
// /var/log/cron 均不存在）——如实上抛由服务层脱敏呈现（D-04 适配归 cronx）。
var ErrNoLogSource = errors.New("cronx: no cron log source available")

// 日志来源路径（D-04 可用性顺序：journalctl -u cron → /var/log/syslog →
// /var/log/cron；编译期常量）。
const (
	SyslogPath  = "/var/log/syslog"
	CronLogPath = "/var/log/cron"
)

// LogSource 是 CronLogSource 实现（D-04）：按可用性适配取尾部 lines 行。
type LogSource struct {
	journalctlBin string
	lookupErr     error
}

// NewLogSource 构造（journalctlBin 默认 "journalctl"，exec.LookPath 预解析；
// 未检出直接降级文件源）。
func NewLogSource(journalctlBin string) *LogSource {
	if journalctlBin == "" {
		journalctlBin = "journalctl"
	}
	ls := &LogSource{journalctlBin: journalctlBin}
	if _, err := exec.LookPath(journalctlBin); err != nil {
		ls.lookupErr = err
	}
	return ls
}

// Tail 取 cron 执行日志尾部（纯文本片段，非 JSON 数组——日志本质为行流）。
func (l *LogSource) Tail(ctx context.Context, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if l.lookupErr == nil {
		cmd := exec.CommandContext(ctx, l.journalctlBin,
			"--no-pager", "-n", strconv.Itoa(lines), "-u", "cron") // 字面量参数数组
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return stdout.String(), nil
		}
		// journalctl 存在但查询失败（无日志/权限）→ 降级文件源（D-04 适配）。
	}
	if text, ok := tailFileLines(SyslogPath, lines); ok {
		return text, nil
	}
	if text, ok := tailFileLines(CronLogPath, lines); ok {
		return text, nil
	}
	return "", ErrNoLogSource
}

// tailFileLines 纯 Go 文件尾部读取：从文件尾按预估窗口回读，切行取尾部 n 行
// （避免整文件载入——syslog 可达 GB 级）。
func tailFileLines(path string, n int) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return "", false
	}
	window := int64(n) * 512
	if window < 64<<10 {
		window = 64 << 10
	}
	var offset int64
	if st.Size() > window {
		offset = st.Size() - window
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", false
	}
	buf, err := io.ReadAll(io.LimitReader(f, window))
	if err != nil {
		return "", false
	}
	all := strings.Split(strings.TrimSuffix(string(buf), "\n"), "\n")
	if offset > 0 && len(all) > 0 {
		all = all[1:] // 窗口首行大概率被截断，丢弃（尾部语义不受影响）
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	if len(all) == 1 && all[0] == "" {
		return "", true
	}
	return strings.Join(all, "\n"), true
}

// TriggerResult 是手动触发代执行结果（D-05 data 载荷；执行失败/超时为业务
// 结果非请求错误——200+code=0）。
type TriggerResult struct {
	ExitCode   *int   `json:"exitCode"` // null=未正常退出（信号终止等）
	Output     string `json:"output"`   // stdout+stderr 合并，尾部截断 64KB
	TimedOut   bool   `json:"timedOut"`
	DurationMs int64  `json:"durationMs"`
}

// OutputMaxBytes 输出尾部截断上限（D-05 编译期常量）。
const OutputMaxBytes = 64 << 10

// OutputTruncationMark 头部省略标记（超限保留尾部）。
const OutputTruncationMark = "…(output exceeds 64KB, head omitted)\n"

// Runner 是 JobRunner 实现（D-05）：以面板运行用户身份 shell 语义代执行、
// 超时进程组终止（平台分离）；命令原文由调用方取自 crontab 记录（I-08）。
type Runner struct {
	shellBin string
}

// NewRunner 构造（shellBin 默认 "/bin/sh"）。
func NewRunner(shellBin string) *Runner {
	if shellBin == "" {
		shellBin = "/bin/sh"
	}
	return &Runner{shellBin: shellBin}
}

// Run 代执行（A-C2：命令串为独立 argv 元素，零拼接——与 cron 自身执行语义
// 同信任级；G-01 人工核对登记见 tasks.md T07）。
func (r *Runner) Run(ctx context.Context, command string, timeout time.Duration) TriggerResult {
	started := time.Now()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, r.shellBin, "-c", command) // argv 数组：["/bin/sh","-c",<原文>]
	setProcGroup(cmd)                                           // 平台：进程组（超时整组终止）
	cmd.Cancel = func() error { return killProcGroup(cmd) }     // 超时→组终止（非仅 shell 进程）
	var merged bytes.Buffer
	cmd.Stdout = &merged
	cmd.Stderr = &merged
	_ = cmd.Run() // 退出码/超时即业务结果（D-05），非通道错误

	res := TriggerResult{Output: truncateTail(merged.String(), OutputMaxBytes)}
	if cmd.ProcessState != nil {
		if code := cmd.ProcessState.ExitCode(); code >= 0 {
			res.ExitCode = &code
		}
	}
	if cctx.Err() != nil {
		res.TimedOut = cctx.Err() == context.DeadlineExceeded
	}
	res.DurationMs = time.Since(started).Milliseconds()
	return res
}

// truncateTail 尾部截断（D-05：超限保留尾部+头部省略标记；按字节窗口回退至
// UTF-8 边界避免劈开多字节字符）。
func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && isContinuationByte(s[cut]) {
		cut++
	}
	return OutputTruncationMark + s[cut:]
}

// isContinuationByte 判定 UTF-8 续字节（0b10xxxxxx）。
func isContinuationByte(b byte) bool { return b&0xC0 == 0x80 }
