package nginxx

import (
	"strings"
	"testing"
)

// round-trip 样本集（M3 验收硬项 ≥20：解析→序列化 byte 级保真，R-07/I-09；
// span 替换模型 A-SM2——未编辑字节恒不变由构造保证，样本实证）。
func TestRoundTripFidelity(t *testing.T) {
	samples := []string{
		0:  "server_name a.example.com;\n",
		1:  "server {\n    listen 80;\n}\n",
		2:  "server {\n    listen 443 ssl;\n    server_name a.com b.com;\n    root /var/www/html;\n}\n",
		3:  "# 全行注释\nserver {\n    # 块内注释\n    listen 80; # 行尾注释\n}\n",
		4:  "server {\n    location / {\n        proxy_pass http://127.0.0.1:8080;\n    }\n}\n",
		5:  "server {\n    location ~ \\.php$ {\n        fastcgi_pass unix:/run/php/php8.2-fpm.sock;\n    }\n}\n",
		6:  "server {\n    location ~* \\.(jpg|png|gif)$ {\n        expires 30d;\n    }\n}\n",
		7:  "server {\n    if ($request_method = POST) {\n        return 405;\n    }\n}\n",
		8:  "server {\n    location / {\n        try_files $uri $uri/ /index.php?$query_string;\n    }\n}\n",
		9:  "upstream backend {\n    server 10.0.0.1:8080 weight=3;\n    server 10.0.0.2:8080;\n    keepalive 16;\n}\n",
		10: "map $http_upgrade $connection_upgrade {\n    default upgrade;\n    ''      close;\n}\n",
		11: "server {\n    ssl_certificate \"/etc/ssl/certs with space/full.pem\";\n    ssl_certificate_key /etc/ssl/key.pem;\n}\n",
		12: "server {\n    server_name \"quoted.example.com\";\n}\n",
		13: "include snippets/ssl-params.conf;\ninclude /etc/nginx/conf.d/*.conf;\n",
		14: "server{listen 80;server_name a.com;}\n",
		15: "\tserver {\n\t\tlisten 80;\n\t}\n",
		16: "server {\n    listen [::]:80 default_server;\n    listen 80;\n}\n",
		17: "# server {\n#     listen 80;\n#     server_name dormant.example.com;\n# }\nserver {\n    listen 8080;\n}\n",
		18: "server {\n    add_header X-Frame-Options \"SAMEORIGIN\" always;\n    add_header Content-Security-Policy \"default-src 'self'; script-src 'self' 'unsafe-inline'\";\n}\n",
		19: "user www-data;\nworker_processes auto;\nevents {\n    worker_connections 1024;\n}\n",
		20: "server {\n    proxy_set_header Upgrade $http_upgrade;\n    proxy_set_header Connection \"upgrade\";\n    proxy_set_header Host $host;\n}\n",
		21: "server {\r\n    listen 80;\r\n}\r\n", // CRLF
		22: "server {\n    access_log /var/log/nginx/a.access.log combined buffer=32k;\n    error_log /var/log/nginx/a.error.log warn;\n}\n",
		23: "server {\n    location ^~ /.well-known/acme-challenge/ {\n        root /var/www/certbot;\n    }\n}\n",
		24: "server {\n    listen 80;\n    listen 443 ssl http2;\n    server_name multi.example.com;\n    return 301 https://$host$request_uri;\n}\n",
		25: "server {\n    charset utf-8;\n    # 中文注释与宽字符内容\n    server_name unicode.example.com;\n}\n",
		26: "",     // 空文件
		27: ";;\n", // 游离分号：Raw 保全
	}
	for i, src := range samples {
		tree := Parse(src)
		if got := tree.Serialize0(); got != src {
			t.Errorf("sample %d round-trip mismatch:\n got=%q\nwant=%q", i, got, src)
		}
	}
}

// Serialize0 序列化（无编辑）——测试命名别名（避免与编辑用例混淆语义）。
func (t *Tree) Serialize0() string {
	out, err := t.Serialize()
	if err != nil {
		return "SERIALIZE-ERROR: " + err.Error()
	}
	return out
}

