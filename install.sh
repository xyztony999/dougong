#!/usr/bin/env bash
# ============================================================================
# 斗拱 Dougong 安装 / 升级脚本（可审计：每一行可读，零遥测，仅访问 GitHub Releases）
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash
#   bash install.sh                            # 同上（从 GitHub Releases 装最新版）
#   bash install.sh ./dougong-linux-amd64      # 从本地二进制安装（离线 / 测试）
#
# 重复执行 = 升级：替换二进制并重启服务；配置与数据目录（/var/lib/dougong）不动。
# 卸载：systemctl disable --now dougong && rm -f /usr/local/bin/dougong /etc/systemd/system/dougong.service
#       （数据目录 /var/lib/dougong 按需自行保留或删除）
# ============================================================================
set -euo pipefail

REPO="xyztony999/dougong"
BIN="/usr/local/bin/dougong"
DATA_DIR="/var/lib/dougong"
UNIT="/etc/systemd/system/dougong.service"

[ "$(id -u)" -eq 0 ] || { echo "错误：请以 root 运行（sudo bash install.sh）"; exit 1; }
command -v systemctl >/dev/null 2>&1 || { echo "错误：需要 systemd（Debian 11+ / Ubuntu 20.04+ / RHEL 9 系）"; exit 1; }

# ── 1) 获取二进制 ────────────────────────────────────────────────────────────
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
if [ "${1:-}" != "" ]; then
  echo ">> 使用本地二进制：$1"
  cp "$1" "$TMP/dougong"
else
  echo ">> 查询最新版本（GitHub Releases API）…"
  VER="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep -oE '"tag_name": *"[^"]+"' | head -1 | cut -d'"' -f4)"
  [ -n "$VER" ] || { echo "错误：未取到最新版本号"; exit 1; }
  echo ">> 最新版本：$VER"
  echo ">> 下载 dougong-linux-amd64.tar.gz 与 sha256sums.txt …"
  curl -fsSL -o "$TMP/pkg.tar.gz"     "https://github.com/$REPO/releases/download/$VER/dougong-linux-amd64.tar.gz"
  curl -fsSL -o "$TMP/sha256sums.txt" "https://github.com/$REPO/releases/download/$VER/sha256sums.txt"
  echo ">> sha256 校验："
  (cd "$TMP" && sha256sum -c sha256sums.txt --ignore-missing)
  tar -xzf "$TMP/pkg.tar.gz" -C "$TMP"
  [ -f "$TMP/dougong-linux-amd64" ] || { echo "错误：包内无 dougong-linux-amd64"; exit 1; }
  mv "$TMP/dougong-linux-amd64" "$TMP/dougong"
fi

# ── 2) 安装二进制（先写 .new 再原子替换，失败不留半成品） ─────────────────────
install -m 0755 "$TMP/dougong" "$BIN.new"
mv -f "$BIN.new" "$BIN"
echo ">> 二进制已安装：$BIN"

# ── 3) 数据目录（dougong.json 配置 / SQLite / 密钥；升级永不触碰） ────────────
mkdir -p "$DATA_DIR"
chmod 0700 "$DATA_DIR"

# ── 4) systemd unit（仅首次写入；已存在则保留，尊重用户改动） ─────────────────
# 注：刻意不加 ProtectSystem/NoNewPrivileges 等沙箱指令——面板的职责就是管理
# 全机（nginx 配置写入、任意路径文件操作、su 语义 PTY），沙箱化会破坏功能本身。
if [ ! -f "$UNIT" ]; then
cat > "$UNIT" <<'EOF'
[Unit]
Description=Dougong server panel (斗拱)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/var/lib/dougong
ExecStart=/usr/local/bin/dougong
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  echo ">> 已写入 systemd unit：$UNIT"
fi

# ── 5) 启动 / 重启 ──────────────────────────────────────────────────────────
systemctl daemon-reload
systemctl enable dougong >/dev/null
systemctl restart dougong
sleep 1
echo ">> 完成。面板监听地址见下面日志（默认随机高位端口）："
journalctl -u dougong -n 5 --no-pager 2>/dev/null | grep -i "listening" || journalctl -u dougong -n 10 --no-pager
echo ">> 浏览器访问该地址完成初始化向导；升级=重复执行本脚本。"
