import React from 'react'
import {
  Server,
  Cloud,
  Activity,
  CheckCircle2,
  Plus,
  RefreshCw,
  ExternalLink,
  Shield,
  Lock,
  ArrowUpRight,
} from 'lucide-react'
import type { ProxyTarget, StatsResponse, DDNSStatus } from '../types'
import { MetricCard } from '../components/MetricCard'

interface DashboardViewProps {
  stats: StatsResponse | null
  proxies: ProxyTarget[]
  ddnsStatus: DDNSStatus | null
  onOpenNewProxy: () => void
  onEditProxy: (proxy: ProxyTarget) => void
  onSyncDDNS: () => void
  onNavigate: (view: string) => void
  isSyncingDDNS?: boolean
}

export const DashboardView: React.FC<DashboardViewProps> = ({
  stats,
  proxies,
  ddnsStatus,
  onOpenNewProxy,
  onEditProxy,
  onSyncDDNS,
  onNavigate,
  isSyncingDDNS,
}) => {
  const activeProxies = proxies.filter((p) => p.enabled)
  const healthyCount = proxies.filter((p) => p.enabled && p.is_healthy).length

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Server size={28} color="var(--accent-cyan)" />
            Gateway Overview
          </h1>
          <p className="page-subtitle">Real-time telemetry, routing health, and automated Cloudflare synchronization</p>
        </div>
        <div style={{ display: 'flex', gap: '0.75rem' }}>
          <button
            id="dash-btn-sync-ddns"
            onClick={onSyncDDNS}
            disabled={isSyncingDDNS}
            className="btn btn-secondary"
          >
            <RefreshCw size={15} className={isSyncingDDNS ? 'spin' : ''} />
            {isSyncingDDNS ? 'Syncing...' : 'Sync Cloudflare'}
          </button>
          <button
            id="dash-btn-new-proxy"
            onClick={onOpenNewProxy}
            className="btn btn-primary"
          >
            <Plus size={16} />
            New Reverse Proxy
          </button>
        </div>
      </div>

      {/* Metrics Row */}
      <div className="metrics-grid">
        <MetricCard
          title="Active Proxies"
          value={`${activeProxies.length} / ${proxies.length}`}
          detail={`${proxies.length - activeProxies.length} disabled`}
          icon={Server}
          color="cyan"
        />
        <MetricCard
          title="Upstream Health"
          value={`${healthyCount} / ${activeProxies.length || 0}`}
          detail={healthyCount === activeProxies.length && activeProxies.length > 0 ? 'All upstreams responding' : `${activeProxies.length - healthyCount} unhealthy/offline`}
          icon={CheckCircle2}
          color={healthyCount === activeProxies.length ? 'emerald' : 'amber'}
        />
        <MetricCard
          title="Total Requests"
          value={stats?.metrics.total_requests.toLocaleString() ?? '0'}
          detail={`2xx: ${stats?.metrics.status_2xx.toLocaleString() ?? 0} • 4xx: ${stats?.metrics.status_4xx.toLocaleString() ?? 0} • 5xx: ${stats?.metrics.status_5xx.toLocaleString() ?? 0}`}
          icon={Activity}
          color="purple"
        />
        <MetricCard
          title="Cloudflare DDNS"
          value={ddnsStatus?.current_ip || 'Offline'}
          detail={ddnsStatus?.enabled ? `Last sync: ${ddnsStatus?.last_sync_time ? new Date(ddnsStatus.last_sync_time).toLocaleTimeString() : 'Pending'}` : 'DDNS disabled in config'}
          icon={Cloud}
          color={ddnsStatus?.enabled ? 'emerald' : 'amber'}
        />
      </div>

      {/* Cloudflare DDNS Banner */}
      <div className="card" style={{ marginBottom: '1.75rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '1rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
            <div style={{
              width: '42px',
              height: '42px',
              borderRadius: 'var(--radius-md)',
              background: '#0c283d',
              border: '1px solid #0369a1',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-cyan)',
            }}>
              <Cloud size={22} />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.65rem' }}>
                <h3 style={{ fontSize: '1.05rem', fontWeight: 600, color: '#fff' }}>Dynamic DNS Status</h3>
                {ddnsStatus?.enabled ? (
                  <span className="badge badge-emerald">
                    <span className="status-dot status-dot-healthy" style={{ width: '6px', height: '6px' }} />
                    Active Sync
                  </span>
                ) : (
                  <span className="badge badge-amber">Disabled</span>
                )}
              </div>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '0.2rem' }}>
                Detected Public IP: <strong style={{ color: 'var(--accent-cyan)', fontFamily: 'monospace' }}>{ddnsStatus?.current_ip || 'N/A'}</strong>
                {ddnsStatus?.previous_ip && ddnsStatus.previous_ip !== ddnsStatus.current_ip && (
                  <span> (Previous: {ddnsStatus.previous_ip})</span>
                )}
              </p>
            </div>
          </div>
          <button
            onClick={() => onNavigate('ddns')}
            className="btn btn-outline-cyan"
            style={{ fontSize: '0.8125rem' }}
          >
            Manage Cloudflare DDNS <ArrowUpRight size={14} />
          </button>
        </div>
      </div>

      {/* Active Routing Table Preview */}
      <div className="card">
        <div className="card-header">
          <div className="card-title">
            <Server size={18} color="var(--accent-cyan)" />
            Configured Reverse Proxies
          </div>
          <button
            onClick={() => onNavigate('proxies')}
            className="btn btn-secondary"
            style={{ fontSize: '0.8125rem', padding: '0.4rem 0.85rem' }}
          >
            View All ({proxies.length})
          </button>
        </div>

        {proxies.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '3rem 1rem', color: 'var(--text-muted)' }}>
            <Server size={36} style={{ margin: '0 auto 1rem', opacity: 0.4 }} />
            <div style={{ fontSize: '1rem', fontWeight: 600, color: '#fff', marginBottom: '0.25rem' }}>No Proxies Configured Yet</div>
            <p style={{ fontSize: '0.875rem', marginBottom: '1.25rem' }}>Add your first internal target to begin proxying domains.</p>
            <button onClick={onOpenNewProxy} className="btn btn-primary">
              <Plus size={16} /> Add First Proxy
            </button>
          </div>
        ) : (
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Target / Domain</th>
                  <th>Upstream Target</th>
                  <th>Health</th>
                  <th>Security</th>
                  <th>Cloudflare</th>
                  <th>Status</th>
                  <th style={{ textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {proxies.slice(0, 8).map((p) => (
                  <tr key={p.id}>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                        <div>
                          <div style={{ fontWeight: 600, color: '#fff' }}>{p.name}</div>
                          <a
                            href={`https://${p.domain}`}
                            target="_blank"
                            rel="noreferrer"
                            style={{ color: 'var(--accent-cyan)', fontSize: '0.8125rem', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '0.25rem' }}
                          >
                            {p.domain} <ExternalLink size={11} />
                          </a>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span style={{ fontFamily: 'monospace', fontSize: '0.8125rem', color: '#cbd5e1', background: '#111a2e', padding: '0.2rem 0.5rem', borderRadius: '4px' }}>
                        {p.scheme}://{p.host}
                      </span>
                    </td>
                    <td>
                      {p.enabled ? (
                        p.is_healthy ? (
                          <span className="badge badge-emerald">
                            <span className="status-dot status-dot-healthy" style={{ width: '6px', height: '6px' }} />
                            Online ({p.latency_ms}ms)
                          </span>
                        ) : (
                          <span className="badge badge-rose" title={p.last_error}>
                            <span className="status-dot status-dot-unhealthy" style={{ width: '6px', height: '6px' }} />
                            Offline
                          </span>
                        )
                      ) : (
                        <span className="badge badge-gray">Disabled</span>
                      )}
                    </td>
                    <td>
                      <div style={{ display: 'flex', gap: '0.35rem' }}>
                        {p.force_ssl && (
                          <span className="badge badge-cyan" title="HTTPS enforced">
                            <Shield size={11} /> SSL
                          </span>
                        )}
                        {p.require_auth && (
                          <span className="badge badge-amber" title="Protected gateway auth">
                            <Lock size={11} /> Auth
                          </span>
                        )}
                        {!p.force_ssl && !p.require_auth && (
                          <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>Standard</span>
                        )}
                      </div>
                    </td>
                    <td>
                      {p.cloudflare_sync ? (
                        <span className="badge badge-cyan">
                          <Cloud size={11} /> Synced
                        </span>
                      ) : (
                        <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>Manual</span>
                      )}
                    </td>
                    <td>
                      {p.enabled ? (
                        <span style={{ color: 'var(--accent-emerald)', fontSize: '0.8125rem', fontWeight: 600 }}>Active</span>
                      ) : (
                        <span style={{ color: 'var(--text-muted)', fontSize: '0.8125rem' }}>Disabled</span>
                      )}
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <button
                        onClick={() => onEditProxy(p)}
                        className="btn btn-secondary"
                        style={{ padding: '0.35rem 0.75rem', fontSize: '0.75rem' }}
                      >
                        Configure
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
