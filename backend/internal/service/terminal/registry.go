package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Session 是 terminal_session 聚合根的进程内形态（零持久化 I-04：面板重启
// 即全部消亡；字段清单对应 api §4.1.1 运行时字段）。生命周期：活跃（内存态
// 唯一存在态）→ 断开（终态=对象销毁，D-02 无 closed 持久观察窗）。
type Session struct {
	ID              string // sessionId（8 字节随机 hex）
	UserID          int64  // 操作者 panel_user.id（并发计数与私有归属 I-05）
	Username        string // 操作者面板用户名（审计 ActorName）
	PanelSessionRef string // 面板会话引用（D-07 30s 复核凭据；非聚合 ID）
	Identity        RunIdentity
	RunUser         string // 解析后的运行身份呈现（ready 帧 runUser）
	SourceIP        string
	UserAgent       string
	CreatedAt       time.Time
	LastActiveAt    time.Time // 仅 input/resize 刷新（D1 从严口径：输出不更新）
	PTY             PtyHandle
	Conn            WSConn // nil=未附着（升级失败/配额竞态的清理路径）
	ring            *outputRing
	svc             *Service
	sessCtx         context.Context    // 会话作用域 ctx（PTY 子进程生命周期锚点）
	cancelCtx       context.CancelFunc // 统一清理时取消（子进程 Kill 双保险）
	closeOnce       sync.Once
	mu              sync.Mutex // 保护调度字段
	nextPingAt      time.Time  // 心跳调度（D-03）
	nextRecheckAt   time.Time  // 面板会话复核调度（D-07）
	HardDeadlineAt  time.Time  // createdAt+8h（I-03）
}

// hardDeadline 计算硬上限截止（createdAt+8h，I-03）。
func hardDeadline(createdAt time.Time, limit time.Duration) time.Time {
	return createdAt.Add(limit)
}

// touch 刷新 lastActiveAt（I-03：input/resize 时刻）。
func (s *Session) touch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActiveAt = now
}

// activityAt 读取最近活跃时刻（扫描判定用）。
func (s *Session) activityAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastActiveAt
}

// duePing / dueRecheck 心跳与复核调度（互斥下读写；now 到点返回 true 并顺延）。
func (s *Session) duePing(now time.Time, interval time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Before(s.nextPingAt) {
		return false
	}
	s.nextPingAt = now.Add(interval)
	return true
}

func (s *Session) dueRecheck(now time.Time, interval time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Before(s.nextRecheckAt) {
		return false
	}
	s.nextRecheckAt = now.Add(interval)
	return true
}

// newSessionID 生成会话标识（8 字节随机 hex，§4.1.1）。
func newSessionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("terminal: session id entropy: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Registry 是 TerminalSessionRegistry 契约（§4.1.5）：并发安全 map +
// 定时扫描的零持久化替代仓储。Create 为原子 CAS（计数判定与登记同锁，
// I-05 配额不超发）。
type Registry struct {
	mu         sync.RWMutex
	byID       map[string]*Session
	maxPerUser int
}

// NewRegistry 构造注册表（maxPerUser=每用户并发上限，ADR-023；空闲/硬上限
// 判定在 Lifecycle 扫描，经 Service limits 参数化）。
func NewRegistry(maxPerUser int) *Registry {
	return &Registry{
		byID:       make(map[string]*Session),
		maxPerUser: maxPerUser,
	}
}

// Create 原子登记（I-05：同操作者计数达到上限拒绝；true=登记成功）。
func (r *Registry) Create(s *Session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.countByUserLocked(s.UserID) >= r.maxPerUser {
		return false
	}
	r.byID[s.ID] = s
	return true
}

// Get 按 sessionId 取会话。
func (r *Registry) Get(sessionID string) (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[sessionID]
	return s, ok
}

// CountByUser 操作者存续会话数（TERM_001 判定）。
func (r *Registry) CountByUser(userID int64) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.countByUserLocked(userID)
}

func (r *Registry) countByUserLocked(userID int64) int {
	n := 0
	for _, s := range r.byID {
		if s.UserID == userID {
			n++
		}
	}
	return n
}

// ListByUser 操作者全部会话（联动断开全量寻址）。
func (r *Registry) ListByUser(userID int64) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, 4)
	for _, s := range r.byID {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out
}

// Remove 注销会话（清理终点，幂等——I-02 重复清理无害）。
func (r *Registry) Remove(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, sessionID)
}

// Scan 快照遍历（Lifecycle 扫描：空闲/硬上限/复核/心跳，I-03/D-03/D-07）。
// 回调在锁外执行（关闭路径会重入 Remove，避免死锁）。
func (r *Registry) Scan(fn func(*Session)) {
	r.mu.RLock()
	snapshot := make([]*Session, 0, len(r.byID))
	for _, s := range r.byID {
		snapshot = append(snapshot, s)
	}
	r.mu.RUnlock()
	for _, s := range snapshot {
		fn(s)
	}
}

// Len 存续会话总数（观测/测试用）。
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}
