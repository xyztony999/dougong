package dbmgmt

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/errcode"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// BuildIdentityKey 组装 identity_key（D-01：socket=engine|socket:{path}、
// tcp=engine|tcp:{host}:{port}；I-01 全表唯一消重锚点）。
func BuildIdentityKey(engine, connMode, socketPath, host string, port int) string {
	switch connMode {
	case storedb.ConnModeSocket:
		return engine + "|socket:" + socketPath
	default:
		return engine + "|tcp:" + host + ":" + strconv.Itoa(port)
	}
}

// instanceIdentityKey 由档案行组装 identity_key（探测归并判据）。
func instanceIdentityKey(inst *storedb.Instance) string {
	socket, host := "", ""
	port := 0
	if inst.SocketPath != nil {
		socket = *inst.SocketPath
	}
	if inst.Host != nil {
		host = *inst.Host
	}
	if inst.Port != nil {
		port = *inst.Port
	}
	return BuildIdentityKey(inst.Engine, inst.ConnMode, socket, host, port)
}

// targetOf 由档案行组装连接目标（I-05 前置链通过后建连用）。
func targetOf(inst *storedb.Instance) dbconn.Target {
	t := dbconn.Target{Engine: inst.Engine, ConnMode: inst.ConnMode}
	if inst.SocketPath != nil {
		t.SocketPath = *inst.SocketPath
	}
	if inst.Host != nil {
		t.Host = *inst.Host
	}
	if inst.Port != nil {
		t.Port = *inst.Port
	}
	return t
}

// targetReachable 传输层可达探测（A-DB4：socket 存在 / TCP 拨通；conn_state
// 探测态主判据，D-07/I-03）。
func targetReachable(t dbconn.Target) bool {
	if t.ConnMode == storedb.ConnModeSocket {
		st, err := os.Stat(t.SocketPath)
		return err == nil && st.Mode()&os.ModeSocket != 0
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)), probeTimeout)
	if err != nil {
		return false
	}
	return conn.Close() == nil
}

// hostPortLabel 是导出文件名 {engine-host-port} 段（tcp=host-port、socket=
// local——D-06 命名规则的 socket 模式呈现裁定）。
func hostPortLabel(t dbconn.Target) string {
	if t.ConnMode == storedb.ConnModeSocket {
		return "local"
	}
	return t.Host + "-" + strconv.Itoa(t.Port)
}

// ConnTargetSpec 是 credentials 请求的连接目标纠正字段组（可选变更，D-03；
// 全零值=沿用档案既有目标）。
type ConnTargetSpec struct {
	ConnMode   *string `json:"connMode"`
	SocketPath *string `json:"socketPath"`
	Host       *string `json:"host"`
	Port       *int    `json:"port"`
}

// resolve 求值纠正后的连接目标：任一字段提供即整体纠正（I-07 互斥校验一次
// 通过）；全未提供沿用档案。返回 AUTH_012 校验错误或新目标与 identity_key。
func (spec ConnTargetSpec) resolve(inst *storedb.Instance) (dbconn.Target, string, error) {
	if spec.ConnMode == nil && spec.SocketPath == nil && spec.Host == nil && spec.Port == nil {
		t := targetOf(inst)
		return t, instanceIdentityKey(inst), nil
	}
	mode := ""
	socket, host := "", ""
	port := 0
	if spec.ConnMode != nil {
		mode = strings.TrimSpace(*spec.ConnMode)
	}
	if mode == "" {
		mode = inst.ConnMode // 未纠正连接方式时沿用档案（其余字段仍按新值组装）
	}
	// I-07 显式互斥校验：请求显式携带与最终 mode 不匹配的纠正字段即 AUTH_012
	//（不静默忽略——服务端校验 R-14）。
	if mode == storedb.ConnModeSocket && (spec.Host != nil || spec.Port != nil) {
		return dbconn.Target{}, "", errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "connMode", Message: "socket 模式不支持 host/port 纠正（I-07）"})
	}
	if mode == storedb.ConnModeTCP && spec.SocketPath != nil {
		return dbconn.Target{}, "", errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "connMode", Message: "tcp 模式不支持 socketPath 纠正（I-07）"})
	}
	// 按最终 mode 求值字段（I-07 互斥：socket 模式唯一来源 socketPath、
	// tcp 模式 host/port 唯一——纠正连接方式即整体切换目标形态）。
	if mode == storedb.ConnModeSocket {
		if spec.SocketPath != nil {
			socket = strings.TrimSpace(*spec.SocketPath)
		} else if inst.SocketPath != nil {
			socket = *inst.SocketPath
		}
		host, port = "", 0
	} else {
		if spec.Host != nil {
			host = strings.TrimSpace(*spec.Host)
		} else if inst.Host != nil {
			host = *inst.Host
		}
		if spec.Port != nil {
			port = *spec.Port
		} else if inst.Port != nil {
			port = *inst.Port
		}
		socket = ""
	}
	if err := validateConnTarget(mode, socket, host, port); err != nil {
		return dbconn.Target{}, "", err
	}
	t := dbconn.Target{Engine: inst.Engine, ConnMode: mode}
	if mode == storedb.ConnModeSocket {
		t.SocketPath = socket
	} else {
		t.Host, t.Port = host, port
	}
	return t, BuildIdentityKey(inst.Engine, mode, socket, host, port), nil
}

