package auth

import (
	"sync"
	"time"
)

// ticketStore 是 login_ticket 内存态存储（D-02：单实例内存 map 足够）。
// 一次性语义：consume 后即失效；未消费的 ticket 在 TTL 后惰性清除（签发时
// 顺带清扫过期项，避免常驻清理 goroutine——G-06 goroutine 最小化）。
// 过期判定统一走注入时钟（与业务时间一致，测试可控）。
type ticketStore struct {
	mu    sync.Mutex
	m     map[string]loginTicket
	clock func() time.Time
}

type loginTicket struct {
	userID    int64
	username  string
	expiresAt time.Time
}

func newTicketStore(clock func() time.Time) *ticketStore {
	return &ticketStore{m: make(map[string]loginTicket), clock: clock}
}

// now 取注入时钟的 UTC 时刻。
func (t *ticketStore) now() time.Time { return t.clock().UTC() }

// issue 签发一次性 ticket 并清扫过期项。
func (t *ticketStore) issue(token string, userID int64, username string, expiresAt time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, v := range t.m {
		if now.After(v.expiresAt) {
			delete(t.m, k)
		}
	}
	t.m[token] = loginTicket{userID: userID, username: username, expiresAt: expiresAt}
}

// lookup 查询未过期 ticket（不消费，允许验证码输错后重试直至过期/成功）。
func (t *ticketStore) lookup(token string) (loginTicket, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tk, ok := t.m[token]
	if !ok || t.now().After(tk.expiresAt) {
		return loginTicket{}, false
	}
	return tk, true
}

// consume 消费 ticket（验证成功后调用，立即失效——一次性）。
func (t *ticketStore) consume(token string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	tk, ok := t.m[token]
	if !ok {
		return false
	}
	if t.now().After(tk.expiresAt) {
		delete(t.m, token)
		return false
	}
	delete(t.m, token)
	return true
}
