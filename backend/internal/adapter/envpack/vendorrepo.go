// vendorrepo.go —— 第三方官方仓库源代配（D-06 v1.1 补真实现，⚠️A10；v1.0
// 为空转日志）。按 /etc/os-release 判定目标发行版与代际，支持矩阵外组合
// 返回明确错误（任务终态 failed，不写任何文件）；文件写入为 os.WriteFile
// 字面量路径（R-08：文件写入非命令拼接）；写入幂等（内容一致跳过、公钥
// 已存在复用不重复下载）。全部外部依赖（读/写/建目录/HTTP）经 Gateway
// 注入点提供，测试环境无关（CI runner 发行版不参与测试）。
package envpack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// 代配常量（api-env-mgmt.md §3.4 业务常量；模板编译期内置）。
const (
	// mongodbTrack MONGODB_ORG_TRACK：MongoDB 仓库版本轨道（8.0 现行稳定主轨，
	// 用户确认 W1 2026-11-17）。路径段用于 repo.mongodb.org URL 与源文件名。
	mongodbTrack = "8.0"

	vendorKeyringDir = "/usr/share/keyrings"     // VENDOR_KEYRING_DIR（apt signed-by 引用）
	vendorAptListDir = "/etc/apt/sources.list.d" // VENDOR_APT_LIST_DIR
	vendorYumRepoDir = "/etc/yum.repos.d"        // VENDOR_YUM_REPO_DIR

	mongodbKeyURL   = "https://www.mongodb.org/static/pgp/server-" + mongodbTrack + ".asc"
	mongodbKeyPath  = vendorKeyringDir + "/mongodb-server-" + mongodbTrack + ".asc"
	mongodbAptList  = vendorAptListDir + "/mongodb-org-" + mongodbTrack + ".list"
	mongodbYumRepo  = vendorYumRepoDir + "/mongodb-org-" + mongodbTrack + ".repo"
	dockerCEYumRepo = vendorYumRepoDir + "/docker-ce.repo"

	pgpArmorHeader = "-----BEGIN PGP PUBLIC KEY BLOCK-----"
)

// osRelease 是 /etc/os-release 判定结论（发行版身份；官方源支持矩阵输入）。
type osRelease struct {
	ID        string // ubuntu / debian / rhel / rocky / almalinux / centos / ol …
	IDLike    string // 空格分隔族谱（rhel fedora…）
	Codename  string // VERSION_CODENAME（Debian 系代际）
	VersionID string // 主版本（RHEL 系代际，如 "9.4"）
}

// rhelMajor 取 RHEL 系主版本（"9.4"→"9"；非数字前缀返回 ""）。
func (r osRelease) rhelMajor() string {
	major, _, _ := strings.Cut(r.VersionID, ".")
	return major
}

// isRHELFamily RHEL 系判定（ID 或 ID_LIKE 含 rhel/fedora 谱系——install.sh
// 支持矩阵：RHEL 9 系含 Rocky/Alma 等）。
func (r osRelease) isRHELFamily() bool {
	switch r.ID {
	case "rhel", "centos", "rocky", "almalinux", "ol", "scientific":
		return true
	}
	for _, like := range strings.Fields(r.IDLike) {
		if like == "rhel" || like == "fedora" {
			return true
		}
	}
	return false
}

// EnsureVendorRepo 代配第三方官方仓库源（EnvPackGateway 契约，D-06 v1.1；
// worker 在索引刷新前调用——新源须参与本次 update/makecache）。
// 返回 summary 进任务输出尾部；错误=任务终态 failed 语义（不进入安装）。
// envType 取 env_type 值（mongodb/docker）；family 取 package_manager_family。
// 非 Linux 生产路径不可达（通道不可用即 ENV_001 前置拒绝），不设 GOOS 短路——
// 依赖全注入（readFile/writeFile/httpGet），测试环境无关；os-release 读失败
// 即如实报错。
func (g *Gateway) EnsureVendorRepo(ctx context.Context, envType, family string) (string, error) {
	rel, err := g.readOSRelease()
	if err != nil {
		return "", err
	}
	switch {
	case envType == "mongodb":
		switch family {
		case "apt":
			return g.ensureMongoApt(ctx, rel)
		case "dnf", "yum":
			return g.ensureMongoYum(rel)
		}
		return "", fmt.Errorf("envpack: vendor repo unsupported family %q", family)
	case envType == "docker" && (family == "dnf" || family == "yum"):
		return g.ensureDockerCEYum(rel)
	}
	return "（该环境-通道组合无需第三方源，跳过）", nil
}

