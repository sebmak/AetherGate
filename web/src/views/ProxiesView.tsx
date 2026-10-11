import React, { useState } from 'react'
import {
  Server,
  Plus,
  Search,
  ExternalLink,
  Shield,
  ShieldCheck,
  Lock,
  Cloud,
  Zap,
  Edit2,
  Trash2,
  Copy,
  Check,
  RefreshCw,
  AlertTriangle,
  AlertCircle,
  X,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
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

export type SortField = 'name' | 'domain' | 'host' | 'ssl' | 'health' | 'enabled'
export type SortDirection = 'asc' | 'desc'

export const ProxiesView: React.FC<ProxiesViewProps> = ({
  proxies,
  onOpenNewProxy,
  onEditProxy,
  onDeleteProxy,
  onToggleProxy,
  onRefresh,
}) => {
  const [searchTerm, setSearchTerm] = useState('')
  const [filterStatus, setFilterStatus] = useState<'all' | 'active' | 'disabled' | 'healthy' | 'unhealthy'>('all')
  const [sortField, setSortField] = useState<SortField>(() => {
    return (localStorage.getItem('proxies_sort_field') as SortField) || 'name'
  })
  const [sortDirection, setSortDirection] = useState<SortDirection>(() => {
    return (localStorage.getItem('proxies_sort_direction') as SortDirection) || 'asc'
  })
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<Record<string, { success: boolean; latency: number; message: string }>>({})
  const [copiedDomain, setCopiedDomain] = useState<string | null>(null)
  const [renewingId, setRenewingId] = useState<string | null>(null)
  const [certNotification, setCertNotification] = useState<{ id: string; success: boolean; message: string } | null>(null)

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      const nextDir: SortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
      setSortDirection(nextDir)
      localStorage.setItem('proxies_sort_direction', nextDir)
    } else {
      setSortField(field)
      setSortDirection('asc')
      localStorage.setItem('proxies_sort_field', field)
      localStorage.setItem('proxies_sort_direction', 'asc')
    }
  }

  const handleSortFieldChange = (field: SortField) => {
    setSortField(field)
    localStorage.setItem('proxies_sort_field', field)
  }

  const toggleSortDirection = () => {
    const nextDir: SortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
    setSortDirection(nextDir)
    localStorage.setItem('proxies_sort_direction', nextDir)
  }

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

  const handleRenewCert = async (p: ProxyTarget) => {
    setRenewingId(p.id)
    setCertNotification(null)
    try {
      const res = await api.renewCertificate(p.id)
      if (res.success) {
        setCertNotification({
          id: p.id,
          success: true,
          message: `Certificate for ${p.domain} renewed successfully (${res.cert?.days_remaining ?? 0} days remaining).`,
        })
      } else {
        setCertNotification({
          id: p.id,
          success: false,
          message: res.error || `Failed to renew certificate for ${p.domain}`,
        })
      }
      onRefresh()
    } catch (err: any) {
      setCertNotification({
        id: p.id,
        success: false,
        message: err.message || `Failed to renew certificate for ${p.domain}`,
      })
      onRefresh()
    } finally {
      setRenewingId(null)
      setTimeout(() => setCertNotification(null), 8000)
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

  const sorted = [...filtered].sort((a, b) => {
    let cmp = 0
    switch (sortField) {
      case 'name':
        cmp = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
        if (cmp === 0) cmp = a.domain.localeCompare(b.domain)
        break
      case 'domain':
        cmp = a.domain.localeCompare(b.domain, undefined, { sensitivity: 'base' })
        if (cmp === 0) cmp = a.name.localeCompare(b.name)
        break
      case 'host':
        cmp = `${a.scheme}://${a.host}`.localeCompare(`${b.scheme}://${b.host}`, undefined, { sensitivity: 'base' })
        break
      case 'ssl': {
        const statusRank = (s?: string) => {
          switch (s) {
            case 'valid': return 1
            case 'expiring_soon': return 2
            case 'self_signed': return 3
            case 'renewing': return 4
            case 'expired': return 5
            case 'error': return 6
            default: return 7
          }
        }
        const rankA = statusRank(a.tls_cert?.status)
        const rankB = statusRank(b.tls_cert?.status)
        cmp = rankA - rankB
        if (cmp === 0) {
          cmp = (a.tls_cert?.days_remaining ?? 0) - (b.tls_cert?.days_remaining ?? 0)
        }
        break
      }
      case 'health': {
        const valA = a.enabled ? (a.is_healthy ? 2 : 1) : 0
        const valB = b.enabled ? (b.is_healthy ? 2 : 1) : 0
        cmp = valA - valB
        break
      }
      case 'enabled':
        cmp = (a.enabled ? 1 : 0) - (b.enabled ? 1 : 0)
        break
    }
    return sortDirection === 'asc' ? cmp : -cmp
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

          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', gap: '0.4rem', flexWrap: 'wrap' }}>
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

            <div style={{ width: '1px', height: '24px', background: 'var(--border-subtle)', margin: '0 0.25rem' }} />

            <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem' }}>
              <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>Sort:</span>
              <select
                className="select"
                style={{ fontSize: '0.75rem', padding: '0.35rem 0.6rem', height: '32px', minWidth: '125px' }}
                value={sortField}
                onChange={(e) => handleSortFieldChange(e.target.value as SortField)}
                aria-label="Sort reverse proxies"
              >
                <option value="name">Name</option>
                <option value="domain">Domain</option>
                <option value="host">Upstream</option>
                <option value="ssl">SSL Status</option>
                <option value="health">Health</option>
                <option value="enabled">Active State</option>
              </select>
              <button
                onClick={toggleSortDirection}
                className="btn btn-secondary btn-icon"
                title={sortDirection === 'asc' ? 'Sorted Ascending (Click for Descending)' : 'Sorted Descending (Click for Ascending)'}
                style={{ padding: '0.35rem', height: '32px', width: '32px' }}
              >
                {sortDirection === 'asc' ? <ArrowUp size={14} color="var(--accent-cyan)" /> : <ArrowDown size={14} color="var(--accent-cyan)" />}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Certificate Notification Banner */}
      {certNotification && (
        <div style={{
          background: certNotification.success ? '#0f291e' : '#2e141a',
          border: `1px solid ${certNotification.success ? '#165b3d' : '#7f1d1d'}`,
          padding: '0.75rem 1.25rem',
          borderRadius: 'var(--radius-md)',
          color: certNotification.success ? '#34d399' : '#f87171',
          fontSize: '0.85rem',
          marginBottom: '1.25rem',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '1rem',
        }}>
          <span>{certNotification.message}</span>
          <button
            onClick={() => setCertNotification(null)}
            className="btn btn-secondary btn-icon"
            style={{ padding: '0.2rem', minWidth: 'auto', background: 'transparent', border: 'none' }}
          >
            <X size={14} />
          </button>
        </div>
      )}

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
                  <th
                    style={{ width: '50px', cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('enabled')}
                    title="Sort by Active State"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem' }}>
                      <span>State</span>
                      {sortField === 'enabled' ? (
                        sortDirection === 'asc' ? <ArrowUp size={12} color="var(--accent-cyan)" /> : <ArrowDown size={12} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={11} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th
                    style={{ cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('domain')}
                    title="Sort by Incoming Domain"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                      <span style={{ color: sortField === 'domain' ? 'var(--accent-cyan)' : 'inherit', fontWeight: sortField === 'domain' ? 600 : 'inherit' }}>
                        Incoming Domain
                      </span>
                      {sortField === 'domain' ? (
                        sortDirection === 'asc' ? <ArrowUp size={13} color="var(--accent-cyan)" /> : <ArrowDown size={13} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={12} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th
                    style={{ cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('name')}
                    title="Sort by Friendly Name"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                      <span style={{ color: sortField === 'name' ? 'var(--accent-cyan)' : 'inherit', fontWeight: sortField === 'name' ? 600 : 'inherit' }}>
                        Friendly Name
                      </span>
                      {sortField === 'name' ? (
                        sortDirection === 'asc' ? <ArrowUp size={13} color="var(--accent-cyan)" /> : <ArrowDown size={13} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={12} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th
                    style={{ cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('host')}
                    title="Sort by Upstream Target"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                      <span style={{ color: sortField === 'host' ? 'var(--accent-cyan)' : 'inherit', fontWeight: sortField === 'host' ? 600 : 'inherit' }}>
                        Upstream Target
                      </span>
                      {sortField === 'host' ? (
                        sortDirection === 'asc' ? <ArrowUp size={13} color="var(--accent-cyan)" /> : <ArrowDown size={13} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={12} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th
                    style={{ cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('ssl')}
                    title="Sort by SSL / Certificate Status"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                      <span style={{ color: sortField === 'ssl' ? 'var(--accent-cyan)' : 'inherit', fontWeight: sortField === 'ssl' ? 600 : 'inherit' }}>
                        SSL / Certificate
                      </span>
                      {sortField === 'ssl' ? (
                        sortDirection === 'asc' ? <ArrowUp size={13} color="var(--accent-cyan)" /> : <ArrowDown size={13} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={12} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th
                    style={{ cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => handleSort('health')}
                    title="Sort by Health Status"
                  >
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                      <span style={{ color: sortField === 'health' ? 'var(--accent-cyan)' : 'inherit', fontWeight: sortField === 'health' ? 600 : 'inherit' }}>
                        Health Status
                      </span>
                      {sortField === 'health' ? (
                        sortDirection === 'asc' ? <ArrowUp size={13} color="var(--accent-cyan)" /> : <ArrowDown size={13} color="var(--accent-cyan)" />
                      ) : (
                        <ArrowUpDown size={12} style={{ opacity: 0.25 }} />
                      )}
                    </div>
                  </th>
                  <th>Security & Features</th>
                  <th style={{ textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {sorted.map((p) => {
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
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
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
                          {p.domain.split('.').length > 3 && (
                            <div
                              style={{
                                display: 'inline-flex',
                                alignItems: 'center',
                                gap: '0.25rem',
                                fontSize: '0.68rem',
                                color: '#fbbf24',
                                background: '#261b07',
                                border: '1px solid #78350f',
                                padding: '0.1rem 0.4rem',
                                borderRadius: '4px',
                                marginTop: '0.25rem',
                              }}
                              title="Multi-level subdomain: Cloudflare free Universal SSL cannot proxy this domain. If proxied, Chrome throws ERR_SSL_VERSION_OR_CIPHER_MISMATCH. Use DNS-Only or a 1st-level subdomain (e.g. movies.makinson.me)."
                            >
                              <AlertTriangle size={10} /> Multi-level (CF DNS-Only required)
                            </div>
                          )}
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
                          background: '#0c283d',
                          border: '1px solid #0369a1',
                          padding: '0.25rem 0.6rem',
                          borderRadius: '4px',
                        }}>
                          {p.scheme}://{p.host}
                        </span>
                      </td>

                      {/* SSL / Certificate */}
                      <td>
                        {p.tls_cert ? (
                          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '0.35rem' }}>
                              {p.tls_cert.status === 'valid' && (
                                <span className="badge badge-emerald" title={`Valid certificate. Issuer: ${p.tls_cert.issuer}`}>
                                  <ShieldCheck size={11} /> Valid • {p.tls_cert.days_remaining}d left
                                </span>
                              )}
                              {p.tls_cert.status === 'expiring_soon' && (
                                <span className="badge badge-amber" title={`Certificate expiring soon in ${p.tls_cert.days_remaining} days`}>
                                  <AlertTriangle size={11} /> Expiring • {p.tls_cert.days_remaining}d left
                                </span>
                              )}
                              {p.tls_cert.status === 'expired' && (
                                <span className="badge badge-rose" title="Certificate has expired">
                                  <AlertCircle size={11} /> Expired
                                </span>
                              )}
                              {p.tls_cert.status === 'self_signed' && (
                                <span className="badge badge-cyan" title="Internal self-signed dev fallback certificate">
                                  <Shield size={11} /> Dev Self-Signed
                                </span>
                              )}
                              {p.tls_cert.status === 'error' && (
                                <span className="badge badge-rose" title={p.tls_cert.last_error}>
                                  <AlertCircle size={11} /> Cert Error
                                </span>
                              )}
                              {p.tls_cert.status === 'renewing' && (
                                <span className="badge badge-cyan">
                                  <RefreshCw size={11} className="spin" /> Renewing...
                                </span>
                              )}
                            </div>

                            {p.tls_cert.expires_at && (
                              <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                                Expires: {new Date(p.tls_cert.expires_at).toLocaleDateString()} ({p.tls_cert.days_remaining}d)
                              </div>
                            )}
                            {p.tls_cert.issued_at && (
                              <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                                Renewed: {new Date(p.tls_cert.issued_at).toLocaleDateString()}
                              </div>
                            )}
                            {p.tls_cert.issuer && (
                              <div
                                style={{ fontSize: '0.7rem', color: 'var(--text-muted)', maxWidth: '170px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                                title={`Issuer: ${p.tls_cert.issuer}`}
                              >
                                {p.tls_cert.issuer}
                              </div>
                            )}
                            {p.tls_cert.last_error && (
                              <div
                                style={{ fontSize: '0.7rem', color: 'var(--accent-rose)', maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                                title={p.tls_cert.last_error}
                              >
                                {p.tls_cert.last_error}
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className="badge badge-gray">No Cert</span>
                        )}
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
                            onClick={() => handleRenewCert(p)}
                            disabled={renewingId === p.id}
                            className="btn btn-secondary btn-icon"
                            title="Regenerate / Renew SSL certificate now"
                          >
                            <RefreshCw size={14} className={renewingId === p.id ? 'spin' : ''} color={renewingId === p.id ? 'var(--accent-cyan)' : undefined} />
                          </button>
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
