package envmgmt

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// ConfigFS 是环境配置读写的注入面（测试伪化；生产走 os/exec）。
type ConfigFS struct {
	Exists    func(string) bool
	ReadFile  func(string) ([]byte, error)
	WriteFile func(string, []byte, fs.FileMode) error
	Glob      func(string) ([]string, error)
	LookPath  func(string) (string, error)
	Run       func(ctx context.Context, name string, args []string) (string, error)
}

type configFS struct {
	exists    func(string) bool
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, fs.FileMode) error
	glob      func(string) ([]string, error)
	lookPath  func(string) (string, error)
	run       func(ctx context.Context, name string, args []string) (string, error)
}

func (c *ConfigFS) asFS() configFS {
	out := prodConfigFS()
	if c.Exists != nil {
		out.exists = c.Exists
	}
	if c.ReadFile != nil {
		out.readFile = c.ReadFile
	}
	if c.WriteFile != nil {
		out.writeFile = c.WriteFile
	}
	if c.Glob != nil {
		out.glob = c.Glob
	}
	if c.LookPath != nil {
		out.lookPath = c.LookPath
	}
	if c.Run != nil {
		out.run = c.Run
	}
	return out
}

func prodConfigFS() configFS {
	return configFS{
		exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		readFile:  os.ReadFile,
		writeFile: os.WriteFile,
		glob:      filepath.Glob,
		lookPath:  exec.LookPath,
		run: func(ctx context.Context, name string, args []string) (string, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			var buf bytes.Buffer
			cmd.Stdout, cmd.Stderr = &buf, &buf
			err := cmd.Run()
			return buf.String(), err
		},
	}
}

// ---- DTO ----

// ConfigFileView 配置文件条目。
type ConfigFileView struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// SettingView 常用配置项。
type SettingView struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Hint    string   `json:"hint"`
	Type    string   `json:"type"` // text | number | switch
	Value   string   `json:"value"`
	FileID  string   `json:"fileId"`
	Options []string `json:"options,omitempty"`
}

// EnvProfileData GET /env/env/profile。
type EnvProfileData struct {
	Item        EnvItem          `json:"item"`
	HasService  bool             `json:"hasService"`
	ConfigFiles []ConfigFileView `json:"configFiles"`
	Settings    []SettingView    `json:"settings"`
	GeneratedAt string           `json:"generatedAt"`
}

