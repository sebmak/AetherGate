export type ViewTab = 'dashboard' | 'proxies' | 'ddns' | 'logs' | 'settings'

export const TAB_ROUTES: Record<ViewTab, string> = {
  dashboard: '/dashboard',
  proxies: '/proxies',
  ddns: '/ddns',
  logs: '/logs',
  settings: '/settings',
}

export function getViewFromPath(pathname: string): ViewTab {
  const normalized = (pathname || '/').toLowerCase().replace(/\/+$/, '') || '/'
  switch (normalized) {
    case '/proxies':
    case '/targets':
      return 'proxies'
    case '/ddns':
    case '/cloudflare':
    case '/dns':
      return 'ddns'
    case '/logs':
    case '/traffic':
      return 'logs'
    case '/settings':
    case '/config':
    case '/security':
      return 'settings'
    case '/':
    case '/dashboard':
    default:
      return 'dashboard'
  }
}

export function getPathForView(view: ViewTab): string {
  return TAB_ROUTES[view] || '/dashboard'
}

export function getViewTitle(view: ViewTab): string {
  switch (view) {
    case 'dashboard':
      return 'Dashboard | ProxyServer Gateway'
    case 'proxies':
      return 'Reverse Proxies | ProxyServer Gateway'
    case 'ddns':
      return 'Cloudflare DDNS | ProxyServer Gateway'
    case 'logs':
      return 'Live Traffic Logs | ProxyServer Gateway'
    case 'settings':
      return 'Settings & Security | ProxyServer Gateway'
    default:
      return 'ProxyServer Gateway'
  }
}
