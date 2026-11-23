// panelcompose.go —— docker-mgmt 的面板落盘根扫描（ADR-033 D-app-07）：
// Compose 列表 = 容器 label 聚合（面板外项目）∪ 落盘根目录扫描（面板创建的
// 项目）。此前列表只按容器 label 聚合，项目 down/stop 后零容器即不可见、无法
// 再启动、项目名被目录占住也无从清理（D0 Q4）。零持久化不变：事实源是文件
// 系统与 daemon。
package dockermgmt

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"gopkg.in/yaml.v3"
)

const (
	// appServiceName 应用编排的服务名（单服务）。
	appServiceName = "app"
)

// appEnvKeyRe 应用环境变量键（环境变量名惯例：字母或下划线开头）。
var appEnvKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// panelCompose 落盘根下某项目目录的编排文件解析结果。
type panelCompose struct {
	project       string // 目录名 = compose 项目名
	workingDir    string
	configFile    string
	parsed        bool   // 编排文件可解析
	isApp         bool   // 含 dougong.app 标签 → 归「应用」页签
	appName       string // 标签值（应用名；无标签时为空）
	image         string // 应用编排的镜像
	hostPort      int    // 应用编排首个端口映射的主机端口（0=未知）
	containerPort int
}

// scanPanelRoot 扫描落盘根：其下每个含编排文件的一级目录视为一个面板项目
// （目录名即项目名）。根不存在=空清单（未创建过任何项目），不报错。
// 编排文件解析失败的目录仍作为项目呈现（未运行、可删除），不当作应用。
func scanPanelRoot(root string) ([]panelCompose, error) {
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取落盘根失败：%w", err)
	}
	var out []panelCompose
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		project := e.Name()
		if !composeProjectRe.MatchString(project) {
			continue // 不符合面板命名规则的目录不认领（不展示、不操作）
		}
		dir := filepath.Join(root, project)
		cfg := filepath.Join(dir, ComposeFileName)
		if _, err := os.Stat(cfg); err != nil {
			continue
		}
		pc, perr := parsePanelCompose(project, dir, cfg)
		if perr != nil {
			out = append(out, panelCompose{project: project, workingDir: dir, configFile: cfg})
			continue
		}
		out = append(out, pc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].project < out[j].project })
	return out, nil
}

// parsePanelCompose 解析编排文件：识别归属标签与应用编排的镜像、端口映射。
// 标签既可写成映射（面板生成的应用编排）也可写成列表（手写常见），两种都认；
// 端口只认短语法字符串，其它形态按未知（0）处理——不因形态问题把整个文件
// 判为解析失败。
func parsePanelCompose(project, workingDir, configFile string) (panelCompose, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return panelCompose{}, err
	}
	var doc struct {
		Services map[string]struct {
			Image  string      `yaml:"image"`
			Ports  []yaml.Node `yaml:"ports"`
			Labels yaml.Node   `yaml:"labels"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return panelCompose{}, err
	}
	pc := panelCompose{project: project, workingDir: workingDir, configFile: configFile, parsed: true}
	// 服务按名排序遍历，保证「首个带标签的服务」的选取与映射迭代顺序无关。
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := doc.Services[name]
		label := labelValue(svc.Labels, AppOwnershipLabel)
		if label == "" {
			continue
		}
		pc.isApp = true
		pc.appName = label
		pc.image = svc.Image
		if len(svc.Ports) > 0 && svc.Ports[0].Kind == yaml.ScalarNode {
			pc.hostPort, pc.containerPort = parseComposePort(svc.Ports[0].Value)
		}
		break // 单服务应用编排：首个带标签的服务即应用本体
	}
	return pc, nil
}

// labelValue 从 labels 节点取指定键的值（映射与列表两种写法）。
func labelValue(n yaml.Node, key string) string {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if strings.TrimSpace(n.Content[i].Value) == key {
				return strings.TrimSpace(n.Content[i+1].Value)
			}
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if k, v, found := strings.Cut(item.Value, "="); found && strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// parseComposePort 解析 ports 短语法「[hostIp:]hostPort:containerPort」的两段
// 端口；其它形态（映射语法、范围）返回 0,0（未知）。
func parseComposePort(entry string) (host, container int) {
	parts := strings.Split(strings.TrimSpace(entry), ":")
	if len(parts) < 2 {
		return 0, 0
	}
	h, err1 := strconv.Atoi(parts[len(parts)-2])
	c, err2 := strconv.Atoi(parts[len(parts)-1])
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return h, c
}

// listProjects 合并两路事实源：容器 label 聚合（权威运行态，含面板外项目）
// ∪ 落盘根扫描（补齐零容器的面板项目）。带应用标签的项目不进 Compose 列表
// （归「应用」页签）。落盘扫描失败不阻断 label 聚合结果——降级为旧行为。
func (s *Service) listProjects(ctx context.Context) ([]dockerx.ComposeProjectFact, error) {
	facts, err := s.comp.ListProjectFacts(ctx)
	if err != nil {
		return nil, err
	}
	panel, perr := scanPanelRoot(s.composeRoot)
	if perr != nil {
		slog.Warn("dockermgmt: compose root scan failed", "root", s.composeRoot, "err", perr)
		panel = nil
	}
	apps := map[string]bool{}
	idx := make(map[string]int, len(facts))
	for i := range facts {
		idx[facts[i].Project] = i
	}
	for _, pc := range panel {
		if pc.isApp {
			apps[pc.project] = true
			continue
		}
		if i, ok := idx[pc.project]; ok {
			facts[i].PanelManaged = true
			continue
		}
		facts = append(facts, dockerx.ComposeProjectFact{
			Project: pc.project, WorkingDir: pc.workingDir, ConfigFile: pc.configFile,
			Locatable: true, PanelManaged: true})
	}
	// 面板应用（带标签）不进 Compose 列表。既有 label 聚合结果的顺序保持不变
	// （真实适配器已按项目名排序）；落盘扫描补进来的零容器项目追加在末尾。
	kept := make([]dockerx.ComposeProjectFact, 0, len(facts))
	for _, f := range facts {
		if !apps[f.Project] {
			kept = append(kept, f)
		}
	}
	return kept, nil
}

// hostPortInUse 主机端口占用探测：能对回环地址的该端口发起监听即视为空闲。
// 监听失败一律按占用处理（宁可拒绝创建，也不让编排起来后因端口冲突失败）。
func hostPortInUse(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return true
	}
	_ = l.Close()
	return false
}