// ConfigFileContentData GET /env/env/config-file。
type ConfigFileContentData struct {
	FileID  string `json:"fileId"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ConfigFileSavedData POST /env/env/save-config-file。
type ConfigFileSavedData struct {
	FileID string `json:"fileId"`
	Path   string `json:"path"`
}

// SettingsSavedData POST /env/env/save-settings。
type SettingsSavedData struct {
	EnvType string   `json:"envType"`
	Keys    []string `json:"keys"`
}

// VersionItem 仓库可装版本。
type VersionItem struct {
	Version string `json:"version"`
	Current bool   `json:"current"`
}

// VersionListData GET /env/env/versions。
type VersionListData struct {
	EnvType  string        `json:"envType"`
	Current  string        `json:"current"`
	Versions []VersionItem `json:"versions"`
}

// ---- 目录（编译期，按发行版常见路径探测）----

type fileDef struct {
	ID         string
	Label      string
	Candidates []string
	Globs      []string
}

type settingDef struct {
	Key     string
	Label   string
	Hint    string
	Type    string
	FileID  string
	Find    *regexp.Regexp
	Write   func(indent, value string) string // 整行替换（含缩进）
	Options []string
}

func envFileDefs(envType string) []fileDef {
	switch envType {
	case storeenv.EnvNginx:
		return []fileDef{{ID: "main", Label: "nginx.conf", Candidates: []string{"/etc/nginx/nginx.conf"}}}
	case storeenv.EnvApache:
		return []fileDef{{ID: "main", Label: "主配置", Candidates: []string{
			"/etc/apache2/apache2.conf", "/etc/httpd/conf/httpd.conf"}}}
	case storeenv.EnvDocker:
		return []fileDef{{ID: "daemon", Label: "daemon.json", Candidates: []string{"/etc/docker/daemon.json"}}}
	case storeenv.EnvMySQL:
		return []fileDef{{ID: "mysqld", Label: "mysqld.cnf", Candidates: []string{
			"/etc/mysql/mysql.conf.d/mysqld.cnf", "/etc/mysql/my.cnf", "/etc/my.cnf"}}}
	case storeenv.EnvRedis:
		return []fileDef{{ID: "redis", Label: "redis.conf", Candidates: []string{
			"/etc/redis/redis.conf", "/etc/redis.conf"}}}
	case storeenv.EnvPHPFPM:
		return []fileDef{
			{ID: "phpini", Label: "php.ini", Candidates: []string{"/etc/php.ini"},
				Globs: []string{"/etc/php/*/fpm/php.ini"}},
			{ID: "pool", Label: "www.conf", Candidates: []string{"/etc/php-fpm.d/www.conf"},
				Globs: []string{"/etc/php/*/fpm/pool.d/www.conf"}},
		}
	case storeenv.EnvMongoDB:
		return []fileDef{{ID: "mongod", Label: "mongod.conf", Candidates: []string{"/etc/mongod.conf"}}}
	}
	return nil
}

func envSettingDefs(envType string) []settingDef {
	text := func(indent, keyFmt, value string) string { return indent + fmt.Sprintf(keyFmt, value) }
	switch envType {
	case storeenv.EnvNginx:
		return []settingDef{
			{Key: "worker_processes", Label: "工作进程数", Hint: "auto 或正整数", Type: "text", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)worker_processes\s+\S+;`),
				Write: func(indent, value string) string { return text(indent, "worker_processes %s;", value) }},
			{Key: "worker_connections", Label: "单进程连接数", Hint: "events 块内", Type: "number", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)worker_connections\s+\S+;`),
				Write: func(indent, value string) string { return text(indent, "worker_connections %s;", value) }},
			{Key: "keepalive_timeout", Label: "Keepalive 超时（秒）", Type: "number", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)keepalive_timeout\s+\S+;`),
				Write: func(indent, value string) string { return text(indent, "keepalive_timeout %s;", value) }},
			{Key: "client_max_body_size", Label: "上传大小限制", Hint: "如 64m", Type: "text", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)client_max_body_size\s+\S+;`),
				Write: func(indent, value string) string { return text(indent, "client_max_body_size %s;", value) }},
			{Key: "gzip", Label: "Gzip 压缩", Type: "switch", FileID: "main", Options: []string{"on", "off"},
				Find:  regexp.MustCompile(`(?m)^(\s*)gzip\s+\S+;`),
				Write: func(indent, value string) string { return text(indent, "gzip %s;", value) }},
		}
	case storeenv.EnvApache:
		return []settingDef{
			{Key: "Timeout", Label: "请求超时（秒）", Type: "number", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)Timeout\s+\S+`),
				Write: func(indent, value string) string { return text(indent, "Timeout %s", value) }},
			{Key: "KeepAlive", Label: "KeepAlive", Type: "switch", FileID: "main", Options: []string{"On", "Off"},
				Find:  regexp.MustCompile(`(?m)^(\s*)KeepAlive\s+\S+`),
				Write: func(indent, value string) string { return text(indent, "KeepAlive %s", value) }},
			{Key: "MaxKeepAliveRequests", Label: "KeepAlive 最大请求数", Type: "number", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)MaxKeepAliveRequests\s+\S+`),
				Write: func(indent, value string) string { return text(indent, "MaxKeepAliveRequests %s", value) }},
			{Key: "KeepAliveTimeout", Label: "KeepAlive 超时（秒）", Type: "number", FileID: "main",
				Find:  regexp.MustCompile(`(?m)^(\s*)KeepAliveTimeout\s+\S+`),
				Write: func(indent, value string) string { return text(indent, "KeepAliveTimeout %s", value) }},
		}
	case storeenv.EnvPHPFPM:
		ini := func(key string) *regexp.Regexp {
			return regexp.MustCompile(`(?m)^(\s*);?\s*` + regexp.QuoteMeta(key) + `\s*=\s*\S+`)
		}
		return []settingDef{
			{Key: "memory_limit", Label: "内存上限", Hint: "如 256M", Type: "text", FileID: "phpini",
				Find: ini("memory_limit"), Write: func(indent, value string) string { return text(indent, "memory_limit = %s", value) }},
			{Key: "upload_max_filesize", Label: "上传文件上限", Type: "text", FileID: "phpini",
				Find: ini("upload_max_filesize"), Write: func(indent, value string) string { return text(indent, "upload_max_filesize = %s", value) }},
			{Key: "post_max_size", Label: "POST 上限", Type: "text", FileID: "phpini",
				Find: ini("post_max_size"), Write: func(indent, value string) string { return text(indent, "post_max_size = %s", value) }},
			{Key: "max_execution_time", Label: "最长执行时间（秒）", Type: "number", FileID: "phpini",
				Find: ini("max_execution_time"), Write: func(indent, value string) string { return text(indent, "max_execution_time = %s", value) }},
			{Key: "pm.max_children", Label: "FPM 最大子进程", Type: "number", FileID: "pool",
				Find:  regexp.MustCompile(`(?m)^(\s*);?\s*pm\.max_children\s*=\s*\S+`),
				Write: func(indent, value string) string { return text(indent, "pm.max_children = %s", value) }},
		}
	case storeenv.EnvMySQL:
		ini := func(key string) *regexp.Regexp {
			return regexp.MustCompile(`(?m)^(\s*);?\s*` + regexp.QuoteMeta(key) + `\s*=\s*\S+`)
		}
		return []settingDef{
			{Key: "port", Label: "端口", Type: "number", FileID: "mysqld",
				Find: ini("port"), Write: func(indent, value string) string { return text(indent, "port = %s", value) }},
			{Key: "max_connections", Label: "最大连接数", Type: "number", FileID: "mysqld",
				Find: ini("max_connections"), Write: func(indent, value string) string { return text(indent, "max_connections = %s", value) }},
			{Key: "bind-address", Label: "监听地址", Type: "text", FileID: "mysqld",
				Find: ini("bind-address"), Write: func(indent, value string) string { return text(indent, "bind-address = %s", value) }},
		}
	case storeenv.EnvRedis:
		sp := func(key string) *regexp.Regexp {
			return regexp.MustCompile(`(?m)^(\s*)#?\s*` + regexp.QuoteMeta(key) + `\s+\S+`)
		}
		return []settingDef{
			{Key: "port", Label: "端口", Type: "number", FileID: "redis",
				Find: sp("port"), Write: func(indent, value string) string { return text(indent, "port %s", value) }},
			{Key: "bind", Label: "监听地址", Type: "text", FileID: "redis",
				Find: sp("bind"), Write: func(indent, value string) string { return text(indent, "bind %s", value) }},
			{Key: "maxmemory", Label: "内存上限", Hint: "如 256mb，空表示不限制", Type: "text", FileID: "redis",
				Find: sp("maxmemory"), Write: func(indent, value string) string { return text(indent, "maxmemory %s", value) }},
			{Key: "timeout", Label: "空闲超时（秒）", Type: "number", FileID: "redis",
				Find: sp("timeout"), Write: func(indent, value string) string { return text(indent, "timeout %s", value) }},
		}
	case storeenv.EnvMongoDB:
		return []settingDef{
			{Key: "port", Label: "端口", Type: "number", FileID: "mongod",
				Find:  regexp.MustCompile(`(?m)^(\s*)port:\s*\S+`),
				Write: func(indent, value string) string { return text(indent, "port: %s", value) }},
			{Key: "bindIp", Label: "监听地址", Type: "text", FileID: "mongod",
				Find:  regexp.MustCompile(`(?m)^(\s*)bindIp:\s*\S+`),
				Write: func(indent, value string) string { return text(indent, "bindIp: %s", value) }},
		}
	}
	return nil
}

