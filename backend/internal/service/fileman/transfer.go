package fileman

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// UploadPayload 是直传单文件载荷（handler 自 multipart 解析；Size 先检后读，
// A-F8）。
type UploadPayload struct {
	Name string
	Size int64
	Data []byte
}

// Upload POST /files/entry/upload（admin+CSRF）：单请求直传（每文件 ≤5MiB，
// D-04）；ask 全量同名预检零落盘（D-07）；overwrite/skip 逐文件临时文件+
// rename（I-09）。
func (s *Service) Upload(ctx context.Context, actor *authsvc.Identity,
	dir string, files []UploadPayload, onConflict string) (*UploadData, error) {
	action := ActionFileUploaded
	if onConflict == "" {
		onConflict = ConflictAsk
	}
	if onConflict != ConflictAsk && onConflict != ConflictOverwrite && onConflict != ConflictSkip {
		return nil, errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "onConflict", Message: "取值域 ask|overwrite|skip",
		})
	}
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			dir, audit.ResultFailure, detailError(err))
		return err
	}
	if len(files) == 0 {
		return nil, fail(errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "files", Message: "至少一个文件",
		}))
	}
	// 超限先检不读体（A-F8：整体拒绝零落盘；D-04 提示走分块）。
	for _, f := range files {
		if f.Size > DirectUploadLimit || int64(len(f.Data)) > DirectUploadLimit {
			return nil, fail(errcode.ErrFileTooLarge.WithData(ReasonData{
				Reason: fmt.Sprintf("单文件上限 %d 字节，超限请走分块上传", DirectUploadLimit),
			}))
		}
		if err := ValidateName(f.Name); err != nil {
			return nil, fail(err)
		}
	}
	dirReal, err := s.resolveDir(ctx, dir)
	if err != nil {
		return nil, fail(err)
	}
	// 全量同名预检（ask → FILE_005 零落盘，D-07）。
	if onConflict == ConflictAsk {
		var conflicts []ConflictItem
		for _, f := range files {
			exists, err := s.gw.Exists(ctx, filepath.Join(dirReal, f.Name))
			if err != nil {
				return nil, fail(s.mapFSErr(err))
			}
			if exists {
				conflicts = append(conflicts, ConflictItem{Name: f.Name})
			}
		}
		if len(conflicts) > 0 {
			return nil, fail(errcode.ErrFileConflict.WithData(ConflictData{Conflicts: conflicts}))
		}
	}
	results := make([]UploadItemResult, 0, len(files))
	uploaded := 0
	var names []string
	for _, f := range files {
		target := filepath.Join(dirReal, f.Name)
		exists, err := s.gw.Exists(ctx, target)
		if err != nil {
			return nil, fail(s.mapFSErr(err))
		}
		if exists {
			if onConflict == ConflictSkip {
				results = append(results, UploadItemResult{Name: f.Name, Path: target, Status: "skipped"})
				continue // skip 不计入 uploadedCount（§4.2.4）
			}
			// overwrite：覆盖前先清条目（防经符号链接逃逸写出，R-04 口径）。
			if _, rerr := s.guard.Resolve(filepath.Join(dirReal, f.Name)); rerr != nil {
				return nil, fail(rerr)
			}
			if rerr := s.gw.RemoveTree(ctx, target); rerr != nil {
				return nil, fail(s.mapFSErr(rerr))
			}
		}
		if werr := s.gw.WriteFileAtomic(ctx, target, f.Data); werr != nil {
			return nil, fail(s.mapFSErr(werr))
		}
		results = append(results, UploadItemResult{Name: f.Name, Path: target, Status: "ok"})
		names = append(names, f.Name)
		uploaded++
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		dirReal, audit.ResultSuccess, map[string]any{"names": names, "uploadedCount": uploaded})
	return &UploadData{Results: results, UploadedCount: uploaded}, nil
}

// ---- 分块上传（D-04 三段式 / D-10 进程内内存态任务） ----

// chunkTask 是分块任务进程内态（P7 过程数据不落库，D-10）。
type chunkTask struct {
	dirReal     string // 目标目录真实路径
	fileName    string
	totalSize   int64
	totalChunks int
	received    map[int]int64 // index → 字节数（去重判定+累计校验）
	tmpDir      string
	lastActive  time.Time
}

// ChunkStore 是分块任务仓库（mutex 保护 map 并发安全；1h 无活动作废，D-10；
// 面板重启全部作废——进程内态天然语义）。
type ChunkStore struct {
	mu    sync.Mutex
	tasks map[string]*chunkTask
	clock func() time.Time
}

// NewChunkStore 构造任务仓库。
func NewChunkStore(clock func() time.Time) *ChunkStore {
	return &ChunkStore{tasks: make(map[string]*chunkTask), clock: clock}
}

