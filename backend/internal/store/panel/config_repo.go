package panel

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ConfigRepo 实现 panel_config 单行表访问（I-P01：全局单行 id=1）。
// 所有语句参数化（? 占位）；动态 SET 列名全部为静态字面量白名单
// （backend-03 §3，G-02）。
type ConfigRepo struct{ x Executor }

// EnsureSingleton 启动自检（SQL 清单 #1）：INSERT OR IGNORE 保证 id=1 行存在。
// listenPort 为插入时端口（安装期随机高位端口语义，api §3.4 配置项），幂等：
// 行已存在时不产生任何变更。
func (r *ConfigRepo) EnsureSingleton(ctx context.Context, listenPort int, at time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`INSERT OR IGNORE INTO panel_config
		   (id, listen_scope, listen_address, listen_port, entry_path_enabled,
		    entry_path, https_mode, update_check, lang, updated_at)
		 VALUES (1, 'all', NULL, ?, 0, NULL, 'off', 0, 'zh-CN', ?)`,
		listenPort, formatTime(at))
	return wrapErr("config ensure singleton", err)
}

const configCols = `id, listen_scope, listen_address, listen_port, entry_path_enabled,
	entry_path, https_mode, update_check, lang, updated_at`

// Get 读取配置单例（SQL 清单 #2：启动绑定读取 + GET detail 数据源）。
// 行不存在（迁移后未自检）返回包装后的 sql.ErrNoRows。
func (r *ConfigRepo) Get(ctx context.Context) (Config, error) {
	var c Config
	var listenAddress, entryPath sql.NullString
	var entryPathEnabled, updateCheck int
	var updatedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT `+configCols+` FROM panel_config WHERE id = 1`).
		Scan(&c.ID, &c.ListenScope, &listenAddress, &c.ListenPort,
			&entryPathEnabled, &entryPath, &c.HTTPSMode, &updateCheck,
			&c.Lang, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, wrapErr("config get", err)
	}
	if err != nil {
		return Config{}, wrapErr("config get", err)
	}
	c.ListenAddress = scanStringPtr(listenAddress)
	c.EntryPathEnabled = entryPathEnabled == 1
	c.EntryPath = scanStringPtr(entryPath)
	c.UpdateCheck = updateCheck == 1
	t, err := scanTime(updatedAt)
	if err != nil {
		return Config{}, wrapErr("config scan updated_at", err)
	}
	c.UpdatedAt = t
	return c, nil
}

// Update 按白名单列部分更新（SQL 清单 #3，I-P07 单事务原子保存由调用方
// （service withTx）保证；updated_at 恒随任意变更刷新）。
func (r *ConfigRepo) Update(ctx context.Context, ch ConfigChanges, at time.Time) error {
	if ch.Empty() {
		return nil // 无列变更（调用方保证至少一字段时不会走到）
	}
	// 列名全部为静态字面量（白名单），值一律 ? 参数绑定——G-02。
	sets := []string{"updated_at = ?"}
	args := []any{formatTime(at)}
	if ch.ListenScope != nil {
		sets = append(sets, "listen_scope = ?")
		args = append(args, *ch.ListenScope)
	}
	if ch.ListenAddress != nil {
		sets = append(sets, "listen_address = ?")
		if *ch.ListenAddress == "" { // 空串 = 显式清空（I-P02 ⇔）
			args = append(args, nil)
		} else {
			args = append(args, *ch.ListenAddress)
		}
	}
	if ch.ListenPort != nil {
		sets = append(sets, "listen_port = ?")
		args = append(args, *ch.ListenPort)
	}
	if ch.EntryPathEnabled != nil {
		sets = append(sets, "entry_path_enabled = ?")
		args = append(args, boolInt(*ch.EntryPathEnabled))
	}
	if ch.EntryPath != nil {
		sets = append(sets, "entry_path = ?")
		args = append(args, *ch.EntryPath)
	}
	if ch.HTTPSMode != nil {
		sets = append(sets, "https_mode = ?")
		args = append(args, *ch.HTTPSMode)
	}
	if ch.UpdateCheck != nil {
		sets = append(sets, "update_check = ?")
		args = append(args, boolInt(*ch.UpdateCheck))
	}
	if ch.Lang != nil {
		sets = append(sets, "lang = ?")
		args = append(args, *ch.Lang)
	}
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_config SET `+strings.Join(sets, ", ")+` WHERE id = 1`, args...)
	return wrapErr("config update", err)
}
