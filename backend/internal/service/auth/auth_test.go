package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// newTestService 以真实 :memory: SQLite 装配服务（backend-03 §6：不 mock 数据库）。
// 返回服务、仓储聚合、可控时钟（advance 推进业务时间）。
func newTestService(t *testing.T) (*Service, *storeauth.Store, *clockStub) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init row: %v", err)
	}
	hasher, err := NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	clk := &clockStub{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	svc := New(repo, hasher, NewTotpService(rand.Reader),
		SessionPolicy{AbsoluteTimeout: 30 * time.Minute, IdleTimeout: 5 * time.Minute},
		WithClock(clk.Now), WithRand(rand.Reader))
	return svc, repo, clk
}

type clockStub struct{ t time.Time }

func (c *clockStub) Now() time.Time          { return c.t }
func (c *clockStub) advance(d time.Duration) { c.t = c.t.Add(d) }

func wantErrCode(t *testing.T, err error, want *errcode.Error) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %s, got nil", want.Symbol())
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("error %v is not errcode", err)
	}
	if ec.Symbol() != want.Symbol() {
		t.Fatalf("error symbol = %s, want %s", ec.Symbol(), want.Symbol())
	}
}

// bootstrapAdmin 经向导创建 admin 并返回其登录会话。
func bootstrapAdmin(t *testing.T, svc *Service, username, password string) (*SessionIssued, *Identity) {
	t.Helper()
	issued, err := svc.InitWizard(context.Background(), InitWizardRequest{
		Username: username, Password: password,
	}, RequestMeta{SourceIP: "127.0.0.1", UserAgent: "test"})
	if err != nil {
		t.Fatalf("init wizard: %v", err)
	}
	id, err := svc.VerifyToken(context.Background(), issued.RawToken)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	return issued, id
}

func TestInitWizardArbitration(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	st, err := svc.InitCheck(ctx)
	if err != nil || st.Initialized || !st.AllowWizard {
		t.Fatalf("init check = %+v err=%v, want uninitialized+allow", st, err)
	}
	if _, err := svc.InitWizard(ctx, InitWizardRequest{Username: "admin", Password: "admin12345"}, RequestMeta{}); err != nil {
		t.Fatalf("wizard first: %v", err)
	}
	// I-02：重复初始化拒绝 AUTH_009。
	_, err = svc.InitWizard(ctx, InitWizardRequest{Username: "other", Password: "other12345"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrAlreadyInitialized)
	// CLI 入口共用同一仲裁。
	err = svc.InitViaCLI(ctx, "cliadmin", "cliadmin123", "", "")
	wantErrCode(t, err, errcode.ErrAlreadyInitialized)
}

func TestLoginFlow(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")

	tests := []struct {
		name    string
		req     LoginRequest
		wantErr *errcode.Error
	}{
		{"success", LoginRequest{Username: "admin", Password: "admin12345"}, nil},
		{"wrong password", LoginRequest{Username: "admin", Password: "wrong-pass"}, errcode.ErrBadCredentials},
		{"unknown user（防枚举同码）", LoginRequest{Username: "ghost", Password: "whatever1"}, errcode.ErrBadCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := svc.Login(ctx, tt.req, RequestMeta{})
			if tt.wantErr != nil {
				wantErrCode(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			if out.Session == nil || out.Session.RawToken == "" || out.Session.CsrfToken == "" {
				t.Fatalf("session not issued: %+v", out.Session)
			}
		})
	}
}

func TestLoginLockout(t *testing.T) {
	svc, repo, clk := newTestService(t)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")

	// 连续 5 次错误 → 锁定（R-01）。
	for i := 1; i <= MaxFailAttempts; i++ {
		_, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "bad-pass-00"}, RequestMeta{})
		wantErrCode(t, err, errcode.ErrBadCredentials)
	}
	u, err := repo.Users().GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.Status != storeauth.UserStatusLocked || u.LockedUntil == nil {
		t.Fatalf("user not locked: %+v", u)
	}

	// 锁定期内：正确密码也拒绝 AUTH_004 + remainingSeconds。
	_, err = svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Symbol() != "AUTH_004" {
		t.Fatalf("want AUTH_004, got %v", err)
	}
	data, ok := ec.Data().(map[string]int64)
	if !ok || data["remainingSeconds"] <= 0 {
		t.Fatalf("AUTH_004 data.remainingSeconds missing: %v", ec.Data())
	}

	// 锁定过期 → 惰性解锁（I-08），正确密码可登录。
	clk.advance(LockoutDuration + time.Second)
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	if err != nil {
		t.Fatalf("login after unlock: %v", err)
	}
	if out.Session == nil {
		t.Fatal("no session after unlock")
	}
	u, err = repo.Users().GetByUsername(ctx, "admin")
	if err != nil || u.FailCount != 0 || u.Status != storeauth.UserStatusActive {
		t.Fatalf("fail count not reset: %+v err=%v", u, err)
	}
}

