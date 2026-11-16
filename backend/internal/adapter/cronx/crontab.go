// Package cronx 是 cron 本机组件适配器（api-cron-backup.md §3.2 进程内契约
// CronTabGateway/CronLogSource/JobRunner 的承载）：crontab 读写投影（root 域
// + 系统预置位只读分区，D-01/D-02）、cron 执行日志尾部适配（D-04）、面板代
// 执行（D-05，shell 语义+进程组终止）。
//
// 外部命令一律 exec.CommandContext 字面量参数数组（R-08）；crontab 写回经
// stdin 管道无临时文件（A-C11）。纯函数（解析/渲染/jobKey）与命令执行分离，
// 便于无 crontab 环境的表驱动测试。
package cronx

import (
	"errors"
	"fmt"
	"strings"
)

// cron_source 枚举（§3.4）：标记注释存在⇔panel，缺失⇔existing（D-02）。
const (
	SourcePanel    = "panel"
	SourceExisting = "existing"
)

// 标记注释语法（D-02）：任务行紧邻上一行两条注释。
const (
	// MarkerManaged 来源标注载体，存在⇔cron_source=panel。
	MarkerManaged = "# dugong: managed"
	// MarkerRemarkPrefix 备注载体前缀（`# dugong: remark: <文本>`）。
	MarkerRemarkPrefix = "# dugong: remark:"
)

// 系统预置位路径（D1 口径-1：只读分区，编译期常量）。
const (
	CronDDir     = "/etc/cron.d"
	CronDirsRoot = "/etc"
)

// CronDirs 是周期脚本目录清单（hourly/daily/weekly/monthly）。
var CronDirs = []string{
	"/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly",
}

// ErrJobNotFound 面板外已被删除（jobKey 无匹配任务行）→ CB_003（如实报错
// 不静默）。
var ErrJobNotFound = errors.New("cronx: cron job not found in crontab")

// CronEntry 是 crontab 任务行投影（读取时组装，§4.3.1 运行时字段清单）。
type CronEntry struct {
	// JobKey 寻址引用（D-01 规范串：规范化表达式 + \t + 规范化命令原文，
	// 多空段压缩；前端不透明透传回）。
	JobKey  string
	Expr    string
	Command string
	// Remark 备注文本指针（无 remark 行则 nil）。
	Remark *string
	// Source panel | existing（标记注释判定，D-02）。
	Source string
}

// CronLine 是系统预置位的任务行粒度条目（/etc/cron.d 文件内）。
type CronLine struct {
	Expr    string `json:"expr"`
	Command string `json:"command"`
}

// CronDFile 是 /etc/cron.d 单文件投影。
type CronDFile struct {
	File    string     `json:"file"`
	Entries []CronLine `json:"entries"`
}

// CronDir 是周期脚本目录投影（脚本文件名清单）。
type CronDir struct {
	Dir     string   `json:"dir"`
	Scripts []string `json:"scripts"`
}

// SystemEntries 是系统预置位只读分区（D-02：文件粒度呈现）。
type SystemEntries struct {
	CronDFiles []CronDFile `json:"cronDFiles"`
	CronDirs   []CronDir   `json:"cronDirs"`
}

// TabProjection 是 crontab 整体投影（root 任务 + 系统预置位）。
type TabProjection struct {
	Entries       []CronEntry   `json:"entries"`
	SystemEntries SystemEntries `json:"systemEntries"`
}

// JobInput 是面板写入的任务定义（Upsert 入参；表达式为归一化五段，D-03）。
type JobInput struct {
	Expr    string
	Command string
	Remark  *string
}

// MakeJobKey 计算 jobKey（D-01：规范化表达式 + \t + 规范化命令，多空段压缩）。
func MakeJobKey(expr, command string) string {
	return normalizeSpaces(expr) + "\t" + normalizeSpaces(command)
}

// normalizeSpaces 压缩空段：任意空白运行（空格/制表符）折叠为单空格并去首尾。
func normalizeSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// SplitJobLine 拆解任务行：五段（6+ 字段）或宏行（@xxx cmd）。第二返回值
// 表示是否为任务行（非注释/非环境变量/形态可解析）。
func SplitJobLine(line string) (expr, command string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	if isEnvLine(trimmed) {
		return "", "", false
	}
	fields := strings.Fields(trimmed)
	if strings.HasPrefix(fields[0], "@") {
		if len(fields) < 2 {
			return "", "", false
		}
		return fields[0], strings.Join(fields[1:], " "), true
	}
	if len(fields) < 6 {
		return "", "", false
	}
	return strings.Join(fields[0:5], " "), strings.Join(fields[5:], " "), true
}

