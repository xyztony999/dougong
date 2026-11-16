package dbmysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// ErrDumpFailed mysqldump 执行失败（磁盘/权限/锁——不虚构成功，PRD N2）→
// DBM_012；ErrImportFailed 导入执行失败（非法 SQL/中断，不自动回滚）→
// DBM_013。
var (
	ErrDumpFailed   = errors.New("dbmysql: mysqldump failed")
	ErrImportFailed = errors.New("dbmysql: import failed")
)

// importTimeout 导入执行硬上限（512MiB 上限 SQL 的执行上限，防挂死；导出
// 为流式受调用方请求 ctx 控制）。
const importTimeout = 10 * time.Minute

// dumpArgs 组装 mysqldump 参数（D-06 模板：--single-transaction --routines
// --triggers --hex-blob + 按库实况组装 --default-character-set；R-08 字面量
// 参数数组）。
func dumpArgs(t dbconn.Target, c dbconn.Creds, db, charset string) []string {
	args := make([]string, 0, 12)
	args = append(args,
		"--single-transaction", "--routines", "--triggers", "--hex-blob")
	if charset != "" {
		args = append(args, "--default-character-set", charset)
	}
	args = append(args, connArgs(t)...)
	args = append(args, "-u", c.Username, db)
	return args
}

// DumpReader 包装 mysqldump stdout 的流式读取（D-06 导出=HTTP 响应文件流，
// 不留存服务器目录）：EOF 时同步 Wait 并将进程失败转换为 ErrDumpFailed
// （stderr 摘要携带 data.reason 语义）；Close 终止进程并回收资源（用后即弃）。
type DumpReader struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr bytes.Buffer

	waitOnce sync.Once
	waitErr  error
	closed   bool
	mu       sync.Mutex
}

// Read 实现 io.Reader：转发子进程 stdout；EOF 时归一进程退出错误。
func (r *DumpReader) Read(p []byte) (int, error) {
	n, err := r.stdout.Read(p)
	if errors.Is(err, io.EOF) {
		if werr := r.wait(); werr != nil {
			return n, werr
		}
	}
	return n, err
}

// Close 关闭输出管道并等待进程退出（幂等；dump 流被调用方提前放弃时终止
// 子进程防僵尸）。
func (r *DumpReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.cmd.Process != nil && r.cmd.ProcessState == nil {
		_ = r.cmd.Process.Kill() // 提前放弃：终止子进程（正常路径已自然退出）
	}
	serr := r.stdout.Close()
	werr := r.cmd.Wait()
	return errors.Join(serr, werr)
}

// wait 单次执行 cmd.Wait 并缓存结果（Read EOF 路径）。
func (r *DumpReader) wait() error {
	r.waitOnce.Do(func() {
		r.waitErr = r.cmd.Wait()
	})
	if r.waitErr != nil {
		reason := firstLine(r.stderr.String())
		if reason == "" {
			reason = r.waitErr.Error()
		}
		r.waitErr = fmt.Errorf("%w: %s", ErrDumpFailed, reason)
	}
	return r.waitErr
}

// DumpDatabase 导出库（mysqldump 参数数组执行，D-06；charset 按库实况由
// 调用方组装传入）。返回流式 ReadCloser：调用方读至 EOF 即进程正常结束；
// 启动即失败（认证/通道/参数）在返回前判定。
func (g *Gateway) DumpDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, charset string) (io.ReadCloser, error) {
	if g.lookupErr != nil {
		return nil, g.lookupErr
	}
	cmd := exec.CommandContext(ctx, g.dumpBin, dumpArgs(t, c, db, charset)...) // 字面量参数数组
	cmd.Env = append(cmdEnv(), "MYSQL_PWD="+c.Password)                        // 凭据进 env 不进 argv
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("dbmysql: dump stdout pipe: %w", err)
	}
	dr := &DumpReader{cmd: cmd, stdout: stdout}
	cmd.Stderr = &dr.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: start mysqldump: %v", ErrConnFailed, err)
	}
	return dr, nil
}

// ImportDatabase 导入 SQL 流（mysql 客户端参数数组执行，目标库为位置参数；
// 临时文件由调用方治理——本方法消费 io.Reader，D-06）。失败返回
// ErrImportFailed（stderr 摘要；不自动回滚，PRD N2）。
func (g *Gateway) ImportDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string, r io.Reader) error {
	if g.lookupErr != nil {
		return g.lookupErr
	}
	cctx, cancel := context.WithTimeout(ctx, importTimeout)
	defer cancel()
	args := append(connArgs(t), "-u", c.Username, db) // 字面量参数数组
	cmd := exec.CommandContext(cctx, g.mysqlBin, args...)
	cmd.Env = append(cmdEnv(), "MYSQL_PWD="+c.Password)
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, ErrAuthRejected) {
			return err
		}
		detail := classifyRun(err, stderr.String(), "")
		if errors.Is(detail, ErrAuthRejected) || errors.Is(detail, ErrConnFailed) {
			return detail
		}
		reason := firstLine(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return fmt.Errorf("%w: %s", ErrImportFailed, reason)
	}
	return nil
}

// cmdEnv 返回子进程环境基线（继承进程环境；每命令追加凭据变量）。
func cmdEnv() []string { return os.Environ() }
