package backupx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// rsyncBaseArgs 组装 rsync 公共参数（字面量参数数组，R-08）。-e 的端口为
// 服务端校验后的整数（1..65535，fmt %d 无注入面——T07 人工核对登记）；
// 22/0 不携带 -e（ssh 默认端口）。
func rsyncBaseArgs(port int) []string {
	args := []string{"-a"}
	if port != 0 && port != 22 {
		args = append(args, "-e", fmt.Sprintf("ssh -p %d", port))
	}
	return args
}

// remoteRef 组装远端引用 user@host:path（user 空省=当前用户，与 ssh 同语义）。
func remoteRef(t TargetSpec, p string) string {
	host := t.Host
	if t.User != "" {
		host = t.User + "@" + t.Host
	}
	return host + ":" + p
}

// runRsync 执行 rsync（参数数组；stderr 首行进错误上下文）。
func (e *Executor) runRsync(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, e.rsyncBin, args...) // 字面量参数数组
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if i := strings.IndexByte(reason, '\n'); i >= 0 {
			reason = reason[:i]
		}
		if reason == "" {
			reason = err.Error()
		}
		return fmt.Errorf("rsync: %w: %s", err, reason)
	}
	return nil
}

// pushDir 推送暂存目录内容到远端产物目录（rsync -a [-e ssh -p N] src/ dest）。
func (e *Executor) pushDir(ctx context.Context, t TargetSpec, staging, remoteDir string) error {
	args := append(rsyncBaseArgs(t.Port), strings.TrimSuffix(filepath.ToSlash(staging), "/")+"/",
		remoteRef(t, remoteDir)+"/")
	return e.runRsync(ctx, args...)
}

// removeRemote 清空远端目录内容（空目录 --delete 推送——rsync 无 rmdir 语义，
// 目录壳留存、内容全删；产物清理语义达成）。
func (e *Executor) removeRemote(ctx context.Context, t TargetSpec, remoteDir string) error {
	empty, err := os.MkdirTemp("", "dougong-rm-*")
	if err != nil {
		return fmt.Errorf("create empty dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(empty) }()
	args := append(rsyncBaseArgs(t.Port), "--delete",
		strings.TrimSuffix(filepath.ToSlash(empty), "/")+"/", remoteRef(t, remoteDir)+"/")
	return e.runRsync(ctx, args...)
}

// ClassifyArtifact 由 artifact_path 判定产物归属：以 "/" 起始=local 绝对路径；
// 含 "user@host:" / "host:" 形态=rsync（拆出 [user@]host 与远端路径）。单字母
// 盘符前缀（Windows 开发机 C:\ 形态）按 local——生产环境 artifact_path 恒为
// Linux 路径，本分支仅开发/测试容错。
func ClassifyArtifact(artifactPath string) (kind, userHost, remotePath string) {
	if strings.HasPrefix(artifactPath, "/") {
		return TypeLocal, "", ""
	}
	if i := strings.IndexByte(artifactPath, ':'); i > 0 && !strings.ContainsAny(artifactPath[:i], "/\\") {
		if len(artifactPath[:i]) == 1 { // 盘符形态（C:）
			return TypeLocal, "", ""
		}
		return TypeRsync, artifactPath[:i], artifactPath[i+1:]
	}
	return TypeLocal, "", "" // 兜底按本地（无冒号前缀的相对形态理论不可达）
}

// PullFile 拉回产物单文件到本地临时文件（download 通道：rsync 目标经 backupx
// 拉回流，§4.2.4）。cleanup 由调用方在流结束后执行（删临时文件）。
func (e *Executor) PullFile(ctx context.Context, artifactPath, name string, port int) (string, func(), error) {
	kind, userHost, remotePath := ClassifyArtifact(artifactPath)
	target := artifactTarget(userHost, port)
	local, err := os.CreateTemp("", "dougong-pull-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp: %w", err)
	}
	localPath := local.Name()
	if err := local.Close(); err != nil {
		_ = os.Remove(localPath)
		return "", nil, fmt.Errorf("close temp: %w", err)
	}
	cleanup := func() { _ = os.Remove(localPath) }
	if kind == TypeLocal {
		src := filepath.Join(artifactPath, filepath.FromSlash(name))
		st, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil, ErrArtifactMissing
			}
			return "", nil, fmt.Errorf("stat artifact: %w", err)
		}
		if st.IsDir() {
			return "", nil, fmt.Errorf("%w: %s is a directory", ErrArtifactMissing, name)
		}
		if err := os.Rename(src, localPath); err != nil { // 同卷 rename 失败回退拷贝
			if cerr := copyFile(src, localPath); cerr != nil {
				return "", nil, cerr
			}
		}
		return localPath, cleanup, nil
	}
	args := append(rsyncBaseArgs(port), remoteRef(target, path.Join(remotePath, name)), localPath)
	if err := e.runRsync(ctx, args...); err != nil {
		cleanup()
		return "", nil, err
	}
	if st, err := os.Stat(localPath); err != nil || st.Size() == 0 {
		cleanup()
		return "", nil, ErrArtifactMissing
	}
	return localPath, cleanup, nil
}

// ReadManifestPath 读取产物 manifest（detail.sources[] 数据源，A-C5；rsync
// 目标 MVP 不拉流——返回 ErrRemoteManifest，service 置 sources=null）。
func (e *Executor) ReadManifestPath(ctx context.Context, artifactPath string, port int) (*Manifest, error) {
	kind, _, _ := ClassifyArtifact(artifactPath)
	if kind == TypeRsync {
		return nil, ErrRemoteManifest
	}
	b, err := os.ReadFile(filepath.Join(artifactPath, ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("%w: manifest unreadable", ErrArtifactMissing)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// ErrRemoteManifest rsync 目标 manifest 不拉流（A-C5 裁定）。
var ErrRemoteManifest = fmt.Errorf("backupx: remote manifest not fetched (mvp)")

// RemoveArtifactPath 轮转产物清理（事务后 best-effort，D-09/A-C4）：local=
// RemoveAll；rsync=空目录 --delete 推送（内容清空）。
func (e *Executor) RemoveArtifactPath(ctx context.Context, artifactPath string, port int) error {
	kind, userHost, remotePath := ClassifyArtifact(artifactPath)
	if kind == TypeLocal {
		if err := os.RemoveAll(artifactPath); err != nil {
			return fmt.Errorf("remove local artifact: %w", err)
		}
		return nil
	}
	return e.removeRemote(ctx, artifactTarget(userHost, port), remotePath)
}

// artifactTarget 由 userHost（可能含 user@）还原 TargetSpec。
func artifactTarget(userHost string, port int) TargetSpec {
	t := TargetSpec{Type: TypeRsync, Port: port}
	if i := strings.IndexByte(userHost, '@'); i >= 0 {
		t.User, t.Host = userHost[:i], userHost[i+1:]
	} else {
		t.Host = userHost
	}
	return t
}

// copyFile 简单文件拷贝（rename 跨卷回退）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create temp copy: %w", err)
	}
	if _, err = out.ReadFrom(in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy artifact: %w", err)
	}
	return out.Close()
}
