package firewallx

import (
	"reflect"
	"testing"
)

// parseUFWStatus 样本（LANG=C `ufw status` 实际表格形态；A-W2~A-W6 投影口径）。
const ufwStatusSample = `Status: active

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    Anywhere
80                         LIMIT IN    Anywhere
6000:6007/udp              DENY IN     10.0.0.0/8
80,443/tcp                 ALLOW IN    10.0.0.0/8
22/tcp (v6)                ALLOW IN    Anywhere (v6)
8080:8090/tcp              REJECT IN   Anywhere
9000/tcp                   ALLOW OUT   Anywhere
80/tcp on eth0             ALLOW IN    Anywhere
Anywhere                   ALLOW IN    10.0.0.0/8
`

func TestParseUFWStatus(t *testing.T) {
	rules := parseUFWStatus(ufwStatusSample)
	type want struct {
		ruleKey, port, protocol, action string
		source                          *string
	}
	src := "10.0.0.0/8"
	wantRules := []want{
		{"allow 22/tcp", "22", "tcp", "allow", nil},                              // 基本行
		{"limit 80", "80", "tcp", "limit", nil},                                  // 裸端口双栈展开 A-W3（tcp）
		{"limit 80", "80", "udp", "limit", nil},                                  // （udp）
		{"deny 6000:6007/udp from 10.0.0.0/8", "6000-6007", "udp", "deny", &src}, // 冒号段→短横线 A-W2 + 来源
		{"allow 80,443/tcp from 10.0.0.0/8", "80,443", "tcp", "allow", &src},     // 面板外逗号多端口如实投影 A-W6
		{"reject 8080:8090/tcp", "8080-8090", "tcp", "reject", nil},              // 拒绝族+段转换
		// v6 复制行去重（A-W4/A-W5）、OUT 方向/接口绑定/非端口级不投影。
	}
	if len(rules) != len(wantRules) {
		t.Fatalf("rules len = %d, want %d: %+v", len(rules), len(wantRules), rules)
	}
	for i, w := range wantRules {
		got := rules[i]
		if got.RuleKey != w.ruleKey || got.Port != w.port || got.Protocol != w.protocol ||
			got.Action != w.action || !reflect.DeepEqual(got.Source, w.source) {
			t.Fatalf("rule[%d] = %+v, want %+v", i, got, w)
		}
		if got.RawSpec == "" {
			t.Fatalf("rule[%d].RawSpec 为空（后端原文语义，§4.1.1）", i)
		}
	}
}

func TestParseUFWStatusEmptyAndNoise(t *testing.T) {
	for _, in := range []string{"", "Status: inactive\n", "Status: active\n\nTo\tAction\tFrom\n--\t------\t----\n"} {
		if rules := parseUFWStatus(in); len(rules) != 0 {
			t.Fatalf("parse(%q) = %d rules, want 0", in, len(rules))
		}
	}
}

