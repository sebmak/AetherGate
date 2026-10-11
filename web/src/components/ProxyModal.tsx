import React, { useState, useEffect } from 'react'
import {
  X,
  Server,
  Shield,
  Lock,
  Cloud,
  CheckCircle2,
  XCircle,
  Plus,
  Trash2,
  Zap,
} from 'lucide-react'
import type { ProxyTarget } from '../types'
import { api } from '../services/api'

interface ProxyModalProps {
  isOpen: boolean
  onClose: () => void
  onSave: (target: Partial<ProxyTarget>) => Promise<void>
  initialData?: ProxyTarget | null
}

export const ProxyModal: React.FC<ProxyModalProps> = ({
  isOpen,
  onClose,
  onSave,
  initialData,
}) => {
  const [name, setName] = useState('')
  const [domain, setDomain] = useState('')
  const [scheme, setScheme] = useState('http')
  const [host, setHost] = useState('')
  const [forceSSL, setForceSSL] = useState(true)
  const [requireAuth, setRequireAuth] = useState(false)
  const [authResource, setAuthResource] = useState('')
  const [passHostHeader, setPassHostHeader] = useState(true)
  const [cloudflareSync, setCloudflareSync] = useState(true)
  const [enabled, setEnabled] = useState(true)

  const [customHeaders, setCustomHeaders] = useState<{ key: string; value: string }[]>([])
  const [newHeaderKey, setNewHeaderKey] = useState('')
  const [newHeaderVal, setNewHeaderVal] = useState('')

  const [isTesting, setIsTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; latency: number; message: string } | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (initialData) {
      setName(initialData.name || '')
      setDomain(initialData.domain || '')
      setScheme(initialData.scheme || 'http')
      setHost(initialData.host || '')
      setForceSSL(initialData.force_ssl ?? true)
      setRequireAuth(initialData.require_auth ?? false)
      setAuthResource(initialData.auth_resource || '')
      setPassHostHeader(initialData.pass_host_header ?? true)
      setCloudflareSync(initialData.cloudflare_sync ?? true)
      setEnabled(initialData.enabled ?? true)

      if (initialData.custom_headers) {
        setCustomHeaders(
          Object.entries(initialData.custom_headers).map(([key, value]) => ({ key, value }))
        )
      } else {
        setCustomHeaders([])
      }
    } else {
      // Defaults for new target
      setName('')
      setDomain('')
      setScheme('http')
      setHost('')
      setForceSSL(true)
      setRequireAuth(false)
      setAuthResource('')
      setPassHostHeader(true)
      setCloudflareSync(true)
      setEnabled(true)
      setCustomHeaders([])
    }
    setTestResult(null)
    setError('')
  }, [initialData, isOpen])

  if (!isOpen) return null

  const handleTestConnection = async () => {
    if (!host) {
      setError('Please provide a target host to test')
      return
    }
    setIsTesting(true)
    setTestResult(null)
    setError('')
    try {
      const res = await api.testUpstream(scheme, host)
      setTestResult({
        success: res.success,
        latency: res.latency_ms,
        message: res.message,
      })
    } catch (err: any) {
      setTestResult({
        success: false,
        latency: 0,
        message: err.message || 'Failed to probe target',
      })
    } finally {
      setIsTesting(false)
    }
  }

  const addHeader = () => {
    if (!newHeaderKey.trim()) return
    setCustomHeaders([...customHeaders, { key: newHeaderKey.trim(), value: newHeaderVal.trim() }])
    setNewHeaderKey('')
    setNewHeaderVal('')
  }

  const removeHeader = (index: number) => {
    setCustomHeaders(customHeaders.filter((_, i) => i !== index))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!domain.trim() || !host.trim()) {
      setError('Domain and Target Host are required')
      return
    }

    setIsSaving(true)
    setError('')

    const headersMap: Record<string, string> = {}
    customHeaders.forEach((h) => {
      if (h.key) headersMap[h.key] = h.value
    })

    try {
      await onSave({
        id: initialData?.id,
        name: name.trim() || domain.trim(),
        domain: domain.trim(),
        scheme,
        host: host.trim(),
        force_ssl: forceSSL,
        require_auth: requireAuth,
        auth_resource: authResource.trim(),
        pass_host_header: passHostHeader,
        cloudflare_sync: cloudflareSync,
        enabled,
        custom_headers: Object.keys(headersMap).length > 0 ? headersMap : undefined,
      })
      onClose()
    } catch (err: any) {
      setError(err.message || 'Failed to save proxy target')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <div style={{
              width: '36px',
              height: '36px',
              borderRadius: '8px',
              background: 'rgba(56, 189, 248, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-cyan)',
            }}>
              <Server size={18} />
            </div>
            <div>
              <h2 style={{ fontSize: '1.25rem', fontWeight: 700, color: '#fff' }}>
                {initialData ? 'Edit Reverse Proxy' : 'New Reverse Proxy'}
              </h2>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                {initialData ? 'Update live routing parameters' : 'Dynamically add an upstream target without restart'}
              </p>
            </div>
          </div>
          <button onClick={onClose} className="btn btn-secondary btn-icon" style={{ borderRadius: '50%' }}>
            <X size={16} />
          </button>
        </div>

        {error && (
          <div style={{
            background: '#2e141a',
            border: '1px solid #7f1d1d',
            borderRadius: 'var(--radius-md)',
            padding: '0.75rem 1rem',
            color: '#f87171',
            fontSize: '0.875rem',
            marginBottom: '1.25rem',
          }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          {/* Friendly Name */}
          <div className="form-group">
            <label className="form-label">Friendly Name</label>
            <input
              id="proxy-input-name"
              type="text"
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Movies Jellyfin Server"
            />
          </div>

          {/* Incoming Domain */}
          <div className="form-group">
            <label className="form-label">Incoming Domain / Hostname *</label>
            <input
              id="proxy-input-domain"
              type="text"
              className="input input-mono"
              value={domain}
              onChange={(e) => setDomain(e.target.value.toLowerCase())}
              placeholder="e.g. movies.home.makinson.me"
              required
            />
            <div className="form-helper">Requests with this Host header will route to the upstream target.</div>
          </div>

          {/* Target Host & Scheme */}
          <div style={{ display: 'grid', gridTemplateColumns: '110px 1fr', gap: '0.75rem', marginBottom: '1.25rem' }}>
            <div>
              <label className="form-label">Scheme</label>
              <select
                id="proxy-input-scheme"
                className="select"
                value={scheme}
                onChange={(e) => setScheme(e.target.value)}
              >
                <option value="http">http://</option>
                <option value="https">https://</option>
              </select>
            </div>
            <div>
              <label className="form-label">Target Upstream (IP:Port or Host) *</label>
              <input
                id="proxy-input-host"
                type="text"
                className="input input-mono"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="e.g. 192.168.1.104:9990"
                required
              />
            </div>
          </div>

          {/* Connectivity Test Bar */}
          <div style={{
            background: '#0e131d',
            border: '1px solid var(--border-subtle)',
            borderRadius: 'var(--radius-md)',
            padding: '0.85rem 1rem',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: '1.5rem',
          }}>
            <div style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              {testResult ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  {testResult.success ? (
                    <>
                      <CheckCircle2 size={16} color="var(--accent-emerald)" />
                      <span style={{ color: 'var(--accent-emerald)', fontWeight: 600 }}>Reachable</span>
                      <span style={{ color: 'var(--text-muted)' }}>({testResult.latency}ms) - {testResult.message}</span>
                    </>
                  ) : (
                    <>
                      <XCircle size={16} color="var(--accent-rose)" />
                      <span style={{ color: 'var(--accent-rose)', fontWeight: 600 }}>Failed:</span>
                      <span style={{ color: 'var(--text-muted)' }}>{testResult.message}</span>
                    </>
                  )}
                </div>
              ) : (
                <span>Test if upstream target is responding</span>
              )}
            </div>
            <button
              id="btn-test-upstream"
              type="button"
              onClick={handleTestConnection}
              disabled={isTesting || !host}
              className="btn btn-outline-cyan"
              style={{ padding: '0.4rem 0.85rem', fontSize: '0.8125rem' }}
            >
              <Zap size={14} className={isTesting ? 'spin' : ''} />
              {isTesting ? 'Testing...' : 'Test Upstream'}
            </button>
          </div>

          {/* Routing & Security Toggles */}
          <div style={{
            display: 'flex',
            flexDirection: 'column',
            gap: '1rem',
            background: '#0e131d',
            border: '1px solid var(--border-subtle)',
            borderRadius: 'var(--radius-md)',
            padding: '1.25rem',
            marginBottom: '1.5rem',
          }}>
            {/* Force SSL */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.875rem', fontWeight: 600, color: '#fff' }}>
                  <Shield size={16} color="var(--accent-cyan)" />
                  Force SSL / HTTPS Redirect
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.2rem' }}>
                  Redirect plain HTTP requests to HTTPS automatically
                </div>
              </div>
              <label className="switch">
                <input
                  id="toggle-force-ssl"
                  type="checkbox"
                  checked={forceSSL}
                  onChange={(e) => setForceSSL(e.target.checked)}
                />
                <span className="switch-slider" />
              </label>
            </div>

            {/* Cloudflare DDNS Sync */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderTop: '1px solid var(--border-subtle)', paddingTop: '1rem' }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.875rem', fontWeight: 600, color: '#fff' }}>
                  <Cloud size={16} color="var(--accent-blue)" />
                  Sync with Cloudflare DDNS
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.2rem' }}>
                  Keep Cloudflare DNS A record pointed to this server's public IP
                </div>
              </div>
              <label className="switch">
                <input
                  id="toggle-cloudflare-sync"
                  type="checkbox"
                  checked={cloudflareSync}
                  onChange={(e) => setCloudflareSync(e.target.checked)}
                />
                <span className="switch-slider" />
              </label>
            </div>

            {/* Require Authentication */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderTop: '1px solid var(--border-subtle)', paddingTop: '1rem' }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.875rem', fontWeight: 600, color: '#fff' }}>
                  <Lock size={16} color="var(--accent-amber)" />
                  Require Gateway Authentication
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.2rem' }}>
                  Prompt users with a secure sign-in gateway before forwarding
                </div>
              </div>
              <label className="switch">
                <input
                  id="toggle-require-auth"
                  type="checkbox"
                  checked={requireAuth}
                  onChange={(e) => setRequireAuth(e.target.checked)}
                />
                <span className="switch-slider" />
              </label>
            </div>

            {requireAuth && (
              <div style={{ marginTop: '0.5rem', paddingLeft: '1.5rem' }}>
                <label className="form-label">Auth Resource Identifier (Optional)</label>
                <input
                  type="text"
                  className="input"
                  value={authResource}
                  onChange={(e) => setAuthResource(e.target.value)}
                  placeholder="e.g. server.local"
                />
              </div>
            )}
          </div>

          {/* Custom Headers Accordion */}
          <div style={{ marginBottom: '1.5rem' }}>
            <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: '#cbd5e1', marginBottom: '0.5rem' }}>
              Custom Headers Forwarded to Upstream
            </div>
            {customHeaders.map((h, idx) => (
              <div key={idx} style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginBottom: '0.4rem' }}>
                <span style={{ flex: 1, padding: '0.4rem 0.6rem', background: '#0e131d', border: '1px solid var(--border-subtle)', borderRadius: '4px', fontSize: '0.8rem', fontFamily: 'monospace' }}>
                  {h.key}: {h.value}
                </span>
                <button
                  type="button"
                  onClick={() => removeHeader(idx)}
                  className="btn btn-secondary btn-icon"
                  style={{ padding: '0.35rem' }}
                >
                  <Trash2 size={13} color="var(--accent-rose)" />
                </button>
              </div>
            ))}
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
              <input
                type="text"
                className="input input-mono"
                style={{ flex: 1, padding: '0.45rem 0.75rem' }}
                placeholder="Header Name"
                value={newHeaderKey}
                onChange={(e) => setNewHeaderKey(e.target.value)}
              />
              <input
                type="text"
                className="input input-mono"
                style={{ flex: 1, padding: '0.45rem 0.75rem' }}
                placeholder="Value"
                value={newHeaderVal}
                onChange={(e) => setNewHeaderVal(e.target.value)}
              />
              <button
                type="button"
                onClick={addHeader}
                className="btn btn-secondary"
                style={{ padding: '0.45rem 0.85rem' }}
              >
                <Plus size={14} /> Add
              </button>
            </div>
          </div>

          {/* Action Buttons */}
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem' }}>
            <button
              type="button"
              onClick={onClose}
              className="btn btn-secondary"
              disabled={isSaving}
            >
              Cancel
            </button>
            <button
              id="btn-save-proxy"
              type="submit"
              className="btn btn-primary"
              disabled={isSaving}
            >
              {isSaving ? 'Saving...' : initialData ? 'Update Proxy' : 'Create Proxy'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
