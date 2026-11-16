package cronback

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dougong/dougong/internal/errcode"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// validate.go 是 SourceValidator/TargetValidator 领域服务（§4.1.1 I-09/I-10，
// R-14 逐字段错误）。路径判定用 path 包（POSIX 语义——目标环境 Linux；本地
// Windows 开发机测试路径以 / 形态书写）。

var (
	hostnameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)
	unixUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	dbNameRe   = regexp.MustCompile(`^[A-Za-z0-9_$]{1,64}$`)
)

// validateJobDef 校验任务定义四要素（name/源清单/目标/保留数/表达式——
// I-06/I-07/I-09/I-10；名称唯一性在 service 层对仓储判定，排除自身）。
func (s *Service) validateJobDef(ctx context.Context, req *BackupJobReq) (*storecron.Job, *errcode.Error) {
	fields := make([]errcode.FieldError, 0, 6)

	name := strings.TrimSpace(req.Name)
	if name == "" {
		fields = append(fields, errcode.FieldError{Field: "name", Message: "任务名不能为空"})
	} else if utf8.RuneCountInString(name) > MaxNameLength {
		fields = append(fields, errcode.FieldError{Field: "name", Message: "任务名不能超过 64 字符"})
	}

	if len(req.SourceItems) == 0 {
		fields = append(fields, errcode.FieldError{Field: "sourceItems", Message: "源清单不能为空"})
	}
	items := make([]storecron.SourceItem, 0, len(req.SourceItems))
	for i, it := range req.SourceItems {
		field := func(msg string) errcode.FieldError {
			return errcode.FieldError{Field: "sourceItems", Message: msg}
		}
		switch it.Type {
		case storecron.SourceSiteDir:
			if !isAbsPOSIX(it.Path) {
				fields = append(fields, field("站点目录项 path 须为绝对路径"))
				continue
			}
			items = append(items, storecron.SourceItem{Type: it.Type, Path: it.Path})
		case storecron.SourceDatabase:
			if it.InstanceID <= 0 {
				fields = append(fields, field("数据库项 instanceId 须为正整数"))
				continue
			}
			if it.Database != "" && !dbNameRe.MatchString(it.Database) {
				fields = append(fields, field("database 为非法库名"))
				continue
			}
			found, _, _, err := s.instArchive.InstanceArchive(ctx, it.InstanceID)
			if err != nil {
				return nil, errcode.ErrInternal
			}
			if !found {
				fields = append(fields, field("数据库项实例不存在（instanceId 悬空）"))
				continue
			}
			items = append(items, storecron.SourceItem{
				Type: it.Type, InstanceID: it.InstanceID, Database: it.Database})
		default:
			fields = append(fields, field("源类型非法（site_dir|database）"))
		}
		_ = i
	}

	var target storecron.TargetConfig
	switch req.TargetType {
	case storecron.TargetLocal:
		if !isAbsPOSIX(req.TargetConfig.Path) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.path", Message: "本地目标须为绝对路径"})
		} else if s.dataDir != "" && overlapsPanelDataDir(req.TargetConfig.Path, s.dataDir) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.path", Message: "本地目标不能与面板数据目录相同或互相包含（防备份递归覆盖自身数据）"})
		}
		target = storecron.TargetConfig{Path: req.TargetConfig.Path}
	case storecron.TargetRsync:
		if !hostnameRe.MatchString(req.TargetConfig.Host) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.host", Message: "host 须为合法主机名或 IP"})
		}
		if !unixUserRe.MatchString(req.TargetConfig.User) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.user", Message: "user 须为合法 Unix 用户名"})
		}
		if !isAbsPOSIX(req.TargetConfig.Path) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.path", Message: "rsync 目标路径须为远端绝对路径"})
		}
		if p := req.TargetConfig.Port; p != 0 && (p < 1 || p > 65535) {
			fields = append(fields, errcode.FieldError{Field: "targetConfig.port", Message: "port 须在 1..65535"})
		}
		target = storecron.TargetConfig{
			Host: req.TargetConfig.Host, User: req.TargetConfig.User,
			Port: req.TargetConfig.Port, Path: req.TargetConfig.Path}
	default:
		fields = append(fields, errcode.FieldError{Field: "targetType", Message: "目标类型非法（local|rsync）"})
	}

	if req.RetentionCount < MinRetention || req.RetentionCount > MaxRetention {
		fields = append(fields, errcode.FieldError{Field: "retentionCount", Message: "保留份数须在 1..100"})
	}

	if _, exprErrs := ParseCronExpr(req.ScheduleCron); len(exprErrs) > 0 {
		// 表达式非法走 CB_002 专用码（段位级 errors[]），不混入 CB_001 fields。
		return nil, errcode.ErrCronExprInvalid.WithData(map[string]any{"errors": exprErrs})
	}

	if len(fields) > 0 {
		return nil, errcode.ErrCBValidation.WithFields(fields...)
	}
	return &storecron.Job{
		Name: name, SourceItems: items, TargetType: req.TargetType,
		TargetConfig: target, RetentionCount: req.RetentionCount,
		ScheduleCron: NormalizeCronExpr(req.ScheduleCron), // 宏归一化写入（D-03）
	}, nil
}

// isAbsPOSIX POSIX 绝对路径判定（目标环境 Linux；path 包跨平台一致）。
func isAbsPOSIX(p string) bool { return len(p) > 0 && p[0] == '/' }

// overlapsPanelDataDir 判定目标路径与面板数据目录的重叠（A-C8 四向拒绝：
// 相同 / 目标含数据目录 / 数据目录含目标——双向包含均引发备份滚雪球或递归
// 覆盖自身 SQLite，D-06）。
func overlapsPanelDataDir(target, dataDir string) bool {
	t := cleanPOSIX(target)
	d := cleanPOSIX(dataDir)
	if t == d {
		return true
	}
	return strings.HasPrefix(t, d+"/") || strings.HasPrefix(d, t+"/")
}

// cleanPOSIX POSIX 语义清洗（斜杠规范化；Windows 开发机写入的数据目录以
// filepath 清洗后转斜杠）。
func cleanPOSIX(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimSuffix(p, "/")
}

// validateCronJobInput 校验计划任务写入（expr 五段 CB_002 / command 非空
// CB_001，§4.3.4 create 校验）。
func validateCronJobInput(expr, command string) (normExpr string, ec *errcode.Error) {
	if strings.TrimSpace(command) == "" {
		return "", errcode.ErrCBValidation.WithFields(
			errcode.FieldError{Field: "command", Message: "命令不能为空"})
	}
	norm := NormalizeCronExpr(expr)
	if _, errs := ParseCronExpr(norm); len(errs) > 0 {
		return "", errcode.ErrCronExprInvalid.WithData(map[string]any{"errors": errs})
	}
	return norm, nil
}
