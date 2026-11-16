package errcode

// file-manager 模块错误码（api-file-manager.md §3.4，FILE_001~015 本模块新增
// 15 个；鉴权/CSRF/越界复用 AUTH 码族）。symbol 即契约 code 列原值，经响应
// 包络 code 字段上线符号串。
var (
	// ErrFileInvalidPath FILE_001 路径形态非法（非绝对路径/含非法段，R-14；sort/order 取值域校验同码承载）。
	ErrFileInvalidPath = New("FILE_001", "路径形态非法（须为绝对路径且不含非法段）")
	// ErrFilePathOutOfScope FILE_002 路径越界（R-04/D-01；data.reason 注明 not_under_root|excluded）。
	ErrFilePathOutOfScope = New("FILE_002", "路径越界：不在受管根内或命中排除目录")
	// ErrFileEntryNotFound FILE_003 目标条目不存在（实况校验）。
	ErrFileEntryNotFound = New("FILE_003", "目标条目不存在")
	// ErrFileEntryExists FILE_004 目标已存在且该操作不支持覆盖（新建/重命名/压缩产物，D-13）。
	ErrFileEntryExists = New("FILE_004", "目标已存在且该操作不支持覆盖")
	// ErrFileConflict FILE_005 onConflict=ask 预检发现同名冲突（data.conflicts[]；零落盘 D-07）。
	ErrFileConflict = New("FILE_005", "检测到同名冲突，请携带决策（overwrite/skip）重试")
	// ErrFileNotEditable FILE_006 不可在线编辑（二进制或超限，D-05；提示下载后本地处理）。
	ErrFileNotEditable = New("FILE_006", "该文件不支持在线编辑（二进制），请下载后本地处理")
	// ErrFileInvalidName FILE_007 名称非法（含路径分隔符/空/保留名，R-14）。
	ErrFileInvalidName = New("FILE_007", "名称非法（不能为空、含路径分隔符或保留名）")
	// ErrFileInvalidArchiveFormat FILE_008 format 不在 archive_format 取值内（tar|tar.gz|zip）。
	ErrFileInvalidArchiveFormat = New("FILE_008", "压缩格式非法（仅支持 tar/tar.gz/zip）")
	// ErrFileArchiveEntryUnsafe FILE_009 解压条目越界（zip slip，R-04 归档延伸；整体不落盘 D-07）。
	ErrFileArchiveEntryUnsafe = New("FILE_009", "压缩包内条目路径不安全（zip slip），已整体拒绝解压")
	// ErrFileChunkInvalid FILE_010 分块非法（uploadId 无效/已作废、index 越界或重复，D-04/D-10）。
	ErrFileChunkInvalid = New("FILE_010", "分块上传参数非法（任务无效、序号越界或重复）")
	// ErrFileTooLarge FILE_011 超限（直传>5MiB 提示走分块 D-04；编辑内容>1MiB D-05）。
	ErrFileTooLarge = New("FILE_011", "文件超出大小上限（直传单文件 5MiB、编辑 1MiB）")
	// ErrFileNotADirectory FILE_012 目标目录不存在或不是目录。
	ErrFileNotADirectory = New("FILE_012", "目标目录不存在或不是目录")
	// ErrFilesystem FILE_013 文件系统错误（磁盘满/权限不足/EXDEV 降级失败/IO，data.reason）。
	ErrFilesystem = New("FILE_013", "文件系统操作失败")
	// ErrFileTrashNotFound FILE_014 回收站条目不存在或不在站（FR-17）。
	ErrFileTrashNotFound = New("FILE_014", "回收站条目不存在或已不在站内")
	// ErrFileInvalidPermission FILE_015 chmod 权限位非法（非 0~0777 八进制）/chown 候选不存在（D-14）。
	ErrFileInvalidPermission = New("FILE_015", "权限位或属主候选非法")
)
