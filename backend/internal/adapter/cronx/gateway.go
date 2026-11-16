package cronx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Gateway 是 CronTabGateway 实现（§3.2）：crontab -l 读 / crontab - 从 stdin
// 写回（参数数组，A-C11）；系统预置位 /etc/cron.d 与四个周期目录纯 Go 只读。
type Gateway struct {
	crontabBin string
	lookupErr  error
}

// New 构造网关（crontabBin 默认 "crontab"，经 exec.LookPath 预解析）。
func New(crontabBin string) *Gateway {
	if crontabBin == "" {
		crontabBin = "crontab"
	}
	g := &Gateway{crontabBin: crontabBin}
	if _, err := exec.LookPath(crontabBin); err != nil {
		g.lookupErr = fmt.Errorf("cronx: locate crontab binary %q: %w", crontabBin, err)
	}
	return g
}

// Available 通道可用性（二进制未检出时携带原因）。
func (g *Gateway) Available() error { return g.lookupErr }

// ReadRaw 读取 root crontab 原文（crontab -l）；「no crontab for <user>」
// 视为合法空表（首建前状态，如实归零）。
func (g *Gateway) ReadRaw(ctx context.Context) (string, error) {
	if g.lookupErr != nil {
		return "", g.lookupErr
	}
	cmd := exec.CommandContext(ctx, g.crontabBin, "-l") // 字面量参数数组
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(strings.ToLower(stderr.String()), "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("cronx: crontab -l: %w: %s", err, firstLine(stderr.String()))
	}
	return stdout.String(), nil
}

// WriteRaw 写回 root crontab（crontab - 从 stdin 读入新表；无临时文件）。
func (g *Gateway) WriteRaw(ctx context.Context, content string) error {
	if g.lookupErr != nil {
		return g.lookupErr
	}
	cmd := exec.CommandContext(ctx, g.crontabBin, "-") // 字面量参数数组
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cronx: crontab install: %w: %s", err, firstLine(stderr.String()))
	}
	return nil
}

// List 投影 root 任务 + 系统预置位只读分区（§4.3.4 job/list data 组装源）。
func (g *Gateway) List(ctx context.Context) (*TabProjection, error) {
	raw, err := g.ReadRaw(ctx)
	if err != nil {
		return nil, err
	}
	proj := &TabProjection{Entries: ParseTabLines(splitLines(raw))}
	sys, err := g.readSystemEntries()
	if err != nil {
		return nil, err
	}
	proj.SystemEntries = sys
	return proj, nil
}

// Upsert 执行编辑语义（D-02：oldJobKey 非空=删旧写新；空=追加），返回新
// jobKey（随内容变更，如实）。
func (g *Gateway) Upsert(ctx context.Context, oldJobKey string, job JobInput) (string, error) {
	raw, err := g.ReadRaw(ctx)
	if err != nil {
		return "", err
	}
	lines, newKey, err := RenderTabJob(splitLines(raw), oldJobKey, job)
	if err != nil {
		return "", err
	}
	if err := g.WriteRaw(ctx, strings.Join(lines, "\n")+"\n"); err != nil {
		return "", err
	}
	return newKey, nil
}

// Delete 删除任务行及其紧邻标记注释（面板外已删返回 ErrJobNotFound→CB_003）。
func (g *Gateway) Delete(ctx context.Context, jobKey string) error {
	raw, err := g.ReadRaw(ctx)
	if err != nil {
		return err
	}
	lines, err := RemoveTabJob(splitLines(raw), jobKey)
	if err != nil {
		return err
	}
	return g.WriteRaw(ctx, strings.Join(lines, "\n")+"\n")
}

// readSystemEntries 读取系统预置位（/etc/cron.d 文件任务行 + 周期目录脚本名；
// 目录/文件缺失=分区为空，如实呈现——非 Linux 环境无此路径）。
func (g *Gateway) readSystemEntries() (SystemEntries, error) {
	sys := SystemEntries{
		CronDFiles: make([]CronDFile, 0, 4),
		CronDirs:   make([]CronDir, 0, 4),
	}
	entries, err := os.ReadDir(CronDDir)
	if err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			content, rerr := os.ReadFile(CronDDir + "/" + name)
			if rerr != nil {
				continue // 单文件不可读跳过（如实呈现可读面）
			}
			sys.CronDFiles = append(sys.CronDFiles, CronDFile{
				File: name, Entries: ParseCronDFile(string(content)),
			})
		}
	} else if !os.IsNotExist(err) {
		return sys, fmt.Errorf("cronx: read %s: %w", CronDDir, err)
	}
	for _, dir := range CronDirs {
		scripts := make([]string, 0, 4)
		files, rerr := os.ReadDir(dir)
		if rerr == nil {
			for _, f := range files {
				if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
					continue
				}
				scripts = append(scripts, f.Name())
			}
			sort.Strings(scripts)
		} else if !os.IsNotExist(rerr) {
			return sys, fmt.Errorf("cronx: read %s: %w", dir, rerr)
		}
		sys.CronDirs = append(sys.CronDirs, CronDir{Dir: dir, Scripts: scripts})
	}
	return sys, nil
}

// splitLines 切行（保留内容行；结尾空行不影响语义）。
func splitLines(raw string) []string {
	if raw == "" {
		return nil
	}
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// firstLine 取首行摘要（错误上下文用）。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
