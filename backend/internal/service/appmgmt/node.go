// node.go —— app-mgmt 的 Node 运行时分支（ADR-033，api-app-mgmt.md v1.2
// D-16~D-18）：Node 项目由 PM2 托管进程；创建=依赖安装 → 注册 PM2 进程 →
// 保存进程列表；开机自启开关=启动/停止并保存（面板仅在创建、删除、切换
// 开关三处保存，手动启停不保存——PM2 按保存时状态恢复，故开关是持久设置）。
// Python 分支（venv + systemd）不变，见 service.go / worker.go。
package appmgmt

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/pmx"
	"github.com/dougong/dougong/internal/errcode"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
)

// NodeGateway 是 Node 本机通道契约（*appx.Gateway 实现；接口化供测试注入替身）。
type NodeGateway interface {
	NpmInstall(ctx context.Context, dir string, timeout time.Duration) (string, error)
	ResolveBins(ctx context.Context) (nodeBin, npmBin string, err error)
}

// PM2Gateway 是 PM2 进程托管契约（*pmx.Gateway 实现；接口化供测试注入替身）。
type PM2Gateway interface {
	Start(ctx context.Context, spec pmx.StartSpec) (string, error)
	StartByName(ctx context.Context, name string) (string, error)
	Stop(ctx context.Context, name string) (string, error)
	Restart(ctx context.Context, name string) (string, error)
	Delete(ctx context.Context, name string) (string, error)
	Save(ctx context.Context) (string, error)
	List(ctx context.Context) ([]pmx.Proc, error)
	SavedStatus(name string) (string, bool, error)
	Tail(ctx context.Context, outPath, errPath string, lines int) ([]string, error)
}

var npmScriptRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// pm2NameFor 派生 PM2 进程名（与 systemd 单元同前缀，不带 .service 后缀；
// 存 app.unit_name 列——Node 项目复用该列承载托管对象名）。
func pm2NameFor(name string) string { return UnitPrefix + name }

func errNodeEnv(missing []string, reason string) *errcode.Error {
	return errcode.ErrAppNodeEnvIncomplete.WithData(map[string]any{"missing": missing, "reason": reason})
}

func errNodeSpec(reason string) *errcode.Error {
	return errcode.ErrAppNodeStartSpecInvalid.WithData(map[string]any{"reason": reason})
}

// ---- 创建前置校验（D-16）----

// checkNodeReady Node 环境就绪度分层校验（APP_015）：逐项列出缺失的
// node / npm / PM2，引导环境管理（已装 Node 的存量主机经「升级」补装 PM2）。
func (s *Service) checkNodeReady(ctx context.Context) error {
	rd, err := s.envReady.NodeReadiness(ctx)
	if err != nil {
		return errNodeEnv(nil, "Node 环境状态未探测（探测不可用），请到环境管理确认")
	}
	var missing []string
	if !rd.NodeAvailable {
		missing = append(missing, "node")
	}
	if !rd.NpmAvailable {
		missing = append(missing, "npm")
	}
	if !rd.Pm2Available {
		missing = append(missing, "pm2")
	}
	if len(missing) > 0 {
		return errNodeEnv(missing,
			"Node 环境不完整，请到环境管理安装或升级 Node（已装 Node 的主机经「升级」补装 PM2）")
	}
	return nil
}

