import React, { useState, useEffect, useRef } from 'react'
import {
  Activity,
  Shield,
  ShieldAlert,
  Cloud,
  Server,
  AlertTriangle,
  AlertCircle,
  Info,
  Terminal,
  Play,
  Pause,
  Trash2,
  Search,
  Download,
  CheckCircle2,
} from 'lucide-react'
import type { RequestLog, SystemLog } from '../types'
import { api } from '../services/api'

export const LogsView: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'system' | 'access'>('system')

  // System Logs State
  const [systemLogs, setSystemLogs] = useState<SystemLog[]>([])
  const [isSystemStreaming, setIsSystemStreaming] = useState(true)
  const [systemSearch, setSystemSearch] = useState('')
  const [levelFilter, setLevelFilter] = useState<'ALL' | 'ERROR' | 'WARN' | 'INFO' | 'DEBUG'>('ALL')
  const [sourceFilter, setSourceFilter] = useState<'ALL' | 'TLS' | 'DDNS' | 'PROXY' | 'AUTH' | 'SYSTEM'>('ALL')
  const systemEventSourceRef = useRef<EventSource | null>(null)

  // Access Logs State
  const [accessLogs, setAccessLogs] = useState<RequestLog[]>([])
  const [isAccessStreaming, setIsAccessStreaming] = useState(true)
  const [accessSearch, setAccessSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | '2xx' | '3xx' | '4xx' | '5xx'>('all')
  const accessEventSourceRef = useRef<EventSource | null>(null)

  // 1. Fetch initial logs
  useEffect(() => {
    api.getSystemLogs({ limit: 200 })
      .then((logs) => setSystemLogs(logs || []))
      .catch(() => {})

    api.getLogs(150)
      .then((logs) => setAccessLogs(logs || []))
      .catch(() => {})
  }, [])

  // 2. Setup SSE Stream for System Logs
  useEffect(() => {
    if (!isSystemStreaming) {
      if (systemEventSourceRef.current) {
        systemEventSourceRef.current.close()
        systemEventSourceRef.current = null
      }
      return
    }

    const token = localStorage.getItem('proxyserver_token')
    const url = `/api/system-logs/stream${token ? `?token=${encodeURIComponent(token)}` : ''}`
    const es = new EventSource(url)
    systemEventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const newLog: SystemLog = JSON.parse(event.data)
        setSystemLogs((prev) => [newLog, ...prev.slice(0, 999)])
      } catch {
        // Ignored
      }
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      es.close()
    }
  }, [isSystemStreaming])

  // 3. Setup SSE Stream for HTTP Access Logs
  useEffect(() => {
    if (!isAccessStreaming) {
      if (accessEventSourceRef.current) {
        accessEventSourceRef.current.close()
        accessEventSourceRef.current = null
      }
      return
    }

    const token = localStorage.getItem('proxyserver_token')
    const url = `/api/logs/stream${token ? `?token=${encodeURIComponent(token)}` : ''}`
    const es = new EventSource(url)
    accessEventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const newLog: RequestLog = JSON.parse(event.data)
        setAccessLogs((prev) => [newLog, ...prev.slice(0, 499)])
      } catch {
        // Ignored
      }
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      es.close()
    }
  }, [isAccessStreaming])

  // Filtered System Logs
  const filteredSystemLogs = systemLogs.filter((log) => {
    if (levelFilter !== 'ALL' && log.level !== levelFilter) return false
    if (sourceFilter !== 'ALL' && log.source !== sourceFilter) return false

    if (systemSearch) {
      const term = systemSearch.toLowerCase()
      const matchMsg = log.message.toLowerCase().includes(term)
      const matchSource = log.source.toLowerCase().includes(term)
      const matchDetails = log.details ? log.details.toLowerCase().includes(term) : false
      if (!matchMsg && !matchSource && !matchDetails) return false
    }

    return true
  })

  // Filtered Access Logs
  const filteredAccessLogs = accessLogs.filter((log) => {
    if (accessSearch) {
      const term = accessSearch.toLowerCase()
      const matches =
        log.host.toLowerCase().includes(term) ||
        log.path.toLowerCase().includes(term) ||
        log.client_ip.toLowerCase().includes(term) ||
        log.target.toLowerCase().includes(term)
      if (!matches) return false
    }

    if (statusFilter === '2xx') return log.status_code >= 200 && log.status_code < 300
    if (statusFilter === '3xx') return log.status_code >= 300 && log.status_code < 400
    if (statusFilter === '4xx') return log.status_code >= 400 && log.status_code < 500
    if (statusFilter === '5xx') return log.status_code >= 500

    return true
  })

  // Statistics for System Logs
  const tlsErrorCount = systemLogs.filter((l) => l.source === 'TLS' && l.level === 'ERROR').length
  const totalErrorCount = systemLogs.filter((l) => l.level === 'ERROR').length
  const totalWarnCount = systemLogs.filter((l) => l.level === 'WARN').length

  const handleClearSystemLogs = async () => {
    try {
      await api.clearSystemLogs()
      setSystemLogs([])
    } catch {
      setSystemLogs([])
    }
  }

  const handleClearAccessLogs = async () => {
    try {
      await api.clearLogs()
      setAccessLogs([])
    } catch {
      setAccessLogs([])
    }
  }

  const exportSystemLogsJSON = () => {
    const blob = new Blob([JSON.stringify(systemLogs, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `proxyserver-system-logs-${new Date().toISOString()}.json`
    a.click()
  }

  const exportAccessLogsJSON = () => {
    const blob = new Blob([JSON.stringify(accessLogs, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `proxyserver-access-logs-${new Date().toISOString()}.json`
    a.click()
  }

  const getLevelBadge = (level: string) => {
    switch (level) {
      case 'ERROR':
        return (
          <span className="badge badge-rose" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem', fontFamily: 'monospace' }}>
            <AlertTriangle size={11} /> ERROR
          </span>
        )
      case 'WARN':
        return (
          <span className="badge badge-amber" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem', fontFamily: 'monospace' }}>
            <AlertCircle size={11} /> WARN
          </span>
        )
      case 'INFO':
        return (
          <span className="badge badge-cyan" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem', fontFamily: 'monospace' }}>
            <Info size={11} /> INFO
          </span>
        )
      case 'DEBUG':
        return (
          <span className="badge badge-purple" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem', fontFamily: 'monospace' }}>
            DEBUG
          </span>
        )
      default:
        return <span className="badge badge-slate">{level}</span>
    }
  }

  const getSourceBadge = (source: string) => {
    switch (source) {
      case 'TLS':
      case 'ACME':
        return (
          <span style={{
            fontSize: '0.72rem',
            padding: '0.15rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
            background: '#0c2e24',
            border: '1px solid #10b981',
            color: '#34d399',
            fontFamily: 'monospace',
            fontWeight: 600,
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.25rem',
          }}>
            <Shield size={11} /> {source}
          </span>
        )
      case 'DDNS':
        return (
          <span style={{
            fontSize: '0.72rem',
            padding: '0.15rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
            background: '#0c283d',
            border: '1px solid #0284c7',
            color: '#38bdf8',
            fontFamily: 'monospace',
            fontWeight: 600,
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.25rem',
          }}>
            <Cloud size={11} /> DDNS
          </span>
        )
      case 'PROXY':
        return (
          <span style={{
            fontSize: '0.72rem',
            padding: '0.15rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
            background: '#2b1b0d',
            border: '1px solid #d97706',
            color: '#fbbf24',
            fontFamily: 'monospace',
            fontWeight: 600,
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.25rem',
          }}>
            <Server size={11} /> PROXY
          </span>
        )
      case 'AUTH':
        return (
          <span style={{
            fontSize: '0.72rem',
            padding: '0.15rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
            background: '#241438',
            border: '1px solid #9333ea',
            color: '#c084fc',
            fontFamily: 'monospace',
            fontWeight: 600,
          }}>
            AUTH
          </span>
        )
      default:
        return (
          <span style={{
            fontSize: '0.72rem',
            padding: '0.15rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
            background: '#161e2b',
            border: '1px solid var(--border-subtle)',
            color: 'var(--text-muted)',
            fontFamily: 'monospace',
            fontWeight: 600,
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.25rem',
          }}>
            <Terminal size={11} /> {source}
          </span>
        )
    }
  }

  const getStatusBadge = (code: number) => {
    if (code >= 200 && code < 300) {
      return <span className="badge badge-emerald" style={{ fontFamily: 'monospace' }}>{code}</span>
    }
    if (code >= 300 && code < 400) {
      return <span className="badge badge-cyan" style={{ fontFamily: 'monospace' }}>{code}</span>
    }
    if (code >= 400 && code < 500) {
      return <span className="badge badge-amber" style={{ fontFamily: 'monospace' }}>{code}</span>
    }
    return <span className="badge badge-rose" style={{ fontFamily: 'monospace' }}>{code}</span>
  }

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Activity size={28} color="var(--accent-cyan)" />
            System & Traffic Telemetry
          </h1>
          <p className="page-subtitle">Real-time internal diagnostics, TLS certificate issuance, and HTTP access telemetry</p>
        </div>

        {/* Action Buttons */}
        <div style={{ display: 'flex', gap: '0.75rem' }}>
          {activeTab === 'system' ? (
            <>
              <button
                onClick={() => setIsSystemStreaming(!isSystemStreaming)}
                className={`btn ${isSystemStreaming ? 'btn-outline-cyan' : 'btn-secondary'}`}
              >
                {isSystemStreaming ? (
                  <>
                    <Pause size={15} /> Pause Stream
                  </>
                ) : (
                  <>
                    <Play size={15} /> Resume Stream
                  </>
                )}
              </button>
              <button onClick={handleClearSystemLogs} className="btn btn-secondary">
                <Trash2 size={15} /> Clear
              </button>
              <button onClick={exportSystemLogsJSON} className="btn btn-secondary" title="Export system logs as JSON">
                <Download size={15} /> Export
              </button>
            </>
          ) : (
            <>
              <button
                onClick={() => setIsAccessStreaming(!isAccessStreaming)}
                className={`btn ${isAccessStreaming ? 'btn-outline-cyan' : 'btn-secondary'}`}
              >
                {isAccessStreaming ? (
                  <>
                    <Pause size={15} /> Pause Stream
                  </>
                ) : (
                  <>
                    <Play size={15} /> Resume Stream
                  </>
                )}
              </button>
              <button onClick={handleClearAccessLogs} className="btn btn-secondary">
                <Trash2 size={15} /> Clear
              </button>
              <button onClick={exportAccessLogsJSON} className="btn btn-secondary" title="Export access logs as JSON">
                <Download size={15} /> Export
              </button>
            </>
          )}
        </div>
      </div>

      {/* Primary Tab Switcher */}
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1.25rem', borderBottom: '1px solid var(--border-subtle)', paddingBottom: '0.75rem' }}>
        <button
          onClick={() => setActiveTab('system')}
          className={`btn ${activeTab === 'system' ? 'btn-primary' : 'btn-secondary'}`}
          style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: '0.5rem' }}
        >
          <Shield size={16} />
          <span>System & TLS Logs</span>
          {tlsErrorCount > 0 ? (
            <span style={{
              background: 'var(--accent-rose)',
              color: '#fff',
              fontSize: '0.7rem',
              fontWeight: 700,
              padding: '0.1rem 0.45rem',
              borderRadius: '10px',
              marginLeft: '0.25rem',
            }}>
              {tlsErrorCount} TLS error{tlsErrorCount > 1 ? 's' : ''}
            </span>
          ) : totalErrorCount > 0 ? (
            <span style={{
              background: 'var(--accent-rose)',
              color: '#fff',
              fontSize: '0.7rem',
              fontWeight: 700,
              padding: '0.1rem 0.45rem',
              borderRadius: '10px',
              marginLeft: '0.25rem',
            }}>
              {totalErrorCount} error{totalErrorCount > 1 ? 's' : ''}
            </span>
          ) : totalWarnCount > 0 ? (
            <span style={{
              background: 'var(--accent-amber)',
              color: '#000',
              fontSize: '0.7rem',
              fontWeight: 700,
              padding: '0.1rem 0.45rem',
              borderRadius: '10px',
              marginLeft: '0.25rem',
            }}>
              {totalWarnCount} warn
            </span>
          ) : null}
        </button>

        <button
          onClick={() => setActiveTab('access')}
          className={`btn ${activeTab === 'access' ? 'btn-primary' : 'btn-secondary'}`}
          style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
        >
          <Activity size={16} />
          <span>HTTP Traffic Logs</span>
          <span style={{ fontSize: '0.75rem', opacity: 0.7, fontFamily: 'monospace' }}>
            ({accessLogs.length})
          </span>
        </button>
      </div>

      {/* ================= TAB 1: SYSTEM & TLS LOGS ================= */}
      {activeTab === 'system' && (
        <>
          {/* Certificate Diagnostic Banner if errors exist */}
          {tlsErrorCount > 0 && (
            <div style={{
              background: '#2b1216',
              border: '1px solid #7f1d1d',
              borderRadius: 'var(--radius-md)',
              padding: '1rem 1.25rem',
              marginBottom: '1.25rem',
              display: 'flex',
              alignItems: 'flex-start',
              gap: '1rem',
            }}>
              <ShieldAlert size={22} color="var(--accent-rose)" style={{ flexShrink: 0, marginTop: '2px' }} />
              <div>
                <div style={{ fontWeight: 600, color: '#fca5a5', fontSize: '0.92rem', marginBottom: '0.25rem' }}>
                  Certificate Issuance Alert ({tlsErrorCount} Error{tlsErrorCount > 1 ? 's' : ''} Logged)
                </div>
                <div style={{ fontSize: '0.8125rem', color: '#cbd5e1', lineHeight: '1.5' }}>
                  Let's Encrypt ACME certificate generation failed for one or more domains, causing ProxyServer to serve a fallback dev certificate (which causes browsers to report <em>Certificate Invalid</em>). Review the highlighted logs below for exact Let's Encrypt error responses and connectivity diagnosis.
                </div>
              </div>
            </div>
          )}

          {/* System Filters */}
          <div className="card" style={{ padding: '1rem 1.25rem', marginBottom: '1.25rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', flexWrap: 'wrap' }}>
              {/* Search */}
              <div style={{ position: 'relative', flex: 1, minWidth: '240px' }}>
                <Search size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
                <input
                  type="text"
                  className="input"
                  style={{ paddingLeft: '2.5rem' }}
                  placeholder="Search errors, domain names, Let's Encrypt diagnostics..."
                  value={systemSearch}
                  onChange={(e) => setSystemSearch(e.target.value)}
                />
              </div>

              {/* Source Filter */}
              <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap', alignItems: 'center' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600, marginRight: '0.25rem' }}>Source:</span>
                {(['ALL', 'TLS', 'DDNS', 'PROXY', 'AUTH', 'SYSTEM'] as const).map((source) => (
                  <button
                    key={source}
                    onClick={() => setSourceFilter(source)}
                    className={`btn ${sourceFilter === source ? 'btn-outline-cyan' : 'btn-secondary'}`}
                    style={{ fontSize: '0.72rem', padding: '0.35rem 0.65rem' }}
                  >
                    {source === 'TLS' ? 'TLS / ACME' : source}
                  </button>
                ))}
              </div>

              {/* Level Filter */}
              <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap', alignItems: 'center' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600, marginRight: '0.25rem' }}>Level:</span>
                {(['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG'] as const).map((level) => (
                  <button
                    key={level}
                    onClick={() => setLevelFilter(level)}
                    className={`btn ${levelFilter === level ? 'btn-outline-cyan' : 'btn-secondary'}`}
                    style={{ fontSize: '0.72rem', padding: '0.35rem 0.65rem' }}
                  >
                    {level}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* System Logs List */}
          <div className="card" style={{ padding: 0 }}>
            {filteredSystemLogs.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '4rem 1rem', color: 'var(--text-muted)' }}>
                <CheckCircle2 size={40} style={{ margin: '0 auto 1rem', opacity: 0.3, color: 'var(--accent-emerald)' }} />
                <h3 style={{ fontSize: '1.1rem', fontWeight: 600, color: '#fff', marginBottom: '0.35rem' }}>
                  No system logs matching criteria
                </h3>
                <p style={{ fontSize: '0.875rem' }}>
                  Internal operations, TLS handshakes, ACME challenges, and DDNS sync events will appear here in real-time.
                </p>
              </div>
            ) : (
              <div className="table-container" style={{ border: 'none', maxHeight: '720px', overflowY: 'auto' }}>
                <table className="table">
                  <thead style={{ position: 'sticky', top: 0, zIndex: 10 }}>
                    <tr>
                      <th style={{ width: '130px' }}>Timestamp</th>
                      <th style={{ width: '95px' }}>Level</th>
                      <th style={{ width: '110px' }}>Source</th>
                      <th>Diagnostic Message</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredSystemLogs.map((log) => {
                      const isTlsError = log.source === 'TLS' && (log.level === 'ERROR' || log.level === 'WARN')
                      return (
                        <tr
                          key={log.id}
                          style={{
                            background: log.level === 'ERROR' ? 'rgba(239, 68, 68, 0.06)' : log.level === 'WARN' ? 'rgba(245, 158, 11, 0.04)' : undefined,
                            borderLeft: log.level === 'ERROR' ? '3px solid var(--accent-rose)' : log.level === 'WARN' ? '3px solid var(--accent-amber)' : '3px solid transparent',
                          }}
                        >
                          <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                            {new Date(log.timestamp).toLocaleTimeString()}
                          </td>
                          <td>{getLevelBadge(log.level)}</td>
                          <td>{getSourceBadge(log.source)}</td>
                          <td>
                            <div style={{
                              fontFamily: "'JetBrains Mono', monospace",
                              fontSize: '0.8125rem',
                              color: log.level === 'ERROR' ? '#fca5a5' : log.level === 'WARN' ? '#fde68a' : 'var(--text-primary)',
                              wordBreak: 'break-word',
                              lineHeight: '1.45',
                            }}>
                              {log.message}
                            </div>
                            {isTlsError && log.message.toLowerCase().includes('failed') && (
                              <div style={{
                                marginTop: '0.4rem',
                                padding: '0.4rem 0.65rem',
                                borderRadius: 'var(--radius-sm)',
                                background: '#1c1524',
                                border: '1px solid #4a284e',
                                fontSize: '0.75rem',
                                color: '#e9d5ff',
                                display: 'flex',
                                alignItems: 'center',
                                gap: '0.4rem',
                              }}>
                                <span>💡 <strong>Certificate Troubleshooting:</strong> Ensure router port forwards port 80 & 443 to this machine. If using Cloudflare proxy, consider Cloudflare SSL "Full" mode or generate a Cloudflare Origin CA certificate.</span>
                              </div>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}

      {/* ================= TAB 2: HTTP ACCESS LOGS ================= */}
      {activeTab === 'access' && (
        <>
          {/* Access Logs Filter */}
          <div className="card" style={{ padding: '1rem 1.25rem', marginBottom: '1.25rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', flexWrap: 'wrap' }}>
              <div style={{ position: 'relative', flex: 1, minWidth: '240px' }}>
                <Search size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
                <input
                  type="text"
                  className="input"
                  style={{ paddingLeft: '2.5rem' }}
                  placeholder="Search by domain, path, IP, or upstream target..."
                  value={accessSearch}
                  onChange={(e) => setAccessSearch(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                {(['all', '2xx', '3xx', '4xx', '5xx'] as const).map((filter) => (
                  <button
                    key={filter}
                    onClick={() => setStatusFilter(filter)}
                    className={`btn ${statusFilter === filter ? 'btn-outline-cyan' : 'btn-secondary'}`}
                    style={{ fontSize: '0.75rem', padding: '0.4rem 0.75rem', textTransform: 'uppercase' }}
                  >
                    {filter}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Access Logs Table */}
          <div className="card" style={{ padding: 0 }}>
            {filteredAccessLogs.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '4rem 1rem', color: 'var(--text-muted)' }}>
                <Activity size={40} style={{ margin: '0 auto 1rem', opacity: 0.3 }} />
                <h3 style={{ fontSize: '1.1rem', fontWeight: 600, color: '#fff', marginBottom: '0.35rem' }}>
                  No requests recorded yet
                </h3>
                <p style={{ fontSize: '0.875rem' }}>
                  Incoming reverse proxy requests will stream here in real-time.
                </p>
              </div>
            ) : (
              <div className="table-container" style={{ border: 'none', maxHeight: '680px', overflowY: 'auto' }}>
                <table className="table">
                  <thead style={{ position: 'sticky', top: 0, zIndex: 10 }}>
                    <tr>
                      <th>Time</th>
                      <th>Method</th>
                      <th>Status</th>
                      <th>Host Domain</th>
                      <th>Path</th>
                      <th>Upstream Target</th>
                      <th>Duration</th>
                      <th>Client IP</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredAccessLogs.map((log) => (
                      <tr key={log.id}>
                        <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                          {new Date(log.timestamp).toLocaleTimeString()}
                        </td>
                        <td>
                          <span style={{
                            fontWeight: 700,
                            fontSize: '0.75rem',
                            fontFamily: 'monospace',
                            color: log.method === 'GET' ? '#38bdf8' : log.method === 'POST' ? '#34d399' : log.method === 'DELETE' ? '#f43f5e' : '#fbbf24',
                          }}>
                            {log.method}
                          </span>
                        </td>
                        <td>{getStatusBadge(log.status_code)}</td>
                        <td>
                          <span style={{ fontWeight: 600, color: '#fff', fontFamily: 'monospace', fontSize: '0.8125rem' }}>
                            {log.host}
                          </span>
                        </td>
                        <td>
                          <span style={{ color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '0.8125rem' }}>
                            {log.path}
                          </span>
                        </td>
                        <td>
                          <span style={{ color: '#cbd5e1', fontSize: '0.75rem', fontFamily: 'monospace' }}>
                            {log.target}
                          </span>
                        </td>
                        <td>
                          <span style={{
                            fontSize: '0.75rem',
                            fontFamily: 'monospace',
                            color: log.duration_ms > 500 ? 'var(--accent-amber)' : 'var(--text-muted)',
                          }}>
                            {log.duration_ms}ms
                          </span>
                        </td>
                        <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontFamily: 'monospace' }}>
                          {log.client_ip}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
