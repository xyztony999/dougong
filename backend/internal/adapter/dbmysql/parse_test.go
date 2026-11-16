package dbmysql

import (
	"strings"
	"testing"
)

func TestQuoteStringEscapes(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "'plain'"},
		{"o'brien", `'o\'brien'`},
		{`a\b`, `'a\\b'`},
		{`he said "hi"`, `'he said \"hi\"'`},
		{"line\nbreak", `'line\nbreak'`},
		{"cr\r", `'cr\r'`},
		{"nul\x00x", `'nul\0x'`},
		{"eof\x1a", `'eof\Z'`},
	}
	for _, c := range cases {
		if got := quoteString(c.in); got != c.want {
			t.Errorf("quoteString(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestQuoteIdentGuardsBacktick(t *testing.T) {
	if got := quoteIdent("mydb"); got != "`mydb`" {
		t.Fatalf("quoteIdent = %s", got)
	}
	// 白名单外的反引号被双写转义（双保险，正常路径不可达）。
	if got := quoteIdent("a`b"); got != "`a``b`" {
		t.Fatalf("quoteIdent backtick escape = %s", got)
	}
}

func TestSplitAndUnescapeTSV(t *testing.T) {
	out := "row1col1\trow1col2\n\nrow2col1\trow2\tcol\n"
	lines := splitTSV(out)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (%v)", len(lines), lines)
	}
	if got := unescapeTSV(`a\tb\nc\\d\0e`); got != "a\tb\nc\\d\x00e" {
		t.Fatalf("unescape = %q", got)
	}
	if got := unescapeTSV("plain"); got != "plain" {
		t.Fatalf("plain passthrough = %q", got)
	}
	if got := unescapeTSV(`lit\x`); got != `lit\x` {
		t.Fatalf("non-escape kept = %q", got)
	}
}

func TestTSVStringPtrNULL(t *testing.T) {
	if p := tsvStringPtr("NULL"); p != nil {
		t.Fatalf("NULL → nil, got %v", *p)
	}
	p := tsvStringPtr("val")
	if p == nil || *p != "val" {
		t.Fatalf("val passthrough failed: %v", p)
	}
}

func TestParseGrantLine(t *testing.T) {
	cases := []struct {
		raw, db string
	}{
		{"GRANT SELECT, INSERT ON `mydb`.* TO 'u'@'localhost'", "mydb"},
		{"GRANT ALL PRIVILEGES ON *.* TO 'root'@'localhost' WITH GRANT OPTION", "*"},
		{"GRANT SELECT ON `somedb`.* TO 'a'@'%'", "somedb"},
		{"malformed line without ON clause", ""},
	}
	for _, c := range cases {
		got := parseGrantLine(c.raw)
		if got.DB != c.db {
			t.Errorf("parseGrantLine(%q).DB = %q, want %q", c.raw, got.DB, c.db)
		}
		if got.Raw != c.raw {
			t.Errorf("raw not preserved: %q", got.Raw)
		}
	}
}

func TestGrantPrivilegesOf(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"GRANT SELECT ON `d`.* TO 'u'@'h'", "SELECT"},
		{"GRANT SELECT, INSERT, UPDATE, DELETE ON `d`.* TO 'u'@'h'", "SELECT, INSERT, UPDATE, DELETE"},
		{"GRANT ALL PRIVILEGES ON *.* TO 'u'@'h'", "ALL PRIVILEGES"},
		{"NOT A GRANT", ""},
	}
	for _, c := range cases {
		if got := GrantPrivilegesOf(c.raw); got != c.want {
			t.Errorf("GrantPrivilegesOf(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestClassifyRunAuthVsConn(t *testing.T) {
	err := classifyRun(errFakeExit(), "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)", "")
	if !isSentinel(err, ErrAuthRejected) {
		t.Fatalf("access denied should classify auth rejected, got %v", err)
	}
	err = classifyRun(errFakeExit(), "ERROR 2002 (HY000): Can't connect to local MySQL server through socket '/tmp/x.sock'", "")
	if !isSentinel(err, ErrConnFailed) {
		t.Fatalf("cant connect should classify conn failed, got %v", err)
	}
	err = classifyRun(errFakeExit(), "ERROR 1064 syntax", "ignored")
	if !isSentinel(err, ErrQueryFailed) {
		t.Fatalf("other should classify query failed, got %v", err)
	}
}

func TestClassifyDDL(t *testing.T) {
	base := errFakeExit()
	qerr := classifyRun(base, "ERROR 1007 (HY000): Can't create database 'x'; database exists", "")
	if got := classifyDDL(qerr, "database exists", ErrDatabaseExists); !isSentinel(got, ErrDatabaseExists) {
		t.Fatalf("ddl exists mismatch: %v", got)
	}
	plain := classifyRun(base, "ERROR 1064", "")
	if got := classifyDDL(plain, "database exists", ErrDatabaseExists); got != plain {
		t.Fatalf("no marker should passthrough: %v", got)
	}
}

type exitFake struct{}

func (exitFake) Error() string { return "exit status 1" }
func errFakeExit() error       { return exitFake{} }
func isSentinel(err, want error) bool {
	return err != nil && strings.Contains(err.Error(), want.Error())
}
