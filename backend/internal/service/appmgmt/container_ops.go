// container_ops.go —— app-mgmt v1.1 容器化应用运维通道（⚠️A13）：启停/重启/
// 日志经 `docker compose` CLI（R-08 参数数组，ADR-013），与 venv 形态的
// SysdGateway 分支并存（⚠️A12 双入口）。轻量通道：构造 ComposeProjectFact
// 后经 dockerx.ComposeRunner 既有互斥/超时承载。
package appmgmt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
)

// ComposeOps 是容器化应用的 compose CLI 运维通道契约（接口定义在消费方——
// 实现为 dockerx.ComposeRunner 共享适配层，沿 SysdGateway 共享先例）。
type ComposeOps interface {
	// Up 启动（up -d）。
	Up(ctx context.Context, project, workDir, configFile string) error
	// Down 停止（stop，非 down——保数据卷/网络不销毁）。
	Stop(ctx context.Context, project, workDir, configFile string) error
	// Restart 重启。
	Restart(ctx context.Context, project, workDir, configFile string) error
	// Logs 项目聚合日志尾部。
	Logs(ctx context.Context, project, workDir, configFile string, tail int) ([]string, error)
}

// composeOpsAdapter ComposeOps 适配器（包装 dockerx.ComposeRunner）。
type composeOpsAdapter struct{ runner *dockerx.ComposeRunner }

func AdaptComposeOps(r *dockerx.ComposeRunner) ComposeOps { return &composeOpsAdapter{runner: r} }

func (a *composeOpsAdapter) Up(ctx context.Context, project, workDir, configFile string) error {
	return a.runner.Run(ctx, composeFact(project, workDir, configFile), "up -d")
}
func (a *composeOpsAdapter) Stop(ctx context.Context, project, workDir, configFile string) error {
	return a.runner.Run(ctx, composeFact(project, workDir, configFile), "stop")
}
func (a *composeOpsAdapter) Restart(ctx context.Context, project, workDir, configFile string) error {
	return a.runner.Run(ctx, composeFact(project, workDir, configFile), "restart")
}
func (a *composeOpsAdapter) Logs(ctx context.Context, project, workDir, configFile string, tail int) ([]string, error) {
	return a.runner.Logs(ctx, composeFact(project, workDir, configFile), tail)
}

func composeFact(project, workDir, configFile string) dockerx.ComposeProjectFact {
	return dockerx.ComposeProjectFact{
		Project: project, WorkingDir: workDir,
		ConfigFile: configFile, Locatable: true,
	}
}

// containerServiceOp 容器化形态服务操作（service.go ServiceOp 分支消费）。
func (s *Service) containerServiceOp(ctx context.Context, appName string, op string) error {
	app, err := s.store.App().GetByName(ctx, appName)
	if err != nil {
		return err
	}
	var opsErr error
	switch op {
	case "start":
		opsErr = s.cops.Up(ctx, appName, app.Dir, "docker-compose.yml")
	case "stop":
		opsErr = s.cops.Stop(ctx, appName, app.Dir, "docker-compose.yml")
	case "restart":
		opsErr = s.cops.Restart(ctx, appName, app.Dir, "docker-compose.yml")
	case "enable", "disable":
		// compose 项目自启由 restart: unless-stopped 策略承载，无独立自启开关
		// （⚠️A4：重启策略在创建时设定，运行期不可改）。
		opsErr = nil
	default:
		opsErr = fmt.Errorf("未知容器操作: %s", op)
	}
	if opsErr != nil {
		return errcode.ErrAppUnitOpFailed.WithData(map[string]any{"reason": opsErr.Error()})
	}
	return nil
}

// containerLogs 容器化应用日志（容器日志通道；journalctl 仅 venv 形态）。
func (s *Service) containerLogs(ctx context.Context, appName string, tail int) (string, error) {
	app, err := s.store.App().GetByName(ctx, appName)
	if err != nil {
		return "", err
	}
	lines, err := s.cops.Logs(ctx, appName, app.Dir, "docker-compose.yml", tail)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

var _ = fmt.Sprintf