func settingValueRe() *regexp.Regexp {
	return regexp.MustCompile(`^[A-Za-z0-9._:+/=,-]{1,64}$`)
}

func sanitizeSettingValue(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("empty")
	}
	if !settingValueRe().MatchString(v) {
		return "", fmt.Errorf("invalid")
	}
	return v, nil
}

func captureValue(re *regexp.Regexp, content string) string {
	m := re.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	line := m[0]
	line = strings.TrimSpace(strings.TrimPrefix(line, m[1]))
	line = strings.TrimLeft(line, ";#")
	line = strings.TrimSpace(line)
	if i := strings.IndexAny(line, "=:"); i >= 0 {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[i+1:]), ";"))
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return strings.TrimSuffix(fields[len(fields)-1], ";")
	}
	return ""
}

func (s *Service) resolveFiles(envType string) []ConfigFileView {
	defs := envFileDefs(envType)
	out := make([]ConfigFileView, 0, len(defs))
	for _, d := range defs {
		cands := append([]string{}, d.Candidates...)
		for _, g := range d.Globs {
			if s.cfs.glob == nil {
				continue
			}
			hits, err := s.cfs.glob(g)
			if err == nil {
				cands = append(hits, cands...)
			}
		}
		view := ConfigFileView{ID: d.ID, Label: d.Label}
		for _, p := range cands {
			if p == "" {
				continue
			}
			if s.cfs.exists(p) {
				view.Path, view.Exists = p, true
				break
			}
			if view.Path == "" {
				view.Path = p
			}
		}
		out = append(out, view)
	}
	return out
}

