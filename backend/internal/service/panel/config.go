package panel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// UpdateConfig POST /panel/config/update（admin）：部分字段集更新。
// 校验顺序（api 4.1.4）：字段合法性（AUTH_012）→ I-P02 custom 跨字段
// （PANEL_001）→ I-P08 语言（PANEL_006）→ httpsMode 切换前置（PANEL_003 /
// 自签生成 / off 直通）；处理：单事务原子保存（I-P07）+ 同事务审计
// panel_config_changed（detail={field,before,after}[]）。
// 响应为更新后配置视图（结构同 detail）；监听项修改不热生效（D-P02）。
func (s *Service) UpdateConfig(ctx context.Context, actor *authsvc.Identity, req UpdateConfigRequest) (*DetailView, error) {
	now := s.now()

	// D-P08：会话参数载体=配置文件，接口层拒绝 session 字段。
	if req.Session != nil {
		return nil, invalid(errcode.FieldError{Field: "session", Message: "会话参数由配置文件管理（改文件+重启），不提供接口修改"})
	}
	if !req.AnySet() {
		return nil, invalid(errcode.FieldError{Field: "body", Message: "未包含任何可更新字段"})
	}

	cur, err := s.st.Config().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("panel: update get config: %w", err)
	}

	// ---- 1) 字段合法性（AUTH_012，R-14：端口范围/地址格式/取值枚举）----
	merged := cur
	changes := storepanel.ConfigChanges{}
	var deltas []fieldDelta

	if req.ListenScope != nil {
		switch v := strings.TrimSpace(*req.ListenScope); v {
		case storepanel.ListenScopeAll, storepanel.ListenScopeLocalhost, storepanel.ListenScopeCustom:
			if v != cur.ListenScope {
				changes.ListenScope = &v
				deltas = append(deltas, fieldDelta{Field: "listenScope", Before: cur.ListenScope, After: v})
				merged.ListenScope = v
			}
		default:
			return nil, invalid(errcode.FieldError{Field: "listenScope", Message: "取值须为 all / localhost / custom"})
		}
	}
	if req.ListenAddress != nil {
		v := strings.TrimSpace(*req.ListenAddress)
		if v == "" {
			// 显式清空（配 scope 切走 custom；空串语义见 store ConfigChanges）。
			if cur.ListenAddress != nil && *cur.ListenAddress != "" {
				changes.ListenAddress = &v
				deltas = append(deltas, fieldDelta{Field: "listenAddress", Before: *cur.ListenAddress, After: nil})
			}
			merged.ListenAddress = nil
		} else if !validHost(v) {
			return nil, invalid(errcode.FieldError{Field: "listenAddress", Message: "监听地址须为合法 IP 或主机名"})
		} else {
			if cur.ListenAddress == nil || *cur.ListenAddress != v {
				changes.ListenAddress = &v
				var before any
				if cur.ListenAddress != nil {
					before = *cur.ListenAddress
				}
				deltas = append(deltas, fieldDelta{Field: "listenAddress", Before: before, After: v})
			}
			merged.ListenAddress = &v
		}
	}
	if req.ListenPort != nil {
		v := *req.ListenPort
		if v < ListenPortMin || v > ListenPortMax {
			return nil, invalid(errcode.FieldError{
				Field:   "listenPort",
				Message: fmt.Sprintf("监听端口须在 %d..%d 范围内", ListenPortMin, ListenPortMax),
			})
		}
		if v != cur.ListenPort {
			changes.ListenPort = &v
			deltas = append(deltas, fieldDelta{Field: "listenPort", Before: cur.ListenPort, After: v})
			merged.ListenPort = v
		}
	}
	if req.EntryPathEnabled != nil {
		v := *req.EntryPathEnabled
		if v != cur.EntryPathEnabled {
			changes.EntryPathEnabled = &v
			deltas = append(deltas, fieldDelta{Field: "entryPathEnabled", Before: cur.EntryPathEnabled, After: v})
			merged.EntryPathEnabled = v
		}
	}
	// bindDomain（D-P12）：携带 null/空串=解绑、非空合法域名=绑定、未携带=不变；
	// null 与缺省语义不同，经 RawMessage 区分（AUTH_012 fields 校验，不设专用码）。
	if req.BindDomain != nil {
		v, ferr := parseBindDomain(req.BindDomain)
		if ferr != nil {
			return nil, invalid(*ferr)
		}
		if v == "" { // 解绑（置空语义）
			if cur.BindDomain != nil && *cur.BindDomain != "" {
				empty := ""
				changes.BindDomain = &empty
				deltas = append(deltas, fieldDelta{Field: "bindDomain", Before: *cur.BindDomain, After: nil})
			}
			merged.BindDomain = nil
		} else {
			if cur.BindDomain == nil || *cur.BindDomain != v {
				changes.BindDomain = &v
				var before any
				if cur.BindDomain != nil {
					before = *cur.BindDomain // 公开访问名明文（A-P10/D-P12），不脱敏
				}
				deltas = append(deltas, fieldDelta{Field: "bindDomain", Before: before, After: v})
			}
			merged.BindDomain = &v
		}
	}
	var modeTransition string
	if req.HTTPSMode != nil {
		switch v := strings.TrimSpace(*req.HTTPSMode); v {
		case storepanel.HTTPSModeOff, storepanel.HTTPSModeSelfSigned,
			storepanel.HTTPSModeImported, storepanel.HTTPSModeAcme:
			if v != cur.HTTPSMode {
				changes.HTTPSMode = &v
				deltas = append(deltas, fieldDelta{Field: "httpsMode", Before: cur.HTTPSMode, After: v})
				merged.HTTPSMode = v
				modeTransition = v
			}
		default:
			return nil, invalid(errcode.FieldError{Field: "httpsMode", Message: "取值须为 off / self_signed / imported / acme"})
		}
	}
	if req.UpdateCheck != nil {
		v := *req.UpdateCheck
		if v != cur.UpdateCheck {
			changes.UpdateCheck = &v
			deltas = append(deltas, fieldDelta{Field: "updateCheck", Before: cur.UpdateCheck, After: v})
			merged.UpdateCheck = v
		}
	}
	if req.Lang != nil {
		v := strings.TrimSpace(*req.Lang)
		if v == "" {
			return nil, invalid(errcode.FieldError{Field: "lang", Message: "界面语言不能为空"})
		}
		if v != cur.Lang {
			changes.Lang = &v
			deltas = append(deltas, fieldDelta{Field: "lang", Before: cur.Lang, After: v})
			merged.Lang = v
		}
	}

	// ---- 2) I-P02 custom 跨字段（PANEL_001；A-P4 裁定：非 custom 不接受
	// 本次请求携带的非空地址；存量旧地址随 scope 切走自动清空，维持存储态 ⇔）----
	if merged.ListenScope == storepanel.ListenScopeCustom {
		if merged.ListenAddress == nil || *merged.ListenAddress == "" {
			return nil, errcode.ErrCustomAddressRequired // custom 缺地址（格式非法已在 1) AUTH_012 拦截）
		}
	} else {
		// 本次请求携带非空地址而结果态 scope 非 custom → 字段级拒绝。
		if req.ListenAddress != nil && strings.TrimSpace(*req.ListenAddress) != "" {
			return nil, invalid(errcode.FieldError{
				Field:   "listenAddress",
				Message: "仅监听范围 custom 接受监听地址（all/localhost 请置空）",
			})
		}
		// 存量地址残留（scope 自 custom 切走且未显式清空）→ 自动清空（落审计 delta）。
		if cur.ListenAddress != nil && *cur.ListenAddress != "" {
			empty := ""
			changes.ListenAddress = &empty
			deltas = append(deltas, fieldDelta{Field: "listenAddress", Before: *cur.ListenAddress, After: nil})
			merged.ListenAddress = nil
		}
	}

	// ---- 3) I-P08 语言枚举（PANEL_006）----
	if merged.Lang != LangZhCN {
		return nil, errcode.ErrLangNotSupported
	}

	// ---- 4) httpsMode 切换前置（I-P04）：to imported 校验证书行存在
	//（PANEL_003）；to self_signed 即时生成（D-P06，行缺失兜底同）；
	// to off 直接生效（不触碰 panel_cert）。----
	needSelfSigned := false
	if merged.HTTPSMode == storepanel.HTTPSModeSelfSigned {
		_, err := s.st.Cert().Get(ctx)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			needSelfSigned = true // 证书行缺失（含切换与同值重写场景）兜底生成
		case err != nil:
			return nil, fmt.Errorf("panel: update get cert: %w", err)
		case modeTransition != "":
			needSelfSigned = true // 切换至 self_signed：即时生成替换（D-P06）
		}
	}
	if modeTransition == storepanel.HTTPSModeImported {
		if _, err := s.st.Cert().Get(ctx); errors.Is(err, sql.ErrNoRows) {
			return nil, errcode.ErrCertRequired // PANEL_003
		} else if err != nil {
			return nil, fmt.Errorf("panel: update get cert: %w", err)
		}
	}
	// to acme（I-P04 v2.0 扩展，D-P14）：前置 panel_cert.source=acme 且推算
	// 未过期（valid/expiring_soon）；否则 PANEL_003。
	if modeTransition == storepanel.HTTPSModeAcme {
		cert, err := s.st.Cert().Get(ctx)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, errcode.ErrCertRequired // PANEL_003
		case err != nil:
			return nil, fmt.Errorf("panel: update get cert: %w", err)
		case cert.Source != storepanel.CertSourceAcme:
			return nil, errcode.ErrCertRequired
		case !acmeCertUsable(CalcCertStatus(cert, now)):
			return nil, errcode.ErrCertRequired // 已过期 acme 证书不可承载模式
		}
	}
	// to off：直接生效（不触碰 panel_cert，api 4.1.4）。

	// ---- 5) entryPathEnabled 0→1 且无路径值时自动生成（等价 regenerate 语义）----
	generatedPath := ""
	if merged.EntryPathEnabled && (cur.EntryPath == nil || *cur.EntryPath == "") {
		p, err := GenerateEntryPath(s.rand)
		if err != nil {
			return nil, fmt.Errorf("panel: generate entry path: %w", err)
		}
		generatedPath = p
		changes.EntryPath = &p
		var before any
		if cur.EntryPath != nil {
			before = maskEntryPath(*cur.EntryPath)
		}
		deltas = append(deltas, fieldDelta{Field: "entryPath", Before: before, After: maskEntryPath(p)})
	}

	// ---- 6) 单事务原子保存 + 同事务审计（I-P07 / I-05）----
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if needSelfSigned {
			certPEM, keyPEM, summary, err := IssueSelfSigned(s.rand, now)
			if err != nil {
				return fmt.Errorf("panel: issue self-signed: %w", err)
			}
			if err := tx.Cert().Upsert(ctx, storepanel.Cert{
				CertPEM: certPEM, KeyPEM: keyPEM, Source: storepanel.CertSourceSelfSigned,
				Issuer: &summary.Issuer, NotBefore: &summary.NotBefore, NotAfter: &summary.NotAfter,
			}, now); err != nil {
				return fmt.Errorf("panel: upsert self-signed: %w", err)
			}
		}
		if !changes.Empty() {
			if err := tx.Config().Update(ctx, changes, now); err != nil {
				return fmt.Errorf("panel: update config: %w", err)
			}
		}
		if len(deltas) > 0 {
			if err := tx.Audit().Append(ctx, auditEntry(actor, ActionPanelConfigChanged, ObjectTypePanelConfig, deltas), now); err != nil {
				return fmt.Errorf("panel: audit config changed: %w", err)
			}
		}
		if generatedPath != "" {
			// 等价 regenerate 语义（A-P9 裁定）：同事务补 panel_entry_path_regenerated（脱敏）。
			detail := map[string]any{"after": maskEntryPath(generatedPath)}
			if cur.EntryPath != nil && *cur.EntryPath != "" {
				detail["before"] = maskEntryPath(*cur.EntryPath)
			}
			if err := tx.Audit().Append(ctx, auditEntry(actor, ActionPanelEntryPathRegenerated, ObjectTypePanelConfig, detail), now); err != nil {
				return fmt.Errorf("panel: audit entry path regenerated: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Detail(ctx)
}

// parseBindDomain 解析 bindDomain 字段载荷（D-P12）：JSON null 或空串 → ""
// （解绑）；非空字符串 → 原值待校验；其他 JSON 类型 → AUTH_012 字段错误。
func parseBindDomain(raw json.RawMessage) (string, *errcode.FieldError) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", &errcode.FieldError{Field: "bindDomain", Message: "bindDomain 须为字符串或 null"}
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil // 空串=解绑（与 null 同义，D-P12 置空语义）
	}
	if !validDomain(v) {
		return "", &errcode.FieldError{Field: "bindDomain",
			Message: "绑定域名须为 RFC 1123 主机名（≤253 字符，不含协议/路径/端口）"}
	}
	return v, nil
}

