package fileman

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// archive_format 取值（§3.4 枚举）。
const (
	FormatTar   = "tar"
	FormatTarGz = "tar.gz"
	FormatZip   = "zip"
)

// archiveExtOf format → 产物扩展名（缺省产物名 archive.{ext}，§4.2.4）。
func archiveExtOf(format string) string {
	switch format {
	case FormatTar:
		return ".tar"
	case FormatTarGz:
		return ".tar.gz"
	case FormatZip:
		return ".zip"
	}
	return ""
}

// detectArchiveFormat 按扩展名识别归档格式（A-F14；.tgz 为 tar.gz 别名）。
func detectArchiveFormat(path string) (string, bool) {
	lower := strings.ToLower(filepath.Base(path))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return FormatZip, true
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return FormatTarGz, true
	case strings.HasSuffix(lower, ".tar"):
		return FormatTar, true
	}
	return "", false
}

// Archive POST /files/entry/archive（admin+CSRF）：format ∈ archive_format
// （FILE_008）；paths 全量守卫预检（任一越界/不存在整体不执行）；产物缺省
// archive.{ext} 落 paths[0] 父目录（A-F3）；同名 FILE_004（D-13）；多选打包
// 含目录递归，流式临时写+rename 原子落盘（I-09/A-F5）。
func (s *Service) Archive(ctx context.Context, actor *authsvc.Identity,
	paths []string, format, targetName string) (*ArchiveData, error) {
	action := ActionFileArchived
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			strings.Join(paths, ","), audit.ResultFailure, detailError(err))
		return err
	}
	if format != FormatTar && format != FormatTarGz && format != FormatZip {
		return nil, fail(errcode.ErrFileInvalidArchiveFormat)
	}
	if len(paths) == 0 {
		return nil, fail(errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "paths", Message: "至少一个条目",
		}))
	}
	// 全量守卫预检 + 实况存在性（任一失败整体不执行，§4.2.4）。
	reals := make([]string, 0, len(paths))
	for _, p := range paths {
		real, err := s.guard.Resolve(p)
		if err != nil {
			return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
		}
		reals = append(reals, real)
	}
	targetDir := filepath.Dir(reals[0]) // A-F3：产物落 paths[0] 父目录
	name := targetName
	if name == "" {
		name = "archive" + archiveExtOf(format)
	}
	if err := ValidateName(name); err != nil {
		return nil, fail(err)
	}
	product := filepath.Join(targetDir, name)
	exists, err := s.gw.Exists(ctx, product)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	if exists {
		return nil, fail(errcode.ErrFileEntryExists) // D-13 产物同名直接错误
	}
	count := 0
	buildErr := s.gw.WriteFileAtomicStreaming(ctx, product, func(w io.Writer) error {
		var err error
		switch format {
		case FormatZip:
			count, err = s.buildZip(ctx, w, reals)
		default:
			count, err = s.buildTar(ctx, w, reals, format == FormatTarGz)
		}
		return err
	})
	if buildErr != nil {
		return nil, fail(s.mapFSErr(buildErr))
	}
	snap, err := s.gw.Stat(ctx, product)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		product, audit.ResultSuccess, map[string]any{"format": format, "count": count})
	return &ArchiveData{Path: product, Size: snap.Size, Format: format}, nil
}

// archiveEntry 是归档内的一个源条目（walk 产物）。
type archiveEntry struct {
	relName string // 归档内相对名（顶层=basename）
	real    string
	isDir   bool
	snap    EntrySnapshot
}

