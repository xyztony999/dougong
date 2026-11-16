package panel

import (
	"sort"
	"sync"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// MenuItem 是构建期菜单登记项（D-P07：模块以 {key,title,path,icon,order,
// visibility} 登记，GET /panel/menu/list 单一数据源，前端不硬编码菜单）。
// Visibility 为可见角色集合（admin/readonly）；空集视作 admin 专属
// （保守缺省，防误暴露）。
type MenuItem struct {
	Key        string
	Title      string
	Path       string
	Icon       string
	Order      int
	Visibility []string
	Children   []MenuItem
}

// MenuNode 是菜单树下发节点（api 4.1.4：data.items 项；不含 visibility——
// 可见性即服务端判定，readonly 客户端不接触 admin 菜单定义，ADR-018）。
type MenuNode struct {
	Key      string     `json:"key"`
	Title    string     `json:"title"`
	Path     string     `json:"path"`
	Icon     string     `json:"icon"`
	Order    int        `json:"order"`
	Children []MenuNode `json:"children"`
}

// MenuRegistry 是构建期菜单登记表（ModuleMenuRegistry，api §5.1 被依赖
// 契约）：main 装配阶段各模块 Register 登记；运行期 Tree 按角色过滤下发。
type MenuRegistry struct {
	mu    sync.RWMutex
	items []MenuItem
}

// NewMenuRegistry 构造空登记表。
func NewMenuRegistry() *MenuRegistry { return &MenuRegistry{} }

// Register 登记菜单项（构建期调用；锁保护为防御性设计——运行期注册同样
// 安全，但不鼓励绕过装配期登记）。
func (r *MenuRegistry) Register(items ...MenuItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, items...)
}

// Tree 按 role 过滤构建菜单树：admin=全量；readonly=仅 visibility 含
// readonly 的查看类节点（D-P07）。同级按 Order 升序、Key 字典序稳定排序；
// 节点被过滤时其子树一并隐藏。
func (r *MenuRegistry) Tree(role string) []MenuNode {
	r.mu.RLock()
	items := make([]MenuItem, len(r.items))
	copy(items, r.items)
	r.mu.RUnlock()
	nodes := filterMenu(items, role)
	sortMenu(nodes)
	return nodes
}

// filterMenu 递归过滤可见节点（children 深拷贝，不复用登记切片）。
func filterMenu(items []MenuItem, role string) []MenuNode {
	var out []MenuNode
	for _, it := range items {
		if !visibleTo(it.Visibility, role) {
			continue
		}
		node := MenuNode{Key: it.Key, Title: it.Title, Path: it.Path, Icon: it.Icon, Order: it.Order}
		if children := filterMenu(it.Children, role); len(children) > 0 {
			node.Children = children
		}
		out = append(out, node)
	}
	return out
}

// visibleTo 判定可见性：admin 全量（D-P07）；其余角色须显式列入。
func visibleTo(visibility []string, role string) bool {
	if role == storeauth.RoleAdmin {
		return true
	}
	for _, v := range visibility {
		if v == role {
			return true
		}
	}
	return false
}

func sortMenu(nodes []MenuNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Order != nodes[j].Order {
			return nodes[i].Order < nodes[j].Order
		}
		return nodes[i].Key < nodes[j].Key
	})
	for _, n := range nodes {
		if len(n.Children) > 0 {
			sortMenu(n.Children)
		}
	}
}

// MenuList GET /panel/menu/list 的数据组装（构建期登记派生，无 IO）。
func (s *Service) MenuList(role string) []MenuNode {
	return s.menus.Tree(role)
}
