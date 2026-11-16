package fileman

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 纯函数内部测试（不引 adapter，避免测试包循环依赖——集成测试在外部包
// fileman_test 中经 dot-import 消费导出面）。

// absT 将 POSIX 风格样例路径转为当前平台绝对形态（Windows 补盘符与反斜杠
// ——filepath.IsAbs 平台差异，A-F7 开发机自验口径）。
func absT(p string) string {
	if runtime.GOOS == "windows" {
		return `C:\` + strings.ReplaceAll(strings.TrimPrefix(p, "/"), "/", `\`)
	}
	return p
}

func TestValidateScopeUpdate(t *testing.T) {
	_, err := ValidateScopeUpdate("relative", nil)
	if err == nil {
		t.Fatal("relative root rejected")
	}
	_, err = ValidateScopeUpdate(absT("/srv"), []string{absT("/srv")})
	if err == nil { // 排除项=受管根（全禁无意义）
		t.Fatal("excluded==root rejected")
	}
	_, err = ValidateScopeUpdate(absT("/srv/www"), []string{absT("/srv")})
	if err == nil { // 排除项包含受管根（祖先）
		t.Fatal("ancestor excluded rejected")
	}
	cfg, err := ValidateScopeUpdate(absT("/srv/www"), []string{absT("/srv/www/cache"), absT("/proc")})
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if cfg.RootPath != filepath.Clean(absT("/srv/www")) || len(cfg.ExcludedDirs) != 2 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestGenerateTrashNameFormat(t *testing.T) { // D-15/A-F6
	now := time.Unix(1763126400, 0)
	name, err := generateTrashName(now)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if !regexp.MustCompile(`^1763126400-[0-9a-f]{16}$`).MatchString(name) {
		t.Fatalf("name = %q", name)
	}
	a, _ := generateTrashName(now)
	b, _ := generateTrashName(now)
	if a == b {
		t.Fatal("random part should differ")
	}
}

func TestSortEntriesDeterministic(t *testing.T) {
	items := []EntryItem{
		{Name: "b", Size: 1, Mtime: "2026-01-01T00:00:00Z"},
		{Name: "a", Size: 1, Mtime: "2026-01-02T00:00:00Z"},
		{Name: "c", Size: 3, Mtime: "2025-01-01T00:00:00Z"},
	}
	sortEntries(items, SortFieldSize, OrderDesc)
	if items[0].Name != "c" || items[1].Name != "a" || items[2].Name != "b" {
		t.Fatalf("size desc with name tiebreak = %+v", items)
	}
	sortEntries(items, SortFieldMtime, OrderAsc)
	if items[0].Name != "c" || items[1].Name != "b" || items[2].Name != "a" {
		t.Fatalf("mtime asc = %+v", items)
	}
}

func TestEditableProbe(t *testing.T) { // D-05：前 8KB 窗口
	if editableProbe([]byte("plain text")) != true {
		t.Fatal("plain text editable")
	}
	if editableProbe([]byte{0x00, 0x01, 0x02}) != false {
		t.Fatal("NUL byte binary")
	}
	big := append([]byte("text..."), make([]byte, EditBinaryProbe+10)...)
	if editableProbe(big) != false {
		t.Fatal("NUL beyond window still binary (window covers prefix only)")
	}
	// NUL 在 8KB 窗口之外 → 前缀合法即放行（窗口语义）。
	windowOK := make([]byte, EditBinaryProbe)
	for i := range windowOK {
		windowOK[i] = 'a'
	}
	if editableProbe(append(windowOK, 0x00)) != true {
		t.Fatal("binary byte beyond probe window accepted (window semantics)")
	}
}

func TestValidModeString(t *testing.T) {
	for _, ok := range []string{"0644", "644", "000", "0777", "777"} {
		if !validModeString(ok) {
			t.Fatalf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"0999", "1777", "abc", "6444", "", "08"} {
		if validModeString(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}