func (s *Service) fileByID(envType, fileID string) (ConfigFileView, bool) {
	for _, f := range s.resolveFiles(envType) {
		if f.ID == fileID {
			return f, true
		}
	}
	return ConfigFileView{}, false
}

func (s *Service) listItem(ctx context.Context, envType string) (EnvItem, error) {
	data, err := s.EnvList(ctx)
	if err != nil {
		return EnvItem{}, err
	}
	for _, it := range data.Items {
		if it.EnvType == envType {
			return it, nil
		}
	}
	return EnvItem{}, errcodeInvalidEnvType()
}

// EnvProfile GET /env/env/profile。
func (s *Service) EnvProfile(ctx context.Context, envType string) (*EnvProfileData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	item, err := s.listItem(ctx, envType)
	if err != nil {
		return nil, err
	}
	files := s.resolveFiles(envType)
	fileContent := map[string]string{}
	for _, f := range files {
		if !f.Exists {
			continue
		}
		b, err := s.cfs.readFile(f.Path)
		if err != nil {
			continue
		}
		fileContent[f.ID] = string(b)
	}
	settings := make([]SettingView, 0)
	for _, d := range envSettingDefs(envType) {
		val := ""
		if c, ok := fileContent[d.FileID]; ok {
			val = captureValue(d.Find, c)
		}
		settings = append(settings, SettingView{
			Key: d.Key, Label: d.Label, Hint: d.Hint, Type: d.Type,
			Value: val, FileID: d.FileID, Options: d.Options,
		})
	}
	hasService := len(entry.UnitByFamily) > 0 || len(entry.UnitPatternByFamily) > 0
	return &EnvProfileData{
		Item: item, HasService: hasService, ConfigFiles: files, Settings: settings,
		GeneratedAt: formatTS(s.clock()),
	}, nil
}

// ConfigFileGet GET /env/env/config-file。
func (s *Service) ConfigFileGet(_ context.Context, envType, fileID string) (*ConfigFileContentData, error) {
	if CatalogByType(envType) == nil {
		return nil, errcodeInvalidEnvType()
	}
	f, ok := s.fileByID(envType, fileID)
	if !ok {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"reason": "未知配置文件"})
	}
	if !f.Exists {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"path": f.Path, "reason": "文件不存在"})
	}
	b, err := s.cfs.readFile(f.Path)
	if err != nil {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"path": f.Path, "reason": err.Error()})
	}
	return &ConfigFileContentData{FileID: f.ID, Path: f.Path, Content: string(b)}, nil
}

// ConfigFileSave POST /env/env/save-config-file：备份→写入→校验→失败回滚→重载服务。
func (s *Service) ConfigFileSave(ctx context.Context, actor *authsvc.Identity, envType, fileID, content string) (*ConfigFileSavedData, error) {
	if CatalogByType(envType) == nil {
		return nil, errcodeInvalidEnvType()
	}
	f, ok := s.fileByID(envType, fileID)
	if !ok || !f.Exists {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"reason": "配置文件不存在"})
	}
	if err := s.writeConfigFile(ctx, envType, f, []byte(content)); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actor, "env_config_updated", ObjectTypeEnv, envType,
		map[string]any{"envType": envType, "fileId": fileID, "path": f.Path})
	return &ConfigFileSavedData{FileID: f.ID, Path: f.Path}, nil
}

// SettingsSave POST /env/env/save-settings。
func (s *Service) SettingsSave(ctx context.Context, actor *authsvc.Identity, envType string, values map[string]string) (*SettingsSavedData, error) {
	if CatalogByType(envType) == nil {
		return nil, errcodeInvalidEnvType()
	}
	defs := envSettingDefs(envType)
	if len(defs) == 0 {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"reason": "该环境没有可调的常用项"})
	}
	byKey := map[string]settingDef{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	// 按文件聚合，一次读改写。
	perFile := map[string]map[string]string{}
	keys := make([]string, 0, len(values))
	for k, raw := range values {
		d, ok := byKey[k]
		if !ok {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{Field: k, Message: "未知配置项"})
		}
		v, err := sanitizeSettingValue(raw)
		if err != nil {
			return nil, errcode.ErrEnvConfigInvalid.WithData(map[string]any{"reason": "取值不合法：" + k})
		}
		if perFile[d.FileID] == nil {
			perFile[d.FileID] = map[string]string{}
		}
		perFile[d.FileID][k] = v
		keys = append(keys, k)
	}
	for fileID, kv := range perFile {
		f, ok := s.fileByID(envType, fileID)
		if !ok || !f.Exists {
			return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"reason": "配置文件不存在", "fileId": fileID})
		}
		b, err := s.cfs.readFile(f.Path)
		if err != nil {
			return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"path": f.Path, "reason": err.Error()})
		}
		content := string(b)
		for _, d := range defs {
			v, ok := kv[d.Key]
			if !ok {
				continue
			}
			if d.Find.MatchString(content) {
				content = d.Find.ReplaceAllString(content, d.Write("${1}", v))
			} else {
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				content += d.Write("", v) + "\n"
			}
		}
		if err := s.writeConfigFile(ctx, envType, f, []byte(content)); err != nil {
			return nil, err
		}
	}
	s.recordAudit(ctx, actor, "env_config_updated", ObjectTypeEnv, envType,
		map[string]any{"envType": envType, "keys": keys})
	return &SettingsSavedData{EnvType: envType, Keys: keys}, nil
}

