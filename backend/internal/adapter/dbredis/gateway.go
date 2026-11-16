// Package dbredis 是 Redis 引擎适配器（api-database-mgmt.md §4.1.5 DbGateway
// 契约 Redis 族方法承载；idea.md §5.3 dbredis 落位）。通道=本机 redis-cli：
// 全部命令经 exec.CommandContext 字面量参数数组执行（R-08：无 shell 无拼接，
// key/pattern 天然单参数安全）；密码经 REDISCLI_AUTH 环境变量注入子进程
// （不进 argv，G-03 纪律）；二进制构造期 exec.LookPath 解析。
//
// 错误分类（引擎协议概念止步于本包）：认证被拒（WRONGPASS/NOAUTH）与
// 连接失败（Could not connect）归一哨兵，service 层翻译 DBM_005/006 语义。
package dbredis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// 哨兵错误（service 层翻译 DBM_ 错误码语义）。
var (
	// ErrAuthRejected 认证被拒（密码错误/未认证）→ DBM_005。
	ErrAuthRejected = errors.New("dbredis: authentication rejected")
	// ErrConnFailed 网络/socket 层连接失败 → DBM_006 / DBM_003。
	ErrConnFailed = errors.New("dbredis: connection failed")
	// ErrUnavailable redis-cli 二进制未检出/无法执行（通道不可用）。
	ErrUnavailable = errors.New("dbredis: client binary unavailable")
	// ErrKeyNotFound TTL 查询的 key 不存在（-2；过期即消失，如实）→ DBM_015。
	ErrKeyNotFound = errors.New("dbredis: key not found")
	// ErrCommandFailed 命令执行失败（其它未分类引擎错误；文本携带原因）。
	ErrCommandFailed = errors.New("dbredis: command failed")
)

// commandTimeout 单次 redis-cli 命令执行上限（通道级防御）。
const commandTimeout = 15 * time.Second

// Gateway 是 redis-cli 网关（bin 通常为 "redis-cli"，LookPath 解析）。
type Gateway struct {
	bin       string
	lookupErr error
}

// NewGateway 构造网关；二进制缺席时 lookupErr 承载原因。
func NewGateway(bin string) *Gateway {
	g := &Gateway{bin: bin}
	p, err := exec.LookPath(bin)
	if err != nil {
		g.lookupErr = fmt.Errorf("%w: %q not found in PATH: %v", ErrUnavailable, bin, err)
		return g
	}
	g.bin = p
	return g
}

// Available 返回通道可用性。
func (g *Gateway) Available() error { return g.lookupErr }

// InfoSections 是 INFO 解析结果（节名小写 → 字段 → 原始值；D-08 白名单过滤
// 在 service 层）。
type InfoSections map[string]map[string]string

// SlowlogEntry 是慢日志单条（§4.1.4 slowlog items）。
type SlowlogEntry struct {
	ID         int64
	Timestamp  int64 // epoch 秒
	DurationUs int64
	Command    string
}

// KeyItem 是 SCAN 返回的 key 条目（§4.4.4 items {key, type}）。
type KeyItem struct {
	Key  string
	Type string
}

// Ping 连接验证（PING → PONG；认证/网络错误归一哨兵）。
func (g *Gateway) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	out, err := g.run(ctx, t, c, "PING")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "PONG" {
		return fmt.Errorf("%w: unexpected reply %q", ErrCommandFailed, firstLine(out))
	}
	return nil
}

// ServerVersion 返回 INFO server 的 redis_version（credentials 编排刷新档案
// version 用，D-04）。
func (g *Gateway) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	sections, err := g.InfoFields(ctx, t, c)
	if err != nil {
		return "", err
	}
	if srv, ok := sections["server"]; ok {
		if v, ok := srv["redis_version"]; ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("dbredis: empty redis_version in INFO")
}

// InfoFields 全量 INFO 解析（节头 # Server → server；字段 k:v；I-04 实时）。
func (g *Gateway) InfoFields(ctx context.Context, t dbconn.Target, c dbconn.Creds) (InfoSections, error) {
	out, err := g.run(ctx, t, c, "INFO")
	if err != nil {
		return nil, err
	}
	return parseInfo(out), nil
}

// Slowlog 慢日志清单（SLOWLOG GET n；--raw 行块解析）。
func (g *Gateway) Slowlog(ctx context.Context, t dbconn.Target, c dbconn.Creds, n int) ([]SlowlogEntry, error) {
	out, err := g.run(ctx, t, c, "SLOWLOG", "GET", strconv.Itoa(n))
	if err != nil {
		return nil, err
	}
	return parseSlowlog(out), nil
}

