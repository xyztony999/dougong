package panel

// 构建期版本信息（FR-03：ldflags 注入；api 4.1.4 GET /panel/system/version
// 数据源）。经 -X 覆盖：
//
//	-X github.com/dougong/dougong/internal/service/panel.buildVersion=1.2.0
//	-X github.com/dougong/dougong/internal/service/panel.buildGitCommit=abc123…
//	-X github.com/dougong/dougong/internal/service/panel.buildTime=2026-11-14T00:00:00Z
var (
	buildVersion   = "dev"
	buildGitCommit = "none"
	buildTime      = "unknown"
)

// VersionInfo 是版本与构建信息（data={version, gitCommit, buildTime}）。
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildTime string `json:"buildTime"`
}

// GetVersionInfo 返回构建期注入的版本信息。
func GetVersionInfo() VersionInfo {
	return VersionInfo{Version: buildVersion, GitCommit: buildGitCommit, BuildTime: buildTime}
}
