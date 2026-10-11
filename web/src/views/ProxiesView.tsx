import React, { useState } from 'react'
import {
  Server,
  Plus,
  Search,
  ExternalLink,
  Shield,
  Lock,
  Cloud,
  Zap,
  Edit2,
  Trash2,
  Copy,
  Check,
} from 'lucide-react'
import type { ProxyTarget } from '../types'
import { api } from '../services/api'

interface ProxiesViewProps {
  proxies: ProxyTarget[]
  onOpenNewProxy: () => void
  onEditProxy: (proxy: ProxyTarget) => void
  onDeleteProxy: (proxy: ProxyTarget) => void
  onToggleProxy: (id: string) => Promise<void>
  onRefresh: () => void
}

export const ProxiesView: React.FC<ProxiesViewProps> = ({
  proxies,
  onOpenNewProxy,
  onEditProxy,
  onDeleteProxy,
  onToggleProxy,
}) => {
  const [searchTerm, setSearchTerm] = useState('')
  const [filterStatus, setFilterStatus] = useState<'all' | 'active' | 'disabled' | 'healthy' | 'unhealthy'>('all')
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<Record<string, { success: boolean; latency: number; message: string }>>({})
  const [copiedDomain, setCopiedDomain] = useState<string | null>(null)

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    setCopiedDomain(text)
    setTimeout(() => setCopiedDomain(null), 1500)
  }

  const handleTestTarget = async (p: ProxyTarget) => {
    setTestingId(p.id)
    try {
      const res = await api.testUpstream(p.scheme, p.host)
      setTestResult((prev) => ({
        ...prev,
        [p.id]: { success: res.success, latency: res.latency_ms, message: res.message },
      }))
    } catch (err: any) {
      setTestResult((prev) => ({
        ...prev,
        [p.id]: { success: false, latency: 0, message: err.message || 'Probe error' },
      }))
    } finally {
      setTestingId(null)
    }
  }

  const filtered = proxies.filter((p) => {
    const term = searchTerm.toLowerCase()
    const matchesSearch =
      p.name.toLowerCase().includes(term) ||
      p.domain.toLowerCase().includes(term) ||
      p.host.toLowerCase().includes(term)

    if (!matchesSearch) return false

    if (filterStatus === 'active') return p.enabled
    if (filterStatus === 'disabled') return !p.enabled
    if (filterStatus === 'healthy') return p.enabled && p.is_healthy
    if (filterStatus === 'unhealthy') return p.enabled && !p.is_healthy

    return true
  })

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Server size={28} color="var(--accent-cyan)" />
            Reverse Proxy Targets
          </h1>
          <p className="page-subtitle">Dynamically managed routes with zero-downtime reconfiguration</p>
        </div>
        <button
          id="btn-add-proxy-page"
          onClick={onOpenNewProxy}
          className="btn btn-primary"
        >
          <Plus size={16} />
          Add Reverse Proxy
        </button>
      </div>

      {/* Filter and Search Bar */}
      <div className="card" style={{ padding: '1rem 1.25rem', marginBottom: '1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', flexWrap: 'wrap' }}>
          <div style={{ position: 'relative', flex: 1, minWidth: '240px' }}>
            <Search size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
            <input
              type="text"
              className="input"
              style={{ paddingLeft: '2.5rem' }}
              placeholder="Search by domain, name, or upstream host..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
          </div>

          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            {(['all', 'active', 'healthy', 'unhealthy', 'disabled'] as const).map((status) => (
              <button
                key={status}
                onClick={() => setFilterStatus(status)}
                className={`btn ${filterStatus === status ? 'btn-outline-cyan' : 'btn-secondary'}`}
                style={{ fontSize: '0.75rem', padding: '0.4rem 0.75rem', textTransform: 'capitalize' }}
              >
                {status}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Proxies Table */}
      <div className="card" style={{ padding: 0 }}>
        {filtered.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '4rem 1rem', color: 'var(--text-muted)' }}>
            <Server size={40} style={{ margin: '0 auto 1rem', opacity: 0.3 }} />
            <h3 style={{ fontSize: '1.1rem', fontWeight: 600, color: '#fff', marginBottom: '0.35rem' }}>
              {searchTerm ? 'No matching proxy routes found' : 'No reverse proxies configured'}
            </h3>
            <p style={{ fontSize: '0.875rem', marginBottom: '1.25rem' }}>
              {searchTerm ? 'Try adjusting your search criteria' : 'Create a new proxy route to begin forwarding traffic'}
            </p>
            {!searchTerm && (
              <button onClick={onOpenNewProxy} className="btn btn-primary">
                <Plus size={16} /> Create First Proxy
              </button>
            )}
          </div>
        ) : (
          <div className="table-container" style={{ border: 'none' }}>
            <table className="table">
              <thead>
                <tr>
                  <th style={{ width: '40px' }}>State</th>
                  <th>Incoming Domain</th>
                  <th>Friendly Name</th>
                  <th>Upstream Target</th>
                  <th>Health Status</th>
                  <th>Security & Features</th>
                  <th style={{ textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((p) => {
                  const test = testResult[p.id]
                  const isBeingTested = testingId === p.id

                  return (
                    <tr key={p.id}>
                      {/* Enable/Disable Toggle */}
                      <td>
                        <label className="switch" style={{ width: '38px', height: '20px' }}>
                          <input
                            type="checkbox"
                            checked={p.enabled}
                            onChange={() => onToggleProxy(p.id)}
                          />
                          <span className="switch-slider" style={{ borderRadius: '20px' }} />
                        </label>
                      </td>

                      {/* Domain */}
                      <td>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                          <span style={{ fontWeight: 600, color: '#fff', fontFamily: 'monospace', fontSize: '0.875rem' }}>
                            {p.domain}
                          </span>
                          <button
                            onClick={() => copyToClipboard(p.domain)}
                            className="btn btn-secondary btn-icon"
                            style={{ padding: '0.2rem', borderRadius: '4px' }}
                            title="Copy domain"
                          >
                            {copiedDomain === p.domain ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
                          </button>
                          <a
                            href={`${p.force_ssl ? 'https' : 'http'}://${p.domain}`}
                            target="_blank"
                            rel="noreferrer"
                            className="btn btn-secondary btn-icon"
                            style={{ padding: '0.2rem', borderRadius: '4px' }}
                            title="Open in new tab"
                          >
                            <ExternalLink size={12} />
                          </a>
                        </div>
                      </td>

                      {/* Name */}
                      <td>
                        <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
                          {p.name}
                        </span>
                      </td>

                      {/* Upstream Host */}
                      <td>
                        <span style={{
                          fontFamily: 'monospace',
                          fontSize: '0.8125rem',
                          color: '#38bdf8',
                          background: 'rgba(56, 189, 248, 0.08)',
                          border: '1px solid rgba(56, 189, 248, 0.2)',
                          padding: '0.25rem 0.6rem',
                          borderRadius: '4px',
                        }}>
                          {p.scheme}://{p.host}
                        </span>
                      </td>

                      {/* Health */}
                      <td>
                        {p.enabled ? (
                          p.is_healthy ? (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                              <span className="badge badge-emerald">
                                <span className="status-dot status-dot-healthy" style={{ width: '6px', height: '6px' }} />
                                Online ({p.latency_ms}ms)
                              </span>
                              {test && (
                                <span style={{ fontSize: '0.7rem', color: test.success ? 'var(--accent-emerald)' : 'var(--accent-rose)' }}>
                                  Test: {test.message}
                                </span>
                              )}
                            </div>
                          ) : (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                              <span className="badge badge-rose" title={p.last_error}>
                                <span className="status-dot status-dot-unhealthy" style={{ width: '6px', height: '6px' }} />
                                Unreachable
                              </span>
                              {p.last_error && (
                                <span style={{ fontSize: '0.7rem', color: 'var(--accent-rose)', maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={p.last_error}>
                                  {p.last_error}
                                </span>
                              )}
                            </div>
                          )
                        ) : (
                          <span className="badge badge-gray">Disabled</span>
                        )}
                      </td>

                      {/* Features */}
                      <td>
                        <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap' }}>
                          {p.force_ssl && (
                            <span className="badge badge-cyan" title="HTTPS SSL redirect enforced">
                              <Shield size={11} /> Force SSL
                            </span>
                          )}
                          {p.require_auth && (
                            <span className="badge badge-amber" title="Protected gateway authentication">
                              <Lock size={11} /> Auth Guard
                            </span>
                          )}
                          {p.cloudflare_sync && (
                            <span className="badge badge-cyan" title="Cloudflare DDNS active">
                              <Cloud size={11} /> Cloudflare
                            </span>
                          )}
                        </div>
                      </td>

                      {/* Actions */}
                      <td style={{ textAlign: 'right' }}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.4rem' }}>
                          <button
                            onClick={() => handleTestTarget(p)}
                            disabled={isBeingTested}
                            className="btn btn-secondary btn-icon"
                            title="Probe upstream connection now"
                          >
                            <Zap size={14} className={isBeingTested ? 'spin' : ''} color="var(--accent-cyan)" />
                          </button>
                          <button
                            onClick={() => onEditProxy(p)}
                            className="btn btn-secondary btn-icon"
                            title="Edit proxy configuration"
                          >
                            <Edit2 size={14} />
                          </button>
                          <button
                            onClick={() => onDeleteProxy(p)}
                            className="btn btn-secondary btn-icon"
                            style={{ color: 'var(--accent-rose)' }}
                            title="Delete proxy"
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
