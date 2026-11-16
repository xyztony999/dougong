package panel

import (
	"context"
	"fmt"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// 本文件是 CLI 管理通道（api-panel-core.md v2.1 4.1.4「CLI 入口」，ADR-030/
// D-P17）的服务层落点：set-port / entrypath regen·off 为 config 域既有写入口
// 的终端等价通道——同一仓储（SQL #2/#3）、同一动作码与脱敏口径，仅
// source=cli、actor=终端操作者标签（无面板会话身份，权限=终端系统权限）。
// 动作码全量复用不新增（§3.4 v2.1 注）；读命令走既有 Detail/Bootstrap，不在本文件。

// cliAuditEntry 构造 source=cli 的审计四要素基底（D-P17：与 auditEntry 同构，
// actor=终端操作者标签，ActorID 缺省——CLI 无面板用户身份）。
func cliAuditEntry(operator, action, objectType string, detail any) audit.Entry {
	return audit.Entry{
		ActorName:  operator,
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   SingletonObjectID,
		Detail:     detail,
		Result:     audit.ResultSuccess,
		Source:     audit.SourceCLI,
	}
}

// SetListenPortViaCLI `dougong set-port <port>`（写，SQL #3 通道）：更新
// panel_config.listen_port。端口范围校验同 config/update（I-P02/R-14，
// 1024..65535，违规返回 AUTH_012 逐字段错误——cmd 层映射退出码 2）；同值
// 输入零写入零审计（与 UpdateConfig「仅审计实际变更」一致，I-05）。
// 需重启生效（config_effect_mode=restart_required，D-P02）——restart 提示由
// cmd 层统一输出。返回变更前后的端口值与是否发生了写入。
func (s *Service) SetListenPortViaCLI(ctx context.Context, operator string, port int) (before, after int, changed bool, err error) {
	if port < ListenPortMin || port > ListenPortMax {
		return 0, 0, false, invalid(errcode.FieldError{
			Field:   "port",
			Message: fmt.Sprintf("监听端口须在 %d..%d 范围内", ListenPortMin, ListenPortMax),
		})
	}
	now := s.now()
	cur, err := s.st.Config().Get(ctx)
	if err != nil {
		return 0, 0, false, fmt.Errorf("panel: cli set-port get config: %w", err)
	}
	if cur.ListenPort == port {
		return cur.ListenPort, port, false, nil
	}
	deltas := []fieldDelta{{Field: "listenPort", Before: cur.ListenPort, After: port}}
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Config().Update(ctx, storepanel.ConfigChanges{ListenPort: &port}, now); err != nil {
			return fmt.Errorf("panel: cli set-port update: %w", err)
		}
		return tx.Audit().Append(ctx, cliAuditEntry(operator,
			ActionPanelConfigChanged, ObjectTypePanelConfig, deltas), now)
	})
	if err != nil {
		return 0, 0, false, err
	}
	return cur.ListenPort, port, true, nil
}

// EntryPathRegenViaCLI `dougong entrypath regen`（写）：EntryPathGenerator
// 生成新随机路径替换 entry_path（保持 enabled 不变），语义与 HTTP
// regenerate-entry-path 同源（I-P03）；审计 panel_entry_path_regenerated
// （脱敏口径同接口——明文路径不入审计）。即时生效（EntryPathGuard 下一请求
// 即用）；restart 提示为 ADR-030 统一口径，由 cmd 层输出。
func (s *Service) EntryPathRegenViaCLI(ctx context.Context, operator string) (newPath string, err error) {
	now := s.now()
	cur, err := s.st.Config().Get(ctx)
	if err != nil {
		return "", fmt.Errorf("panel: cli entrypath regen get config: %w", err)
	}
	newPath, err = GenerateEntryPath(s.rand)
	if err != nil {
		return "", fmt.Errorf("panel: cli entrypath generate: %w", err)
	}
	detail := map[string]any{"after": maskEntryPath(newPath)}
	if cur.EntryPath != nil && *cur.EntryPath != "" {
		detail["before"] = maskEntryPath(*cur.EntryPath)
	}
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Config().Update(ctx, storepanel.ConfigChanges{EntryPath: &newPath}, now); err != nil {
			return fmt.Errorf("panel: cli entrypath regen update: %w", err)
		}
		return tx.Audit().Append(ctx, cliAuditEntry(operator,
			ActionPanelEntryPathRegenerated, ObjectTypePanelConfig, detail), now)
	})
	if err != nil {
		return "", err
	}
	return newPath, nil
}

// EntryPathOffViaCLI `dougong entrypath off`（写，SQL #3 通道）：
// entry_path_enabled=0。已是关闭态时零写入零审计（changed=false）；
// 即时生效（EntryPathGuard 下一请求直通）；restart 提示由 cmd 层输出。
func (s *Service) EntryPathOffViaCLI(ctx context.Context, operator string) (changed bool, err error) {
	now := s.now()
	cur, err := s.st.Config().Get(ctx)
	if err != nil {
		return false, fmt.Errorf("panel: cli entrypath off get config: %w", err)
	}
	if !cur.EntryPathEnabled {
		return false, nil
	}
	deltas := []fieldDelta{{Field: "entryPathEnabled", Before: true, After: false}}
	off := false
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Config().Update(ctx, storepanel.ConfigChanges{EntryPathEnabled: &off}, now); err != nil {
			return fmt.Errorf("panel: cli entrypath off update: %w", err)
		}
		return tx.Audit().Append(ctx, cliAuditEntry(operator,
			ActionPanelConfigChanged, ObjectTypePanelConfig, deltas), now)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
