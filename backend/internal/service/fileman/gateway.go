package fileman

import (
	"context"
	"io"
	"time"
)

// EntrySnapshot 是文件系统条目快照（ListDir 元素与 Stat 单点共用形态，
// §4.2.1 投影条目字段；Name 在 Stat 场景同样填充）。
type EntrySnapshot struct {
	Name       string
	EntryType  string // file | directory | symlink
	Size       int64
	Mode       string // 八进制权限位（如 0644）
	OwnerUser  string
	OwnerGroup string
	Mtime      time.Time
}

// SystemAccounts 是系统用户/组实况（D-14 实时读取，无缓存不落库）。
type SystemAccounts struct {
	Users  []SysAccountUser
	Groups []SysAccountGroup
}

// FSGateway 是文件系统通道网关契约（§4.2.5，进程内适配层非 SQL）。
// 约束：入参一律为 PathScopeGuard 校验后的真实路径（唯一放行口径，R-04
// 出口收敛）；错误统一语义化（ErrEntryNotExist / ErrNotADirectory 哨兵 +
// 其余归一 FILE_013 语义由 service 映射）。
//
// 实现在 internal/adapter/fslocal（接口定义在消费方，backend-01 惯例）。
type FSGateway interface {
	// ListDir 目录投影（I-06 当次快照；含隐藏条目 I-10；符号链接按链接自身呈现）。
	ListDir(ctx context.Context, realPath string) ([]EntrySnapshot, error)
	// Stat 单点属性快照。
	Stat(ctx context.Context, realPath string) (EntrySnapshot, error)
	// ReadFile 读取文件内容（limit>0 时至多 limit 字节；编辑打开场景 D-05）。
	ReadFile(ctx context.Context, realPath string, limit int64) ([]byte, error)
	// WriteFileAtomic 临时文件+rename 原子写（I-09）。
	WriteFileAtomic(ctx context.Context, realPath string, data []byte) error
	// WriteFileAtomicStreaming 流式原子写（同目录临时文件+rename，I-09 同口径；
	// 归档产物等大体量载荷免全量进内存——A-F5 裁定增补）。
	WriteFileAtomicStreaming(ctx context.Context, realPath string, fn func(w io.Writer) error) error
	// Mkdir 新建空目录。
	Mkdir(ctx context.Context, realPath string) error
	// CreateFile 新建空文件。
	CreateFile(ctx context.Context, realPath string) error
	// Rename 重命名/移动（跨设备 EXDEV 错误上抛触发调用方降级，D-15）。
	Rename(ctx context.Context, from, to string) error
	// CopyThenRemove EXDEV 降级 copy+delete（D-15；降级事实由调用方记审计）。
	CopyThenRemove(ctx context.Context, from, to string) error
	// RemoveTree 递归删除（仅 purge 清 trash 内容与解压/移动覆盖前清理用）。
	RemoveTree(ctx context.Context, realPath string) error
	// Chmod 权限位修改（仅目标自身不递归，D-06；mode 为服务层已校验的
	// 0~0777 八进制串，如 "0644"）。
	Chmod(ctx context.Context, realPath string, mode string) error
	// Chown 属主修改（仅目标自身，D-06；uid/gid 由服务层经候选清单实况
	// 解析（D-14 单一校验通道），-1 表示该项不变；unix 完整/windows 退化 A-F7）。
	Chown(ctx context.Context, realPath string, uid, gid int) error
	// OpenReadStream 打开只读流（下载：校验后开流，D-16；调用方负责 Close）。
	OpenReadStream(ctx context.Context, realPath string) (io.ReadCloser, error)
	// ListSystemAccounts 系统用户/组候选清单实时读取（D-14）。
	ListSystemAccounts(ctx context.Context) (SystemAccounts, error)
	// EnsureDir 保证目录存在（MkdirAll 语义；trash 目录惰性建立用）。
	EnsureDir(ctx context.Context, realPath string) error
	// SizeOf 累计大小（文件=自身；目录=子树累计，trash 入站登记 size 列语义）。
	SizeOf(ctx context.Context, realPath string) (int64, error)
	// Exists 存在性探测（不跟随符号链接的 Lstat 语义；冲突预检用）。
	Exists(ctx context.Context, realPath string) (bool, error)
	// RemoveSingle 删除单个条目（不递归；还原/上传覆盖前清理文件/空场景用）。
	RemoveSingle(ctx context.Context, realPath string) error
}

// 哨兵错误（fs 通道错误的细粒度语义；service 映射为 FILE_003/FILE_012/FILE_013）。
var (
	// ErrEntryNotExist 目标条目不存在（Lstat/Stat ENOENT）。
	ErrEntryNotExist = errSentinel("fs: entry not exists")
	// ErrNotADirectory 目标不是目录。
	ErrNotADirectory = errSentinel("fs: not a directory")
	// ErrIsDirectory 目标是目录（期望文件的场景：download/content）。
	ErrIsDirectory = errSentinel("fs: is a directory")
	// ErrCrossDeviceRename 跨设备 rename（EXDEV，调用方降级 D-15）。
	ErrCrossDeviceRename = errSentinel("fs: cross-device rename")
	// ErrUnsupportedPlatform 平台不支持的操作（windows 退化路径，A-F7）。
	ErrUnsupportedPlatform = errSentinel("fs: unsupported on this platform")
)

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