// ensureMongoApt：Debian 系 mongodb-org 源（Ubuntu：focal/jammy/noble +
// multiverse 组件；Debian：bullseye/bookworm + main）。signed-by 引用面板
// 下载的官方公钥 .asc（Go net/http，不依赖 curl/gpg 外部命令；armored
// .asc 作 signed-by 在 noble（apt 2.7/3.2）实测可用——前提是选项分隔正确）。
func (g *Gateway) ensureMongoApt(ctx context.Context, rel osRelease) (string, error) {
	suite, component := "", ""
	switch rel.ID {
	case "ubuntu":
		suite, component = rel.Codename, "multiverse"
		switch suite {
		case "focal", "jammy", "noble": // MongoDB 8.0 支持代际
		default:
			return unsupportedMsg("mongodb", rel)
		}
	case "debian":
		suite, component = rel.Codename, "main"
		switch suite {
		case "bullseye", "bookworm":
		default:
			return unsupportedMsg("mongodb", rel)
		}
	default:
		return unsupportedMsg("mongodb", rel)
	}
	// v0.1.3b 实机修正：单行源格式多个选项以空格分隔——逗号会被并入 arch
	// 值、signed-by 被静默吞掉 → apt 退回默认信任库 → NO_PUBKEY（实机教训，
	// WSL noble 复现：gpgv 手验 Good signature 而 apt 报公钥不可用）。
	// 组件仅发行版对应组件（ubuntu=multiverse / debian=main），无附加 main。
	opts := "signed-by=" + mongodbKeyPath
	switch runtime.GOARCH { // MongoDB 仅 amd64/arm64；其余架构不限定（apt 自行解析，失败如实）
	case "amd64", "arm64":
		opts = "arch=" + runtime.GOARCH + " " + opts
	}
	content := fmt.Sprintf(
		"deb [%s] https://repo.mongodb.org/apt/%s %s/mongodb-org/%s %s\n",
		opts, rel.ID, suite, mongodbTrack, component)
	listPart, err := g.writeKeyAndList(ctx, content, mongodbAptList, rel.ID+" "+suite)
	if err != nil {
		return "", err
	}
	return "mongodb-org " + mongodbTrack + "（apt）: " + listPart, nil
}

// ensureMongoYum：RHEL 系 mongodb-org 源（$releasever/$basearch 由 dnf·yum
// 展开；gpgkey URL 内联，首装时包管理器导入）。支持主版本 8/9。
func (g *Gateway) ensureMongoYum(rel osRelease) (string, error) {
	if !rel.isRHELFamily() || rel.rhelMajor() != "8" && rel.rhelMajor() != "9" {
		return unsupportedMsg("mongodb", rel)
	}
	content := fmt.Sprintf(`[mongodb-org-%s]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/%s/$basearch/
gpgcheck=1
enabled=1
gpgkey=%s
`, mongodbTrack, mongodbTrack, mongodbKeyURL)
	s, err := g.writeVendorFile(mongodbYumRepo, content)
	if err != nil {
		return "", err
	}
	return "mongodb-org " + mongodbTrack + "（" + rel.ID + " " + rel.rhelMajor() + "）: " + s, nil
}

