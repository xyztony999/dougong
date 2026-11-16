package terminal

import (
	"sync"
)

// outputRing 是每会话 256KB 输出环形缓冲（D-04 背压）：PTY 读泵永不阻塞，
// 溢出丢弃最旧数据并置 truncated 标记；WS 写泵等待并排空。不因输出洪峰
// 断开连接、不重置空闲计时。
type outputRing struct {
	mu        sync.Mutex
	cond      *sync.Cond
	buf       []byte // 环形底层数组
	size      int    // 容量（OutputRingSize）
	head      int    // 最旧字节下标
	n         int    // 当前字节数
	truncated bool   // 自上次排空起是否丢弃过数据
	stopped   bool   // 停止标记（唤醒写泵退出）
}

func newOutputRing(size int) *outputRing {
	r := &outputRing{buf: make([]byte, size), size: size}
	r.cond = sync.NewCond(&r.mu)
	return r
}

// write 追加输出字节；容量不足时丢弃最旧数据并置 truncated（D-04）。
// 单次写入大于总容量时仅保留最新 size 字节。
func (r *outputRing) write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	if len(p) >= r.size { // 整体覆盖：只留最新
		copy(r.buf, p[len(p)-r.size:])
		r.head, r.n = 0, r.size
		r.truncated = true
		r.cond.Broadcast()
		return
	}
	free := r.size - r.n
	if len(p) > free { // 丢弃最旧腾位
		drop := len(p) - free
		r.head = (r.head + drop) % r.size
		r.n -= drop
		r.truncated = true
	}
	for _, b := range p {
		pos := (r.head + r.n) % r.size
		r.buf[pos] = b
		r.n++
	}
	r.cond.Broadcast()
}

// take 阻塞取出一批可用字节（空则等待）；返回 nil 表示已停止（写泵退出）。
// truncated 标记随本批一并消费（恢复时发 truncated:true 标记帧，D-04）。
func (r *outputRing) take() (chunk []byte, truncated bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.n == 0 && !r.stopped {
		r.cond.Wait()
	}
	if r.n == 0 && r.stopped {
		return nil, false
	}
	chunk = make([]byte, r.n)
	end := r.head + r.n
	if end <= r.size {
		copy(chunk, r.buf[r.head:end])
	} else { // 环形回绕：先尾后头
		first := r.size - r.head
		copy(chunk, r.buf[r.head:])
		copy(chunk[first:], r.buf[:end-r.size])
	}
	truncated = r.truncated
	r.truncated = false
	r.head = end % r.size
	r.n = 0
	return chunk, truncated
}

// stop 唤醒写泵并使其退出（统一清理第一步；幂等）。
func (r *outputRing) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	r.cond.Broadcast()
}
