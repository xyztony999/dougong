package cronback

import (
	"strings"
	"testing"
	"time"
)

// 测试固定时区：cron 语义为本地时区，向量统一用 UTC 避免环境差异。
func at(t *testing.T, s string) time.Time {
	t.Helper()
	tv, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return tv.UTC()
}

func TestParseCronExprValid(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 0 * * *",
		"30 2 * * 1",
		"0 12 1 * *",
		"*/15 * * * *",
		"0 0 1 1 *",
		"5,35 8-18/2 * * 1-5",
		"0 0 * * 7",  // 7=周日（vixie 归一）
		"0 0 29 2 *", // 形态合法（无解表达式解析层面通过）
	}
	for _, expr := range valid {
		if _, errs := ParseCronExpr(expr); len(errs) > 0 {
			t.Errorf("ParseCronExpr(%q) errors = %v, want none", expr, errs)
		}
	}
}

func TestParseCronExprInvalid(t *testing.T) {
	cases := []struct {
		expr    string
		wantSeg string // 期望错误中包含的段名片段
	}{
		{"* * * *", ""},
		{"* * * * * *", ""},
		{"60 * * * *", "分钟"},
		{"* 24 * * *", "小时"},
		{"* * 0 * *", "日"},
		{"* * * 13 *", "月"},
		{"* * * * 8", "周"},
		{"*/0 * * * *", "步进"},
		{"a * * * *", "非数字"},
		{"5-1 * * * *", "区间"},
	}
	for _, tc := range cases {
		_, errs := ParseCronExpr(tc.expr)
		if len(errs) == 0 {
			t.Errorf("ParseCronExpr(%q) errors = none, want some", tc.expr)
			continue
		}
		if tc.wantSeg != "" && !strings.Contains(errs[0], tc.wantSeg) {
			t.Errorf("ParseCronExpr(%q) first error %q missing %q", tc.expr, errs[0], tc.wantSeg)
		}
	}
}

func TestNormalizeCronExprMacros(t *testing.T) {
	cases := map[string]string{
		"@yearly":     "0 0 1 1 *",
		"@monthly":    "0 0 1 * *",
		"@weekly":     "0 0 * * 0",
		"@daily":      "0 0 * * *",
		"@hourly":     "0 * * * *",
		"@DAILY":      "0 0 * * *",   // 大小写不敏感
		"*/5 * * * *": "*/5 * * * *", // 非宏原样（含两端空白裁剪）
	}
	for in, want := range cases {
		if got := NormalizeCronExpr(in); got != want {
			t.Errorf("NormalizeCronExpr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCronExprNextVectors(t *testing.T) {
	// 2026-11-15 是周日（程序推导断言，防硬编码漂移）。
	base := at(t, "2026-11-15T08:00:00Z")
	if base.Weekday() != time.Sunday {
		t.Fatalf("vector premise broken: 2026-11-15 is %s, want Sunday", base.Weekday())
	}
	nextSunday := base.AddDate(0, 0, 7).Truncate(24 * time.Hour)
	nextMonday := base.AddDate(0, 0, 1)
	cases := []struct {
		name string
		expr string
		from string
		want time.Time
	}{
		{"每分钟", "* * * * *", "2026-11-15T08:00:30Z", at(t, "2026-11-15T08:01:00Z")},
		{"每小时整点", "0 * * * *", "2026-11-15T08:00:00Z", at(t, "2026-11-15T09:00:00Z")},
		{"每日 3:30", "30 3 * * *", "2026-11-15T03:30:00Z", at(t, "2026-11-16T03:30:00Z")},
		{"步进 15 分钟", "*/15 * * * *", "2026-11-15T08:01:00Z", at(t, "2026-11-15T08:15:00Z")},
		{"区间+列表", "5,35 8-18/2 * * 1-5", "2026-11-15T10:00:00Z", nextMonday.Truncate(24 * time.Hour).Add(8*time.Hour + 5*time.Minute)}, // 周日→周一 08:05
		{"月日组合", "0 0 1 1 *", "2026-11-15T00:00:00Z", at(t, "2027-01-01T00:00:00Z")},
		{"周日=7 归一", "0 0 * * 7", "2026-11-15T08:00:00Z", nextSunday}, // 7=0 周日，从周日 08:00 顺推下个周日 00:00
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, errs := ParseCronExpr(tc.expr)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			got := e.Next(at(t, tc.from))
			if !got.Equal(tc.want) {
				t.Fatalf("Next(%s) = %s, want %s", tc.from, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestCronExprDomDowORSemantics(t *testing.T) {
	// vixie：dom 与 dow 均受限时 OR——「13 日 或 周五」任一命中。
	// 前提：2026-11-13 是周五（程序推导断言）。
	fri := at(t, "2026-11-13T00:00:00Z")
	if fri.Weekday() != time.Friday {
		t.Fatalf("vector premise broken: 2026-11-13 is %s, want Friday", fri.Weekday())
	}
	e, errs := ParseCronExpr("0 0 13 * 5")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	// 纯 dow 命中日期：2026-12-04 周五非 13 日——OR 语义必须命中（AND 语义不会）。
	e2 := mustParse(t, "0 0 13 * 5")
	if n := e2.Next(at(t, "2026-12-03T00:00:00Z")); n.Day() != 4 || n.Weekday() != time.Friday {
		t.Fatalf("pure-dow OR hit failed: next = %s", n.Format(time.RFC3339))
	}
	// 纯 dom 命中日期：2026-11-13 恰为 13 日（周五）——仅 dom 受限表达式命中。
	e3 := mustParse(t, "0 0 13 * *")
	if n := e3.Next(at(t, "2026-11-12T00:00:00Z")); !n.Equal(fri) {
		t.Fatalf("dom-only next = %s, want 2026-11-13", n.Format(time.RFC3339))
	}
	_ = e
}

func mustParse(t *testing.T, expr string) *CronExpr {
	t.Helper()
	e, errs := ParseCronExpr(expr)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", expr, errs)
	}
	return e
}

func TestCronExprUnsatisfiable(t *testing.T) {
	e, errs := ParseCronExpr("0 0 31 2 *") // 2 月 31 日不存在
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if next := e.Next(at(t, "2026-01-01T00:00:00Z")); !next.IsZero() {
		t.Fatalf("unsatisfiable expr should be zero, got %s", next.Format(time.RFC3339))
	}
}

func TestPreviewFireStrs(t *testing.T) {
	got := PreviewFireStrs("@daily", at(t, "2026-11-15T09:00:00Z"), PreviewCount)
	if len(got) != PreviewCount {
		t.Fatalf("len = %d, want %d", len(got), PreviewCount)
	}
	wantFirst := "2026-11-16T00:00:00Z"
	if got[0] != wantFirst {
		t.Fatalf("first = %s, want %s", got[0], wantFirst)
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("not strictly increasing at %d: %s <= %s", i, got[i], got[i-1])
		}
	}
}

func TestNormalizeLogLines(t *testing.T) {
	cases := map[string]int{
		"":       LogLinesDefault,
		"abc":    LogLinesDefault,
		"50":     50,
		"100":    100,
		"200":    200,
		"500":    500,
		"1000":   1000,
		"123":    LogLinesDefault, // 越档归默认
		"999999": LogLinesDefault,
		"-5":     LogLinesDefault, // '5' 后 '-' 在前即非数字 → 默认
	}
	for in, want := range cases {
		if got := NormalizeLogLines(in); got != want {
			t.Errorf("NormalizeLogLines(%q) = %d, want %d", in, got, want)
		}
	}
}