// cleanupLocked 清理过期任务与临时目录（调用方持锁；惰性清理兜底——
// 每次分块操作触发，主清理循环由 main 编排 RunSweeper）。
func (cs *ChunkStore) cleanupLocked(ctx context.Context, gw FSGateway) {
	now := cs.clock().Unix()
	for id, t := range cs.tasks {
		if now-t.lastActive.Unix() > ChunkTaskTTL {
			_ = gw.RemoveTree(ctx, t.tmpDir) // 尽力清理；失败由 sweeper 下轮重试
			delete(cs.tasks, id)
		}
	}
}

// Create 登记任务并建立临时分块目录（A-F15：目标目录下保证同文件系统）。
func (cs *ChunkStore) Create(ctx context.Context, gw FSGateway,
	dirReal, fileName string, totalSize int64, totalChunks int) (string, string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", "", fmt.Errorf("chunk store rand: %w", err)
	}
	uploadID := hex.EncodeToString(id)
	tmpDir := filepath.Join(dirReal, ".dougong-upload-"+uploadID)
	if err := gw.EnsureDir(ctx, tmpDir); err != nil {
		return "", "", err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cleanupLocked(ctx, gw)
	cs.tasks[uploadID] = &chunkTask{
		dirReal:     dirReal,
		fileName:    fileName,
		totalSize:   totalSize,
		totalChunks: totalChunks,
		received:    make(map[int]int64),
		tmpDir:      tmpDir,
		lastActive:  cs.clock(),
	}
	return uploadID, tmpDir, nil
}

// Get 取有效任务（不存在/已过期作废 → FILE_010；顺带惰性清理）。
func (cs *ChunkStore) Get(ctx context.Context, gw FSGateway, uploadID string) (*chunkTask, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cleanupLocked(ctx, gw)
	t, ok := cs.tasks[uploadID]
	if !ok {
		return nil, errcode.ErrFileChunkInvalid
	}
	return t, nil
}

// chunkPathOf 分块文件路径（index 定宽命名保证按序合并）。
func chunkPathOf(t *chunkTask, index int) string {
	return filepath.Join(t.tmpDir, fmt.Sprintf("%08d.part", index))
}

// Put 写入单块并登记（重复块拒绝 FILE_010，D-10）。
func (cs *ChunkStore) Put(ctx context.Context, gw FSGateway,
	uploadID string, index int, data []byte) (*ChunkProgressData, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cleanupLocked(ctx, gw)
	t, ok := cs.tasks[uploadID]
	if !ok {
		return nil, errcode.ErrFileChunkInvalid
	}
	if index < 0 || index >= t.totalChunks {
		return nil, errcode.ErrFileChunkInvalid
	}
	if _, dup := t.received[index]; dup {
		return nil, errcode.ErrFileChunkInvalid // 重复块（幂等语义：拒绝）
	}
	if int64(len(data)) > UploadChunkSize {
		return nil, errcode.ErrFileChunkInvalid // 块大小=分块定值（D-04）
	}
	if err := gw.WriteFileAtomic(ctx, chunkPathOf(t, index), data); err != nil {
		return nil, err
	}
	t.received[index] = int64(len(data))
	t.lastActive = cs.clock()
	return &ChunkProgressData{UploadID: uploadID, Received: len(t.received), Total: t.totalChunks}, nil
}

// SnapshotComplete 校验齐全性（全部块在 + 累计大小=totalSize，D-04）。
func (cs *ChunkStore) SnapshotComplete(uploadID string) (*chunkTask, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	t, ok := cs.tasks[uploadID]
	if !ok {
		return nil, errcode.ErrFileChunkInvalid
	}
	if len(t.received) != t.totalChunks {
		return nil, errcode.ErrFileChunkInvalid
	}
	var sum int64
	for _, n := range t.received {
		sum += n
	}
	if sum != t.totalSize {
		return nil, errcode.ErrFileChunkInvalid
	}
	return t, nil
}

// Remove 移除任务（complete 成功/作废后清理临时目录）。
func (cs *ChunkStore) Remove(ctx context.Context, gw FSGateway, uploadID string) {
	cs.mu.Lock()
	t, ok := cs.tasks[uploadID]
	if ok {
		delete(cs.tasks, uploadID)
	}
	cs.mu.Unlock()
	if ok {
		_ = gw.RemoveTree(ctx, t.tmpDir)
	}
}

// RunSweeper 周期清理过期任务（main 以 goroutine 编排，唯一退出路径=ctx
// 取消，G-06；D-10 1h 无活动作废）。
func (cs *ChunkStore) RunSweeper(ctx context.Context, gw FSGateway, interval time.Duration) error {
	cs.sweepOnce(ctx, gw)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			cs.sweepOnce(ctx, gw)
		}
	}
}

func (cs *ChunkStore) sweepOnce(ctx context.Context, gw FSGateway) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cleanupLocked(ctx, gw)
}

