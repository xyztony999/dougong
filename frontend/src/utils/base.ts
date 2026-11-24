/**
 * 应用基址（api D-P03 安全入口路径）：面板的页面、脚本、图标、接口、WebSocket 全部位于同一个
 * 路径前缀之下——安全入口没开启时前缀是 /，开启后是 /{entryPath}/。
 *
 * 前缀只有一个来源：面板后端在下发的页面里写的 <base href="/…/">。这里读它，不在前端写死入口
 * 路径，也不去读配置接口（首屏请求发出时配置还没加载）。面板自己的地址都要经 withBase()，
 * 否则开着安全入口时请求不带前缀，被入口网关拦掉（标准 404）。
 */

/** 读页面里的 <base href>。只认以单个 / 开头的路径；没有（开发服务器、占位页面）或写法不对时返回 null。 */
function readBaseTag(): string | null {
  if (typeof document === 'undefined') return null
  const href = document.querySelector('base[href]')?.getAttribute('href')
  if (href === null || href === undefined || !href.startsWith('/') || href.startsWith('//')) return null
  return href.endsWith('/') ? href : `${href}/`
}

/** 应用基址：以 / 开头、以 / 结尾，例如 / 或 /abc12345/ */
export function appBasePath(): string {
  return readBaseTag() ?? '/'
}

/** 是否是面板自己的路径（以单个 / 开头）。完整地址（http://…）和 //host/… 形式不是。 */
function isSitePath(path: string): boolean {
  return path.startsWith('/') && !path.startsWith('//')
}

/** 把面板路径拼到某个基址下：base=/abc/、path=/settings → /abc/settings；base=/ 时原样。 */
export function joinBase(base: string, path: string): string {
  const head = base.endsWith('/') ? base.slice(0, -1) : base
  return head + (path.startsWith('/') ? path : `/${path}`)
}

/**
 * 面板路径 → 实际请求地址：加上应用基址。
 * 入参写法不变（照旧写 '/auth/me'）；不是面板路径的入参（完整地址）原样返回。
 */
export function withBase(path: string): string {
  return isSitePath(path) ? joinBase(appBasePath(), path) : path
}
