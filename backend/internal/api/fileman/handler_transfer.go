package fileman

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	filemansvc "github.com/dougong/dougong/internal/service/fileman"
	"github.com/gin-gonic/gin"
)

// Upload POST /files/entry/upload（admin+CSRF，multipart/form-data）：
// `dir` 字段 + 多个 `files` 字段 + `onConflict` 字段（默认 ask）。
// 超限先检后读体（A-F8：FileHeader.Size 判定 FILE_011 不读文件内容）。
func (h *Handler) Upload(c *gin.Context) {
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		response.Fail(c, bindError(err))
		return
	}
	dir := form.Value["dir"]
	onConflict := form.Value["onConflict"]
	if len(dir) == 0 {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "dir", Message: "必填",
		}))
		return
	}
	oc := ""
	if len(onConflict) > 0 {
		oc = onConflict[0]
	}
	files := form.File["files"]
	payloads := make([]filemansvc.UploadPayload, 0, len(files))
	for _, fh := range files {
		if fh.Size > filemansvc.DirectUploadLimit { // 先检不读体（A-F8）
			response.Fail(c, errcode.ErrFileTooLarge.WithData(filemansvc.ReasonData{
				Reason: fmt.Sprintf("%s 超过单文件 %d 字节上限，请走分块上传", fh.Filename, filemansvc.DirectUploadLimit),
			}))
			return
		}
		data, err := readMultipartFile(fh)
		if err != nil {
			response.Fail(c, errcode.ErrFilesystem.WithData(filemansvc.ReasonData{Reason: "io_error"}))
			return
		}
		payloads = append(payloads, filemansvc.UploadPayload{Name: fh.Filename, Size: fh.Size, Data: data})
	}
	data, err := h.svc.Upload(c.Request.Context(), actor, dir[0], payloads, oc)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// readMultipartFile 读取单个 multipart 文件部件（≤5MiB，service 前置已检）。
func readMultipartFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open multipart %s: %w", fh.Filename, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read multipart %s: %w", fh.Filename, err)
	}
	return data, nil
}

// ChunkInit POST /files/upload/chunk-init（admin+CSRF）：分块任务初始化
// （D-10；totalChunks=ceil(totalSize/5MiB) 校验 FILE_010）。
func (h *Handler) ChunkInit(c *gin.Context) {
	var req struct {
		Dir         string `json:"dir"`
		FileName    string `json:"fileName"`
		TotalSize   int64  `json:"totalSize"`
		TotalChunks int64  `json:"totalChunks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.ChunkInit(c.Request.Context(), req.Dir, req.FileName, req.TotalSize, req.TotalChunks)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ChunkUpload POST /files/upload/chunk（admin+CSRF，multipart）：
// `uploadId` + `index` + `chunk` 字段；逐块校验（FILE_010）。
func (h *Handler) ChunkUpload(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		response.Fail(c, bindError(err))
		return
	}
	ids := form.Value["uploadId"]
	indexes := form.Value["index"]
	chunks := form.File["chunk"]
	if len(ids) == 0 || len(indexes) == 0 || len(chunks) == 0 {
		response.Fail(c, errcode.ErrFileChunkInvalid)
		return
	}
	var index int
	index, err = strconv.Atoi(indexes[0])
	if err != nil {
		response.Fail(c, errcode.ErrFileChunkInvalid)
		return
	}
	data, rerr := readMultipartFile(chunks[0])
	if rerr != nil {
		response.Fail(c, errcode.ErrFilesystem.WithData(filemansvc.ReasonData{Reason: "io_error"}))
		return
	}
	out, err := h.svc.ChunkUpload(c.Request.Context(), ids[0], index, data)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, out)
}

// ChunkComplete POST /files/upload/chunk-complete（admin+CSRF）：合并落盘
// （校验+按 index 序合并+原子 rename+审计，D-04/I-09）。
func (h *Handler) ChunkComplete(c *gin.Context) {
	var req struct {
		UploadID string `json:"uploadId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.ChunkComplete(c.Request.Context(), actor, req.UploadID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Download GET /files/entry/download（admin）：单文件流式下载（数据取出，
// D-08/D-16：校验链全过才写头开流；开流后 IO 错误以截断流呈现、服务端
// 日志兜底——200 头已发不可改写 JSON）。
func (h *Handler) Download(c *gin.Context) {
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	target, err := h.svc.Download(c.Request.Context(), actor, c.Query("path"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer func() { _ = target.Stream.Close() }()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": target.Name,
	}))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(target.Size, 10))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, target.Stream); err != nil {
		// 截断流兜底日志（D-16）；审计已按开流成功记录，如实不翻转。
		slog.WarnContext(c.Request.Context(), "file-manager: download stream truncated",
			"path", target.RealPath, "err", err)
	}
}