func TestUFWAddArgs(t *testing.T) {
	tests := []struct {
		name string
		spec RuleSpec
		want []string
	}{
		{"plain", RuleSpec{Port: "22", Protocol: "tcp", Action: "allow"},
			[]string{"allow", "22/tcp"}},
		{"range colon form", RuleSpec{Port: "8080-8090", Protocol: "udp", Action: "deny"},
			[]string{"deny", "8080:8090/udp"}}, // 契约短横线→ufw 原生冒号 A-W2
		{"with source", RuleSpec{Port: "8080-8090", Protocol: "tcp", Action: "deny", Source: "10.0.0.0/8"},
			[]string{"deny", "8080:8090/tcp", "from", "10.0.0.0/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ufwAddArgs(tt.spec); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ufwAddArgs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUFWDeleteArgs(t *testing.T) {
	got := ufwDeleteArgs("deny 8080:8090/tcp from 10.0.0.0/8")
	want := []string{"delete", "deny", "8080:8090/tcp", "from", "10.0.0.0/8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ufwDeleteArgs = %v, want %v", got, want)
	}
}

func TestParsePortTokens(t *testing.T) {
	rules := parsePortTokens("80/tcp 53/udp 8080-8090/tcp\nbogus 90/nope")
	if len(rules) != 3 {
		t.Fatalf("rules len = %d, want 3: %+v", len(rules), rules)
	}
	for i, w := range []struct{ key, port, proto string }{
		{"80/tcp", "80", "tcp"},
		{"53/udp", "53", "udp"},
		{"8080-8090/tcp", "8080-8090", "tcp"},
	} {
		r := rules[i]
		if r.RuleKey != w.key || r.Port != w.port || r.Protocol != w.proto ||
			r.Action != "allow" || r.Source != nil || r.RawSpec != w.key {
			t.Fatalf("rule[%d] = %+v, want key=%s port=%s proto=%s action=allow source=nil", i, r, w.key, w.port, w.proto)
		}
	}
}

func TestParseRichRules(t *testing.T) {
	in := "rule family=\"ipv4\" source address=\"10.0.0.0/8\" port port=\"80\" protocol=\"tcp\" accept\n" +
		"rule family=\"ipv6\" source address=\"2001:db8::/32\" port port=\"8080-8090\" protocol=\"udp\" drop\n" +
		"rule family=\"ipv4\" port port=\"80\" protocol=\"tcp\" reject\n" +
		"rule family=\"ipv4\" port port=\"80\" protocol=\"tcp\" mark set 1\n" +
		"\n"
	rules := parseRichRules(in)
	if len(rules) != 3 {
		t.Fatalf("rules len = %d, want 3（mark 不可映射跳过 A-W4）: %+v", len(rules), rules)
	}
	src4, src6 := "10.0.0.0/8", "2001:db8::/32"
	type want struct {
		action, port, proto string
		source              *string
	}
	wants := []want{
		{"allow", "80", "tcp", &src4},
		{"deny", "8080-8090", "udp", &src6},
		{"reject", "80", "tcp", nil},
	}
	for i, w := range wants {
		r := rules[i]
		if r.Action != w.action || r.Port != w.port || r.Protocol != w.proto || !reflect.DeepEqual(r.Source, w.source) {
			t.Fatalf("rich[%d] = %+v, want %+v", i, r, w)
		}
		if r.RuleKey == "" || r.RuleKey != r.RawSpec {
			t.Fatalf("rich[%d].RuleKey 须为原文（--remove-rich-rule 原生入参，A-W7）: %+v", i, r)
		}
	}
}

func TestBuildRichRule(t *testing.T) {
	v4 := buildRichRule(RuleSpec{Port: "8080-8090", Protocol: "tcp", Source: "10.0.0.0/8"})
	wantV4 := `rule family="ipv4" source address="10.0.0.0/8" port port="8080-8090" protocol="tcp" accept`
	if v4 != wantV4 {
		t.Fatalf("buildRichRule v4 = %q, want %q", v4, wantV4)
	}
	v6 := buildRichRule(RuleSpec{Port: "80", Protocol: "udp", Source: "2001:db8::/32"})
	wantV6 := `rule family="ipv6" source address="2001:db8::/32" port port="80" protocol="udp" accept`
	if v6 != wantV6 {
		t.Fatalf("buildRichRule v6 = %q, want %q", v6, wantV6)
	}
	// round-trip：构造串可被解析回同语义（A-W7 寻址闭环）。
	parsed, ok := parseRichRule(v4)
	if !ok || parsed.RuleKey != v4 || parsed.Action != "allow" || parsed.Source == nil || *parsed.Source != "10.0.0.0/8" {
		t.Fatalf("rich round-trip 失败: %+v ok=%v", parsed, ok)
	}
}

func TestQuotedValue(t *testing.T) {
	line := `rule family="ipv4" source address="10.0.0.0/8" port port="80" protocol="tcp" accept`
	for key, want := range map[string]string{
		"family=":   "ipv4",
		"address=":  "10.0.0.0/8",
		"port=":     "80", // `port port="80"` 子串唯一命中（protocol= 不含 port=）
		"protocol=": "tcp",
		"missing=":  "",
	} {
		if got := quotedValue(line, key); got != want {
			t.Fatalf("quotedValue(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestSupportedActionsConstants(t *testing.T) {
	if BackendUFW != "ufw" || BackendFirewalld != "firewalld" || BackendNone != "none" {
		t.Fatalf("firewall_backend 枚举取值漂移（04-business-enum 定稿）")
	}
}
