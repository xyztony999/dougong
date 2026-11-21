import io

p = r'D:\Project\dougong-panel\backend\internal\service\appmgmt\service.go'
s = io.open(p, encoding='utf-8').read()

# Service 增 composeCreator 依赖
old = '''	envReady     EnvReadySource
	siteLink     SiteLinkGateway // nil=未装配（hasSite 呈现 null；site/link 拒绝）
	clock        func() time.Time
	syncDispatch bool // 测试缝：同步执行 runTask（确定性断言沿 env-mgmt 测试先例）
}'''
new = '''	envReady     EnvReadySource
	siteLink     SiteLinkGateway // nil=未装配（hasSite 呈现 null；site/link 拒绝）
	composeCre   ComposeProjectCreator // nil=未装配（容器化形态拒绝并引导 docker-mgmt）
	clock        func() time.Time
	syncDispatch bool // 测试缝：同步执行 runTask（确定性断言沿 env-mgmt 测试先例）
}'''
assert old in s
s = s.replace(old, new, 1)

old2 = '''type Options struct {
	EnvReady     EnvReadySource
	SiteLink     SiteLinkGateway
	Clock        func() time.Time
	SyncDispatch bool
}'''
new2 = '''type Options struct {
	EnvReady     EnvReadySource
	SiteLink     SiteLinkGateway
	ComposeCre   ComposeProjectCreator
	Clock        func() time.Time
	SyncDispatch bool
}'''
assert old2 in s
s = s.replace(old2, new2, 1)

old3 = '''	s := &Service{py: py, sysd: sysdGw, store: store,
		envReady: opts.EnvReady, siteLink: opts.SiteLink, clock: time.Now,
		syncDispatch: opts.SyncDispatch}'''
new3 = '''	s := &Service{py: py, sysd: sysdGw, store: store,
		envReady: opts.EnvReady, siteLink: opts.SiteLink, composeCre: opts.ComposeCre,
		clock: time.Now, syncDispatch: opts.SyncDispatch}'''
assert old3 in s
s = s.replace(old3, new3, 1)

# CreateRequest 扩容器化字段
old4 = '''type CreateRequest struct {
	Name       string    `json:"name"`
	Dir        string    `json:"dir"`
	Entry      string    `json:"entry"`
	StartMode  string    `json:"startMode"`
	StartSpec  StartSpec `json:"startSpec"`
	ListenPort int       `json:"listenPort"`
	StartNow   bool      `json:"startNow"`
}'''
new4 = '''type CreateRequest struct {
	Name       string    `json:"name"`
	Dir        string    `json:"dir,omitempty"`   // venv 形态必填
	Entry      string    `json:"entry,omitempty"` // venv 形态必填
	StartMode  string    `json:"startMode"`
	StartSpec  StartSpec `json:"startSpec,omitempty"` // venv 形态
	ListenPort int       `json:"listenPort"`
	StartNow   bool      `json:"startNow"`
	// v1.1 容器化形态字段（startMode=container 时生效，⚠️A13）：
	Image         string          `json:"image,omitempty"`
	Env           []EnvKVDTO      `json:"env,omitempty"`
	Mounts        []MountDTOMount `json:"mounts,omitempty"`
	RestartPolicy string          `json:"restartPolicy,omitempty"`
}'''
assert old4 in s
s = s.replace(old4, new4, 1)

# Create 方法 container 分支前置
old5 = old_create_sig = '''func (s *Service) Create(ctx context.Context, actor *authsvc.Identity, req CreateRequest) (*TaskCreatedData, error) {
	if !appNameRe.MatchString(req.Name) {'''
new5 = '''func (s *Service) Create(ctx context.Context, actor *authsvc.Identity, req CreateRequest) (*TaskCreatedData, error) {
	// v1.1 容器化形态分支（⚠️A13，D0 Q5）：镜像必填；无 dir/entry/Python 校验。
	if req.StartMode == storeapp.StartModeContainer {
		return s.createContainerized(ctx, actor, req)
	}
	if !appNameRe.MatchString(req.Name) {'''
assert old5 in s
s = s.replace(old5, new5, 1)

io.open(p, 'w', encoding='utf-8', newline='\n').write(s)
print('OK service dual-form')
