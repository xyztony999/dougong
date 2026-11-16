package cronx

import (
	"errors"
	"strings"
	"testing"
)

func TestSplitJobLine(t *testing.T) {
	cases := []struct {
		line          string
		expr, command string
		ok            bool
	}{
		{"* * * * * /usr/bin/echo hi", "* * * * *", "/usr/bin/echo hi", true},
		{"  0\t2 * * *  /scripts/job.sh  ", "0 2 * * *", "/scripts/job.sh", true}, // 制表符/多空段
		{"@daily /usr/local/bin/backup", "@daily", "/usr/local/bin/backup", true},
		{"# comment", "", "", false},
		{"", "", "", false},
		{"SHELL=/bin/bash", "", "", false},                               // 环境变量行
		{"PATH=/usr/bin:/bin", "", "", false},                            // 环境变量行
		{"* * * * *", "", "", false},                                     // 不足 6 段
		{"* * * * * cmd arg1  arg2", "* * * * *", "cmd arg1 arg2", true}, // 命令内多空段压缩
	}
	for _, tc := range cases {
		expr, command, ok := SplitJobLine(tc.line)
		if ok != tc.ok || expr != tc.expr || command != tc.command {
			t.Errorf("SplitJobLine(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.line, expr, command, ok, tc.expr, tc.command, tc.ok)
		}
	}
}

func TestMakeJobKey(t *testing.T) {
	// 多空段压缩：同一任务面板外重排空白仍同 key（D-01 内容寻址稳定）。
	a := MakeJobKey("0  2 * * *", "/scripts/job.sh --fast")
	b := MakeJobKey("0 2 * * *", "/scripts/job.sh   --fast")
	if a != b {
		t.Fatalf("jobKey not stable under whitespace compression: %q vs %q", a, b)
	}
	if !strings.Contains(a, "\t") {
		t.Fatalf("jobKey missing TAB separator: %q", a)
	}
	if a == MakeJobKey("0 3 * * *", "/scripts/job.sh --fast") {
		t.Fatal("different expr must yield different key")
	}
}

func TestParseTabLinesMarkers(t *testing.T) {
	remark := "每日备份"
	lines := []string{
		"# 普通注释",
		"",
		MarkerManaged,
		MarkerRemarkPrefix + " " + remark,
		"0 2 * * * /scripts/backup.sh",
		"*/5 * * * * /scripts/poll.sh",
	}
	entries := ParseTabLines(lines)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	first := entries[0]
	if first.Source != SourcePanel {
		t.Errorf("first source = %s, want panel", first.Source)
	}
	if first.Remark == nil || *first.Remark != remark {
		t.Errorf("first remark = %v, want %q", first.Remark, remark)
	}
	if first.Expr != "0 2 * * *" || first.Command != "/scripts/backup.sh" {
		t.Errorf("first expr/command = %q/%q", first.Expr, first.Command)
	}
	second := entries[1]
	if second.Source != SourceExisting || second.Remark != nil {
		t.Errorf("second = %+v, want existing/no remark", second)
	}
	// 仅 managed 无 remark（合法形态，D-02「无则仅 managed 行」）。
	entries = ParseTabLines([]string{MarkerManaged, "@hourly /h.sh"})
	if len(entries) != 1 || entries[0].Source != SourcePanel || entries[0].Remark != nil {
		t.Fatalf("managed-only marker parse wrong: %+v", entries)
	}
	// 标记注释与任务行之间被插入其他行 → 判定断裂=existing（邻接性要求）。
	entries = ParseTabLines([]string{MarkerManaged, "# other", "0 2 * * * /x.sh"})
	if len(entries) != 1 || entries[0].Source != SourceExisting {
		t.Fatalf("adjacency broken: %+v", entries)
	}
}

func TestRenderTabJobCreateAndUpdate(t *testing.T) {
	// create：追加 managed+remark+任务行。
	lines := []string{"0 1 * * * /old.sh"}
	remark := "面板任务"
	out, key, err := RenderTabJob(lines, "", JobInput{
		Expr: "0 2 * * *", Command: "/new.sh   --flag", Remark: &remark})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out) != 5 { // 旧行 + 空行 + managed + remark + 新行
		t.Fatalf("out = %v", out)
	}
	if out[len(out)-1] != "0 2 * * * /new.sh --flag" {
		t.Fatalf("job line = %q", out[len(out)-1])
	}
	if key != MakeJobKey("0 2 * * *", "/new.sh --flag") {
		t.Fatalf("key = %q", key)
	}
	// update：删旧写新（旧任务行与旧标记注释一并移除；新任务行带新标记）。
	old := []string{MarkerManaged, MarkerRemarkPrefix + " 旧备注", "0 2 * * * /old.sh", "0 3 * * * /keep.sh"}
	oldKey := MakeJobKey("0 2 * * *", "/old.sh")
	out2, newKey, err := RenderTabJob(old, oldKey, JobInput{Expr: "0 4 * * *", Command: "/newer.sh"})
	if err != nil {
		t.Fatalf("render update: %v", err)
	}
	joined := strings.Join(out2, "\n")
	if strings.Contains(joined, "/old.sh") || strings.Contains(joined, "旧备注") {
		t.Fatalf("old job + old markers not removed: %v", out2)
	}
	if !strings.Contains(joined, "/keep.sh") {
		t.Fatalf("unrelated job lost: %v", out2)
	}
	if !strings.Contains(joined, MarkerManaged) {
		t.Fatalf("new panel marker missing: %v", out2)
	}
	if newKey != MakeJobKey("0 4 * * *", "/newer.sh") {
		t.Fatalf("newKey = %q", newKey)
	}
	// update 不存在 → ErrJobNotFound（面板外已删，CB_003）。
	_, _, err = RenderTabJob(old, MakeJobKey("9 9 * * *", "/ghost.sh"), JobInput{Expr: "0 0 * * *", Command: "/x"})
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestRemoveTabJob(t *testing.T) {
	lines := []string{
		MarkerManaged,
		MarkerRemarkPrefix + " r",
		"0 2 * * * /a.sh",
		"0 3 * * * /b.sh",
	}
	out, err := RemoveTabJob(lines, MakeJobKey("0 2 * * *", "/a.sh"))
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(out) != 1 || out[0] != "0 3 * * * /b.sh" {
		t.Fatalf("out = %v", out)
	}
	if _, err := RemoveTabJob(out, MakeJobKey("0 2 * * *", "/a.sh")); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("second remove err = %v, want ErrJobNotFound", err)
	}
}

