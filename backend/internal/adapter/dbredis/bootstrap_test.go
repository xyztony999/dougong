package dbredis

import "testing"

// TestConfigGetValue CONFIG GET 输出解析（--raw field/value 交替行；纯函数）。
func TestConfigGetValue(t *testing.T) {
	cases := []struct {
		out, field, want string
	}{
		{"requirepass\n\n", "requirepass", ""},                   // 未设置（空值）
		{"requirepass\nsecret123\n", "requirepass", "secret123"}, // 已设置
		{"bind\n127.0.0.1 ::1\n", "bind", "127.0.0.1 ::1"},
		{"requirepass\r\nabc\r\n", "requirepass", "abc"}, // CRLF 归一
		{"bind\n127.0.0.1\n", "requirepass", ""},         // 字段不匹配
		{"", "requirepass", ""},
	}
	for _, c := range cases {
		if got := configGetValue(c.out, c.field); got != c.want {
			t.Errorf("configGetValue(%q,%q)=%q want %q", c.out, c.field, got, c.want)
		}
	}
}

// TestIsLoopbackBind bind 本机判定（纯函数；fresh install 默认 127.0.0.1 ::1）。
func TestIsLoopbackBind(t *testing.T) {
	cases := []struct {
		bind string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1 ::1", true},
		{"::1", true},
		{"localhost", true},
		{"0.0.0.0", false},
		{"*", false},
		{"127.0.0.1 0.0.0.0", false},
		{"", false}, // 空 bind=监听全部接口，保守判非本机
		{"192.168.1.1", false},
	}
	for _, c := range cases {
		if got := isLoopbackBind(c.bind); got != c.want {
			t.Errorf("isLoopbackBind(%q)=%v want %v", c.bind, got, c.want)
		}
	}
}
