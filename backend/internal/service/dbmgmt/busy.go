package dbmgmt

import "sync"

// BusyTable 是导出/导入在途互斥任务表（D-12：进程内 map，P7 过程数据；
// 单机单进程内存态足够——无集群语义）。Acquire 原子占用，Release 解除，
// busy 时 remove/再次 export/import 返回 DBM_016（service 层翻译）。
type BusyTable struct {
	mu   sync.Mutex
	busy map[int64]struct{}
}

// NewBusyTable 构造空任务表。
func NewBusyTable() *BusyTable { return &BusyTable{busy: make(map[int64]struct{})} }

// Acquire 尝试标记实例 busy；已 busy 返回 false（DBM_016 判据）。
func (b *BusyTable) Acquire(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.busy[id]; ok {
		return false
	}
	b.busy[id] = struct{}{}
	return true
}

// Release 解除 busy（任务结束成功/失败均调用，D-12）。
func (b *BusyTable) Release(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.busy, id)
}

// Busy 报告实例当前是否在途（remove 前置守卫）。
func (b *BusyTable) Busy(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.busy[id]
	return ok
}