// validDomain 域名格式校验（D-P12/I-P09 输入约束）：RFC 1123 主机名标签规则、
// 总长 ≤253、不含 scheme/path/port（出现 : / ? # @ 即拒——IPv6 字面量因含
// 冒号被拒，与「不含端口」共用一条判定）。
func validDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	if strings.ContainsAny(domain, ":/?#@") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, ch := range label {
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			case ch == '-':
				if i == 0 || i == len(label)-1 {
					return false // 标签首尾不得为连字符（RFC 1123）
				}
			default:
				return false
			}
		}
	}
	return true
}

// RegenerateEntryPath POST /panel/config/regenerate-entry-path（admin）：
// 生成新随机路径替换 entry_path（保持 enabled 状态不变），即时生效
// （EntryPathGuard 下一请求即用新值）；审计 panel_entry_path_regenerated
// （detail 不含明文路径值——脱敏为长度+前 2 字符掩码）。
func (s *Service) RegenerateEntryPath(ctx context.Context, actor *authsvc.Identity) (*RegenerateEntryPathResult, error) {
	now := s.now()
	cur, err := s.st.Config().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("panel: regenerate get config: %w", err)
	}
	newPath, err := GenerateEntryPath(s.rand)
	if err != nil {
		return nil, fmt.Errorf("panel: generate entry path: %w", err)
	}
	detail := map[string]any{"after": maskEntryPath(newPath)}
	if cur.EntryPath != nil && *cur.EntryPath != "" {
		detail["before"] = maskEntryPath(*cur.EntryPath)
	}
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Config().Update(ctx, storepanel.ConfigChanges{EntryPath: &newPath}, now); err != nil {
			return fmt.Errorf("panel: regenerate update: %w", err)
		}
		return tx.Audit().Append(ctx, auditEntry(actor, ActionPanelEntryPathRegenerated, ObjectTypePanelConfig, detail), now)
	})
	if err != nil {
		return nil, err
	}
	return &RegenerateEntryPathResult{EntryPath: newPath}, nil
}