// walkSources 递归收集归档源条目（全部经 gateway 读取；目录递归含隐藏条目）。
// 顶层条目名=各自 basename（跨目录多选安全：归档内不含 ../ 段，A-F3 注）。
// 符号链接按目标内容归档为常规文件（PRD 未列符号链接归档语义，从简处理）。
func (s *Service) walkSources(ctx context.Context, reals []string) ([]archiveEntry, error) {
	var out []archiveEntry
	var walk func(rel, real string) error
	walk = func(rel, real string) error {
		snap, err := s.gw.Stat(ctx, real)
		if err != nil {
			return err
		}
		if snap.EntryType != TypeDirectory {
			out = append(out, archiveEntry{relName: rel, real: real, isDir: false, snap: snap})
			return nil
		}
		out = append(out, archiveEntry{relName: rel, real: real, isDir: true, snap: snap})
		children, err := s.gw.ListDir(ctx, real)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := walk(filepath.Join(rel, c.Name), filepath.Join(real, c.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range reals {
		if err := walk(filepath.Base(r), r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// buildZip 流式写 zip 归档。
func (s *Service) buildZip(ctx context.Context, w io.Writer, reals []string) (int, error) {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	entries, err := s.walkSources(ctx, reals)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if e.isDir {
			hdr := &zip.FileHeader{Name: e.relName + "/", Modified: e.snap.Mtime}
			if _, err := zw.CreateHeader(hdr); err != nil {
				return 0, err
			}
			continue
		}
		hdr := &zip.FileHeader{Name: e.relName, Method: zip.Deflate, Modified: e.snap.Mtime}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return 0, err
		}
		if err := s.copyStream(ctx, e.real, fw); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

// buildTar 流式写 tar/tar.gz 归档（gzip 层独立作用域，避免 nil writer Close）。
func (s *Service) buildTar(ctx context.Context, w io.Writer, reals []string, gzipped bool) (int, error) {
	if gzipped {
		gzw := gzip.NewWriter(w)
		tw := tar.NewWriter(gzw)
		defer func() { _ = tw.Close(); _ = gzw.Close() }() // 顺序：先 tar 后 gzip
		return s.writeTarEntries(ctx, tw, reals)
	}
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()
	return s.writeTarEntries(ctx, tw, reals)
}

func (s *Service) writeTarEntries(ctx context.Context, tw *tar.Writer, reals []string) (int, error) {
	entries, err := s.walkSources(ctx, reals)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.relName, Mode: parseMode(e.snap.Mode), ModTime: e.snap.Mtime}
		if e.isDir {
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
			if err := tw.WriteHeader(hdr); err != nil {
				return 0, err
			}
			continue
		}
		hdr.Typeflag = tar.TypeReg
		hdr.Size = e.snap.Size
		if err := tw.WriteHeader(hdr); err != nil {
			return 0, err
		}
		if err := s.copyStream(ctx, e.real, tw); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

func (s *Service) copyStream(ctx context.Context, real string, w io.Writer) error {
	stream, err := s.gw.OpenReadStream(ctx, real)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	if _, err := io.Copy(w, stream); err != nil {
		return err
	}
	return nil
}

// parseMode 八进制权限串 → 数值（异常回退 0644）。
func parseMode(mode string) int64 {
	v, err := strconv.ParseUint(strings.TrimSpace(mode), 8, 32)
	if err != nil {
		return 0o644
	}
	return int64(v)
}

// extractPlan 是解压预检产物：单条目声明（clean 为归一化相对名）。
type extractPlan struct {
	clean string
	isDir bool
}

// Unarchive POST /files/entry/unarchive（admin+CSRF）：格式识别（FILE_008）
// → targetDir 为目录（FILE_012）→ 全量条目预检（D-07：①逐条目 zip slip
// FILE_009 整体不落盘；②目标同名 ask→FILE_005 零落盘/overwrite/skip）→
// 预检全过才解压落盘。tar/tar.gz 纯流式；zip 经内存全量读入（A-F18：Go
// 标准库 zip 读取器需 ReaderAt，中央目录在文件尾）。
func (s *Service) Unarchive(ctx context.Context, actor *authsvc.Identity,
	archivePath, targetDir, onConflict string) (*UnarchiveData, error) {
	action := ActionFileUnarchived
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
			archivePath, audit.ResultFailure, detailError(err))
		return err
	}
	format, ok := detectArchiveFormat(archivePath)
	if !ok {
		return nil, fail(errcode.ErrFileInvalidArchiveFormat)
	}
	arcReal, err := s.guard.Resolve(archivePath)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	snap, err := s.gw.Stat(ctx, arcReal)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	if snap.EntryType == TypeDirectory {
		return nil, fail(errcode.ErrFileNotADirectory)
	}
	dirReal, err := s.resolveDir(ctx, targetDir)
	if err != nil {
		return nil, fail(err)
	}
	// 预检①安全（zip slip 全量预检，任一越界整体不落盘）。
	plans, err := s.planEntries(ctx, format, arcReal, dirReal)
	if err != nil {
		return nil, fail(err)
	}
	// 预检②占用（ask → FILE_005 零落盘；skip → 收集跳过集；overwrite → 落盘前清理）。
	skip := make(map[string]bool)
	var conflicts []ConflictItem
	for _, p := range plans {
		full := filepath.Join(dirReal, p.clean)
		exists, err := s.gw.Exists(ctx, full)
		if err != nil {
			return nil, fail(s.mapFSErr(err))
		}
		if !exists {
			continue
		}
		switch onConflict {
		case ConflictAsk:
			conflicts = append(conflicts, ConflictItem{Path: full})
		case ConflictSkip:
			skip[p.clean] = true
		}
	}
	if len(conflicts) > 0 {
		return nil, fail(errcode.ErrFileConflict.WithData(ConflictData{Conflicts: conflicts}))
	}
	// 预检全过才解压落盘（D-07/FR-15 整体回滚语义：预检在先=天然零落盘）。
	extracted, err := s.extractEntries(ctx, format, arcReal, dirReal, skip, onConflict == ConflictOverwrite)
	if err != nil {
		return nil, fail(err)
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		arcReal, audit.ResultSuccess, map[string]any{"format": format, "extractedCount": extracted})
	return &UnarchiveData{ExtractedCount: extracted, TargetDir: dirReal}, nil
}

// planEntries 全量安全预检（D-07①）：逐条目归一化 + zip slip 校验 + 守卫
// 静态校验；符号链接/硬链接条目拒绝（A-F17：链接种类条目为逃逸向量）。
func (s *Service) planEntries(ctx context.Context, format, arcReal, dirReal string) ([]extractPlan, error) {
	if format == FormatZip {
		return s.planZip(ctx, arcReal, dirReal)
	}
	stream, err := s.gw.OpenReadStream(ctx, arcReal)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	defer func() { _ = stream.Close() }()
	tr, err := tarStream(stream, format)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	var plans []extractPlan
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, s.mapFSErr(err)
		}
		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeDir:
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			continue // pax 元数据条目
		default: // 符号链接/硬链接/设备/FIFO 等（A-F17）
			return nil, errcode.ErrFileArchiveEntryUnsafe
		}
		clean, err := s.cleanEntryName(hdr.Name, dirReal)
		if err != nil {
			return nil, err
		}
		plans = append(plans, extractPlan{clean: clean, isDir: hdr.Typeflag == tar.TypeDir})
	}
	return plans, nil
}

// planZip zip 侧安全预检（内存全量读入，A-F18）。
func (s *Service) planZip(ctx context.Context, arcReal, dirReal string) ([]extractPlan, error) {
	data, err := s.gw.ReadFile(ctx, arcReal, 0) // 0 = 无上限（§4.2.5 ReadFile 契约）
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, s.mapFSErr(fmt.Errorf("zip open: %w", err))
	}
	var plans []extractPlan
	for _, f := range zr.File {
		if f.FileInfo().Mode()&0o170000 == 0o120000 { // 符号链接条目（A-F17）
			return nil, errcode.ErrFileArchiveEntryUnsafe
		}
		clean, err := s.cleanEntryName(f.Name, dirReal)
		if err != nil {
			return nil, err
		}
		plans = append(plans, extractPlan{clean: clean, isDir: f.FileInfo().IsDir()})
	}
	return plans, nil
}