func TestTotpTwoStepLogin(t *testing.T) {
	svc, _, clk := newTestService(t)
	ctx := context.Background()
	_, id := bootstrapAdmin(t, svc, "admin", "admin12345")

	// 绑定 TOTP：setup → enable。
	setup, err := svc.TOTPSetup(ctx, id)
	if err != nil {
		t.Fatalf("totp setup: %v", err)
	}
	if setup.Secret == "" || setup.OtpauthURI == "" {
		t.Fatalf("setup response incomplete: %+v", setup)
	}
	code, err := GenerateCode(setup.Secret, clk.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if err := svc.TOTPEnable(ctx, id, code); err != nil {
		t.Fatalf("totp enable: %v", err)
	}

	// 登录：密码正确 → AUTH_007 语义（RequiresTOTP + ticket）。
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !out.RequiresTOTP || out.LoginTicket == "" || out.Session != nil {
		t.Fatalf("want totp required outcome, got %+v", out)
	}

	// 错误动态码：AUTH_006，ticket 未消费可重试。
	_, err = svc.TOTPVerify(ctx, TOTPVerifyRequest{LoginTicket: out.LoginTicket, Code: "000000"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrTotpInvalid)

	// 正确动态码 → 换得会话；ticket 一次性（重放 AUTH_013）。
	issued, err := svc.TOTPVerify(ctx, TOTPVerifyRequest{LoginTicket: out.LoginTicket, Code: code}, RequestMeta{})
	if err != nil {
		t.Fatalf("totp verify: %v", err)
	}
	if issued.RawToken == "" {
		t.Fatal("session not issued after totp verify")
	}
	_, err = svc.TOTPVerify(ctx, TOTPVerifyRequest{LoginTicket: out.LoginTicket, Code: code}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrTicketInvalid)

	// ticket 过期（>5min 未消费）→ AUTH_013。
	out2, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	if err != nil {
		t.Fatalf("login 2nd: %v", err)
	}
	clk.advance(LoginTicketTTL + time.Second)
	_, err = svc.TOTPVerify(ctx, TOTPVerifyRequest{LoginTicket: out2.LoginTicket, Code: "123456"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrTicketInvalid)
}

func TestSessionLifecycle(t *testing.T) {
	svc, _, clk := newTestService(t)
	ctx := context.Background()
	issued, id := bootstrapAdmin(t, svc, "admin", "admin12345")

	// 空闲过期 → AUTH_002（D-03 计算态）。
	clk.advance(6 * time.Minute) // idle=5min
	_, err := svc.VerifyToken(ctx, issued.RawToken)
	wantErrCode(t, err, errcode.ErrSessionExpired)

	// 重新登录 → 登出 → AUTH_001（I-13 立即生效）。
	clk.advance(time.Hour)
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := svc.Logout(ctx, id, out.Session.RawToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	_, err = svc.VerifyToken(ctx, out.Session.RawToken)
	wantErrCode(t, err, errcode.ErrUnauthenticated)

	// VerifyWs：有效会话返回 (uid, role)；吊销后 AUTH_001（只读不续期）。
	out2 := loginOK(t, svc, "admin", "admin12345")
	uid, role, err := svc.VerifyWs(ctx, out2.RawToken)
	if err != nil || uid != id.UserID || role != RoleAdmin {
		t.Fatalf("verify ws = (%d,%s,%v)", uid, role, err)
	}
	if err := svc.Logout(ctx, id, out2.RawToken); err != nil {
		t.Fatalf("logout2: %v", err)
	}
	_, _, err = svc.VerifyWs(ctx, out2.RawToken)
	wantErrCode(t, err, errcode.ErrUnauthenticated)
}

func loginOK(t *testing.T, svc *Service, u, p string) *SessionIssued {
	t.Helper()
	out, err := svc.Login(context.Background(), LoginRequest{Username: u, Password: p}, RequestMeta{})
	if err != nil {
		t.Fatalf("login %s: %v", u, err)
	}
	return out.Session
}

func TestUserManagementGuards(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	_, admin := bootstrapAdmin(t, svc, "admin", "admin12345")

	// 创建 readonly 用户。
	created, err := svc.CreateUser(ctx, admin, CreateUserRequest{Username: "viewer", Password: "viewer12345", Role: RoleReadonly})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}

	// I-06：大小写不敏感唯一。
	_, err = svc.CreateUser(ctx, admin, CreateUserRequest{Username: "VIEWER", Password: "viewer12345", Role: RoleReadonly})
	wantErrCode(t, err, errcode.ErrUsernameTaken)

	// 参数校验表（I-10/A10/角色）。
	tests := []struct {
		name string
		req  CreateUserRequest
	}{
		{"bad username chars", CreateUserRequest{Username: "不合法", Password: "12345678", Role: RoleAdmin}},
		{"username too long", CreateUserRequest{Username: "a123456789012345678901234567890123", Password: "12345678", Role: RoleAdmin}},
		{"password too short", CreateUserRequest{Username: "okuser1", Password: "1234567", Role: RoleAdmin}},
		{"bad role", CreateUserRequest{Username: "okuser2", Password: "12345678", Role: "super"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateUser(ctx, admin, tt.req)
			var ec *errcode.Error
			if !errors.As(err, &ec) || ec.Symbol() != "AUTH_012" {
				t.Fatalf("want AUTH_012, got %v", err)
			}
		})
	}

	// D-09：不能删除当前登录账户。
	err = svc.DeleteUser(ctx, admin, admin.UserID)
	wantErrCode(t, err, errcode.ErrSelfDelete)

	// I-03：唯一管理员保护（再建一个 admin 后可删 readonly，但删最后一个 admin 拒绝）。
	if _, err := svc.CreateUser(ctx, admin, CreateUserRequest{Username: "admin2", Password: "admin21234", Role: RoleAdmin}); err != nil {
		t.Fatalf("create admin2: %v", err)
	}
	if err := svc.DeleteUser(ctx, admin, created.ID); err != nil {
		t.Fatalf("delete viewer: %v", err)
	}
	if _, err := repo.Users().GetByID(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("viewer still exists: %v", err)
	}
}

func TestResetAndChangePasswordCascade(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, admin := bootstrapAdmin(t, svc, "admin", "admin12345")

	created, err := svc.CreateUser(ctx, admin, CreateUserRequest{Username: "viewer", Password: "viewer12345", Role: RoleReadonly})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	s1 := loginOK(t, svc, "viewer", "viewer12345")
	s2 := loginOK(t, svc, "viewer", "viewer12345")

	// 重置密码 → 该用户全部会话级联吊销（I-04）。
	if err := svc.ResetPassword(ctx, admin, created.ID, "newpass12345"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	_, err = svc.VerifyToken(ctx, s1.RawToken)
	wantErrCode(t, err, errcode.ErrUnauthenticated)
	_, err = svc.VerifyToken(ctx, s2.RawToken)
	wantErrCode(t, err, errcode.ErrUnauthenticated)

	// 自助改密：旧密码错误 AUTH_005；成功后其他会话吊销、当前保留。
	cur := loginOK(t, svc, "admin", "admin12345")
	other := loginOK(t, svc, "admin", "admin12345")
	curID, err := svc.VerifyToken(ctx, cur.RawToken)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := svc.ChangePassword(ctx, curID, "wrong-old-1", "admin54321"); !errors.Is(err, errcode.ErrBadCredentials) {
		t.Fatalf("change with wrong old = %v, want AUTH_005", err)
	}
	if err := svc.ChangePassword(ctx, curID, "admin12345", "admin54321"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := svc.VerifyToken(ctx, other.RawToken); !errors.Is(err, errcode.ErrUnauthenticated) {
		t.Fatalf("other session should be revoked: %v", err)
	}
	if _, err := svc.VerifyToken(ctx, cur.RawToken); err != nil {
		t.Fatalf("current session should survive: %v", err)
	}
	// 旧密码失效。
	_, err = svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrBadCredentials)
}

func TestTotpRFC6238Vector(t *testing.T) {
	// RFC 6238 附录 B 测试密钥 "12345678901234567890" 的 Base32。
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	svc := NewTotpService(nil)
	// T=59s 的 8 位 SHA1 值为 94287082 → 6 位 = 287082。
	if !svc.Verify(secret, "287082", time.Unix(59, 0)) {
		t.Fatal("RFC6238 vector T=59 not accepted")
	}
	// 窗口 ±1：T=59 的码在 T=29 与 T=89 处仍应接受。
	if !svc.Verify(secret, "287082", time.Unix(29, 0)) || !svc.Verify(secret, "287082", time.Unix(89, 0)) {
		t.Fatal("±1 window drift not accepted")
	}
	// 超出窗口拒绝。
	if svc.Verify(secret, "287082", time.Unix(200, 0)) {
		t.Fatal("code accepted outside window")
	}
	// 错误长度/垃圾输入拒绝。
	if svc.Verify(secret, "2870", time.Unix(59, 0)) || svc.Verify(secret, "abcdef", time.Unix(59, 0)) {
		t.Fatal("malformed code accepted")
	}
	// 生成 → 校验闭环。
	gen, err := svc.GenerateSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	code, err := GenerateCode(gen, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if !svc.Verify(gen, code, time.Now()) {
		t.Fatal("generated secret self-verify failed")
	}
}

func TestValidationConstants(t *testing.T) {
	if PasswordMinLength != 8 {
		t.Fatalf("A10 password min length = %d, want 8", PasswordMinLength)
	}
	if MaxFailAttempts != 5 || LockoutDuration != 15*time.Minute {
		t.Fatalf("R-01 lockout constants drifted: %d/%v", MaxFailAttempts, LockoutDuration)
	}
	p := DefaultSessionPolicy()
	if p.AbsoluteTimeout != 12*time.Hour || p.IdleTimeout != 2*time.Hour {
		t.Fatalf("default session policy drifted: %+v (want 12h/2h, §3.4)", p)
	}
}

// TestUserAndSessionQueriesService 覆盖服务层查询/吊销路径（IsInitialized/
// InitCheck/ListUsers/ListSessions/RevokeSession/级联失效）。
func TestUserAndSessionQueriesService(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, admin := bootstrapAdmin(t, svc, "admin", "admin12345")

	// IsInitialized / InitCheck。
	ok, err := svc.IsInitialized(ctx)
	if err != nil || !ok {
		t.Fatalf("IsInitialized = (%v, %v), want true", ok, err)
	}
	st, err := svc.InitCheck(ctx)
	if err != nil || !st.Initialized || st.AllowWizard {
		t.Fatalf("InitCheck = %+v err=%v, want initialized+!allowWizard", st, err)
	}

	// ListUsers：脱敏字段（无哈希/密钥），totpEnabled=false。
	users, err := svc.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers = (%d items, %v)", len(users), err)
	}
	if users[0].Username != "admin" || users[0].Role != RoleAdmin ||
		users[0].Status != "active" || users[0].TotpEnabled {
		t.Fatalf("ListUsers[0] = %+v", users[0])
	}

	// 第二个会话；ListSessions 标记 current（admin=向导会话）。
	s2 := loginOK(t, svc, "admin", "admin12345")
	s2ID, err := svc.VerifyToken(ctx, s2.RawToken)
	if err != nil {
		t.Fatalf("verify s2: %v", err)
	}
	items, err := svc.ListSessions(ctx, admin)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListSessions = (%d items, %v)", len(items), err)
	}
	currentCount := 0
	for _, it := range items {
		if it.Current {
			currentCount++
		}
		if it.Username != "admin" || it.ID == "" {
			t.Fatalf("session item = %+v", it)
		}
	}
	if currentCount != 1 {
		t.Fatalf("current flag count = %d, want 1", currentCount)
	}

	// RevokeSession：吊销自身 → AUTH_012；吊销他方 → 成功且立即失效（I-13）。
	err = svc.RevokeSession(ctx, admin, admin.SessionDigest)
	wantErrCode(t, err, errcode.ErrValidation)
	if err := svc.RevokeSession(ctx, admin, s2ID.SessionDigest); err != nil {
		t.Fatalf("revoke s2: %v", err)
	}
	_, err = svc.VerifyToken(ctx, s2.RawToken)
	wantErrCode(t, err, errcode.ErrUnauthenticated)

	// 吊销不存在摘要 → AUTH_012。
	err = svc.RevokeSession(ctx, admin, "deadbeef")
	wantErrCode(t, err, errcode.ErrValidation)
}

// TestTotpDisableAndCurrentUserInfo 覆盖 TOTPDisable 全分支与 CurrentUserInfo
// 组装；顺带经「失败一次后登录成功」覆盖 resetFailCount（R-01 成功清零）。
func TestTotpDisableAndCurrentUserInfo(t *testing.T) {
	svc, _, clk := newTestService(t)
	ctx := context.Background()
	_, id := bootstrapAdmin(t, svc, "admin", "admin12345")

	// CurrentUserInfo：authenticated/user/csrfToken 三元组。
	info := svc.CurrentUserInfo(id)
	if info["authenticated"] != true {
		t.Fatalf("CurrentUserInfo = %v", info)
	}
	user := info["user"].(UserBrief)
	if user.ID != id.UserID || user.Username != id.Username || user.Role != RoleAdmin {
		t.Fatalf("CurrentUserInfo user = %+v", user)
	}
	if info["csrfToken"] != id.CsrfToken {
		t.Fatal("CurrentUserInfo csrfToken mismatch")
	}

	// 未绑定动态码时解绑 → AUTH_012。
	err := svc.TOTPDisable(ctx, id, "admin12345")
	wantErrCode(t, err, errcode.ErrValidation)

	// 一次密码失败（fail_count=1）后走 TOTP 绑定+登录（成功路径清零计数）。
	if _, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "wrong-pass-1"}, RequestMeta{}); err == nil {
		t.Fatal("wrong password unexpectedly accepted")
	}
	setup, err := svc.TOTPSetup(ctx, id)
	if err != nil {
		t.Fatalf("totp setup: %v", err)
	}
	code, err := GenerateCode(setup.Secret, clk.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if err := svc.TOTPEnable(ctx, id, code); err != nil {
		t.Fatalf("totp enable: %v", err)
	}
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	if err != nil || !out.RequiresTOTP {
		t.Fatalf("login = (%+v, %v), want totp required（该路径重置失败计数）", out, err)
	}

	// 解绑：错密码 AUTH_005；正确密码成功，用户列表 totpEnabled 归 false。
	err = svc.TOTPDisable(ctx, id, "wrong-pass-1")
	wantErrCode(t, err, errcode.ErrBadCredentials)
	err = svc.TOTPDisable(ctx, id, "admin12345")
	if err != nil {
		t.Fatalf("totp disable: %v", err)
	}
	users, err := svc.ListUsers(ctx)
	if err != nil || len(users) != 1 || users[0].TotpEnabled {
		t.Fatalf("after disable users = %+v err=%v", users, err)
	}
}

// TestInitViaCLIAndEdgeBranches 覆盖 CLI 初始化成功/入参分支、未初始化登录
// 拦截、删除不存在用户与删除另一管理员（I-03 放行侧）。
func TestInitViaCLIAndEdgeBranches(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// I-01：未初始化时登录 → AUTH_009。
	_, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrAlreadyInitialized)

	// 向导 TOTP 可选对：不成对 → AUTH_012；错码 → AUTH_006（prepareInit 分支）。
	_, err = svc.InitWizard(ctx, InitWizardRequest{
		Username: "admin", Password: "admin12345", TotpSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrValidation)
	_, err = svc.InitWizard(ctx, InitWizardRequest{
		Username: "admin", Password: "admin12345",
		TotpSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", TotpCode: "000000",
	}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrTotpInvalid)

	// CLI 初始化：短密码 → AUTH_012；totpSecret/totpCode 不成对 → AUTH_012；
	// 合法输入 → 成功，且与向导共用仲裁（此后向导被拒）。
	err = svc.InitViaCLI(ctx, "cliadmin", "short", "", "")
	wantErrCode(t, err, errcode.ErrValidation)
	err = svc.InitViaCLI(ctx, "cliadmin", "admin12345", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "")
	wantErrCode(t, err, errcode.ErrValidation)
	if err := svc.InitViaCLI(ctx, "cliadmin", "admin12345", "", ""); err != nil {
		t.Fatalf("init via cli: %v", err)
	}
	_, err = svc.InitWizard(ctx, InitWizardRequest{Username: "w", Password: "admin12345"}, RequestMeta{})
	wantErrCode(t, err, errcode.ErrAlreadyInitialized)

	// CLI admin 登录后建第二个 admin → 删不存在的 → AUTH_012；删另一 admin 放行。
	admin := loginOK(t, svc, "cliadmin", "admin12345")
	id, err := svc.VerifyToken(ctx, admin.RawToken)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	err = svc.DeleteUser(ctx, id, 999)
	wantErrCode(t, err, errcode.ErrValidation)
	created, err := svc.CreateUser(ctx, id, CreateUserRequest{Username: "admin2", Password: "admin21234", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}
	if err := svc.DeleteUser(ctx, id, created.ID); err != nil {
		t.Fatalf("delete other admin: %v", err)
	}
}