// isAbsAny 判定路径是否为绝对路径。只看字面、不调用 filepath.IsAbs，为的是同一份
// 输入在 Windows（开发机）和 Linux（服务器、CI）上得到相同的判定：
//   - 以 / 或 \ 开头（POSIX / 根相对 / UNC）
//   - 第二个字符是冒号、其后紧跟 / 或 \（盘符路径，如 C:\x、C:/x；与 Windows 一致，
//     冒号前不限定为字母）
//
// filepath.IsAbs 是平台相关的：Windows 上不认「/…」却认「1:/x」，类 Unix 上不认
// 「C:\…」。v0.4.0 发版时它在 Linux 上把盘符路径当成项目内的相对路径放行过。
func isAbsAny(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// normalizeNodeStartSpec 校验并规整 Node 启动方式值对象（APP_016）：
// node_file=入口文件须项目内相对路径；npm_script=脚本名白名单（缺省 start）。
// 返回规整后的值对象。
func normalizeNodeStartSpec(mode string, sp StartSpec) (StartSpec, error) {
	switch mode {
	case storeapp.StartModeNodeFile:
		f := strings.TrimSpace(sp.File)
		if f == "" {
			return sp, errNodeSpec("须填写入口文件（项目内的相对路径，如 server.js）")
		}
		if isAbsAny(f) || strings.Contains(f, "..") {
			return sp, errNodeSpec("入口文件须为项目内的相对路径")
		}
		sp.File = filepath.ToSlash(filepath.Clean(f))
	case storeapp.StartModeNpmScript:
		s := strings.TrimSpace(sp.Script)
		if s == "" {
			s = "start"
		}
		if !npmScriptRe.MatchString(s) {
			return sp, errNodeSpec("脚本名只允许字母、数字与 . _ : -")
		}
		sp.Script = s
	default:
		return sp, errNodeSpec("未知的 Node 启动方式")
	}
	return sp, nil
}

// launcherArgs 组装启动器参数：<项目目录> <node 绝对路径> <npm 绝对路径>
// <file|script> <目标>（launcher.js 约定）。
func launcherArgs(dir, nodeBin, npmBin, mode string, sp StartSpec) []string {
	if mode == storeapp.StartModeNpmScript {
		return []string{dir, nodeBin, npmBin, "script", sp.Script}
	}
	return []string{dir, nodeBin, npmBin, "file", sp.File}
}

// ---- 任务执行链 ----

// runNodeCreate Node 创建执行链（D-05 同构）：npm install（有 package.json
// 时）→ 落启动器 → 注册 PM2 进程 → 未选立即启动则停止 → 保存进程列表 →
// succeeded + 档案登记同事务（创建失败不残留半档案）。
func (s *Service) runNodeCreate(ctx context.Context, task storeapp.AppTask, plan createPlan, ring *outputRing) {
	if _, err := os.Stat(filepath.Join(plan.Dir, "package.json")); err == nil {
		ring.append("[deps] npm install")
		out, err := s.node.NpmInstall(ctx, plan.Dir, PYOpTimeout)
		ring.append(out)
		if err != nil {
			e := "依赖安装失败：" + err.Error()
			s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring.appendErr(out), nil)
			return
		}
	} else {
		ring.append("[deps] 无 package.json，跳过依赖安装")
	}
	if s.dataDir == "" {
		e := "面板数据目录未配置，无法落启动器"
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	launcher, err := pmx.EnsureLauncher(filepath.Join(s.dataDir, "app"))
	if err != nil {
		e := "启动器落位失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	name := pm2NameFor(task.AppName)
	ring.append("[pm2] 注册进程 " + name)
	out, err := s.pm2.Start(ctx, pmx.StartSpec{Name: name, Cwd: plan.Dir, Launcher: launcher, NodeBin: plan.NodeBin,
		Args: launcherArgs(plan.Dir, plan.NodeBin, plan.NpmBin, plan.StartMode, plan.StartSpec)})
	ring.append(out)
	if err != nil {
		e := "注册 PM2 进程失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring.appendErr(out), nil)
		return
	}
	if !plan.StartNow {
		ring.append("[pm2] 按未选「立即启动」停止进程（保留注册）")
		if sout, serr := s.pm2.Stop(ctx, name); serr != nil {
			ring.append("[warn] 停止进程失败（保守继续）：" + serr.Error())
		} else {
			ring.append(sout)
		}
	}
	ring.append("[pm2] 保存进程列表")
	if sout, serr := s.pm2.Save(ctx); serr != nil {
		ring.append("[warn] 保存进程列表失败（开机自启可能不生效，可切换一次开机自启开关重试）：" + serr.Error())
	} else {
		ring.append(sout)
	}
	archive := func(ctx context.Context, tx *storeapp.TxStore) error {
		_, err := tx.App().Create(ctx, storeapp.App{
			Name: task.AppName, Dir: plan.Dir, Entry: plan.Entry,
			Runtime: storeapp.RuntimeNode, StartMode: plan.StartMode, StartSpec: mustJSON(plan.StartSpec),
			ListenPort: plan.ListenPort, UnitName: name, CreatedBy: task.RequestedBy,
		})
		return err
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, archive)
}

// runNodeDeps Node 依赖重装（npm install；失败可重试；完成不自动重启）。
func (s *Service) runNodeDeps(ctx context.Context, task storeapp.AppTask, app storeapp.App, ring *outputRing) {
	ring.append("[deps] npm install")
	out, err := s.node.NpmInstall(ctx, app.Dir, PYOpTimeout)
	ring.append(out)
	if err != nil {
		e := "依赖安装失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring.appendErr(out), nil)
		return
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, nil)
}

// runNodeDelete Node 删除执行链（D-07 同构）：删除 PM2 进程注册 → 保存进程
// 列表 →〔purgeDir〕删目录 → succeeded + 档案 DELETE 同事务。
func (s *Service) runNodeDelete(ctx context.Context, task storeapp.AppTask, app storeapp.App, ring *outputRing) {
	ring.append("[pm2] 删除进程 " + app.UnitName)
	if out, err := s.pm2.Delete(ctx, app.UnitName); err != nil {
		// 进程已不存在（面板外删除）不阻断：注册已失即达目的。
		ring.append("[warn] 删除进程返回失败（保守继续）：" + err.Error())
	} else {
		ring.append(out)
	}
	ring.append("[pm2] 保存进程列表")
	if out, err := s.pm2.Save(ctx); err != nil {
		ring.append("[warn] 保存进程列表失败（开机后该进程可能仍被恢复，可在服务器执行 pm2 save）：" + err.Error())
	} else {
		ring.append(out)
	}
	if task.PurgeDir {
		ring.append("[dir] 删除代码目录（purgeDir=true）：" + app.Dir)
		if err := s.py.RemoveDir(app.Dir); err != nil {
			ring.append("[warn] 代码目录删除失败（档案照常撤除，目录残留请手动清理）：" + err.Error())
		}
	} else {
		ring.append("[dir] 代码目录保留：" + app.Dir)
	}
	ring.append("[note] 关联站点不受影响（如已建站请到站点管理自行处置）")
	archive := func(ctx context.Context, tx *storeapp.TxStore) error {
		return tx.App().Delete(ctx, app.ID)
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, archive)
}

// ---- 服务操作（D-10 瞬时通道的 Node 分支；D-17 开机自启语义）----

// nodeServiceOp Node 服务操作：start/stop/restart 经 PM2；enable=启动并保存
// 进程列表、disable=停止并保存进程列表（会改变当前运行状态——与 systemd
// 的 enable/disable 不同，界面带说明）。
func (s *Service) nodeServiceOp(ctx context.Context, app storeapp.App, op string) error {
	name := app.UnitName
	if op == "enable" || op == "disable" {
		var err error
		if op == "enable" {
			_, err = s.pm2.StartByName(ctx, name)
		} else {
			_, err = s.pm2.Stop(ctx, name)
		}
		if err != nil {
			return errUnitOp(err.Error())
		}
		if _, err := s.pm2.Save(ctx); err != nil {
			return errUnitOp("保存进程列表失败，开机自启设置未生效：" + err.Error())
		}
		return nil
	}
	var err error
	switch op {
	case "start":
		_, err = s.pm2.StartByName(ctx, name)
	case "stop":
		_, err = s.pm2.Stop(ctx, name)
	case "restart":
		_, err = s.pm2.Restart(ctx, name)
	default:
		return errParams("未知服务操作")
	}
	if err != nil {
		return errUnitOp(err.Error())
	}
	return nil
}

// nodeReregister 启动方式变更后以新参数重新注册 PM2 进程（PM2 进程的启动参数
// 在注册时固定，改参数必须重新注册）。保留原运行态与开机自启设置：先记下
// 「原是否运行」「保存文件里是否为 online」→ 注销 → 注册 → 原为停止则停止 →
// 原开机自启开启则重新保存。运行中的进程会随之重启。
// 顺序上先把新注册所需资源（启动器、node/npm 路径）备齐，再动现有进程，尽量
// 不出现「已注销但注册不上」的中间态。
func (s *Service) nodeReregister(ctx context.Context, app storeapp.App, sp StartSpec) error {
	if s.node == nil || s.pm2 == nil || s.dataDir == "" {
		return errUnitOp("Node 运行时通道未装配")
	}
	launcher, err := pmx.EnsureLauncher(filepath.Join(s.dataDir, "app"))
	if err != nil {
		return err
	}
	nodeBin, npmBin, err := s.node.ResolveBins(ctx)
	if err != nil {
		return err
	}
	wasOnline := false
	if procs, lerr := s.pm2.List(ctx); lerr == nil {
		for _, p := range procs {
			if p.Name == app.UnitName {
				wasOnline = p.Status == "online"
			}
		}
	}
	enabled := false
	if saved, found, serr := s.pm2.SavedStatus(app.UnitName); serr == nil {
		enabled = found && saved == "online"
	}
	// 注销现有进程。本就未注册时 pm2 会报错，属预期；若因其它原因未能注销，
	// 随后的注册会以「同名进程已存在」失败并如实返回——故此处不判注销错误。
	_, _ = s.pm2.Delete(ctx, app.UnitName)
	if _, err := s.pm2.Start(ctx, pmx.StartSpec{Name: app.UnitName, Cwd: app.Dir, Launcher: launcher,
		NodeBin: nodeBin, Args: launcherArgs(app.Dir, nodeBin, npmBin, app.StartMode, sp)}); err != nil {
		return err
	}
	if !wasOnline {
		if _, err := s.pm2.Stop(ctx, app.UnitName); err != nil {
			return err
		}
	}
	if enabled {
		if _, err := s.pm2.Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

// nodeLogTail Node 日志：PM2 日志文件尾部（out 与 error 两路合并）。
func (s *Service) nodeLogTail(ctx context.Context, app storeapp.App, lines int) (string, error) {
	procs, err := s.pm2.List(ctx)
	if err != nil {
		return "", errUnitOp("日志查询失败：" + err.Error())
	}
	for _, p := range procs {
		if p.Name != app.UnitName {
			continue
		}
		out, err := s.pm2.Tail(ctx, p.OutLogPath, p.ErrLogPath, lines)
		if err != nil {
			return "", errUnitOp("日志查询失败：" + err.Error())
		}
		return strings.Join(out, "\n"), nil
	}
	return "", errUnitOp("日志查询失败：PM2 进程未注册（可能已在面板外删除）")
}

// nodeProbe Node 实况探测：进程未注册时全部 null（如实区分「未注册」与
// 「已停止」）；探测失败亦 null。
func (s *Service) nodeProbe(ctx context.Context, app storeapp.App) (status *string, running, enabled *bool, restarts *int, mem *int64) {
	procs, err := s.pm2.List(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil
	}
	for _, p := range procs {
		if p.Name != app.UnitName {
			continue
		}
		st := p.Status
		status = &st
		r := st == "online"
		running = &r
		rs := p.Restarts
		restarts = &rs
		m := p.MemoryBytes
		mem = &m
		if saved, found, serr := s.pm2.SavedStatus(app.UnitName); serr == nil {
			e := found && saved == "online"
			enabled = &e
		}
		return status, running, enabled, restarts, mem
	}
	return nil, nil, nil, nil, nil
}

// nodeDepsInstalled Node 依赖就绪度：有 package.json 时看 node_modules 是否
// 存在；无 package.json 为 null（不适用）。
func nodeDepsInstalled(dir string) *bool {
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return nil
	}
	_, err := os.Stat(filepath.Join(dir, "node_modules"))
	v := err == nil
	return &v
}