// extractEntries 解压落盘（预检后第二遍；dirs=EnsureDir，files=原子写）。
func (s *Service) extractEntries(ctx context.Context, format, arcReal, dirReal string,
	skip map[string]bool, overwrite bool) (int, error) {
	if format == FormatZip {
		return s.extractZip(ctx, arcReal, dirReal, skip, overwrite)
	}
	stream, err := s.gw.OpenReadStream(ctx, arcReal)
	if err != nil {
		return 0, s.mapFSErr(err)
	}
	defer func() { _ = stream.Close() }()
	tr, err := tarStream(stream, format)
	if err != nil {
		return 0, s.mapFSErr(err)
	}
	extracted := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return extracted, s.mapFSErr(err)
		}
		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeDir:
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			continue
		default:
			return extracted, errcode.ErrFileArchiveEntryUnsafe // 预检后归档被替换的防御
		}
		clean, cerr := s.cleanEntryName(hdr.Name, dirReal)
		if cerr != nil {
			return extracted, cerr
		}
		full := filepath.Join(dirReal, clean)
		if hdr.Typeflag == tar.TypeDir {
			if err := s.gw.EnsureDir(ctx, full); err != nil {
				return extracted, s.mapFSErr(err)
			}
			extracted++
			continue
		}
		if skip[clean] {
			continue
		}
		if overwrite {
			if exists, eerr := s.gw.Exists(ctx, full); eerr == nil && exists {
				if rerr := s.gw.RemoveTree(ctx, full); rerr != nil {
					return extracted, s.mapFSErr(rerr)
				}
			} else if eerr != nil {
				return extracted, s.mapFSErr(eerr)
			}
		}
		werr := s.gw.WriteFileAtomicStreaming(ctx, full, func(w io.Writer) error {
			_, err := io.Copy(w, tr) // tar 条目内容随 reader 定位
			return err
		})
		if werr != nil {
			return extracted, s.mapFSErr(werr)
		}
		extracted++
	}
	return extracted, nil
}

