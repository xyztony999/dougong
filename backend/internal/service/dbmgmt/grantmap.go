package dbmgmt

import (
	"sort"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dbmysql"
)

// GrantLevelMapper 是 D-10 反向映射领域服务：SHOW GRANTS raw 权限串 → 三档
// grant_level。未识别的库级权限不呈现不回收（如实口径）；全局 *.* 行与
// 未识别形态一律不映射。
//
// 档位判定（正向映射表 levelPrivileges 的像集）：
//   - ALL PRIVILEGES → all
//   - 权限集合恰为 {SELECT} → readonly
//   - 权限集合恰为 {SELECT, INSERT, UPDATE, DELETE} → write
//   - 其它（子集/超集/交集外的库级权限）→ ""（不呈现）
func GrantLevelMapper(raw string) string {
	perms := normalizePrivileges(dbmysql.GrantPrivilegesOf(raw))
	switch {
	case perms == nil:
		return ""
	case perms["ALL PRIVILEGES"]:
		return LevelAll
	case len(perms) == 1 && perms["SELECT"]:
		return LevelReadonly
	case len(perms) == 4 && perms["SELECT"] && perms["INSERT"] && perms["UPDATE"] && perms["DELETE"]:
		return LevelWrite
	default:
		return ""
	}
}

// normalizePrivileges 把逗号分隔权限串解析为集合（大小写归一、空白剔除）。
func normalizePrivileges(s string) map[string]bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	set := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			set[p] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// mapUserGrants 把网关实况用户清单投影为响应条目（grants 仅含可映射档位的
// 库级授权行，按库名稳定排序；D-10）。
func mapUserGrants(users []dbmysql.UserGrants) []DBUserItem {
	items := make([]DBUserItem, 0, len(users))
	for _, u := range users {
		item := DBUserItem{Username: u.Username, Host: u.Host}
		for _, g := range u.Grants {
			if g.DB == "" || g.DB == "*" {
				continue // 全局行/未识别形态：不呈现不回收（D-10）
			}
			level := GrantLevelMapper(g.Raw)
			if level == "" {
				continue // 未识别档位：不呈现
			}
			item.Grants = append(item.Grants, GrantItem{Database: g.DB, Level: level})
		}
		items = append(items, item)
	}
	return items
}

// hasDBGrant 报告用户在指定库是否存在任意库级授权行（DBM_011 revoke 前置
// 判据——含未识别档位：可回收但不可呈现，D-10 如实口径）。
func hasDBGrant(lines []dbmysql.GrantLine, db string) bool {
	for _, g := range lines {
		if g.DB == db {
			return true
		}
	}
	return false
}

// sortGrantItems 按 database 稳定排序（响应确定性）。
func sortGrantItems(items []GrantItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Database < items[j].Database })
}