// UploadCertificate POST /panel/config/upload-certificate（admin）：导入证书
// 与私钥 PEM（D-P05：JSON 文本字段）。CertPairValidator 解析/配对校验失败
// → PANEL_002（R-14）；处理：UPSERT panel_cert（source=imported，提取
// issuer/notBefore/notAfter，D-P04 替换式）；https_mode 不变（模式切换走
// update，api 2.3 两接口顺序协作）；审计 panel_cert_updated（object_type=
// panel_cert，detail 不含 PEM 材料——I-P06）。
func (s *Service) UploadCertificate(ctx context.Context, actor *authsvc.Identity, req UploadCertificateRequest) (*UploadCertificateResult, error) {
	now := s.now()
	if strings.TrimSpace(req.CertPEM) == "" || strings.TrimSpace(req.KeyPEM) == "" {
		return nil, invalid(
			errcode.FieldError{Field: "certPem", Message: "证书 PEM 内容不能为空"},
			errcode.FieldError{Field: "keyPem", Message: "私钥 PEM 内容不能为空"},
		)
	}
	fields, err := ValidateCertPair(req.CertPEM, req.KeyPEM)
	if err != nil {
		// PANEL_002：细节进日志，响应保持契约文案（防探测/信息收敛）。
		slog.WarnContext(ctx, "panel: certificate pair invalid", "err", err)
		return nil, errcode.ErrCertInvalid
	}
	issuer := fields.Issuer
	notBefore, notAfter := fields.NotBefore, fields.NotAfter
	cert := storepanel.Cert{
		CertPEM: req.CertPEM, KeyPEM: req.KeyPEM, Source: storepanel.CertSourceImported,
		Issuer: &issuer, NotBefore: &notBefore, NotAfter: &notAfter,
	}
	detail := map[string]any{
		"source":    storepanel.CertSourceImported,
		"issuer":    issuer,
		"notBefore": notBefore.Format(time.RFC3339),
		"notAfter":  notAfter.Format(time.RFC3339),
	}
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Cert().Upsert(ctx, cert, now); err != nil {
			return fmt.Errorf("panel: upsert imported cert: %w", err)
		}
		return tx.Audit().Append(ctx, auditEntry(actor, ActionPanelCertUpdated, ObjectTypePanelCert, detail), now)
	})
	if err != nil {
		return nil, err
	}
	return &UploadCertificateResult{Cert: CertSummary{
		Source: cert.Source, Issuer: cert.Issuer, NotBefore: cert.NotBefore, NotAfter: cert.NotAfter,
	}}, nil
}
