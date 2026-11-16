package fslocal

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	filemansvc "github.com/dougong/dougong/internal/service/fileman"
)

func TestGatewayAtomicWriteAndRead(t *testing.T) {
	g := New()
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := g.WriteFileAtomic(ctx, p, []byte("v1")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := g.ReadFile(ctx, p, 0)
	if err != nil || string(got) != "v1" {
		t.Fatalf("read: %v %q", err, got)
	}
	// 覆盖写 + limit 读取。
	if err := g.WriteFileAtomic(ctx, p, []byte("v2-longer")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err = g.ReadFile(ctx, p, 2)
	if err != nil || string(got) != "v2" {
		t.Fatalf("limited read: %v %q", err, got)
	}
	// 失败不留临时文件（writeAtomic defer 清理）。
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("tmp leak: %d entries", len(entries))
	}
}

func TestGatewayStreamingWrite(t *testing.T) {
	g := New()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "stream.bin")
	err := g.WriteFileAtomicStreaming(ctx, p, func(w io.Writer) error {
		_, err := w.Write([]byte("streamed"))
		return err
	})
	if err != nil {
		t.Fatalf("stream write: %v", err)
	}
	got, _ := g.ReadFile(ctx, p, 0)
	if string(got) != "streamed" {
		t.Fatalf("got %q", got)
	}
}

func TestGatewayDirOps(t *testing.T) {
	g := New()
	ctx := context.Background()
	root := t.TempDir()
	if err := g.Mkdir(ctx, filepath.Join(root, "d1")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := g.EnsureDir(ctx, filepath.Join(root, "d1", "d2")); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := g.CreateFile(ctx, filepath.Join(root, "d1", "d2", "f.txt")); err != nil {
		t.Fatalf("create file: %v", err)
	}
	// Exists/Stat/ListDir。
	ok, err := g.Exists(ctx, filepath.Join(root, "d1", "d2", "f.txt"))
	if err != nil || !ok {
		t.Fatalf("exists: %v %v", ok, err)
	}
	snap, err := g.Stat(ctx, filepath.Join(root, "d1"))
	if err != nil || snap.EntryType != filemansvc.TypeDirectory || snap.Size != 0 {
		t.Fatalf("stat dir: %v %+v", err, snap)
	}
	entries, err := g.ListDir(ctx, filepath.Join(root, "d1"))
	if err != nil || len(entries) != 1 || entries[0].Name != "d2" {
		t.Fatalf("listdir: %v %+v", err, entries)
	}
	// SizeOf 子树累计。
	size, err := g.SizeOf(ctx, filepath.Join(root, "d1"))
	if err != nil || size != 0 { // 空文件累计 0
		t.Fatalf("size of: %v %d", err, size)
	}
	// Rename + CopyThenRemove。
	if err := g.Rename(ctx, filepath.Join(root, "d1", "d2", "f.txt"), filepath.Join(root, "d1", "g.txt")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := g.CopyThenRemove(ctx, filepath.Join(root, "d1", "g.txt"), filepath.Join(root, "h.txt")); err != nil {
		t.Fatalf("copy+remove: %v", err)
	}
	ok, _ = g.Exists(ctx, filepath.Join(root, "d1", "g.txt"))
	if ok {
		t.Fatal("source should be removed after copy")
	}
	got, _ := g.ReadFile(ctx, filepath.Join(root, "h.txt"), 0)
	if len(got) != 0 {
		t.Fatalf("copied content mismatch")
	}
	// RemoveTree。
	if err := g.RemoveTree(ctx, filepath.Join(root, "d1")); err != nil {
		t.Fatalf("remove tree: %v", err)
	}
	if ok, _ := g.Exists(ctx, filepath.Join(root, "d1")); ok {
		t.Fatal("tree removed")
	}
	// 不存在路径的哨兵。
	_, err = g.Stat(ctx, filepath.Join(root, "void"))
	if !errors.Is(err, filemansvc.ErrEntryNotExist) {
		t.Fatalf("stat void err = %v", err)
	}
	if _, err := g.OpenReadStream(ctx, filepath.Join(root, "void")); !errors.Is(err, filemansvc.ErrEntryNotExist) {
		t.Fatalf("open void err = %v", err)
	}
}

func TestGatewayChmodMode(t *testing.T) {
	g := New()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "c.txt")
	if err := g.WriteFileAtomic(ctx, p, []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := g.Chmod(ctx, p, "0600"); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != fs.FileMode(0o600) {
			t.Fatalf("perm = %v", fi.Mode().Perm())
		}
	}
	// 非法串（服务层已校验，适配层防御）。
	if err := g.Chmod(ctx, p, "zz"); err == nil {
		t.Fatal("invalid mode rejected at adapter")
	}
}
