// Package fileman 是文件管理器应用服务（api-file-manager.md §4）：
// PathScopeGuard（R-04 两步校验全接口前置）、FileProjector（目录投影+排序）、
// EntryActionService/TransferService/EditService/MetaService/ArchiveService/
// TrashService（编排）与 FSGateway 契约消费（adapter/fslocal 实现）。
// file_entry 零持久化（kb 拍板）：投影恒为当次目录读取快照（I-06 无缓存层）。
package fileman

// 操作参数与阈值（api-file-manager.md §3.4 业务常量，编译期定值无配置面）。
const (
	// DirectUploadLimit 单请求直传单文件上限 5MiB（D-04；超限 FILE_011 走分块）。
	DirectUploadLimit = 5 << 20
	// UploadChunkSize 分块大小 5MiB（D-04）。
	UploadChunkSize = 5 << 20
	// ChunkTaskTTL 分块任务无活动作废时限 1h（D-10）。
	ChunkTaskTTL = int64(3600)
	// EditSizeLimit 在线编辑大小上限 1MiB（D-05）。
	EditSizeLimit = 1 << 20
	// EditBinaryProbe 二进制检测窗口：前 8KB（D-05）。
	EditBinaryProbe = 8 << 10
	// TrashDirName 单一回收站目录名（R-17/ADR-021，受管根下）。
	TrashDirName = ".dougong-trash"
)

// 排序参数取值域（SORT_FIELDS/ORDERS，接口参数非业务枚举）。
const (
	SortFieldName  = "name"
	SortFieldSize  = "size"
	SortFieldMtime = "mtime"
	OrderAsc       = "asc"
	OrderDesc      = "desc"
)

// onConflict 冲突决策参数取值域（D-07；ask 默认）。
const (
	ConflictAsk       = "ask"
	ConflictOverwrite = "overwrite"
	ConflictSkip      = "skip"
)

// file_type 枚举（§3.4：entry/list 响应 entryType；符号链接按链接自身呈现）。
const (
	TypeFile      = "file"
	TypeDirectory = "directory"
	TypeSymlink   = "symlink"
)

// 审计细粒度动作码（api-file-manager.md §3.4 业务常量，14 个）。
const (
	ActionPathScopeUpdated = "path_scope_updated"
	ActionFileUploaded     = "file_uploaded"
	ActionFileDownloaded   = "file_downloaded"
	ActionFileContentSaved = "file_content_saved"
	ActionFileCreated      = "file_created"
	ActionFileRenamed      = "file_renamed"
	ActionFileMoved        = "file_moved"
	ActionFileDeleted      = "file_deleted"
	ActionFileChmod        = "file_chmod"
	ActionFileChown        = "file_chown"
	ActionFileArchived     = "file_archived"
	ActionFileUnarchived   = "file_unarchived"
	ActionTrashRestored    = "trash_restored"
	ActionTrashPurged      = "trash_purged"
)

// 审计对象类型（object_type 取值，§3.4）。
const (
	ObjectTypeFileEntry   = "file_entry"
	ObjectTypeTrashEntry  = "trash_entry"
	ObjectTypeFsPathScope = "fs_path_scope"
)

// FILE_002 data.reason 取值（D-01：两类越界原因）。
const (
	ReasonNotUnderRoot = "not_under_root"
	ReasonExcluded     = "excluded"
)