// extractZip zip 侧解压（内存读入，A-F18）。
func (s *Service) extractZip(ctx context.Context, arcReal, dirReal string,
	skip map[string]bool, overwrite bool) (int, error) {
	data, err := s.gw.ReadFile(ctx, arcReal, 0)
	if err != nil {
		return 0, s.mapFSErr(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, s.mapFSErr(fmt.Errorf("zip open: %w", err))
	}
	extracted := 0
	for _, f := range zr.File {
		clean, cerr := s.cleanEntryName(f.Name, dirReal)
		if cerr != nil {
			return extracted, cerr
		}
		full := filepath.Join(dirReal, clean)
		if f.FileInfo().IsDir() {
			if err := s.gw.EnsureDir(ctx, full); err != nil {
				return extracted, s.mapFSErr(err)
			}
			extracted++
			continue
		}
		if skip[clean] {
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return extracted, s.mapFSErr(oerr)
		}
		if overwrite {
			if exists, eerr := s.gw.Exists(ctx, full); eerr == nil && exists {
				if rerr := s.gw.RemoveTree(ctx, full); rerr != nil {
					_ = rc.Close()
					return extracted, s.mapFSErr(rerr)
				}
			}
		}
		werr := s.gw.WriteFileAtomicStreaming(ctx, full, func(w io.Writer) error {
			defer func() { _ = rc.Close() }()
			_, err := io.Copy(w, rc)
			return err
		})
		if werr != nil {
			return extracted, s.mapFSErr(werr)
		}
		extracted++
	}
	return extracted, nil
}

// tarStream 组装 tar（±gzip）阅读器。
func tarStream(r io.Reader, format string) (*tar.Reader, error) {
	if format == FormatTarGz {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		return tar.NewReader(gz), nil
	}
	return tar.NewReader(r), nil
}

// cleanEntryName 归档条目声明路径归一化与安全校验（zip slip，R-04 延伸）：
// 拒绝绝对路径与 .. 段（FILE_009）；拼合目标过守卫静态校验（D-07①）。
func (s *Service) cleanEntryName(name, dirReal string) (string, error) {
	if name == "" {
		return "", errcode.ErrFileArchiveEntryUnsafe
	}
	clean := filepath.Clean(strings.TrimPrefix(filepath.ToSlash(name), "./"))
	if clean == "" {
		return "", errcode.ErrFileArchiveEntryUnsafe
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errcode.ErrFileArchiveEntryUnsafe
	}
	if err := s.guard.CheckStatic(filepath.Join(dirReal, clean)); err != nil {
		return "", errcode.ErrFileArchiveEntryUnsafe // 拼合后越界 → 整体不落盘
	}
	return clean, nil
}
