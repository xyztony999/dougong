package dbmgmt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ExportStream 是导出编排结果（D-06 双形态响应载体）：Body 为 mysqldump
// 流式输出（不留存服务器目录）；Filename 按 {engine-host-port}_{db}_{ts}.sql
// 规则；Finish/Abort 收尾（审计 + busy 解除），由 handler 在流生命周期两端
// 调用（A-DB7：首字节探测失败 → Abort 返回 DBM_012 JSON；流完 → Finish 审计
// sizeBytes）。
type ExportStream struct {
	Body     io.ReadCloser
	Filename string

	actor  *authsvc.Identity
	instID int64
	db     string
	svc    *Service
}

// Finish 流结束收尾：解除 busy + 成功审计（detail={database, sizeBytes}）。
func (e *ExportStream) Finish(sizeBytes int64) {
	e.svc.busy.Release(e.instID)
	e.svc.recordAudit(context.Background(), auditEntry(e.actor, ActionDatabaseExported,
		ObjectTypeDatabase, dbObjectID(&storedb.Instance{ID: e.instID}, e.db),
		map[string]any{"database": e.db, "sizeBytes": sizeBytes}, true))
}

// Abort 流失败收尾：关闭流 + 解除 busy + 失败审计 + 返回 DBM_012（reason）。
func (e *ExportStream) Abort(cause error) error {
	_ = e.Body.Close()
	e.svc.busy.Release(e.instID)
	reason := reasonOf(cause)
	e.svc.recordAudit(context.Background(), auditEntry(e.actor, ActionDatabaseExported,
		ObjectTypeDatabase, dbObjectID(&storedb.Instance{ID: e.instID}, e.db),
		map[string]string{"database": e.db, "reason": reason}, false))
	return errcode.ErrDBMDumpFailed.WithData(map[string]string{"reason": reason})
}

// Broken 流中途失败收尾（响应已开始只能断流，A-DB7）：关闭流 + 解除 busy +
// 失败审计（detail 含已传输字节数，如实呈现中断）。
func (e *ExportStream) Broken(transferred int64) {
	_ = e.Body.Close()
	e.svc.busy.Release(e.instID)
	e.svc.recordAudit(context.Background(), auditEntry(e.actor, ActionDatabaseExported,
		ObjectTypeDatabase, dbObjectID(&storedb.Instance{ID: e.instID}, e.db),
		map[string]any{"database": e.db, "transferredBytes": transferred, "reason": "stream broken"},
		false))
}

// ExportDatabase POST /db/backup/export 编排（D-06/D-12）：I-05 前置链 + 库
// 实况存在（DBM_007）+ 在途（DBM_016）→ 标记 busy → mysqldump（charset 按库
// 实况组装）→ 流式响应。成功审计与 busy 解除由 ExportStream 收尾方法承载。
func (s *Service) ExportDatabase(ctx context.Context, actor *authsvc.Identity, instanceID int64, database string) (*ExportStream, error) {
	inst, t, c, err := s.governTargetByID(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	dbs, err := s.mysql.ListDatabases(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	charset := ""
	found := false
	for _, d := range dbs {
		if d.Name == database {
			found, charset = true, d.Charset
			break
		}
	}
	if !found {
		return nil, errcode.ErrDBMDatabaseNotFound
	}
	if !s.busy.Acquire(inst.ID) {
		return nil, errcode.ErrDBMInstanceBusy
	}
	body, err := s.mysql.DumpDatabase(ctx, t, c, database, charset)
	if err != nil {
		s.busy.Release(inst.ID)
		return nil, translateGatewayErr(err)
	}
	ts := s.clock().UTC().Format("20060102150405")
	name := fmt.Sprintf("%s-%s_%s_%s.sql", inst.Engine, hostPortLabel(t), database, ts)
	return &ExportStream{
		Body: body, Filename: name,
		actor: actor, instID: inst.ID, db: database, svc: s,
	}, nil
}

// ImportDatabase POST /db/backup/import 编排（D-06/D-12）：I-05 前置链 + 库
// 存在 + 大小校验（DBM_014，≤512MiB）+ 在途（DBM_016）→ 标记 busy → mysql
// 客户端参数数组导入（multipart 临时文件流由调用方治理）→ 解除 busy →
// 审计（detail={database, sizeBytes}）。失败 DBM_013（data.reason，不自动
// 回滚）；非幂等（交互层二次确认前置）。
func (s *Service) ImportDatabase(ctx context.Context, actor *authsvc.Identity,
	instanceID int64, database string, sizeBytes int64, r io.Reader) (*ImportData, error) {
	inst, t, c, err := s.governTargetByID(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	if sizeBytes <= 0 {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "file", Message: "导入文件为空"})
	}
	if sizeBytes > DumpSizeLimit {
		return nil, errcode.ErrDBMDumpSizeExceeded
	}
	exists, err := s.mysql.DatabaseExists(ctx, t, c, database)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if !exists {
		return nil, errcode.ErrDBMDatabaseNotFound
	}
	if !s.busy.Acquire(inst.ID) {
		return nil, errcode.ErrDBMInstanceBusy
	}
	defer s.busy.Release(inst.ID) // 任务结束（成功/失败）即解除（D-12）

	if err := s.mysql.ImportDatabase(ctx, t, c, database, r); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDatabaseImported, ObjectTypeDatabase,
			dbObjectID(inst, database),
			map[string]string{"database": database, "sizeBytes": strconv.FormatInt(sizeBytes, 10), "result": "failure", "reason": reasonOf(err)}, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDatabaseImported, ObjectTypeDatabase,
		dbObjectID(inst, database),
		map[string]any{"database": database, "sizeBytes": sizeBytes}, true))
	return &ImportData{InstanceID: inst.ID, Database: database, SizeBytes: sizeBytes}, nil
}

// PeekExport 首字节探测（A-DB7）：dump 流已有产出 → (br, nil)（br 含已缓冲
// 首字节，后续 io.Copy 直接消费）；进程启动即失败 → Abort 归一为 DBM_012
// （此时响应头未写，handler 仍可回 JSON 错误体——双形态契约 D-06）。
func PeekExport(stream *ExportStream) (*bufio.Reader, error) {
	br := bufio.NewReader(stream.Body)
	if _, err := br.Peek(1); err != nil {
		if errors.Is(err, io.EOF) {
			err = errors.New("empty dump output")
		}
		return nil, stream.Abort(fmt.Errorf("%w: %v", errDumpEOF, err))
	}
	return br, nil
}

// errDumpEOF 首字节探测失败包装哨兵（reason 提取用）。
var errDumpEOF = errors.New("dump stream failed")

// CountingWriter 计数字节写入（导出审计 sizeBytes，A-DB7；handler 流写时
// 包装响应写入器）。
type CountingWriter struct {
	N int64
	W io.Writer
}

// Write 实现 io.Writer 并累计字节数。
func (cw *CountingWriter) Write(p []byte) (int, error) {
	n, err := cw.W.Write(p)
	cw.N += int64(n)
	return n, err
}