func (s *Service) writeConfigFile(ctx context.Context, envType string, f ConfigFileView, content []byte) error {
	orig, err := s.cfs.readFile(f.Path)
	if err != nil {
		return errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"path": f.Path, "reason": err.Error()})
	}
	if err := s.cfs.writeFile(f.Path, content, 0o644); err != nil {
		return errcode.ErrEnvConfigInvalid.WithData(map[string]any{"reason": "写入失败：" + err.Error()})
	}
	if verr := s.validateConfig(ctx, envType); verr != nil {
		_ = s.cfs.writeFile(f.Path, orig, 0o644)
		return errcode.ErrEnvConfigInvalid.WithData(map[string]any{"reason": verr.Error()})
	}
	s.reloadAfterConfig(ctx, envType)
	return nil
}

func (s *Service) validateConfig(ctx context.Context, envType string) error {
	var bin string
	var args []string
	switch envType {
	case storeenv.EnvNginx:
		bin, args = "nginx", []string{"-t"}
	case storeenv.EnvApache:
		for _, name := range []string{"apache2ctl", "apachectl", "httpd"} {
			if p, err := s.cfs.lookPath(name); err == nil {
				if name == "httpd" {
					bin, args = p, []string{"-t"}
				} else {
					bin, args = p, []string{"configtest"}
				}
				break
			}
		}
		if bin == "" {
			return nil // 找不到校验器不阻断写入，靠重启暴露
		}
	default:
		return nil
	}
	if bin == "nginx" {
		p, err := s.cfs.lookPath("nginx")
		if err != nil {
			return fmt.Errorf("找不到 nginx，无法校验配置")
		}
		bin = p
	}
	out, err := s.cfs.run(ctx, bin, args)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (s *Service) reloadAfterConfig(ctx context.Context, envType string) {
	entry := CatalogByType(envType)
	if entry == nil {
		return
	}
	ch := s.pack.DetectChannel()
	unit, _, _ := s.unitFor(ctx, entry, ch.Family)
	if unit == "" {
		return
	}
	if envType == storeenv.EnvNginx {
		if p, err := s.cfs.lookPath("nginx"); err == nil {
			if _, err := s.cfs.run(ctx, p, []string{"-s", "reload"}); err == nil {
				return
			}
		}
	}
	_, _ = s.sysd.RestartUnit(ctx, unit)
}

// ListVersions GET /env/env/versions。
func (s *Service) ListVersions(ctx context.Context, envType string) (*VersionListData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		return nil, errcodeChannelUnavailable("包管理器通道不可用（apt/dnf·yum 均未检出）")
	}
	cands := entry.PkgByFamily[ch.Family]
	if len(cands) == 0 {
		return &VersionListData{EnvType: envType, Versions: []VersionItem{}}, nil
	}
	pkg := cands[0]
	_, cur, found, _ := s.installedPkg(ctx, entry, ch.Family)
	if found && pkg == "" {
		pkg = cands[0]
	}
	vers, err := s.pack.ListVersions(ctx, ch.Family, pkg)
	if err != nil {
		return nil, errcode.ErrEnvConfigUnavailable.WithData(map[string]any{"reason": err.Error()})
	}
	current := ""
	if found {
		current = cur
	}
	items := make([]VersionItem, 0, len(vers))
	seen := false
	for _, v := range vers {
		isCur := current != "" && (v == current || strings.HasPrefix(current, v) || strings.HasPrefix(v, current))
		if isCur {
			seen = true
		}
		items = append(items, VersionItem{Version: v, Current: isCur})
	}
	if current != "" && !seen {
		items = append([]VersionItem{{Version: current, Current: true}}, items...)
	}
	return &VersionListData{EnvType: envType, Current: current, Versions: items}, nil
}
