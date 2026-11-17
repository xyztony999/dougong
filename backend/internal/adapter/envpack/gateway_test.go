// QueryInstalled 判定口径测试（D-14，api-env-mgmt.md v1.1）：伪执行器注入
// （runCmd 注入点），断言环境无关——CI runner 的真实 dpkg/rpm 数据库状态
// 不参与测试（第 5 起环境耦合预防；沿 NewGatewayWithProbe 注入先例）。
// 含 W1 实机缺陷 2 回归：dpkg-query「no packages found matching」=未装非错误。
package envpack

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRun 按 (name, 末参=包名) 返回预置输出/错误；并记录 Env 注入供断言。
type fakeRun struct {
	out map[string]string // "dpkg-query:pkg" / "rpm:pkg" → stdout+stderr 合并输出
	err map[string]error  // 同键 → 执行错误（退出码非 0 语义）
	env map[string]string // 最后一次调用的 Env 注入捕获
}

func (f *fakeRun) run(_ context.Context, env map[string]string, name string, args []string) (string, error) {
	f.env = env
	key := name + ":" + args[len(args)-1]
	if e, ok := f.err[key]; ok {
		return f.out[key], e
	}
	if o, ok := f.out[key]; ok {
		return o, nil
	}
	return "", errors.New("fakeRun: unexpected call " + key)
}

func TestQueryInstalledApt(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		err      error
		wantVer  string
		wantInst bool
		wantErr  bool
	}{
		{"installed", "install ok installed 1.24.0-1\n", nil, "1.24.0-1", true, false},
		{"held installed", "hold ok installed 8.0.36-0ubuntu0.22.04.1", nil, "8.0.36-0ubuntu0.22.04.1", true, false},
		{"config-files remains（remove 未 purge）", "deinstall ok config-files 5.0.7", nil, "", false, false},
		{"not-installed status", "unknown ok not-installed", nil, "", false, false},
		{"no packages found matching（W1 缺陷 2 回归）",
			"dpkg-query: no packages found matching docker.io\n", errors.New("exit status 1"), "", false, false},
		{"real error（超时/IO）", "", errors.New("signal: killed"), "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRun{out: map[string]string{"dpkg-query:docker.io": tc.out},
				err: map[string]error{"dpkg-query:docker.io": tc.err}}
			g := NewGatewayWithRunners(nil, fr.run)
			ver, inst, err := g.QueryInstalled(context.Background(), "apt", "docker.io")
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if inst != tc.wantInst || ver != tc.wantVer {
				t.Fatalf("got (%q,%v), want (%q,%v)", ver, inst, tc.wantVer, tc.wantInst)
			}
			if fr.env["LC_ALL"] != "C" {
				t.Fatalf("LC_ALL=C not injected (D-14): %v", fr.env)
			}
		})
	}
}

func TestQueryInstalledRpm(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		err      error
		wantVer  string
		wantInst bool
		wantErr  bool
	}{
		{"installed", "8.0.36-1.el9", nil, "8.0.36-1.el9", true, false},
		{"not installed", "package docker-ce is not installed\n", errors.New("exit status 1"), "", false, false},
		{"real error", "error reading rpm database", errors.New("exit status 2"), "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRun{out: map[string]string{"rpm:docker-ce": tc.out},
				err: map[string]error{"rpm:docker-ce": tc.err}}
			g := NewGatewayWithRunners(nil, fr.run)
			ver, inst, err := g.QueryInstalled(context.Background(), "dnf", "docker-ce")
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if inst != tc.wantInst || ver != tc.wantVer {
				t.Fatalf("got (%q,%v), want (%q,%v)", ver, inst, tc.wantVer, tc.wantInst)
			}
		})
	}
}

func TestQueryInstalledChannelUnavailable(t *testing.T) {
	g := NewGateway()
	if _, _, err := g.QueryInstalled(context.Background(), "", "nginx"); err == nil ||
		!strings.Contains(err.Error(), "channel unavailable") {
		t.Fatalf("want channel unavailable error, got %v", err)
	}
}

// TestQueryInstalledArgvShape R-08 回归：参数为字面量数组（-W -f=${Status}
// ${Version} 逐元素），无 shell 拼接。
func TestQueryInstalledArgvShape(t *testing.T) {
	var gotName string
	var gotArgs []string
	fr := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		gotName, gotArgs = name, args
		return "install ok installed 1.0", nil
	}
	g := NewGatewayWithRunners(nil, fr)
	if _, _, err := g.QueryInstalled(context.Background(), "apt", "nginx"); err != nil {
		t.Fatal(err)
	}
	if gotName != "dpkg-query" || len(gotArgs) != 3 || gotArgs[0] != "-W" ||
		gotArgs[1] != "-f=${Status} ${Version}" || gotArgs[2] != "nginx" {
		t.Fatalf("argv shape = %s %v", gotName, gotArgs)
	}
}

// TestInstallAptUnattendedShape 无人值守升级命令形态（v0.1.3 实机教训回归：
// dpkg conffile 交互提示打崩无 stdin 的面板进程）——apt-get + DEBIAN_FRONTEND
// + --force-confdef/--force-confold（本地已改配置保持现版，新版落 .dpkg-dist）。
func TestInstallAptUnattendedShape(t *testing.T) {
	var gotName string
	var gotArgs []string
	var gotEnv map[string]string
	fr := func(_ context.Context, env map[string]string, name string, args []string) (string, error) {
		gotEnv, gotName, gotArgs = env, name, args
		return "", nil
	}
	g := NewGatewayWithRunners(nil, fr)
	if _, err := g.Install(context.Background(), "apt", "nginx"); err != nil {
		t.Fatal(err)
	}
	if gotName != "apt-get" {
		t.Fatalf("bin = %s, want apt-get", gotName)
	}
	if gotEnv["DEBIAN_FRONTEND"] != "noninteractive" {
		t.Fatalf("env = %v, want DEBIAN_FRONTEND=noninteractive", gotEnv)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{
		"-o Dpkg::Options::=--force-confdef",
		"-o Dpkg::Options::=--force-confold",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if !strings.Contains(joined, "install nginx") {
		t.Fatalf("args %q missing 'install nginx'", joined)
	}
}

// TestRefreshIndexUsesAptGet 脚本场景用 apt-get（apt 自警无稳定 CLI 接口）。
func TestRefreshIndexUsesAptGet(t *testing.T) {
	var gotName string
	var gotArgs []string
	fr := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		gotName, gotArgs = name, args
		return "", nil
	}
	g := NewGatewayWithRunners(nil, fr)
	if _, err := g.RefreshIndex(context.Background(), "apt"); err != nil {
		t.Fatal(err)
	}
	if gotName != "apt-get" || len(gotArgs) != 1 || gotArgs[0] != "update" {
		t.Fatalf("refresh = %s %v, want apt-get [update]", gotName, gotArgs)
	}
}