// ChunkInit POST /files/upload/chunk-init（admin+CSRF）：totalChunks=
// ceil(totalSize/5MiB) 校验（FILE_010）；同名预检 ask → FILE_005 零动作。
// 未生效不审计（§4.2.4）。
func (s *Service) ChunkInit(ctx context.Context, dir, fileName string,
	totalSize, totalChunks int64) (*ChunkInitData, error) {
	if err := ValidateName(fileName); err != nil {
		return nil, err
	}
	if totalSize < 1 || totalChunks < 1 {
		return nil, errcode.ErrFileChunkInvalid
	}
	if totalChunks != (totalSize+UploadChunkSize-1)/UploadChunkSize {
		return nil, errcode.ErrFileChunkInvalid
	}
	dirReal, err := s.resolveDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	exists, err := s.gw.Exists(ctx, filepath.Join(dirReal, fileName))
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	if exists { // ask 语义预检（零动作，D-07）
		return nil, errcode.ErrFileConflict.WithData(ConflictData{
			Conflicts: []ConflictItem{{Name: fileName}},
		})
	}
	uploadID, _, err := s.chunks.Create(ctx, s.gw, dirReal, fileName, totalSize, int(totalChunks))
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	return &ChunkInitData{UploadID: uploadID}, nil
}

// ChunkUpload POST /files/upload/chunk（admin+CSRF，multipart）：任务有效、
// index ∈ [0,totalChunks)、无重复（FILE_010）；过程块不审计。
func (s *Service) ChunkUpload(ctx context.Context, uploadID string, index int, data []byte) (*ChunkProgressData, error) {
	if _, err := s.chunks.Get(ctx, s.gw, uploadID); err != nil {
		return nil, err
	}
	progress, err := s.chunks.Put(ctx, s.gw, uploadID, index, data)
	if err != nil {
		if _, ok := err.(*errcode.Error); ok {
			return nil, err
		}
		return nil, s.mapFSErr(err)
	}
	return progress, nil
}

// ChunkComplete POST /files/upload/chunk-complete（admin+CSRF）：齐全性校验
// （FILE_010）→ 按 index 序合并 → rename 原子落盘（I-09）→ 清理任务；
// 目标被占 FILE_004（A-F4）。审计 file_uploaded（detail={chunked:true}）。
func (s *Service) ChunkComplete(ctx context.Context, actor *authsvc.Identity, uploadID string) (*ChunkCompleteData, error) {
	action := ActionFileUploaded
	fail := func(err error) error {
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			uploadID, audit.ResultFailure, detailError(err))
		return err
	}
	t, err := s.chunks.SnapshotComplete(uploadID)
	if err != nil {
		return nil, fail(err)
	}
	target := filepath.Join(t.dirReal, t.fileName)
	exists, err := s.gw.Exists(ctx, target)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	if exists {
		return nil, fail(errcode.ErrFileEntryExists) // A-F4（D-13 从紧）
	}
	mergeErr := s.gw.WriteFileAtomicStreaming(ctx, target, func(w io.Writer) error {
		for i := 0; i < t.totalChunks; i++ {
			stream, err := s.gw.OpenReadStream(ctx, chunkPathOf(t, i))
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, stream); err != nil {
				_ = stream.Close()
				return err
			}
			if err := stream.Close(); err != nil {
				return err
			}
		}
		return nil
	})
	if mergeErr != nil {
		return nil, fail(s.mapFSErr(mergeErr)) // 任务保留：客户端可重试 complete
	}
	s.chunks.Remove(ctx, s.gw, uploadID)
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		target, audit.ResultSuccess, map[string]any{"chunked": true, "totalSize": t.totalSize})
	return &ChunkCompleteData{Path: target, Size: t.totalSize}, nil
}

// DownloadTarget 是下载编排产物：校验全过后打开的流（D-16：开流前完成
// 全部校验与审计；handler 负责写头、拷贝与关闭）。
type DownloadTarget struct {
	RealPath string
	Name     string
	Size     int64
	Stream   io.ReadCloser
}

// Download GET /files/entry/download（admin）：校验链（守卫→存在→为文件，
// 目录 FILE_012 拒绝——目录打包下载 OUT-3）→ 开流 → 审计 file_downloaded
// （security，D-08）。开流后 IO 错误以截断流呈现、服务端日志兜底（D-16）。
func (s *Service) Download(ctx context.Context, actor *authsvc.Identity, path string) (*DownloadTarget, error) {
	action := ActionFileDownloaded
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategorySecurity, action, ObjectTypeFileEntry,
			path, audit.ResultFailure, detailError(err))
		return err
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	if snap.EntryType == TypeDirectory {
		return nil, fail(errcode.ErrFileNotADirectory) // 目录打包下载 OUT-3
	}
	stream, err := s.gw.OpenReadStream(ctx, real) // 校验全过才开流（D-16）
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	s.auditAction(ctx, actor, audit.CategorySecurity, action, ObjectTypeFileEntry,
		real, audit.ResultSuccess, nil)
	return &DownloadTarget{
		RealPath: real,
		Name:     filepath.Base(real),
		Size:     snap.Size,
		Stream:   stream,
	}, nil
}
