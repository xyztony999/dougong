// 斗拱面板 Node 项目启动器（ADR-033 / api-app-mgmt.md v1.2 D-16）。
// 由 PM2 以本脚本为执行目标拉起：读取项目目录 .env 注入进程环境，再按启动
// 方式加载应用。.env 是环境变量的唯一来源——面板不把其内容复制到任何别处。
//
// 参数（由面板经 pm2 start … -- 传入）：
//   node pm2-launcher.js <项目目录> <node|npm> <node 绝对路径> <npm 绝对路径> <file|script> <目标>
//     file   目标=项目内入口文件相对路径（在 PM2 直接管理的进程内加载）
//     script 目标=npm 脚本名（以 npm 绝对路径执行 npm run <脚本名>）
'use strict';
const fs = require('fs');
const path = require('path');
const cp = require('child_process');

const [dir, mode, nodeBin, npmBin, kind, target] = process.argv.slice(2);
if (!dir || !kind || !target) {
  console.error('[dougong] 启动器参数不完整');
  process.exit(2);
}

// .env：KEY=VALUE 逐行；# 注释与空行跳过；值两侧成对引号剥离；同名变量以
// .env 为准（用户参数优先）。无 .env 或不可读不阻断启动。
try {
  const text = fs.readFileSync(path.join(dir, '.env'), 'utf8');
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq <= 0) continue;
    const key = line.slice(0, eq).trim();
    let val = line.slice(eq + 1).trim();
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    if (key) process.env[key] = val;
  }
} catch (e) { /* 无 .env：照常启动 */ }

process.chdir(dir);

if (kind === 'file') {
  const root = path.resolve(dir);
  const entry = path.resolve(dir, target);
  if (entry !== root && !entry.startsWith(root + path.sep)) {
    console.error('[dougong] 入口文件越出项目目录');
    process.exit(2);
  }
  require(entry);
} else if (kind === 'script') {
  const npm = npmBin || 'npm';
  const child = cp.spawn(npm, ['run', target], {
    cwd: dir,
    stdio: 'inherit',
    env: process.env,
    shell: process.platform === 'win32',
  });
  child.on('error', (err) => {
    console.error('[dougong] 启动 npm 脚本失败: ' + err.message);
    process.exit(1);
  });
  child.on('exit', (code, signal) => process.exit(signal ? 1 : (code == null ? 1 : code)));
  for (const sig of ['SIGINT', 'SIGTERM']) {
    process.on(sig, () => { try { child.kill(sig); } catch (e) { /* 已退出 */ } });
  }
} else {
  console.error('[dougong] 未知启动方式: ' + kind);
  process.exit(2);
}
