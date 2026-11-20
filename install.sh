#!/usr/bin/env bash
# ============================================================================
# 斗拱 Dougong 安装 / 升级脚本 / Install & upgrade script
# 可审计：每一行可读，零遥测，仅访问 GitHub Releases。
# Auditable: every line readable, zero telemetry, GitHub Releases only.
#
# 用法 / Usage:
#   curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
#   bash install.sh                            # 同上（从 GitHub Releases 装最新版）
#   bash install.sh ./dougong-linux-amd64      # 从本地二进制安装（离线 / 测试）
#   DOUGONG_LANG=en bash install.sh            # 非交互指定语言（zh|en；交互终端会询问）
#   DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh      # 国内加速前缀（三源统一）
#   DOUGONG_VERSION=v0.1.2 DOUGONG_MIRROR=... sudo -E bash install.sh  # 指定版本（跳过 API 查询）
#
# 重复执行 = 升级：替换二进制并重启服务；配置与数据目录（/var/lib/dougong）不动。
# Re-running = upgrade: binary replaced & service restarted; config and the
# data dir (/var/lib/dougong) are left untouched.
# 卸载 / Uninstall:
#   systemctl disable --now dougong && rm -f /usr/local/bin/dougong \
#     /etc/systemd/system/dougong.service
# ============================================================================
set -euo pipefail

REPO="xyztony999/dougong"
BIN="/usr/local/bin/dougong"
DATA_DIR="/var/lib/dougong"
UNIT="/etc/systemd/system/dougong.service"

# ── 网络源 / Network sources ────────────────────────────────────────────────
# 国内服务器连 GitHub 常 RST——两条自助通道：
#   DOUGONG_MIRROR=https://ghproxy.com  拼接式加速前缀（对 github/raw/api 三源
#                                       统一生效；也可用任何同协议镜像）
#   DOUGONG_VERSION=v0.1.2              直接指定版本，跳过 api.github.com 最新版
#                                       查询（镜像不代理 API 时用这个）
# Mainland servers often get RST towards GitHub — two knobs:
#   DOUGONG_MIRROR: prefix-style accelerator applied to all three hosts
#   DOUGONG_VERSION: pin a version, skipping the api.github.com query
MIRROR="${DOUGONG_MIRROR:-}"
GH_BASE="https://github.com"
RAW_BASE="https://raw.githubusercontent.com"
API_BASE="https://api.github.com"
if [ -n "$MIRROR" ]; then
  GH_BASE="$MIRROR/$GH_BASE"
  RAW_BASE="$MIRROR/$RAW_BASE"
  API_BASE="$MIRROR/$API_BASE"
fi
# 短超时+有限重试：直连被 RST 时快速失败给出指引，而非挂死几分钟。
# Short timeout + bounded retries: fail fast with guidance instead of hanging.
CURL_OPTS="--connect-timeout 8 --retry 2 --retry-delay 2"

# ── 0) 语言选择 / Language selection ────────────────────────────────────────
# 优先级：DOUGONG_LANG 环境变量 > 交互询问（经 /dev/tty——curl|bash 的 stdin 是
# 脚本管道，不能直接 read；无终端的非交互场景回退默认 zh）。
# Priority: DOUGONG_LANG env > interactive prompt (via /dev/tty; stdin is the
# script pipe under `curl | bash`) > default zh for non-interactive runs.
LANG_SEL="${DOUGONG_LANG:-}"
if [ -z "$LANG_SEL" ]; then
  # 探测放子壳、read 先置 stderr 抑制再打开 tty——重定向从左到右生效，
  # 顺序错了会把「无控制终端」报错泄漏到输出（Git Bash 实测坑）。
  if [ -e /dev/tty ] && (exec 3</dev/tty) 2>/dev/null; then
    printf '选择语言 / Select language: 1) 中文  2) English [1]: '
    sel=""
    IFS= read -r sel 2>/dev/null < /dev/tty || sel=1
    case "$sel" in
      2 | en | EN | english | English) LANG_SEL="en" ;;
      *) LANG_SEL="zh" ;; # 回车/EOF/其他输入 → 默认中文
    esac
  else
    LANG_SEL="zh" # /dev/tty 不可用（CI/容器等全非交互环境）→ 默认中文
  fi