// isEnvLine 判定 crontab 环境变量行（首字段含 '=' 且名字段无空白）。
func isEnvLine(trimmed string) bool {
	idx := strings.Index(trimmed, "=")
	if idx <= 0 {
		return false
	}
	return !strings.ContainsAny(trimmed[:idx], " \t")
}

// IsManagedLine / ExtractRemark 判定/提取标记注释（D-02 语法）。
func IsManagedLine(line string) bool {
	return strings.TrimSpace(line) == MarkerManaged
}

// ExtractRemark 从注释行提取备注文本；非 remark 标记行返回 ok=false。
func ExtractRemark(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, MarkerRemarkPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, MarkerRemarkPrefix)), true
}

// ExtractRemarkOK 判定是否为 remark 标记行（布尔形态，邻接判定用）。
func ExtractRemarkOK(line string) bool {
	_, ok := ExtractRemark(line)
	return ok
}

// ParseTabLines 解析 crontab 内容为任务投影（行序如实呈现，D-02 不重排）：
// 任务行紧邻上一行为 managed 标记⇔panel，再上一行为 remark 行则提取备注。
func ParseTabLines(lines []string) []CronEntry {
	entries := make([]CronEntry, 0, 8)
	for i, line := range lines {
		expr, command, ok := SplitJobLine(line)
		if !ok {
			continue
		}
		e := CronEntry{
			JobKey:  MakeJobKey(expr, command),
			Expr:    expr,
			Command: command,
			Source:  SourceExisting,
		}
		// 邻接结构（D-02）：任务行紧邻上一行=managed 标记或 remark，remark 的
		// 再上一行为 managed（managed 必在）。
		if i > 0 && IsManagedLine(lines[i-1]) {
			e.Source = SourcePanel
		} else if i > 1 && ExtractRemarkOK(lines[i-1]) && IsManagedLine(lines[i-2]) {
			e.Source = SourcePanel
			remark, _ := ExtractRemark(lines[i-1])
			e.Remark = &remark
		}
		entries = append(entries, e)
	}
	return entries
}

// RenderTabJob 在 crontab 行集合上执行编辑语义（D-02）：oldJobKey 非空=删旧
// （任务行+其紧邻标记注释）后追加新三行（update=删旧写新，jobKey 随内容
// 变更）；oldJobKey 空=纯追加（create）。返回新行集与新 jobKey。
func RenderTabJob(lines []string, oldJobKey string, job JobInput) ([]string, string, error) {
	if oldJobKey != "" {
		var err error
		lines, err = removeJobLines(lines, oldJobKey)
		if err != nil {
			return nil, "", err
		}
	}
	expr := normalizeSpaces(job.Expr)
	command := normalizeSpaces(job.Command)
	out := make([]string, 0, len(lines)+3)
	out = append(out, lines...)
	if len(out) > 0 && out[len(out)-1] != "" {
		out = append(out, "") // 与既有内容空行分隔
	}
	out = append(out, MarkerManaged)
	if job.Remark != nil && strings.TrimSpace(*job.Remark) != "" {
		out = append(out, MarkerRemarkPrefix+" "+strings.TrimSpace(*job.Remark))
	}
	out = append(out, expr+" "+command)
	return out, MakeJobKey(expr, command), nil
}

// RemoveTabJob 删除任务行及其紧邻标记注释行（D-02）。
func RemoveTabJob(lines []string, jobKey string) ([]string, error) {
	return removeJobLines(lines, jobKey)
}

// removeJobLines 定位 jobKey 任务行并连同其紧邻标记注释（managed 与可选
// remark，均倒序紧邻）一并移除；未找到返回 ErrJobNotFound。
func removeJobLines(lines []string, jobKey string) ([]string, error) {
	idx := -1
	for i, line := range lines {
		expr, command, ok := SplitJobLine(line)
		if ok && MakeJobKey(expr, command) == jobKey {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: jobKey not matched", ErrJobNotFound)
	}
	start := idx
	// 邻接结构（D-02）：任务行紧邻上一行=可选 remark，再上一行=managed。
	if start > 0 {
		if _, isRemark := ExtractRemark(lines[start-1]); isRemark {
			start--
		}
	}
	if start > 0 && IsManagedLine(lines[start-1]) {
		start--
	}
	out := make([]string, 0, len(lines))
	out = append(out, lines[:start]...)
	out = append(out, lines[idx+1:]...)
	return out, nil
}

// ParseCronDFile 解析 /etc/cron.d 单文件任务行（任务行粒度；命令含用户域
// 呈现，不解释——如实投影）。
func ParseCronDFile(content string) []CronLine {
	entries := make([]CronLine, 0, 8)
	for _, line := range strings.Split(content, "\n") {
		expr, command, ok := SplitJobLine(line)
		if !ok {
			continue
		}
		entries = append(entries, CronLine{Expr: expr, Command: command})
	}
	return entries
}