func TestParseCronDFile(t *testing.T) {
	content := "# digong jobs\nSHELL=/bin/sh\n0 5 * * * root /usr/bin/daily.sh\n15 3 * * 0 root /usr/bin/weekly.sh arg\n"
	entries := ParseCronDFile(content)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (注释/环境行跳过)", len(entries))
	}
	if entries[0].Expr != "0 5 * * *" || entries[0].Command != "root /usr/bin/daily.sh" {
		t.Errorf("entry0 = %+v（用户域随命令如实呈现）", entries[0])
	}
}

func TestTruncateTail(t *testing.T) {
	if got := truncateTail("short", OutputMaxBytes); got != "short" {
		t.Fatalf("short passthrough broken: %q", got)
	}
	// 超限：保留尾部 + 头部省略标记。
	long := strings.Repeat("x", OutputMaxBytes+100)
	got := truncateTail(long, OutputMaxBytes)
	if !strings.HasPrefix(got, OutputTruncationMark) {
		t.Fatalf("missing head-omitted mark")
	}
	if len(got) != len(OutputTruncationMark)+OutputMaxBytes {
		t.Fatalf("tail size = %d", len(got)-len(OutputTruncationMark))
	}
	// UTF-8 边界：多字节字符不劈开（尾部为中文重复串）。
	cjk := strings.Repeat("斗", OutputMaxBytes/3+50)
	got = truncateTail(cjk, OutputMaxBytes)
	for _, r := range got[len(OutputTruncationMark):] {
		if r == 0xFFFD {
			t.Fatal("multi-byte rune split")
		}
	}
}

func TestExtractRemark(t *testing.T) {
	if r, ok := ExtractRemark("# dugong: remark: 每日 02:00 备份"); !ok || r != "每日 02:00 备份" {
		t.Fatalf("extract = (%q,%v)", r, ok)
	}
	if _, ok := ExtractRemark("# some comment"); ok {
		t.Fatal("non-remark comment misidentified")
	}
	if !IsManagedLine("  # dugong: managed  ") {
		t.Fatal("managed line with whitespace not recognized")
	}
}
