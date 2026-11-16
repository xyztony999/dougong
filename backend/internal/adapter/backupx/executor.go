// Package backupx 是备份执行适配器（api-cron-backup.md §3.2 BackupExecutor
// 进程内契约承载）：tar 打包（纯 Go archive/tar）+ 数据库导出流 gzip（经
// database-mgmt OHS DumpSource——凭据解密在其域内完成 ⚠️A18）+ 产物写目标
// （本地目录 / rsync 推送）+ manifest 写入（D-10）+ 产物拉回/清理（download
// 与轮转清理通道）。
//
// rsync 一律 exec.CommandContext 字面量参数数组（R-08）；-e 的端口参数为
// 服务端校验后的整数（fmt %d 无注入面，T07 人工核对登记）。
package backupx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// 源/目标类型（backup_source_type / backup_target_type 枚举）。
const (
	KindSiteDir  = "site_dir"
	KindDatabase = "database"
	TypeLocal    = "local"
	TypeRsync    = "rsync"
)

// ErrArtifactMissing 产物文件缺失（面板外被删）→ CB_009。
var ErrArtifactMissing = errors.New("backupx: artifact missing")

// SourceSpec 是执行期源项（service 从 job 定义+实例档案组装；整实例全库项
// Database 为空，执行期经 DumpSource.DatabaseNames 展开，A-C6）。
// Ref 是契约引用字段（§4.1.4 sourceSummary/§4.2.4 detail sources 共用形状
// {type, ref}，Gate 2 E1 修复）：site_dir→目录路径 / database→「实例名·库名」
// ——manifest 快照落盘，detail.sources 经 manifest 语义外发（A-C5）。
type SourceSpec struct {
	Kind         string `json:"type"`
	Ref          string `json:"ref"`
	Path         string `json:"path,omitempty"`
	InstanceID   int64  `json:"instanceId,omitempty"`
	InstanceName string `json:"instanceName,omitempty"` // 档案显示名（产物命名用）
	Database     string `json:"database,omitempty"`
}

// SourceRef 组装 site_dir 项引用串（目录路径原样，E1）。
func SourceRef(path string) string { return path }

// DBRef 组装 database 项引用串（实例名·库名；库名缺省=* 整实例语义，E1）。
func DBRef(instanceName, database string) string {
	if database == "" {
		database = "*"
	}
	return instanceName + "·" + database
}

// TargetSpec 是执行期目标值对象（D-06：无凭据字段——ssh 免密由运行环境提供）。
type TargetSpec struct {
	Type string `json:"type"`
	Path string `json:"path"` // local 绝对路径 / rsync 远端根路径
	Host string `json:"host,omitempty"`
	User string `json:"user,omitempty"`
	Port int    `json:"port,omitempty"`
}

// JobSpec 是执行编排输入（trigger 时 job 定义快照，A-C10——执行期不回读
// job 行，运行中删除不中断执行）。
type JobSpec struct {
	JobID          int64
	Name           string
	Sources        []SourceSpec
	Target         TargetSpec
	RetentionCount int
}