fi

# msg "中文" "English" —— 按所选语言输出一行。
# set -u 防护（v0.1.15 实机修复）：用 ${n:-} 取值，单参数调用不再 unbound variable；
# 英文缺第二参数时回退第一参数（旧混合中英串兼容），中文恒用第一参数。
msg() {
  if [ "$LANG_SEL" = "en" ]; then printf '%s\n' "${2:-${1:-}}"; else printf '%s\n' "${1:-}"; fi
}

# ── 1) 前置检查 / Preflight ─────────────────────────────────────────────────
[ "$(id -u)" -eq 0 ] || { msg "错误：请以 root 运行（sudo bash install.sh）" "Error: run as root (sudo bash install.sh)"; exit 1; }
command -v systemctl >/dev/null 2>&1 || {
  msg "错误：需要 systemd（Debian 11+ / Ubuntu 20.04+ / RHEL 9 系）" "Error: systemd required (Debian 11+ / Ubuntu 20.04+ / RHEL 9 family)"
  exit 1
}

# ── 2) 获取二进制 / Fetch the binary ────────────────────────────────────────
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
if [ "${1:-}" != "" ]; then
  msg ">> 使用本地二进制：$1" ">> Using local binary: $1"
  cp "$1" "$TMP/dougong"
else
  # 网络失败指引（直连 RST / 镜像不支持 API 等场景的统一出口）。
  # Unified guidance for network failures (direct RST, mirror without API).
  net_fail_hint() {
    msg "" ""
    msg "无法访问 GitHub（国内服务器常见 RST）。三种解法：" \
        "Cannot reach GitHub (mainland servers often get RST). Three ways out:"
    msg "  1) 加镜像前缀重试 / Retry with a mirror prefix:" \
        "     1) Retry with a mirror prefix:"
    msg "     DOUGONG_MIRROR=https://ghproxy.com bash install.sh" \
        "     DOUGONG_MIRROR=https://ghproxy.com bash install.sh"
    msg "  2) 指定版本跳过 API 查询（镜像不代理 api.github.com 时）/" \
        "     2) Pin a version to skip the API query (mirrors often don't proxy it):"
    msg "     DOUGONG_VERSION=v0.1.2 DOUGONG_MIRROR=https://ghproxy.com bash install.sh" \
        "     DOUGONG_VERSION=v0.1.2 DOUGONG_MIRROR=https://ghproxy.com bash install.sh"
    msg "  3) 本地模式（本地下载后上传，零外网依赖）/" \
        "     3) Local mode (download elsewhere, upload — zero WAN dependency):"
    msg "     bash install.sh ./dougong-linux-amd64" \
        "     bash install.sh ./dougong-linux-amd64"
  }
  if [ -n "${DOUGONG_VERSION:-}" ]; then
    VER="$DOUGONG_VERSION"
    msg ">> 使用指定版本：$VER" ">> Pinned version: $VER"
  else
    msg ">> 查询最新版本（GitHub Releases API）…" ">> Querying the latest release (GitHub Releases API)..."
    VER="$(curl -fsSL $CURL_OPTS "$API_BASE/repos/$REPO/releases/latest" \
          | grep -oE '"tag_name": *"[^"]+"' | head -1 | cut -d'"' -f4)" || { net_fail_hint; exit 1; }
    [ -n "$VER" ] || { net_fail_hint; exit 1; }
    msg ">> 最新版本：$VER" ">> Latest version: $VER"
  fi
  msg ">> 下载 dougong-linux-amd64.tar.gz 与 sha256sums.txt …" ">> Downloading dougong-linux-amd64.tar.gz and sha256sums.txt..."
  curl -fsSL $CURL_OPTS -o "$TMP/dougong-linux-amd64.tar.gz" "$GH_BASE/$REPO/releases/download/$VER/dougong-linux-amd64.tar.gz" || { net_fail_hint; exit 1; }
  curl -fsSL $CURL_OPTS -o "$TMP/sha256sums.txt" "$GH_BASE/$REPO/releases/download/$VER/sha256sums.txt" || { net_fail_hint; exit 1; }
  msg ">> sha256 校验：" ">> sha256 verification:"
  # 包名与 sha256sums.txt 完全一致才能命中；--ignore-missing 跳过未下载的
  # arm64 条目（校验数必须 ≥1，否则说明文件名对不上——立即失败）。
  # The tarball is saved under its release name so the amd64 entry actually
  # verifies; --ignore-missing skips the (not downloaded) arm64 entry — at
  # least one file MUST verify or the install aborts.
  (cd "$TMP" && sha256sum -c sha256sums.txt --ignore-missing)
  tar -xzf "$TMP/dougong-linux-amd64.tar.gz" -C "$TMP"
  [ -f "$TMP/dougong-linux-amd64" ] || { msg "错误：包内无 dougong-linux-amd64" "Error: dougong-linux-amd64 missing from the archive"; exit 1; }
  mv "$TMP/dougong-linux-amd64" "$TMP/dougong"
fi

# ── 3) 安装二进制（先写 .new 再原子替换，失败不留半成品）──────────────────────
# Atomic binary replace via a .new staging file — no half-written state.
install -m 0755 "$TMP/dougong" "$BIN.new"
mv -f "$BIN.new" "$BIN"
msg ">> 二进制已安装：$BIN" ">> Binary installed: $BIN"

# ── 4) 数据目录（dougong.json 配置 / SQLite / 密钥；升级永不触碰）────────────
# Data dir (config / SQLite / keys) — never touched on upgrades.
mkdir -p "$DATA_DIR"
chmod 0700 "$DATA_DIR"

# ── 5) systemd unit（仅首次写入；已存在则保留，尊重用户改动）──────────────────
# Written once on first install; an existing unit is preserved as-is.
# 注：刻意不加 ProtectSystem/NoNewPrivileges 等沙箱指令——面板的职责就是管理
# 全机（nginx 配置写入、任意路径文件操作、su 语义 PTY），沙箱化会破坏功能本身。
# Note: no sandbox directives by design — the panel's job is managing the
# whole machine (nginx config writes, file access everywhere, su-style PTY).
if [ ! -f "$UNIT" ]; then
cat > "$UNIT" <<'EOF'
[Unit]
Description=Dougong server panel (斗拱)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/var/lib/dougong
ExecStart=/usr/local/bin/dougong start
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  msg ">> 已写入 systemd unit：$UNIT" ">> systemd unit written: $UNIT"
fi

# ── 6) 启动 / 重启 ──────────────────────────────────────────────────────────
systemctl daemon-reload
systemctl enable dougong >/dev/null
systemctl restart dougong
sleep 1
msg ">> 完成。浏览器访问以下地址完成初始化向导（默认随机高位端口）：" ">> Done. Open one of the addresses below to run the first-time wizard (random high port by default):"
LISTEN="$(journalctl -u dougong -n 20 --no-pager 2>/dev/null | grep -oE 'addr=[0-9.]+:[0-9]+.*tls=(true|false)' | tail -1)"
ADDR="$(echo "$LISTEN" | grep -oE 'addr=[0-9.]+:[0-9]+' | cut -d= -f2)"
SCHEME="http"
echo "$LISTEN" | grep -q 'tls=true' && SCHEME="https"
if [ -n "$ADDR" ]; then
  HOST_PART="${ADDR%%:*}"; PORT="${ADDR##*:}"
  if [ "$HOST_PART" = "0.0.0.0" ]; then
    for ip in $(hostname -I 2>/dev/null); do echo "     ${SCHEME}://${ip}:${PORT}/"; done
  else
    echo "     ${SCHEME}://${ADDR}/"
  fi
else
  journalctl -u dougong -n 10 --no-pager
fi
msg ">> 升级=重复执行本脚本；服务管理=systemctl status/restart dougong。" ">> Re-run this script to upgrade; manage with systemctl status/restart dougong."
