package dbmongo

// 适配器纯函数测试（CI 环境耦合预防第 6 起：不依赖 runner 上有无 mongod/
// 27017 实况——连接行为经 service 层 fakeMongo 替身覆盖，本文件仅测分类与
// URI 组装纯逻辑）。

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoRoleValidTable(t *testing.T) {
	for _, ok := range []string{"read", "readWrite", "dbAdmin"} {
		if !MongoRoleValid(ok) {
			t.Errorf("%q must be valid", ok)
		}
	}
	for _, bad := range []string{"", "root", "readWrite ", "READ"} {
		if MongoRoleValid(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}

// connectURI：tcp=host:port 直组；socket=路径整体 percent 编码（驱动约定——
// / 须转义为 %2F）；凭据不进 URI（options.Credential 结构化传递）——@ 密码
// 无 URI 转义风险的实证锚点（D-15/D-16）。
func TestConnectURI(t *testing.T) {
	if got := connectURI(dbconn.Target{ConnMode: "tcp", Host: "localhost", Port: 27017}); got != "mongodb://localhost:27017" {
		t.Fatalf("tcp uri = %q", got)
	}
	if got := connectURI(dbconn.Target{ConnMode: "tcp", Host: "127.0.0.1", Port: 27018}); got != "mongodb://127.0.0.1:27018" {
		t.Fatalf("tcp uri = %q", got)
	}
	sock := connectURI(dbconn.Target{ConnMode: "socket", SocketPath: "/tmp/mongodb-27017.sock"})
	if sock != "mongodb://%2Ftmp%2Fmongodb-27017.sock" {
		t.Fatalf("socket uri must percent-encode path, got %q", sock)
	}
	if strings.Contains(sock, "/tmp/mongodb-27017.sock") {
		t.Fatalf("raw path must not appear in uri: %q", sock)
	}
}

// classify：错误码优先（18/13/11/51003/其余 CommandError）；认证失败文本穿透
// （ServerSelectionError 可能包裹认证失败）；连接层文本与 net.Error/超时兜底。
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"code 18 auth", mongo.CommandError{Code: 18, Message: "Authentication failed."}, ErrAuthRejected},
		{"code 13 unauthorized", mongo.CommandError{Code: 13, Message: "not authorized on admin"}, ErrUnauthorized},
		{"code 11 user not found", mongo.CommandError{Code: 11, Message: "User 'x@admin' not found"}, ErrUserNotFound},
		{"code 51003 user exists", mongo.CommandError{Code: 51003, Message: "User x@admin already exists"}, ErrUserExists},
		{"other command error", mongo.CommandError{Code: 59, Message: "CommandNotFound"}, ErrQueryFailed},
		{"auth text passthrough", errors.New("server selection error: context deadline exceeded, auth error (AuthenticationFailed)"), ErrAuthRejected},
		{"server selection", errors.New("server selection error: context deadline exceeded, current topology"), ErrConnFailed},
		{"connection refused", errors.New("dial tcp 127.0.0.1:27017: connect: connection refused"), ErrConnFailed},
		{"timed out", errors.New("connection(...) incomplete read of message header: timed out"), ErrConnFailed},
		{"net error", netOpError{}, ErrConnFailed},
		{"deadline", context.DeadlineExceeded, ErrConnFailed},
		{"unknown", errors.New("boom"), ErrQueryFailed},
	}
	for _, c := range cases {
		got := classify(c.err)
		if !errors.Is(got, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
	if classify(nil) != nil {
		t.Fatal("classify(nil) must be nil")
	}
}

type netOpError struct{}

func (netOpError) Error() string   { return "write tcp: broken pipe" }
func (netOpError) Timeout() bool   { return false }
func (netOpError) Temporary() bool { return false }

var _ net.Error = netOpError{}

// 通道可用性：进程内库恒可用（区别于 dbmysql/dbredis 的二进制 LookPath）。
func TestGatewayAvailableAlways(t *testing.T) {
	g := NewGateway()
	if err := g.Available(); err != nil {
		t.Fatalf("in-process channel must always be available, got %v", err)
	}
}