// SetRequirepass 设置 requirepass（CONFIG SET 参数数组——newPassword 为单
// 独参数不经 shell；连接以 currentPassword 验证=「验证旧密码」语义天然满足
// D-05；newPassword 组入参数后即弃，I-02）。
func (g *Gateway) SetRequirepass(ctx context.Context, t dbconn.Target, c dbconn.Creds, currentPassword, newPassword string) error {
	out, err := g.run(ctx, t, dbconn.Creds{Password: currentPassword},
		"CONFIG", "SET", "requirepass", newPassword)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "OK" {
		return fmt.Errorf("%w: unexpected reply %q", ErrCommandFailed, firstLine(out))
	}
	return nil
}

// ScanKeys SCAN 游标分页（D-09：cursor/MATCH pattern/COUNT count；禁 KEYS
// 全量命令——pattern 服务端校验 DBM_017 在 service 层）。返回 nextCursor 与
// 逐 key TYPE 实况条目。
func (g *Gateway) ScanKeys(ctx context.Context, t dbconn.Target, c dbconn.Creds, cursor, pattern string, count int) (string, []KeyItem, error) {
	out, err := g.run(ctx, t, c,
		"SCAN", cursor, "MATCH", pattern, "COUNT", strconv.Itoa(count))
	if err != nil {
		return "", nil, err
	}
	lines := splitLines(out)
	if len(lines) == 0 {
		return "", nil, fmt.Errorf("%w: empty SCAN reply", ErrCommandFailed)
	}
	next := strings.TrimSpace(lines[0])
	if _, cerr := strconv.ParseUint(next, 10, 64); cerr != nil {
		return "", nil, fmt.Errorf("%w: bad cursor %q", ErrCommandFailed, firstLine(next))
	}
	items := make([]KeyItem, 0, len(lines)-1)
	for _, key := range lines[1:] {
		if key == "" {
			continue
		}
		typ, terr := g.run(ctx, t, c, "TYPE", key)
		if terr != nil {
			return "", nil, terr
		}
		items = append(items, KeyItem{Key: key, Type: strings.TrimSpace(typ)})
	}
	return next, items, nil
}

// KeyTTL 单 key TTL（-1=无过期如实透传；-2=不存在 → ErrKeyNotFound）。
func (g *Gateway) KeyTTL(ctx context.Context, t dbconn.Target, c dbconn.Creds, key string) (int64, error) {
	out, err := g.run(ctx, t, c, "TTL", key)
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if perr != nil {
		return 0, fmt.Errorf("%w: bad TTL reply %q", ErrCommandFailed, firstLine(out))
	}
	if v == -2 {
		return 0, ErrKeyNotFound
	}
	return v, nil
}

// run 执行单条 redis-cli 命令（--raw 单行值输出；字面量参数数组；密码经
// REDISCLI_AUTH env 注入）。
func (g *Gateway) run(ctx context.Context, t dbconn.Target, c dbconn.Creds, args ...string) (string, error) {
	if g.lookupErr != nil {
		return "", g.lookupErr
	}
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	full := make([]string, 0, len(args)+6)
	full = append(full, "--raw", "--no-auth-warning")
	if t.ConnMode == "socket" {
		full = append(full, "-s", t.SocketPath)
	} else {
		full = append(full, "-h", t.Host, "-p", strconv.Itoa(t.Port))
	}
	full = append(full, args...)
	cmd := exec.CommandContext(cctx, g.bin, full...) // 字面量参数数组，无拼接
	cmd.Env = append(cmdEnv(), "REDISCLI_AUTH="+c.Password)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", classifyRun(err, stderr.String(), stdout.String())
	}
	return stdout.String(), nil
}

// classifyRun 将子进程结果归一哨兵（文本分类——redis-cli 无结构化错误码）。
func classifyRun(runErr error, stderr, stdout string) error {
	detail := stderr + stdout
	switch {
	case strings.Contains(detail, "Could not connect") ||
		strings.Contains(detail, "Connection refused") ||
		strings.Contains(detail, "timed out") ||
		strings.Contains(detail, "Name or service not known"):
		return fmt.Errorf("%w: %s", ErrConnFailed, firstLine(detail))
	case strings.Contains(detail, "WRONGPASS") ||
		strings.Contains(detail, "NOAUTH") ||
		strings.Contains(detail, "ERR Client sent AUTH") ||
		strings.Contains(detail, "unknown command 'AUTH'"):
		return fmt.Errorf("%w: %s", ErrAuthRejected, firstLine(detail))
	default:
		return fmt.Errorf("%w: %s: %v", ErrCommandFailed, firstLine(detail), runErr)
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// cmdEnv 返回子进程环境基线（继承进程环境；每命令追加凭据变量）。
func cmdEnv() []string { return os.Environ() }