// FileEntry 是产物清单项（manifest files 与 run.files 快照共用形态）。
type FileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Manifest 是产物自描述清单（D-10：{jobId, jobName, sources[], startedAt,
// finishedAt, files:[{name,size}]}——恢复时的核对依据）。
type Manifest struct {
	JobID      int64        `json:"jobId"`
	JobName    string       `json:"jobName"`
	Sources    []SourceSpec `json:"sources"`
	StartedAt  string       `json:"startedAt"`
	FinishedAt string       `json:"finishedAt"`
	Files      []FileEntry  `json:"files"`
}

// ManifestFileName manifest 文件名（产物目录内）。
const ManifestFileName = "manifest.json"

// ExecResult 是执行成功产物定位（run 终态回写载荷，I-04）。
type ExecResult struct {
	ArtifactPath string
	ArtifactSize int64
	Files        []FileEntry
}

// DumpSource 是 database-mgmt OHS 消费契约（§3.2 DbGateway.DumpDatabase 消费
// 登记：导出流 + 实况库清单；实现=service/dbmgmt.Service，main 装配注入）。
type DumpSource interface {
	// DumpStream 单库导出流（凭据解密在 database-mgmt 域内完成）。
	DumpStream(ctx context.Context, instanceID int64, database string) (io.ReadCloser, error)
	// DatabaseNames 实况库清单（整实例全库展开，已滤系统库）。
	DatabaseNames(ctx context.Context, instanceID int64) ([]string, error)
}

// Executor 是 BackupExecutor 实现。
type Executor struct {
	rsyncBin string
	dump     DumpSource
	clock    func() time.Time
}

// NewExecutor 构造（rsyncBin 默认 "rsync"；dump 为 database-mgmt OHS 视图）。
func NewExecutor(rsyncBin string, dump DumpSource) *Executor {
	if rsyncBin == "" {
		rsyncBin = "rsync"
	}
	return &Executor{rsyncBin: rsyncBin, dump: dump, clock: time.Now}
}

// Execute 执行一次备份（2.3 执行编排的逐源执行段）：产物目录
// {目标根}/{job 名}/{yyyyMMdd-HHmmss}/（D-10），每源一份产物文件+manifest；
// 任一源失败→整体失败（I-04），错误前缀列明失败源；失败时 best-effort 清理
// 半成品（local 直接删 / rsync 远端删）。
func (e *Executor) Execute(ctx context.Context, spec JobSpec, startedAt time.Time) (ExecResult, error) {
	ts := startedAt.UTC().Format("20060102-150405")
	staging, finalDir, err := e.prepareStaging(spec, spec.Name+"/"+ts)
	if err != nil {
		return ExecResult{}, err
	}
	res, execErr := e.executeStaging(ctx, spec, staging, startedAt)
	if execErr != nil {
		e.cleanupPartial(ctx, spec, staging, finalDir)
		return ExecResult{}, execErr
	}
	if spec.Target.Type == TypeRsync {
		if err := e.pushDir(ctx, spec.Target, staging, finalDir); err != nil {
			e.cleanupPartial(ctx, spec, staging, finalDir)
			return ExecResult{}, fmt.Errorf("rsync push: %w", err)
		}
		_ = os.RemoveAll(staging) // 推送完成暂存即弃（产物已在远端）
	}
	res.ArtifactPath = finalDir // local=绝对路径 / rsync=远端路径（I-04 终态定位）
	return res, nil
}

// prepareStaging 建立产物目录：local=目标内直接建（MkdirAll 0755）；rsync=
// 本地暂存目录（推送后即弃）。
func (e *Executor) prepareStaging(spec JobSpec, rel string) (staging, finalDir string, err error) {
	if spec.Target.Type == TypeLocal {
		dir := filepath.Join(spec.Target.Path, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", "", fmt.Errorf("create artifact dir %s: %w", dir, err)
		}
		return dir, dir, nil
	}
	tmp, err := os.MkdirTemp("", "dougong-backup-*")
	if err != nil {
		return "", "", fmt.Errorf("create staging dir: %w", err)
	}
	return tmp, path.Join(spec.Target.Path, rel), nil // 远端路径恒 POSIX（path.Join）
}

// executeStaging 逐源写产物到 staging 目录并落 manifest（local/rsync 共用）。
func (e *Executor) executeStaging(ctx context.Context, spec JobSpec, staging string, startedAt time.Time) (ExecResult, error) {
	files := make([]FileEntry, 0, 8)
	sources := make([]SourceSpec, 0, len(spec.Sources)*2)
	siteSeq := 0
	for _, src := range spec.Sources {
		switch src.Kind {
		case KindSiteDir:
			siteSeq++
			if src.Ref == "" { // 兜底（正常由 service 投影层填，E1）：manifest 完备
				src.Ref = SourceRef(src.Path)
			}
			name := fmt.Sprintf("site-%d.tar.gz", siteSeq)
			size, err := tarGzDir(staging, name, src.Path)
			if err != nil {
				return ExecResult{}, fmt.Errorf("site_dir %s: %w", src.Path, err)
			}
			files = append(files, FileEntry{Name: name, Size: size})
			sources = append(sources, src)
		case KindDatabase:
			dbs := []string{src.Database}
			if src.Database == "" { // 整实例全库：执行期展开（A-C6）
				names, err := e.dump.DatabaseNames(ctx, src.InstanceID)
				if err != nil {
					return ExecResult{}, fmt.Errorf("database %s: list databases: %w", src.InstanceName, err)
				}
				if len(names) == 0 {
					return ExecResult{}, fmt.Errorf("database %s: instance has no databases", src.InstanceName)
				}
				dbs = names
			}
			for _, db := range dbs {
				name := fmt.Sprintf("db-%s-%s.sql.gz", sanitizeLabel(src.InstanceName), sanitizeLabel(db))
				size, err := e.dumpOne(ctx, staging, name, src.InstanceID, db)
				if err != nil {
					return ExecResult{}, fmt.Errorf("database %s/%s: %w", src.InstanceName, db, err)
				}
				files = append(files, FileEntry{Name: name, Size: size})
				executed := src
				executed.Database = db
				executed.Ref = DBRef(src.InstanceName, db) // 展开后逐库引用（E1）
				sources = append(sources, executed)
			}
		default:
			return ExecResult{}, fmt.Errorf("source kind %q invalid", src.Kind)
		}
	}
	finished := e.clock().UTC()
	manifest := Manifest{
		JobID: spec.JobID, JobName: spec.Name, Sources: sources,
		StartedAt:  startedAt.UTC().Format(time.RFC3339),
		FinishedAt: finished.Format(time.RFC3339),
		Files:      files,
	}
	if err := writeJSONFile(filepath.Join(staging, ManifestFileName), manifest); err != nil {
		return ExecResult{}, fmt.Errorf("write manifest: %w", err)
	}
	var total int64
	for _, f := range files {
		total += f.Size
	}
	mStat, err := os.Stat(filepath.Join(staging, ManifestFileName))
	if err != nil {
		return ExecResult{}, fmt.Errorf("stat manifest: %w", err)
	}
	return ExecResult{Files: files, ArtifactSize: total + mStat.Size()}, nil
}

// cleanupPartial 失败清理（best-effort）：暂存目录连同其 job 级父目录（local；
// I-01 互斥保证同 job 无并发产物，父目录属本 run 专属）+ rsync 远端删——
// 半成品不残留为可下载假象（I-04 failure 不可下载）。
func (e *Executor) cleanupPartial(ctx context.Context, spec JobSpec, staging, finalDir string) {
	if spec.Target.Type == TypeRsync {
		_ = os.RemoveAll(staging)
		if err := e.removeRemote(ctx, spec.Target, finalDir); err != nil {
			slogWarn(ctx, "backupx: cleanup partial remote artifact", "path", finalDir, "err", err)
		}
		return
	}
	_ = os.RemoveAll(filepath.Dir(staging)) // {目标根}/{job 名} 整组（本 run 专属）
}

// dumpOne 单库导出：OHS 流 → gzip → 产物文件。
func (e *Executor) dumpOne(ctx context.Context, staging, name string, instanceID int64, db string) (int64, error) {
	stream, err := e.dump.DumpStream(ctx, instanceID, db)
	if err != nil {
		return 0, fmt.Errorf("dump open: %w", err)
	}
	defer func() { _ = stream.Close() }()
	out, err := os.Create(filepath.Join(staging, name))
	if err != nil {
		return 0, fmt.Errorf("create artifact: %w", err)
	}
	gz := gzip.NewWriter(out)
	_, copyErr := io.Copy(gz, stream)
	gzErr := gz.Close()
	if cerr := out.Close(); cerr != nil {
		return 0, fmt.Errorf("close artifact: %w", cerr)
	}
	if copyErr != nil {
		return 0, fmt.Errorf("dump copy: %w", copyErr)
	}
	if gzErr != nil {
		return 0, fmt.Errorf("gzip close: %w", gzErr)
	}
	st, err := os.Stat(filepath.Join(staging, name))
	if err != nil {
		return 0, fmt.Errorf("stat artifact: %w", err)
	}
	return st.Size(), nil
}

// tarGzDir 站点目录打包（纯 Go：目录/常规文件内容 + 符号链接条目不跟随——
// 防逃逸；其余类型跳过）。返回产物字节数。
func tarGzDir(staging, name, srcDir string) (int64, error) {
	out, err := os.Create(filepath.Join(staging, name))
	if err != nil {
		return 0, fmt.Errorf("create archive: %w", err)
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	walkErr := filepath.WalkDir(srcDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			hdr.Typeflag = tar.TypeDir
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write dir header %s: %w", rel, err)
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(p)
			if lerr != nil {
				return fmt.Errorf("read link %s: %w", rel, lerr)
			}
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = filepath.ToSlash(link)
			hdr.Size = 0
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write link header %s: %w", rel, err)
			}
		case info.Mode().IsRegular():
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write file header %s: %w", rel, err)
			}
			f, ferr := os.Open(p)
			if ferr != nil {
				return fmt.Errorf("open %s: %w", rel, ferr)
			}
			_, cerr := io.Copy(tw, f)
			_ = f.Close()
			if cerr != nil {
				return fmt.Errorf("copy %s: %w", rel, cerr)
			}
		default: // socket/device/fifo 等：跳过（打包语义外，如实不含）
			return nil
		}
		return nil
	})
	twErr := tw.Close()
	gzErr := gz.Close()
	if cerr := out.Close(); cerr != nil {
		return 0, fmt.Errorf("close archive: %w", cerr)
	}
	if walkErr != nil {
		return 0, fmt.Errorf("walk %s: %w", srcDir, walkErr)
	}
	if twErr != nil {
		return 0, fmt.Errorf("tar close: %w", twErr)
	}
	if gzErr != nil {
		return 0, fmt.Errorf("gzip close: %w", gzErr)
	}
	st, err := os.Stat(filepath.Join(staging, name))
	if err != nil {
		return 0, fmt.Errorf("stat archive: %w", err)
	}
	return st.Size(), nil
}

// writeJSONFile 写 JSON 文件（0644）。
func writeJSONFile(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// sanitizeLabel 文件名安全化（实例名/库名可能含 ':'、'/'、空格等——socket
// 路径形态实例名）。
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

// slogWarn 结构化日志（log/slog 标准库，adapter 无请求语义仍携带 ctx）。
func slogWarn(ctx context.Context, msg string, args ...any) {
	slog.WarnContext(ctx, msg, args...)
}
