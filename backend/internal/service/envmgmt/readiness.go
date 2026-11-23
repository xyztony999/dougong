package envmgmt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// 就绪度探测不可确认的内部哨兵（调用方按「探测不可用」如实呈现，不与未装混同）。
var errProbeUnavailable = errors.New("envmgmt: python readiness probe unavailable")

func errInvalidPythonCatalog() error { return fmt.Errorf("envmgmt: python entry missing in catalog") }
func errChannelDown() error          { return fmt.Errorf("envmgmt: package channel unavailable") }
func errProbeFailed() error          { return errProbeUnavailable }

// PythonReadiness Python 运行环境就绪度（v1.4 环境实况读契约 OHS 提供方，
// api-env-mgmt.md v1.4 §5.1 / FR-09② ⚠️A18；消费方 app-mgmt，api-app-mgmt
// D-08——进程内单向读，无 HTTP 绕行）。分层字段：Installed（python3 候选任一
// 命中）/ Version（python3 --version，缺席 null 如实）/ VenvSupport（venv 模块
// 能力位——Debian 系 python3-venv 单独包）/ PipAvailable（pip 能力位）。
type PythonReadiness struct {
	Installed    bool
	Version      string
	VenvSupport  bool
	PipAvailable bool
}

// PythonReadiness 探测 Python 运行环境就绪度（env_type=python 目录条目，
// ⚠️A17 同构：无服务单元不涉及）。就绪度不可确认时 Installed=false 且
// ProbeOK=false 语义经 error 承载（调用方如实呈现「探测不可用」不虚构）。
func (s *Service) PythonReadiness(ctx context.Context) (PythonReadiness, error) {
	entry := CatalogByType(storeenv.EnvPython)
	if entry == nil {
		return PythonReadiness{}, errInvalidPythonCatalog()
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		return PythonReadiness{}, errChannelDown()
	}
	_, _, found, probeOK := s.installedPkg(ctx, entry, ch.Family)
	if !probeOK {
		return PythonReadiness{}, errProbeFailed()
	}
	rd := PythonReadiness{Installed: found}
	if !found {
		return rd, nil // 未装：能力位不探测（分层呈现 missing=python）
	}
	if bv, err := s.pack.BinaryVersion(ctx, "python3"); err == nil {
		rd.Version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bv), "Python"))
	}
	if venvOK, err := s.pack.ProbeVenvSupport(ctx); err == nil {
		rd.VenvSupport = venvOK
	}
	if pipOK, err := s.pack.ProbePipAvailable(ctx); err == nil {
		rd.PipAvailable = pipOK
	}
	return rd, nil
}

// NodeReadiness Node 运行环境就绪度（ADR-033 宝塔式重组，api-app-mgmt v1.2
// D-16；消费方 app-mgmt——进程内单向读，无 HTTP 绕行）。分层字段：
// NodeAvailable（node 可执行）/ NpmAvailable（npm 可执行）/ Pm2Available
// （PM2 可执行，Node 项目进程托管依赖）/ NodeVersion（node --version，
// 缺席空串如实）。Node 可能系统预装，故不依赖包通道判定。
type NodeReadiness struct {
	NodeAvailable bool
	NpmAvailable  bool
	Pm2Available  bool
	NodeVersion   string
}

// NodeReadiness 探测 Node 运行环境就绪度。探测失败逐项 false 如实（不虚构）。
func (s *Service) NodeReadiness(ctx context.Context) (NodeReadiness, error) {
	rd := NodeReadiness{}
	if bv, err := s.pack.BinaryVersion(ctx, "node"); err == nil {
		rd.NodeAvailable = true
		rd.NodeVersion = strings.TrimPrefix(strings.TrimSpace(bv), "v")
	}
	if ok, err := s.pack.ProbeNpmAvailable(ctx); err == nil {
		rd.NpmAvailable = ok
	}
	if ok, err := s.pack.ProbePm2Available(ctx); err == nil {
		rd.Pm2Available = ok
	}
	return rd, nil
}
