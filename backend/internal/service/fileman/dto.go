package fileman

// DTO 是 HTTP 契约数据形态（api-file-manager.md §4.2.4/§4.1.4；camelCase、
// RFC3339 UTC）。service 不感知 HTTP，仅定义传输中立的数据结构。

// EntryItem 是目录条目投影项（§4.2.1 投影条目字段清单）。
type EntryItem struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	EntryType  string `json:"entryType"` // file | directory | symlink
	Size       int64  `json:"size"`
	Mode       string `json:"mode"` // 八进制权限位（如 0644）
	OwnerUser  string `json:"ownerUser"`
	OwnerGroup string `json:"ownerGroup"`
	Mtime      string `json:"mtime"` // RFC3339 UTC
}

// ListData 是 GET /files/entry/list 响应（D-03 全量 + 服务端排序）。
type ListData struct {
	Path        string      `json:"path"`
	Items       []EntryItem `json:"items"`
	Total       int         `json:"total"`
	GeneratedAt string      `json:"generatedAt"`
}

// ScopeData 是受管根配置（path-scope/get 与 update 响应）。
type ScopeData struct {
	RootPath     string   `json:"rootPath"`
	ExcludedDirs []string `json:"excludedDirs"`
}

// UploadItemResult 是直传单文件结果（status: ok | skipped）。
type UploadItemResult struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// UploadData 是 POST /files/entry/upload 响应。
type UploadData struct {
	Results       []UploadItemResult `json:"results"`
	UploadedCount int                `json:"uploadedCount"`
}

// ChunkInitData 是 POST /files/upload/chunk-init 响应。
type ChunkInitData struct {
	UploadID string `json:"uploadId"`
}

// ChunkProgressData 是 POST /files/upload/chunk 响应。
type ChunkProgressData struct {
	UploadID string `json:"uploadId"`
	Received int    `json:"received"`
	Total    int    `json:"total"`
}

// ChunkCompleteData 是 POST /files/upload/chunk-complete 响应。
type ChunkCompleteData struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ContentData 是 GET /files/entry/content 响应（UTF-8 文本全量，D-05）。
type ContentData struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mtime   string `json:"mtime"`
	Content string `json:"content"`
}

// SaveResultData 是 content-save / 单点写后快照（新值供前端刷新）。
type SaveResultData struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// CreateData 是 POST /files/entry/create 响应。
type CreateData struct {
	Path      string `json:"path"`
	EntryType string `json:"entryType"`
}

// RenameData 是 POST /files/entry/rename 响应（新路径）。
type RenameData struct {
	Path string `json:"path"`
}

// MoveItem 是移动单项（from→to）。
type MoveItem struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MoveData 是 POST /files/entry/move 响应。
type MoveData struct {
	Moved        []MoveItem `json:"moved"`
	SkippedCount int        `json:"skippedCount"`
}

// DeleteData 是 POST /files/entry/delete 响应（入站登记凭据，D-15）。
type DeleteData struct {
	TrashID   int64  `json:"trashId"`
	TrashName string `json:"trashName"`
}

// ChmodData 是 POST /files/entry/chmod 响应。
type ChmodData struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// ChownData 是 POST /files/entry/chown 响应。
type ChownData struct {
	Path       string `json:"path"`
	OwnerUser  string `json:"ownerUser"`
	OwnerGroup string `json:"ownerGroup"`
}

// SysAccountUser 是系统用户候选（name/uid；home/shell 不下发，§4.2.4）。
type SysAccountUser struct {
	Name string `json:"name"`
	UID  int    `json:"uid"`
}

// SysAccountGroup 是系统组候选。
type SysAccountGroup struct {
	Name string `json:"name"`
	GID  int    `json:"gid"`
}

// SystemAccountsData 是 GET /files/system-accounts/list 响应（实时读取，D-14）。
type SystemAccountsData struct {
	Users  []SysAccountUser  `json:"users"`
	Groups []SysAccountGroup `json:"groups"`
}

// ArchiveData 是 POST /files/entry/archive 响应。
type ArchiveData struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Format string `json:"format"`
}

// UnarchiveData 是 POST /files/entry/unarchive 响应。
type UnarchiveData struct {
	ExtractedCount int    `json:"extractedCount"`
	TargetDir      string `json:"targetDir"`
}

// TrashItem 是回收站在站条目（§4.1.4 trash/list items 六键）。
type TrashItem struct {
	ID           int64  `json:"id"`
	OriginalPath string `json:"originalPath"`
	EntryType    string `json:"entryType"`
	Size         int64  `json:"size"`
	OperatorName string `json:"operatorName"`
	DeletedAt    string `json:"deletedAt"`
}

// TrashListData 是 GET /files/trash/list 响应（D-03 全量，deleted_at 倒序）；
// Warning 非空时提示孤儿记录剔除事实（PRD N2，A-F11）。
type TrashListData struct {
	Items   []TrashItem `json:"items"`
	Total   int         `json:"total"`
	Warning string      `json:"warning,omitempty"`
}

// RestoreData 是 POST /files/trash/restore 响应。
type RestoreData struct {
	ID           int64  `json:"id"`
	OriginalPath string `json:"originalPath"`
	Status       string `json:"status"`
}

// PurgeData 是 POST /files/trash/purge 响应（空站清空幂等成功，PRD N3）。
type PurgeData struct {
	PurgedCount int `json:"purgedCount"`
}

// ConflictItem 是 FILE_005 的 data.conflicts[] 单项（upload 用 name，
// 还原/移动/解压用 path，§4.2.4 各接口详情）。
type ConflictItem struct {
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

// ConflictData 是 FILE_005 载荷。
type ConflictData struct {
	Conflicts []ConflictItem `json:"conflicts"`
}

// ReasonData 是 FILE_002/FILE_013 的 data.reason 载荷。
type ReasonData struct {
	Reason string `json:"reason"`
}
