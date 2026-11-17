package dbmysql

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// connArgs 连接参数组装（v0.1.4 修复轮）：tcp 模式必须显式 --protocol=TCP——
// mysql 客户端对 -h localhost 特例走编译默认 Unix socket 并忽略 -P，tcp 语义
// 会被静默改道（容器/远端 TCP 实例直连错误通道）；socket 模式不受影响。
func TestConnArgsProtocolTCP(t *testing.T) {
	tcp := connArgs(dbconn.Target{ConnMode: "tcp", Host: "localhost", Port: 3306})
	joined := strings.Join(tcp, " ")
	if !strings.Contains(joined, "--protocol=TCP") {
		t.Fatalf("tcp args must force --protocol=TCP (localhost socket special-case), got %v", tcp)
	}
	if !strings.Contains(joined, "-h localhost") || !strings.Contains(joined, "-P 3306") {
		t.Fatalf("tcp args must carry host/port, got %v", tcp)
	}

	remote := connArgs(dbconn.Target{ConnMode: "tcp", Host: "10.0.0.8", Port: 3307})
	if !strings.Contains(strings.Join(remote, " "), "--protocol=TCP") {
		t.Fatalf("remote tcp args must also force --protocol=TCP, got %v", remote)
	}

	sock := connArgs(dbconn.Target{ConnMode: "socket", SocketPath: "/run/mysqld/mysqld.sock"})
	sj := strings.Join(sock, " ")
	if strings.Contains(sj, "--protocol=TCP") || strings.Contains(sj, "-h ") {
		t.Fatalf("socket args must not carry tcp flags, got %v", sock)
	}
	if !strings.Contains(sj, "--socket /run/mysqld/mysqld.sock") {
		t.Fatalf("socket args must carry socket path, got %v", sock)
	}
}

// classifyRun 回归：认证被拒/连接失败文本分类不变；密码含 @ 等特殊字符不是
// 分类输入（密码经 MYSQL_PWD env 注入子进程，不进 argv/DSN——通道无拼接面，
// v0.1.4 用户反馈「@ 破坏 DSN」假设对 CLI 通道不成立的实证锚点）。
func TestClassifyRunSentinels(t *testing.T) {
	auth := classifyRun(fmt.Errorf("exit 1"),
		"ERROR 1045 (28000): Access denied for user 'tonyxyz'@'localhost' (using password: YES)", "")
	if !errors.Is(auth, ErrAuthRejected) {
		t.Fatalf("access denied want ErrAuthRejected, got %v", auth)
	}
	conn := classifyRun(fmt.Errorf("exit 1"),
		"ERROR 2002 (HY000): Can't connect to MySQL server on 'localhost:3306' (111)", "")
	if !errors.Is(conn, ErrConnFailed) {
		t.Fatalf("cant connect want ErrConnFailed, got %v", conn)
	}
	// 未分类失败（如超时 signal: killed）→ ErrQueryFailed（service 层凭据验证
	// 语境归 DBM_006，不再裸 INTERNAL）。
	other := classifyRun(fmt.Errorf("signal: killed"), "", "")
	if !errors.Is(other, ErrQueryFailed) {
		t.Fatalf("unclassified want ErrQueryFailed, got %v", other)
	}
}