// Raw 保全：残段按原文本写回（R-07）。
func TestRawPreservation(t *testing.T) {
	src := "server {\n    listen 80;\n}\n;;garbage\n"
	tree := Parse(src)
	if got := tree.Serialize0(); got != src {
		t.Fatalf("raw round-trip: got=%q want=%q", got, src)
	}
	var raws []*Node
	for _, n := range tree.Nodes {
		if n.Kind == KindRaw {
			raws = append(raws, n)
		}
	}
	if len(raws) == 0 {
		t.Fatal("expected raw nodes for stray tokens")
	}
}

// ValidateSyntax 语法自检（A-SM8）：括号不配平/坏引号/残段报 issue；正常样本零 issue。
func TestValidateSyntax(t *testing.T) {
	bad := []string{
		"server {\n    listen 80;\n",    // 未闭合
		"server_name \"unterminated;\n", // 坏引号
		"}\n",                           // 多余闭括号
		"listen 80;;;\n",                // 游离分号
	}
	for i, src := range bad {
		if issues := ValidateSyntax(src); len(issues) == 0 {
			t.Errorf("bad sample %d: expected issues, got none", i)
		}
	}
	if issues := ValidateSyntax("server {\n    listen 80;\n    server_name a.com;\n}\n"); len(issues) != 0 {
		t.Fatalf("good sample issues: %v", issues)
	}
}

// ExtractServers：active/dormant 混合与序号稳定性（A-SM3：序号含 dormant）。
func TestExtractServersDormant(t *testing.T) {
	src := "# server {\n#     listen 80;\n#     server_name dormant.com;\n# }\n" +
		"server {\n    listen 8080;\n    server_name active.com;\n}\n" +
		"# server {\n#     listen 9090;\n#     server_name dormant2.com;\n# }\n"
	servers := ExtractServers(Parse(src))
	if len(servers) != 3 {
		t.Fatalf("servers = %d, want 3 (dormant counted)", len(servers))
	}
	if servers[0].Dormant != true || servers[1].Dormant != false || servers[2].Dormant != true {
		t.Fatalf("dormant flags = %v %v %v", servers[0].Dormant, servers[1].Dormant, servers[2].Dormant)
	}
	if servers[0].ServerNames[0] != "dormant.com" || servers[1].ServerNames[0] != "active.com" {
		t.Fatalf("serverNames mismatch: %v / %v", servers[0].ServerNames, servers[1].ServerNames)
	}
	if servers[2].Listen[0] != "9090" {
		t.Fatalf("dormant listen = %v", servers[2].Listen)
	}
}

// 非整体注释的注释组不判 dormant（A-SM3 如实投影）。
func TestDormantRejectsMixedCommentGroup(t *testing.T) {
	src := "# some note\n# server {\n#     listen 80;\n# }\n"
	servers := ExtractServers(Parse(src))
	if len(servers) != 0 {
		t.Fatalf("mixed comment group should not be dormant, got %d", len(servers))
	}
}

// CommentOut ↔ StripCommentPrefix 往返（D-16 启停 byte 级还原基础）。
func TestCommentRoundTrip(t *testing.T) {
	src := "server {\n    listen 80;\n    server_name a.com;\n}\n"
	commented := CommentOut(src)
	if !strings.Contains(commented, "# server {") || !strings.Contains(commented, "#     listen 80;") {
		t.Fatalf("CommentOut output unexpected:\n%s", commented)
	}
	if back := StripCommentPrefix(commented); back != src {
		t.Fatalf("strip mismatch:\n got=%q\nwant=%q", back, src)
	}
}

