import {
  Server,
  Cloud,
  Activity,
  Settings,
  LayoutDashboard,
} from 'lucide-react'

interface SidebarProps {
  currentView: string
  setCurrentView: (view: string) => void
  uptimeSeconds: number
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  setCurrentView,
  uptimeSeconds,
}) => {
  const navItems = [
    { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { id: 'proxies', label: 'Reverse Proxies', icon: Server },
    { id: 'ddns', label: 'Cloudflare DDNS', icon: Cloud },
    { id: 'logs', label: 'Live Traffic Logs', icon: Activity },
    { id: 'settings', label: 'Settings & Security', icon: Settings },
  ]

  const formatUptime = (sec: number) => {
    if (sec <= 0) return '0m'
    const h = Math.floor(sec / 3600)
    const m = Math.floor((sec % 3600) / 60)
    if (h > 0) return `${h}h ${m}m`
    return `${m}m`
  }

  return (
    <aside className="sidebar">
      {/* Brand Header */}
      <div style={{ padding: '1.25rem 1.5rem', display: 'flex', alignItems: 'center', gap: '0.75rem', borderBottom: '1px solid var(--border-subtle)' }}>
        <img
          src="/icon.png"
          alt="ProxyServer Logo"
          style={{
            width: '36px',
            height: '36px',
            borderRadius: '8px',
            border: '1px solid var(--border-subtle)',
            objectFit: 'cover',
          }}
        />
        <div className="logo-text">
          <div style={{ fontWeight: 700, fontSize: '1.05rem', letterSpacing: '-0.02em', color: '#fff' }}>
            Proxy<span style={{ color: 'var(--accent-cyan)' }}>Server</span>
          </div>
          <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, letterSpacing: '0.05em' }}>
            GATEWAY & DDNS
          </div>
        </div>
      </div>

      {/* Nav Menu */}
      <nav style={{ padding: '1rem 0.65rem', display: 'flex', flexDirection: 'column', gap: '0.25rem', flex: 1 }}>
        {navItems.map((item) => {
          const Icon = item.icon
          const isActive = currentView === item.id
          return (
            <button
              key={item.id}
              id={`nav-${item.id}`}
              onClick={() => setCurrentView(item.id)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '0.85rem',
                padding: '0.65rem 0.85rem',
                borderRadius: 'var(--radius-md)',
                border: 'none',
                background: isActive ? '#1c2538' : 'transparent',
                color: isActive ? 'var(--accent-cyan)' : 'var(--text-secondary)',
                fontWeight: isActive ? 600 : 500,
                fontSize: '0.875rem',
                cursor: 'pointer',
                textAlign: 'left',
                width: '100%',
                transition: 'background-color 0.15s',
                borderLeft: isActive ? '3px solid var(--accent-cyan)' : '3px solid transparent',
              }}
              onMouseEnter={(e) => {
                if (!isActive) {
                  e.currentTarget.style.background = '#18202e'
                  e.currentTarget.style.color = '#fff'
                }
              }}
              onMouseLeave={(e) => {
                if (!isActive) {
                  e.currentTarget.style.background = 'transparent'
                  e.currentTarget.style.color = 'var(--text-secondary)'
                }
              }}
            >
              <Icon size={18} color={isActive ? 'var(--accent-cyan)' : 'currentColor'} />
              <span className="nav-text">{item.label}</span>
            </button>
          )
        })}
      </nav>

      {/* Uptime and Status widget */}
      <div style={{ padding: '0.85rem 1rem', margin: '0.75rem', background: '#0e131d', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.25rem' }}>
          <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600 }}>SYSTEM STATUS</span>
          <span className="status-dot status-dot-healthy" />
        </div>
        <div style={{ fontSize: '0.8125rem', color: 'var(--text-primary)', fontWeight: 500, display: 'flex', justifyContent: 'space-between' }}>
          <span>Uptime:</span>
          <span style={{ fontFamily: 'monospace', color: 'var(--accent-cyan)' }}>{formatUptime(uptimeSeconds)}</span>
        </div>
        <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '0.35rem' }}>
          Go 1.27 • Dynamic Routing
        </div>
      </div>
    </aside>
  )
}
