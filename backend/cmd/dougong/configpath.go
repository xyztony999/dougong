package main

import (
	"os"
	"path/filepath"
)

// serviceDataDir 是 install.sh 建立的 systemd 服务数据目录（unit 的
// WorkingDirectory；两处须保持一致——install.sh DATA_DIR 与本常量）。
const serviceDataDir = "/var/lib/dougong"

// serviceDataDirExists 可注入的存在性探测（测试用；默认 os.Stat）。
var serviceDataDirExists = func() bool {
	_, err := os.Stat(serviceDataDir)
	return err == nil
}

// resolveConfigPath 解析配置文件路径（CLI 子命令与裸启动共用，ADR-030 修正）：
//  1. 显式 -config 指定 → 原样；
//  2. 已安装布局（serviceDataDir 存在）→ {serviceDataDir}/dougong.json——
//     杜绝 CLI 在任意 CWD 读到误启实例的孤儿配置（v0.1.3：/root 裸跑 dougong
//     生成第二套 dougong.json，dougong status 在 /root 执行即读到它）；
//  3. 回退 ./dougong.json（开发模式——开发机上不存在安装目录）。
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if serviceDataDirExists() {
		return filepath.Join(serviceDataDir, "dougong.json")
	}
	return "dougong.json"
}