// 块内建模/非建模指令投影：rawBlocks 收录非建模指令（A-SM4），建模字段就位。
func TestExtractProjection(t *testing.T) {
	src := "server {\n" +
		"    listen 80;\n" +
		"    server_name a.com;\n" +
		"    root /var/www;\n" +
		"    index index.php;\n" +
		"    ssl_certificate /etc/c.pem;\n" +
		"    ssl_certificate_key /etc/k.pem;\n" +
		"    access_log /log/a.log;\n" +
		"    error_log /log/e.log;\n" +
		"    add_header X-Test 1;\n" +
		"    location /api/ {\n" +
		"        proxy_pass http://127.0.0.1:9;\n" +
		"        proxy_set_header Host $host;\n" +
		"        proxy_set_header Upgrade $http_upgrade;\n" +
		"    }\n" +
		"    location ~ \\.php$ {\n" +
		"        fastcgi_pass unix:/run/x.sock;\n" +
		"    }\n" +
		"}\n"
	servers := ExtractServers(Parse(src))
	if len(servers) != 1 {
		t.Fatalf("servers=%d", len(servers))
	}
	s := servers[0]
	if s.Root != "/var/www" || s.SSLCert != "/etc/c.pem" || s.SSLCertKey != "/etc/k.pem" {
		t.Fatalf("modeled fields: %+v", s)
	}
	if s.AccessLog != "/log/a.log" || s.ErrorLog != "/log/e.log" {
		t.Fatalf("log fields: %+v", s)
	}
	// index/add_header 非建模 → rawBlocks（A-SM4）。
	joined := rawText(s.RawBlocks)
	if !strings.Contains(joined, "index index.php;") || !strings.Contains(joined, "add_header X-Test 1;") {
		t.Fatalf("rawBlocks missing unmodeled directives: %+v", s.RawBlocks)
	}
	if len(s.Locations) != 2 {
		t.Fatalf("locations=%d", len(s.Locations))
	}
	if s.Locations[0].ProxyPass != "http://127.0.0.1:9" || s.Locations[1].FastCGIPass != "unix:/run/x.sock" {
		t.Fatalf("locations content: %+v", s.Locations)
	}
	p := s.Proxy()
	if p == nil || p.Upstream != "http://127.0.0.1:9" || p.HostHeader != "$host" || !p.Websocket {
		t.Fatalf("proxy projection: %+v", p)
	}
}

func rawText(blocks []RawBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		sb.WriteString(b.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

// span 编辑：定点替换与插入（A-SM2 写回机制）。
func TestSpanEdits(t *testing.T) {
	src := "server {\n    listen 80;\n    server_name old.com;\n    root /old;\n}\n"
	tree := Parse(src)
	_ = ExtractServers(tree)
	var server *Node
	for _, n := range tree.Nodes {
		if n.Kind == KindBlock && n.Name == "server" {
			server = n
		}
	}
	if server == nil {
		t.Fatal("server block not found")
	}
	editedStart := -1
	for _, ch := range server.Children {
		if ch.Kind == KindDirective && ch.Name == "server_name" {
			if err := tree.Apply(ch.Start, ch.End, "server_name new.com extra.com;"); err != nil {
				t.Fatal(err)
			}
			editedStart = ch.Start
		}
	}
	out := tree.Serialize0()
	want := "server {\n    listen 80;\n    server_name new.com extra.com;\n    root /old;\n}\n"
	if out != want {
		t.Fatalf("span edit:\n got=%q\nwant=%q", out, want)
	}
	// 重叠编辑拒绝（与已编辑区间相交）。
	if err := tree.Apply(editedStart, editedStart+2, "x"); err == nil {
		t.Fatal("overlapping edit should fail")
	}
}

// dormant 块的区间定位（写回锚点：CommentStart/End 指向文件内注释组）。
func TestDormantSpanAnchors(t *testing.T) {
	prefix := "# header comment\n\n"
	src := prefix + "# server {\n#     listen 80;\n# }\n"
	tree := Parse(src)
	servers := ExtractServers(tree)
	if len(servers) != 1 || !servers[0].Dormant {
		t.Fatalf("servers=%+v", servers)
	}
	cs, ce := servers[0].CommentStart, servers[0].CommentEnd
	if !strings.HasPrefix(src[cs:ce], "# server {") {
		t.Fatalf("comment span = %q", src[cs:ce])
	}
	if cs < len(prefix) { // 不含前置普通注释
		t.Fatalf("comment start %d before prefix %d", cs, len(prefix))
	}
}