// ensureDockerCEYum：RHEL 系 docker-ce 源（download.docker.com centos 路径；
// Rocky/Alma 的 $releasever 同构映射）。支持主版本 8/9。
func (g *Gateway) ensureDockerCEYum(rel osRelease) (string, error) {
	if !rel.isRHELFamily() || rel.rhelMajor() != "8" && rel.rhelMajor() != "9" {
		return unsupportedMsg("docker-ce", rel)
	}
	content := `[docker-ce-stable]
name=Docker CE Stable - $basearch
baseurl=https://download.docker.com/linux/centos/$releasever/$basearch/stable
gpgcheck=1
enabled=1
gpgkey=https://download.docker.com/linux/centos/gpg
`
	s, err := g.writeVendorFile(dockerCEYumRepo, content)
	if err != nil {
		return "", err
	}
	return "docker-ce（" + rel.ID + " " + rel.rhelMajor() + "）: " + s, nil
}

// writeKeyAndList apt 双写：公钥（不存在时经 HTTPS 下载，校验 PGP armored
// 头防错页落盘；已存在复用不重复下载）+ 源 list 文件（幂等写入）。
func (g *Gateway) writeKeyAndList(ctx context.Context, listContent, listPath, distro string) (string, error) {
	keyPart := ""
	if _, err := g.readFile(mongodbKeyPath); err == nil {
		keyPart = "公钥已存在（复用 " + mongodbKeyPath + "）"
	} else {
		key, err := g.httpGet(ctx, mongodbKeyURL)
		if err != nil {
			return "", fmt.Errorf("envpack: download mongodb key: %w", err)
		}
		if !strings.Contains(string(key), pgpArmorHeader) {
			return "", fmt.Errorf("envpack: mongodb key content invalid (missing %q)", pgpArmorHeader)
		}
		if err := g.mkdirAll(vendorKeyringDir, 0o755); err != nil {
			return "", fmt.Errorf("envpack: mkdir %s: %w", vendorKeyringDir, err)
		}
		if err := g.writeFile(mongodbKeyPath, key, 0o644); err != nil {
			return "", fmt.Errorf("envpack: write key %s: %w", mongodbKeyPath, err)
		}
		keyPart = "公钥已写入 " + mongodbKeyPath
	}
	if err := g.mkdirAll(vendorAptListDir, 0o755); err != nil {
		return "", fmt.Errorf("envpack: mkdir %s: %w", vendorAptListDir, err)
	}
	listPart, err := g.writeVendorFile(listPath, listContent)
	if err != nil {
		return "", err
	}
	return listPart + "（" + distro + "；" + keyPart + "）", nil
}

// writeVendorFile 幂等写入源文件（内容一致跳过；差异覆盖为模板规范值）。
func (g *Gateway) writeVendorFile(path, content string) (string, error) {
	if old, err := g.readFile(path); err == nil && string(old) == content {
		return "源文件已存在且一致（" + path + "）", nil
	}
	if err := g.writeFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("envpack: write %s: %w", path, err)
	}
	return "已写入 " + path, nil
}

// readOSRelease 解析 /etc/os-release（KEY=VALUE；引号剥离；缺字段为空）。
func (g *Gateway) readOSRelease() (osRelease, error) {
	b, err := g.readFile("/etc/os-release")
	if err != nil {
		return osRelease{}, fmt.Errorf("envpack: read os-release: %w", err)
	}
	rel := osRelease{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			rel.ID = v
		case "ID_LIKE":
			rel.IDLike = v
		case "VERSION_CODENAME":
			rel.Codename = v
		case "VERSION_ID":
			rel.VersionID = v
		}
	}
	if rel.ID == "" {
		return osRelease{}, fmt.Errorf("envpack: os-release missing ID")
	}
	return rel, nil
}

// unsupportedMsg 支持矩阵外组合：明确错误（任务 failed 如实），零文件写入。
func unsupportedMsg(what string, rel osRelease) (string, error) {
	detail := rel.ID + " " + rel.VersionID
	if rel.Codename != "" {
		detail = rel.ID + " " + rel.Codename
	}
	return "", fmt.Errorf("envpack: %s 官方仓库不支持 %s（支持矩阵外，未写入任何源文件）", what, detail)
}

// fetchURL 生产公钥下载（httpGet 注入点默认实现；30s 超时；≤1MB 防错页）。
func fetchURL(ctx context.Context, url string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // teardown 弃错：读取完成后关闭，错误不影响主流程（Gate 2 I5）
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
