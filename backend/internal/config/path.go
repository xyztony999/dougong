package config

import "path/filepath"

// ResolveDBPath 将相对 db_path 解析为相对**配置文件所在目录**（配置文件即
// 数据目录锚点）：
//   - systemd 安装布局 /var/lib/dougong/dougong.json → /var/lib/dougong/data/dougong.db
//     （与既有服务布局逐字节一致——服务 WorkingDirectory 即该目录）；
//   - 开发布局 ./dougong.json → ./data/dougong.db（与历史行为一致）；
//   - 绝对路径原样返回。
//
// 背景（v0.1.3 CLI bug）：db_path 历史上相对 CWD 解析——CLI 在任意目录执行时
// 会读到与 systemd 服务不同的孤儿数据库（如 /root 下误启实例）。锚定到配置
// 文件目录后，配置路径解析（cmd/dougong.resolveConfigPath）与数据路径解析
// 使用同一锚点，CLI 与服务永远指向同一套数据。
func ResolveDBPath(cfgPath, dbPath string) string {
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	return filepath.Join(filepath.Dir(cfgPath), dbPath)
}
