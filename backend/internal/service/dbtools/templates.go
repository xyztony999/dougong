package dbtools

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ---- nginx 注入模板（D-03/4.4.4；A-DBT8 标记 span，A-DBT9 php location 形态）----

// gateLocationInsert 内部 gate location（internal 指令外部请求 404；
// auth_request 子请求携面板会话 cookie 原样透传 + 内部秘密头；Host 头按
// bind_domain 设定过 HostGuard，未绑定透传 $host）。TLS 面板形态（v0.6.9
// 实机修复）：回环目标为 https——Go TLS 监听对明文请求回 400（"Client sent
// an HTTP request to an HTTPS server"），auth_request 拿到非 401/403 状态
// 即 500；证书为面板托管（Origin CA/自签/ACME）无法走系统 CA 校验，安全由
// 回环地址+内部秘密头承载（D-11 三重防护不变），proxy_ssl_verify off 仅关
// 公网语义的证书校验。
func gateLocationInsert(indent string, pa PanelAccess, secret string) string {
	host := "$host"
	if pa.BindDomain != "" {
		host = pa.BindDomain
	}
	loopback := panelLoopbackBase(pa) + "/dbtool/gate/verify"
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s    %s%s\n", indent, markerBeginPrefix, markerKeyGate)
	fmt.Fprintf(&b, "%s    location = %s {\n", indent, GateLocationPath)
	fmt.Fprintf(&b, "%s        internal;\n", indent)
	fmt.Fprintf(&b, "%s        proxy_pass %s;\n", indent, loopback)
	fmt.Fprintf(&b, "%s        proxy_pass_request_body off;\n", indent)
	if pa.HTTPSEnabled {
		fmt.Fprintf(&b, "%s        proxy_ssl_verify off;\n", indent)
	}
	fmt.Fprintf(&b, "%s        proxy_set_header Content-Length \"\";\n", indent)
	fmt.Fprintf(&b, "%s        proxy_set_header %s %s;\n", indent, InternalSecretHeader, secret)
	fmt.Fprintf(&b, "%s        proxy_set_header Host %s;\n", indent, host)
	fmt.Fprintf(&b, "%s        proxy_set_header Cookie $http_cookie;\n", indent)
	fmt.Fprintf(&b, "%s    }\n", indent)
	fmt.Fprintf(&b, "%s    %s%s\n%s", indent, markerEndPrefix, markerKeyGate, indent)
	return b.String()
}

