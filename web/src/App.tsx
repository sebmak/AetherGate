import React, { useState, useEffect, useCallback } from 'react'
import type { ProxyTarget, StatsResponse, DDNSStatus, CloudflareConfig } from './types'
import { api } from './services/api'
import { getViewFromPath, getPathForView, getViewTitle, type ViewTab } from './router'
import { Sidebar } from './components/Sidebar'
import { Navbar } from './components/Navbar'
import { ProxyModal } from './components/ProxyModal'
import { ConfirmModal } from './components/ConfirmModal'
import { DashboardView } from './views/DashboardView'
import { ProxiesView } from './views/ProxiesView'
import { CloudflareDdnsView } from './views/CloudflareDdnsView'
import { LogsView } from './views/LogsView'
import { SettingsView } from './views/SettingsView'
import { LoginView } from './views/LoginView'

export const App: React.FC = () => {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null)
  const [isSetupRequired, setIsSetupRequired] = useState(false)
  const [username, setUsername] = useState('Admin')
  const [currentView, setCurrentView] = useState<ViewTab>(() => getViewFromPath(window.location.pathname))

  // Global State
  const [proxies, setProxies] = useState<ProxyTarget[]>([])
  const [stats, setStats] = useState<StatsResponse | null>(null)
  const [ddnsStatus, setDdnsStatus] = useState<DDNSStatus | null>(null)
  const [ddnsConfig, setDdnsConfig] = useState<CloudflareConfig | null>(null)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [isSyncingDDNS, setIsSyncingDDNS] = useState(false)

  // Modals
  const [proxyModalOpen, setProxyModalOpen] = useState(false)
  const [editingProxy, setEditingProxy] = useState<ProxyTarget | null>(null)
  const [deletingProxy, setDeletingProxy] = useState<ProxyTarget | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  // Check auth and initial status on load
  const checkAuth = useCallback(async () => {
    try {
      const setupRes = await api.getSetupStatus()
      if (!setupRes.is_configured) {
        setIsSetupRequired(true)
        setIsAuthenticated(false)
        return
      }

      const me = await api.getMe()
      setUsername(me.username)
      setIsAuthenticated(true)
      setIsSetupRequired(false)
    } catch {
      setIsAuthenticated(false)
    }
  }, [])

  useEffect(() => {
    checkAuth()

    const handleUnauthorized = () => {
      setIsAuthenticated(false)
    }
    const handleSetup = () => {
      setIsSetupRequired(true)
      setIsAuthenticated(false)
    }

    window.addEventListener('auth:unauthorized', handleUnauthorized)
    window.addEventListener('auth:setup-required', handleSetup)

    return () => {
      window.removeEventListener('auth:unauthorized', handleUnauthorized)
      window.removeEventListener('auth:setup-required', handleSetup)
    }
  }, [checkAuth])

  // Fetch all core data
  const fetchData = useCallback(async () => {
    if (!isAuthenticated) return
    setIsRefreshing(true)
    try {
      const [proxiesData, statsData, ddnsData] = await Promise.all([
        api.getProxies(),
        api.getStats().catch(() => null),
        api.getDDNS().catch(() => null),
      ])
      setProxies(proxiesData || [])
      if (statsData) setStats(statsData)
      if (ddnsData) {
        setDdnsStatus(ddnsData.status)
        setDdnsConfig(ddnsData.config)
      }
    } catch {
      // Ignored
    } finally {
      setIsRefreshing(false)
    }
  }, [isAuthenticated])

  useEffect(() => {
    if (isAuthenticated) {
      fetchData()
      const interval = setInterval(fetchData, 10000)
      return () => clearInterval(interval)
    }
  }, [isAuthenticated, fetchData])

  // Proxy Actions
  const handleSaveProxy = async (target: Partial<ProxyTarget>) => {
    if (target.id) {
      await api.updateProxy(target.id, target)
    } else {
      await api.createProxy(target)
    }
    await fetchData()
  }

  const handleToggleProxy = async (id: string) => {
    // Optimistic toggle
    setProxies((prev) =>
      prev.map((p) => (p.id === id ? { ...p, enabled: !p.enabled } : p))
    )
    try {
      await api.toggleProxy(id)
      await fetchData()
    } catch {
      await fetchData()
    }
  }

  const handleConfirmDelete = async () => {
    if (!deletingProxy) return
    setIsDeleting(true)
    try {
      await api.deleteProxy(deletingProxy.id)
      setDeletingProxy(null)
      await fetchData()
    } catch (err: any) {
      alert(`Delete failed: ${err.message}`)
    } finally {
      setIsDeleting(false)
    }
  }

  const handleSyncDDNS = async () => {
    setIsSyncingDDNS(true)
    try {
      await api.triggerDDNSSync()
      setTimeout(fetchData, 2000)
    } catch (err: any) {
      alert(`DDNS sync failed: ${err.message}`)
    } finally {
      setIsSyncingDDNS(false)
    }
  }

  // Client-side Navigation
  const navigateTo = useCallback((view: ViewTab | string, replace = false) => {
    const targetView = (['dashboard', 'proxies', 'ddns', 'logs', 'settings'].includes(view as any)
      ? view
      : getViewFromPath(view)) as ViewTab

    const targetPath = getPathForView(targetView)

    if (window.location.pathname !== targetPath) {
      if (replace) {
        window.history.replaceState({ view: targetView }, '', targetPath)
      } else {
        window.history.pushState({ view: targetView }, '', targetPath)
      }
    }

    document.title = getViewTitle(targetView)
    setCurrentView(targetView)
  }, [])

  // Browser history popstate (Back / Forward buttons)
  useEffect(() => {
    const handlePopState = (e: PopStateEvent) => {
      const view = e.state?.view || getViewFromPath(window.location.pathname)
      setCurrentView(view)
      document.title = getViewTitle(view)
    }

    const initialView = getViewFromPath(window.location.pathname)
    document.title = getViewTitle(initialView)

    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  const handleLogout = async () => {
    await api.logout()
    setIsAuthenticated(false)
  }

  if (isAuthenticated === null) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-primary)', color: 'var(--accent-cyan)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', fontSize: '1rem', fontWeight: 600 }}>
          <div className="spin" style={{ width: '20px', height: '20px', border: '2px solid var(--accent-cyan)', borderTopColor: 'transparent', borderRadius: '50%' }} />
          Loading ProxyServer Gateway...
        </div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return (
      <LoginView
        isSetup={isSetupRequired}
        onAuthenticated={(user) => {
          setUsername(user)
          setIsAuthenticated(true)
          setIsSetupRequired(false)
        }}
      />
    )
  }

  return (
    <div className="app-container">
      {/* Sidebar */}
      <Sidebar
        currentView={currentView}
        onNavigate={navigateTo}
        uptimeSeconds={stats?.uptime_seconds ?? 0}
      />

      {/* Main Content Area */}
      <div className="main-content">
        <Navbar
          publicIP={ddnsStatus?.current_ip || ''}
          ddnsEnabled={ddnsStatus?.enabled ?? false}
          username={username}
          onLogout={handleLogout}
          onRefresh={fetchData}
          isRefreshing={isRefreshing}
        />

        <main className="page-body">
          {currentView === 'dashboard' && (
            <DashboardView
              stats={stats}
              proxies={proxies}
              ddnsStatus={ddnsStatus}
              onOpenNewProxy={() => {
                setEditingProxy(null)
                setProxyModalOpen(true)
              }}
              onEditProxy={(proxy) => {
                setEditingProxy(proxy)
                setProxyModalOpen(true)
              }}
              onSyncDDNS={handleSyncDDNS}
              onNavigate={navigateTo}
              isSyncingDDNS={isSyncingDDNS}
            />
          )}

          {currentView === 'proxies' && (
            <ProxiesView
              proxies={proxies}
              onOpenNewProxy={() => {
                setEditingProxy(null)
                setProxyModalOpen(true)
              }}
              onEditProxy={(proxy) => {
                setEditingProxy(proxy)
                setProxyModalOpen(true)
              }}
              onDeleteProxy={(proxy) => setDeletingProxy(proxy)}
              onToggleProxy={handleToggleProxy}
              onRefresh={fetchData}
            />
          )}

          {currentView === 'ddns' && (
            <CloudflareDdnsView
              status={ddnsStatus}
              config={ddnsConfig}
              onRefresh={fetchData}
              onSyncNow={handleSyncDDNS}
              isSyncing={isSyncingDDNS}
            />
          )}

          {currentView === 'logs' && <LogsView />}

          {currentView === 'settings' && <SettingsView onRefresh={fetchData} />}
        </main>
      </div>

      {/* Modals */}
      <ProxyModal
        isOpen={proxyModalOpen}
        onClose={() => setProxyModalOpen(false)}
        onSave={handleSaveProxy}
        initialData={editingProxy}
      />

      <ConfirmModal
        isOpen={Boolean(deletingProxy)}
        title="Delete Reverse Proxy"
        message={`Are you sure you want to delete "${deletingProxy?.name}" (${deletingProxy?.domain})? Traffic to this domain will immediately return 404.`}
        confirmText="Delete Proxy"
        confirmVariant="danger"
        isLoading={isDeleting}
        onConfirm={handleConfirmDelete}
        onCancel={() => setDeletingProxy(null)}
      />
    </div>
  )
}
export default App