// validateConnTarget 连接目标形态校验（I-07 互斥空 / I-08 端口区间与绝对
// socket 路径；AUTH_012 逐字段）。
func validateConnTarget(mode, socketPath, host string, port int) error {
	var fields []errcode.FieldError
	switch mode {
	case storedb.ConnModeSocket:
		if socketPath == "" || !strings.HasPrefix(socketPath, "/") {
			fields = append(fields, errcode.FieldError{Field: "socketPath", Message: "socket 模式须提供绝对路径"})
		}
		if host != "" || port != 0 {
			fields = append(fields, errcode.FieldError{Field: "connMode", Message: "socket 模式 host/port 必须为空（I-07）"})
		}
	case storedb.ConnModeTCP:
		if host == "" {
			fields = append(fields, errcode.FieldError{Field: "host", Message: "tcp 模式 host 必填（默认 localhost）"})
		}
		if port < 1 || port > 65535 {
			fields = append(fields, errcode.FieldError{Field: "port", Message: "tcp 模式端口须为 1~65535（I-08）"})
		}
		if socketPath != "" {
			fields = append(fields, errcode.FieldError{Field: "connMode", Message: "tcp 模式 socketPath 必须为空（I-07）"})
		}
	default:
		fields = append(fields, errcode.FieldError{Field: "connMode", Message: "connMode 须为 socket|tcp"})
	}
	if len(fields) > 0 {
		return errcode.ErrValidation.WithFields(fields...)
	}
	return nil
}

// ---- 标识符白名单（R-14 / A-DB3：引擎侧 SQL 组装前置防线）----

var (
	databaseNameRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	usernameRe     = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	hostRe         = regexp.MustCompile(`^[A-Za-z0-9_.:%-]+$`)
	keyPatternRe   = regexp.MustCompile(`^[!-~]+$`) // 可见 ASCII 无空白（DBM_017）
)

// validateDatabaseName 库名白名单校验。
func validateDatabaseName(name string) error {
	if name == "" || len(name) > maxDatabaseNameLen || !databaseNameRe.MatchString(name) {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "name", Message: fmt.Sprintf("库名须为 %d 字符内的字母/数字/下划线", maxDatabaseNameLen)})
	}
	return nil
}

// validateUsername 用户名白名单校验。
func validateUsername(username string) error {
	if username == "" || len(username) > maxUsernameLen || !usernameRe.MatchString(username) {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "username", Message: fmt.Sprintf("用户名须为 %d 字符内的字母/数字/下划线", maxUsernameLen)})
	}
	return nil
}

// validateUserHost host 白名单校验（% 通配惯例，4.3.4）。
func validateUserHost(host string) error {
	if host == "" || len(host) > maxHostLen || !hostRe.MatchString(host) {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "host", Message: "host 形态非法（字母/数字/点/冒号/百分号/连字符）"})
	}
	return nil
}

// validatePassword 密码入参校验（非空+长度上限；内容自由——引擎侧经字符串
// 转义组装，A-DB3）。
func validatePassword(field, password string) error {
	if password == "" || len(password) > maxPasswordLen {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: field, Message: fmt.Sprintf("密码须为 1~%d 字符", maxPasswordLen)})
	}
	return nil
}

// validateKeyPattern Redis key 匹配模式校验（DBM_017：禁空/超长/空白与控制
// 字符——空模式等同 KEYS 全量语义，D-09）。
func validateKeyPattern(pattern string) error {
	if pattern == "" || len(pattern) > MaxPatternLen || !keyPatternRe.MatchString(pattern) {
		return errcode.ErrDBMInvalidPattern
	}
	return nil
}

// validateRedisKey TTL 查询 key 校验。
func validateRedisKey(key string) error {
	if key == "" || len(key) > maxKeyLen {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "key", Message: "key 须为 1~512 字符"})
	}
	return nil
}

// ---- v2.3 表级/集合治理白名单（R-14 / A-DB3）----

// validateTableName 表名白名单校验（沿库名形态 ^[A-Za-z0-9_]+$ ≤64——
// quoteIdent 引擎侧组装前置防线）。
func validateTableName(name string) error {
	if name == "" || len(name) > maxTableNameLen || !databaseNameRe.MatchString(name) {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "table", Message: fmt.Sprintf("表名须为 %d 字符内的字母/数字/下划线", maxTableNameLen)})
	}
	return nil
}

// validateCollectionName Mongo 集合名形态校验（引擎规则服务端承载：非空/
// 禁 $ 前缀/禁空字符/禁 system. 保留前缀，§4.8.4 mcoll/create）。
func validateCollectionName(name string) error {
	if name == "" || len(name) > maxCollectionNameLen ||
		strings.HasPrefix(name, "$") || strings.ContainsRune(name, 0) ||
		strings.HasPrefix(name, "system.") {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "name", Message: "集合名须非空且不含 $ 前缀/空字符/system. 保留前缀"})
	}
	return nil
}

// parsePagination 分页入参形态（A-DB19：空→默认；非整数或 <1→AUTH_012；
// pageSize 超限截断至上限——api「超限截断至上限」口径）。
func parsePagination(pageRaw, pageSizeRaw string, defaultSize, limit int) (page, pageSize int, err error) {
	page, pageSize = 1, defaultSize
	if pageRaw != "" {
		p, perr := strconv.Atoi(pageRaw)
		if perr != nil || p < 1 {
			return 0, 0, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "page", Message: "page 须为 ≥1 的整数"})
		}
		page = p
	}
	if pageSizeRaw != "" {
		ps, perr := strconv.Atoi(pageSizeRaw)
		if perr != nil || ps < 1 {
			return 0, 0, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "pageSize", Message: "pageSize 须为 ≥1 的整数"})
		}
		pageSize = ps
	}
	if pageSize > limit {
		pageSize = limit
	}
	return page, pageSize, nil
}
