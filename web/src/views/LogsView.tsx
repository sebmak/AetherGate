import React, { useState, useEffect, useRef } from 'react'
import {
  Activity,
  Play,
  Pause,
  Trash2,
  Search,
  Download,
} from 'lucide-react'
import type { RequestLog } from '../types'
import { api } from '../services/api'

export const LogsView: React.FC = () => {
  const [logs, setLogs] = useState<RequestLog[]>([])
  const [isStreaming, setIsStreaming] = useState(true)
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | '2xx' | '3xx' | '4xx' | '5xx'>('all')
  const eventSourceRef = useRef<EventSource | null>(null)

  // Fetch initial logs
  useEffect(() => {
    api.getLogs().then((initialLogs) => {
      setLogs(initialLogs || [])
    }).catch(() => {})
  }, [])

  // Setup SSE stream
  useEffect(() => {
    if (!isStreaming) {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      return
    }

    const token = localStorage.getItem('proxyserver_token')
    const url = `/api/logs/stream${token ? `?token=${encodeURIComponent(token)}` : ''}`
    const es = new EventSource(url)
    eventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const newLog: RequestLog = JSON.parse(event.data)
        setLogs((prev) => [newLog, ...prev.slice(0, 499)])
      } catch (err) {
        // Ignored
      }
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      es.close()
    }
  }, [isStreaming])

  const filteredLogs = logs.filter((log) => {
    const term = searchTerm.toLowerCase()
    const matchesSearch =
      log.host.toLowerCase().includes(term) ||
      log.path.toLowerCase().includes(term) ||
      log.client_ip.toLowerCase().includes(term) ||
      log.target.toLowerCase().includes(term)

    if (!matchesSearch) return false

    if (statusFilter === '2xx') return log.status_code >= 200 && log.status_code < 300
    if (statusFilter === '3xx') return log.status_code >= 300 && log.status_code < 400
    if (statusFilter === '4xx') return log.status_code >= 400 && log.status_code < 500
    if (statusFilter === '5xx') return log.status_code >= 500

    return true
  })

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

  const exportLogsJSON = () => {
    const blob = new Blob([JSON.stringify(logs, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `proxyserver-logs-${new Date().toISOString()}.json`
    a.click()
  }

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Activity size={28} color="var(--accent-cyan)" />
            Live Access Logs
          </h1>
          <p className="page-subtitle">Real-time HTTP request telemetry and reverse proxy audit stream</p>
        </div>
        <div style={{ display: 'flex', gap: '0.75rem' }}>
          <button
            onClick={() => setIsStreaming(!isStreaming)}
            className={`btn ${isStreaming ? 'btn-outline-cyan' : 'btn-secondary'}`}
          >
            {isStreaming ? (
              <>
                <Pause size={15} /> Pause Stream
              </>
            ) : (
              <>
                <Play size={15} /> Resume Stream
              </>
            )}
          </button>
          <button onClick={() => setLogs([])} className="btn btn-secondary">
            <Trash2 size={15} /> Clear
          </button>
          <button onClick={exportLogsJSON} className="btn btn-secondary" title="Export logs as JSON">
            <Download size={15} /> Export
          </button>
        </div>
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
              placeholder="Search by domain, path, IP, or upstream..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
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

      {/* Live Logs Table */}
      <div className="card" style={{ padding: 0 }}>
        {filteredLogs.length === 0 ? (
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
                {filteredLogs.map((log) => (
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
    </div>
  )
}