// phpMyAdminLocationInsert 工具 location（FastCGI pass 至 PHP-FPM pool +
// SCRIPT_FILENAME 显式映射 + auth_request；A-DBT9：外层 alias 直服静态资源，
// 嵌套 regex php location capture 显式 SCRIPT_FILENAME）。
func phpMyAdminLocationInsert(indent, toolDir, poolSocket string) string {
	dir := toolDir + "/phpmyadmin"
	pass := poolSocket
	if strings.HasPrefix(pass, "/") {
		pass = "unix:" + pass
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s    %s%s\n", indent, markerBeginPrefix, "phpmyadmin")
	fmt.Fprintf(&b, "%s    location ^~ %s {\n", indent, ToolPathPrefixPHPMyAdmin)
	fmt.Fprintf(&b, "%s        auth_request %s;\n", indent, GateLocationPath)
	fmt.Fprintf(&b, "%s        alias %s/;\n", indent, dir)
	fmt.Fprintf(&b, "%s        index index.php;\n", indent)
	fmt.Fprintf(&b, "%s        location ~ ^%s(.+\\.php)$ {\n", indent, ToolPathPrefixPHPMyAdmin)
	fmt.Fprintf(&b, "%s            auth_request %s;\n", indent, GateLocationPath)
	fmt.Fprintf(&b, "%s            alias %s/;\n", indent, dir)
	fmt.Fprintf(&b, "%s            fastcgi_pass %s;\n", indent, pass)
	fmt.Fprintf(&b, "%s            include %s/%s;\n", indent, toolDir, PHPMyAdminFastCGIConfName)
	fmt.Fprintf(&b, "%s            fastcgi_index index.php;\n", indent)
	fmt.Fprintf(&b, "%s            fastcgi_param SCRIPT_FILENAME %s/$1;\n", indent, dir)
	fmt.Fprintf(&b, "%s            fastcgi_param DOCUMENT_ROOT %s;\n", indent, dir)
	fmt.Fprintf(&b, "%s        }\n", indent)
	fmt.Fprintf(&b, "%s    }\n", indent)
	fmt.Fprintf(&b, "%s    %s%s\n%s", indent, markerEndPrefix, "phpmyadmin", indent)
	return b.String()
}

// mongoExpressLocationInsert 工具 location（proxy_pass 127.0.0.1:8081 +
// auth_request + Authorization 注入秘密文件 include，D-07/D-08）。
func mongoExpressLocationInsert(indent, toolDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s    %s%s\n", indent, markerBeginPrefix, "mongo_express")
	fmt.Fprintf(&b, "%s    location ^~ %s {\n", indent, ToolPathPrefixMongoExpress)
	fmt.Fprintf(&b, "%s        auth_request %s;\n", indent, GateLocationPath)
	fmt.Fprintf(&b, "%s        include %s/%s;\n", indent, toolDir, MongoExpressAuthConfName)
	fmt.Fprintf(&b, "%s        proxy_pass http://%s:%d%s;\n", indent, MongoExpressBind, MongoExpressPort, ToolPathPrefixMongoExpress)
	fmt.Fprintf(&b, "%s        proxy_set_header Host $host;\n", indent)
	fmt.Fprintf(&b, "%s        proxy_set_header X-Real-IP $remote_addr;\n", indent)
	fmt.Fprintf(&b, "%s        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n", indent)
	fmt.Fprintf(&b, "%s        proxy_set_header X-Forwarded-Proto $scheme;\n", indent)
	fmt.Fprintf(&b, "%s        proxy_http_version 1.1;\n", indent)
	fmt.Fprintf(&b, "%s    }\n", indent)
	fmt.Fprintf(&b, "%s    %s%s\n%s", indent, markerEndPrefix, "mongo_express", indent)
	return b.String()
}

// panelLoopbackBase 面板后端 loopback 基址（内部端点 URL 构造：gate 子请求
// /signon 回调共用；entryPath 安装时烘焙，A-DBT6）。scheme 跟随面板 TLS 态
// （v0.6.9 实机修复：TLS 面板的 Go 监听只讲 TLS，明文回环请求被 Go 以 400
// 拒答——gate/signon 须 https，配合 gate proxy_ssl_verify off 与 signon
// curl 校验豁免，安全由回环地址+秘密头承载不变）。
func panelLoopbackBase(pa PanelAccess) string {
	scheme := "http"
	if pa.HTTPSEnabled {
		scheme = "https"
	}
	return scheme + "://127.0.0.1:" + strconv.Itoa(pa.ListenPort) + pa.EntryPath
}

// ---- phpMyAdmin 配置模板（D-06/D-09；R-09 零遥测 VersionCheck=false）----

// configIncPHPParams config.inc.php 模板参数。SignonURLExpr 为 PHP 表达式
// （静态基址或动态 Host 回退——落点无具体域名且未绑定时用 $_SERVER）。
type configIncPHPParams struct {
	BlowfishSecret   string
	SignonScriptPath string
	SignonURLExpr    string
	TempDir          string
}

func configIncPHP(p configIncPHPParams) string {
	return `<?php
/* dougong panel db-tools generated — do not edit manually (reinstall regenerates). */
$cfg['blowfish_secret'] = '` + phpEscape(p.BlowfishSecret) + `';
$cfg['VersionCheck'] = false;
$cfg['SendErrorReports'] = 'never';
$i = 1;
$cfg['Servers'][$i]['auth_type'] = 'signon';
$cfg['Servers'][$i]['SignonSession'] = 'SignonSession';
$cfg['Servers'][$i]['SignonScript'] = '` + phpEscape(p.SignonScriptPath) + `';
$dgToolInstance = isset($_COOKIE['` + ToolInstanceCookie + `']) ? $_COOKIE['` + ToolInstanceCookie + `'] : '';
$dgScheme = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off') ? 'https' : 'http';
$cfg['Servers'][$i]['SignonURL'] = ` + p.SignonURLExpr + `;
$cfg['Servers'][$i]['host'] = 'localhost';
$cfg['Servers'][$i]['AllowNoPassword'] = false;
$cfg['TempDir'] = '` + phpEscape(p.TempDir) + `';
$cfg['UploadDir'] = '';
$cfg['SaveDir'] = '';
`
}

// signonURLExpr 构造 SignonURL PHP 表达式（instanceId 经 dg_tool_instance
// cookie 动态拼接，D-06 v1.2；base 含 entryPath——reopen 为面板后端 API）。
// v0.6.9 实机修复：URL 串的结构性闭合引号不得进 phpEscape——此前把
// `?instanceId='` 的收尾引号一并转义成 \'，生成的字符串字面量永不闭合，
// 吞掉下一行直至 $cfg['Servers'] 的引号处炸出 parse error（PHP-FPM 实机
// php -l 证实）；正确形态=转义仅覆盖 URL 前缀，闭合引号+拼接裸露在外。
func signonURLExpr(base, entryPath string, dynamicHost bool) string {
	prefix := entryPath + SignonReopenPath + "?instanceId="
	if dynamicHost {
		return "$dgScheme . '://' . $_SERVER['HTTP_HOST'] . '" + phpEscape(prefix) + "' . $dgToolInstance"
	}
	return "'" + phpEscape(base+prefix) + "' . $dgToolInstance"
}

// signonURLExprPanelPort 自动落地块形态 SignonURL 表达式（v1.3，A-DBT11
// 落码裁定：reopen 为面板后端 API——自动块形态面板 Go 直连，基址须含面板
// 监听端口而非工具落地端口 18090）：staticBase 非空（已绑域名安装期烘焙）
// →静态基址；否则动态——HTTP_HOST 去端口（工具以 IP:18090 访问，HTTP_HOST
// 携工具端口不可直用）+面板端口拼接；scheme 按面板 HTTPS 态（pa.HTTPSEnabled，
// 工具落地块恒 http 不决定面板 scheme）。结构引号不进 phpEscape（同
// signonURLExpr v0.6.9 修复）。
func signonURLExprPanelPort(pa PanelAccess, staticBase string) string {
	scheme := "http"
	if pa.HTTPSEnabled {
		scheme = "https"
	}
	prefix := pa.EntryPath + SignonReopenPath + "?instanceId="
	if staticBase != "" {
		return "'" + phpEscape(staticBase+prefix) + "' . $dgToolInstance"
	}
	return "'" + scheme + "' . '://' . preg_replace('/:\\d+$/', '', $_SERVER['HTTP_HOST']) . ':" +
		strconv.Itoa(pa.ListenPort) + phpEscape(prefix) + "' . $dgToolInstance"
}

// signonPHP SignonScript（PHP 服务端 loopback 回调面板 signon/verify；凭据
// 响应仅服务端通道消费永不进浏览器，D-09/D-11；W4：转发 dg_session cookie
// 供 sessionDigest 绑定校验 + dg_tool_instance 供实例绑定校验）。
// v0.6.9 实机修复（两处）：①SignonScript 契约=phpMyAdmin include 后调用
// get_login_credentials()（AuthenticationSignon.php:156 单参调用）——原「文件
// 作用域 curl+写 $_SESSION」形态缺该函数，PMA include 即 fatal → 裸 "Error"
// 页；函数须定义 $dgPass 默认值（PMA 单参调用，否则 ArgumentCountError）；
// ②verifyURL 为 https（TLS 面板）时 curl 关闭证书校验——面板证书为托管
// 形态（Origin CA/自签）无系统 CA 链，安全由回环地址+内部秘密头承载（同
// gate proxy_ssl_verify off 口径）。
func signonPHP(verifyURL, secret string) string {
	tlsOpts := ""
	if strings.HasPrefix(verifyURL, "https://") {
		tlsOpts = `
        CURLOPT_SSL_VERIFYPEER => false,
        CURLOPT_SSL_VERIFYHOST => 0,`
	}
	return `<?php
/* dougong panel db-tools generated — signon loopback callback (server-side only).
   phpMyAdmin SignonScript contract: define get_login_credentials() returning
   array(user, password) — called with a single argument (defaults required). */
function get_login_credentials($dgUser, $dgPass = '') {
    $dgToken = isset($_GET['dg_signon']) ? $_GET['dg_signon'] : '';
    $dgSession = isset($_COOKIE['dg_session']) ? $_COOKIE['dg_session'] : '';
    $dgInstance = isset($_COOKIE['` + ToolInstanceCookie + `']) ? $_COOKIE['` + ToolInstanceCookie + `'] : '';
    $dgPayload = json_encode(array('token' => $dgToken, 'session' => $dgSession, 'instanceCookie' => $dgInstance));
    $dgCh = curl_init('` + phpEscape(verifyURL) + `');
    curl_setopt_array($dgCh, array(
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => $dgPayload,
        CURLOPT_HTTPHEADER => array('Content-Type: application/json', '` + InternalSecretHeader + `: ` + phpEscape(secret) + `'),
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT => 10,
        CURLOPT_CONNECTTIMEOUT => 5,` + tlsOpts + `
    ));
    $dgResp = curl_exec($dgCh);
    curl_close($dgCh);
    $dgData = json_decode((string)$dgResp, true);
    if (is_array($dgData) && isset($dgData['code']) && $dgData['code'] === 0 && isset($dgData['data']['user'])) {
        return array($dgData['data']['user'], $dgData['data']['password']);
    }
    return array($dgUser, $dgPass);
}
`
}

// phpEscape PHP 单引号字符串转义（\\ 与 \'）。
func phpEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// fastCGIConf FastCGI 参数模板（SCRIPT_FILENAME/DOCUMENT_ROOT 由 location
// 显式设定，此处不含——避免重复参数）。
func fastCGIConf() string {
	return `# dougong panel db-tools generated — phpMyAdmin FastCGI params.
fastcgi_param QUERY_STRING $query_string;
fastcgi_param REQUEST_METHOD $request_method;
fastcgi_param CONTENT_TYPE $content_type;
fastcgi_param CONTENT_LENGTH $content_length;
fastcgi_param SCRIPT_NAME $fastcgi_script_name;
fastcgi_param REQUEST_URI $request_uri;
fastcgi_param DOCUMENT_URI $document_uri;
fastcgi_param SERVER_PROTOCOL $server_protocol;
fastcgi_param REQUEST_SCHEME $scheme;
fastcgi_param HTTPS $https if_not_empty;
fastcgi_param GATEWAY_INTERFACE CGI/1.1;
fastcgi_param SERVER_SOFTWARE nginx/$nginx_version;
fastcgi_param REMOTE_ADDR $remote_addr;
fastcgi_param REMOTE_PORT $remote_port;
fastcgi_param SERVER_ADDR $server_addr;
fastcgi_param SERVER_PORT $server_port;
fastcgi_param SERVER_NAME $server_name;
fastcgi_param REDIRECT_STATUS 200;
`
}

// ---- mongo-express systemd 单元模板（D-07：凭据仅经 Environment 注入不进
// argv/日志；单元文件 0600 秘密文件通道；bind 127.0.0.1 显式）----

type mongoUnitParams struct {
	AppPath       string
	MongoURL      string
	BasicUsername string
	BasicPassword string
	CookieSecret  string
	SessionSecret string
}

func mongoExpressUnit(p mongoUnitParams) string {
	env := func(k, v string) string {
		return "Environment=" + k + `="` + systemdEscape(v) + "\"\n"
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=dougong panel managed mongo-express (db-tools)\n")
	b.WriteString("After=network.target\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=" + NodeBinaryPath + " " + p.AppPath + "\n")
	b.WriteString(env("ME_CONFIG_MONGODB_URL", p.MongoURL))
	b.WriteString(env("ME_CONFIG_MONGODB_ENABLE_ADMIN", "true"))
	b.WriteString(env("ME_CONFIG_BASICAUTH_USERNAME", p.BasicUsername))
	b.WriteString(env("ME_CONFIG_BASICAUTH_PASSWORD", p.BasicPassword))
	b.WriteString(env("ME_CONFIG_SITE_BASEURL", ToolPathPrefixMongoExpress))
	b.WriteString(env("ME_CONFIG_SITE_PORT", strconv.Itoa(MongoExpressPort)))
	// bind host 无 env 支持（mongo-express 1.0.0 config.default.js 仅
	// BASEURL/COOKIESECRET/SESSIONSECRET/SSL/GRIDFS 有 env）——由包根
	// config.js 覆盖文件钉死 127.0.0.1（mongoExpressConfigJS，install/
	// upgrade 均在 npm install 后重写）。
	b.WriteString(env("ME_CONFIG_SITE_COOKIESECRET", p.CookieSecret))
	b.WriteString(env("ME_CONFIG_SITE_SESSIONSECRET", p.SessionSecret))
	b.WriteString(env("ME_CONFIG_OPTIONS_READONLY", "false"))
	b.WriteString(env("ME_CONFIG_SITE_SSL_ENABLED", "false"))
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=5\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

// systemdEscape Environment 双引号值转义（\ 与 "）。
func systemdEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// buildMongoURL Mongo 连接串（凭据 URL 编码；authSource=admin，D-07）。
func buildMongoURL(c ToolCreds) string {
	u := url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(c.Username, c.Password),
		Host:     c.Host + ":" + strconv.Itoa(c.Port),
		Path:     "/",
		RawQuery: "authSource=admin",
	}
	return u.String()
}

// mongoExpressConfigJS 包根 config.js 覆盖文件（v0.6.9 实机修复：mongo-express
// 1.0.0 的 bind host 无 env 支持——config.default.js 仅 BASEURL/COOKIESECRET/
// SESSIONSECRET/SSL/GRIDFS 有 env，默认 localhost 被 Node 17+ 解析到 ::1 单栈
// IPv6 回环，nginx 127.0.0.1 proxy_pass 连不上 502）。app.js 存在自定义
// config.js 即加载并优先于 config.default.js；require 默认配置后覆盖
// site.host/port 钉死 127.0.0.1 回环（⚠️A6 安全边界不变）。install/upgrade
// 在 npm install（覆盖包目录）之后重写。
func mongoExpressConfigJS() string {
	return `/* dougong panel db-tools generated - pin loopback bind (env unsupported in mongo-express 1.0.0). */
var config = require('./config.default');
config.site.host = '127.0.0.1';
config.site.port = ` + strconv.Itoa(MongoExpressPort) + `;
module.exports = config;
`
}

// mongoExpressAuthConf basic auth 注入秘密文件内容（D-08：nginx 服务端
// proxy_set_header 注入，浏览器全程无感知；0600）。
func mongoExpressAuthConf(password string) string {
	b64 := basicAuthHeader(BasicAuthUsername, password)
	return "# dougong panel db-tools generated — mongo-express basic auth injection (0600).\n" +
		"proxy_set_header Authorization \"Basic " + b64 + "\";\n"
}
