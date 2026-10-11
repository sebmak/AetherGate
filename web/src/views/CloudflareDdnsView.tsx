import React, { useState, useEffect } from 'react'
import {
  Cloud,
  RefreshCw,
  CheckCircle2,
  AlertTriangle,
  Key,
  Layers,
  Clock,
  ShieldAlert,
} from 'lucide-react'
import type { DDNSStatus, CloudflareConfig, CFZone } from '../types'
import { api } from '../services/api'

interface CloudflareDdnsViewProps {
  status: DDNSStatus | null
  config: CloudflareConfig | null
  onRefresh: () => void
  onSyncNow: () => Promise<void>
  isSyncing?: boolean
}

export const CloudflareDdnsView: React.FC<CloudflareDdnsViewProps> = ({
  status,
  config,
  onRefresh,
  onSyncNow,
  isSyncing,
}) => {
  const [enabled, setEnabled] = useState(false)
  const [apiToken, setApiToken] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [email, setEmail] = useState('')
  const [authMethod, setAuthMethod] = useState<'token' | 'key'>('token')
  const [zoneId, setZoneId] = useState('')
  const [zoneName, setZoneName] = useState('')
  const [proxied, setProxied] = useState(true)
  const [intervalSec, setIntervalSec] = useState(300)
  const [autoSyncProxies, setAutoSyncProxies] = useState(true)
  const [customDomains, setCustomDomains] = useState('')

  const [availableZones, setAvailableZones] = useState<CFZone[]>([])
  const [isTestingToken, setIsTestingToken] = useState(false)
  const [tokenVerifyMessage, setTokenVerifyMessage] = useState<{ success: boolean; message: string } | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const [saveSuccess, setSaveSuccess] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (config) {
      setEnabled(config.enabled)
      setApiToken(config.api_token || '')
      setApiKey(config.api_key || '')
      setEmail(config.email || '')
      if (config.api_key && !config.api_token) {
        setAuthMethod('key')
      } else {
        setAuthMethod('token')
      }
      setZoneId(config.zone_id || '')
      setZoneName(config.zone_name || '')
      setProxied(config.proxied ?? true)
      setIntervalSec(config.interval_seconds || 300)
      setAutoSyncProxies(config.auto_sync_proxies ?? true)
      setCustomDomains(config.domains ? config.domains.join(', ') : '')
    }
  }, [config])

  const handleTestToken = async () => {
    const tokenToTest = apiToken.trim()
    if (!tokenToTest) {
      setTokenVerifyMessage({
        success: false,
        message: 'Please enter a Cloudflare API token first',
      })
      return
    }

    setIsTestingToken(true)
    setTokenVerifyMessage(null)
    setError('')
    try {
      const res = await api.testCloudflareToken(tokenToTest)
      if (res.valid) {
        setTokenVerifyMessage({
          success: true,
          message: res.warning ? `Token verified! (${res.warning})` : 'Cloudflare API Token verified successfully!',
        })
        if (res.zones && res.zones.length > 0) {
          setAvailableZones(res.zones)
          if (!zoneId) {
            setZoneId(res.zones[0].id)
            setZoneName(res.zones[0].name)
          }
        }
      } else {
        setTokenVerifyMessage({
          success: false,
          message: res.error || 'Token verification failed',
        })
      }
    } catch (err: any) {
      setTokenVerifyMessage({
        success: false,
        message: err.message || 'Token verification failed',
      })
    } finally {
      setIsTestingToken(false)
    }
  }

  const handleSaveConfig = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSaving(true)
    setError('')
    setSaveSuccess(false)

    const domainsList = customDomains
      .split(',')
      .map((d) => d.trim().toLowerCase())
      .filter(Boolean)

    try {
      await api.updateDDNSConfig({
        enabled,
        api_token: authMethod === 'token' ? apiToken.trim() : '',
        api_key: authMethod === 'key' ? apiKey.trim() : '',
        email: authMethod === 'key' ? email.trim() : '',
        zone_id: zoneId.trim(),
        zone_name: zoneName.trim(),
        proxied,
        ttl: 1, // Auto
        interval_seconds: intervalSec,
        auto_sync_proxies: autoSyncProxies,
        domains: domainsList,
      })
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
      onRefresh()
    } catch (err: any) {
      setError(err.message || 'Failed to save Cloudflare configuration')
    } finally {
      setIsSaving(false)
    }
  }

  const recordsList = status?.records ? Object.values(status.records) : []

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Cloud size={28} color="var(--accent-cyan)" />
            Cloudflare Dynamic DNS (DDNS)
          </h1>
          <p className="page-subtitle">
            Combines and replaces docker-cloudflare-ddns with native Go synchronization
          </p>
        </div>
        <button
          id="btn-sync-now"
          onClick={onSyncNow}
          disabled={isSyncing || !status?.enabled}
          className="btn btn-primary"
        >
          <RefreshCw size={16} className={isSyncing ? 'spin' : ''} />
          {isSyncing ? 'Synchronizing...' : 'Sync DNS Now'}
        </button>
      </div>

      {/* Status Bar Card */}
      <div className="card" style={{ marginBottom: '2rem' }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '1.5rem' }}>
          <div>
            <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase' }}>
              Detected Public IP
            </div>
            <div style={{ fontSize: '1.5rem', fontWeight: 700, color: '#fff', fontFamily: 'monospace', marginTop: '0.25rem' }}>
              {status?.current_ip || 'Detecting...'}
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.25rem' }}>
              Auto-resolved via multiple external providers
            </div>
          </div>

          <div>
            <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase' }}>
              Sync Status
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginTop: '0.5rem' }}>
              {status?.enabled ? (
                status?.last_error ? (
                  <span className="badge badge-rose">
                    <AlertTriangle size={12} /> Sync Error
                  </span>
                ) : (
                  <span className="badge badge-emerald">
                    <CheckCircle2 size={12} /> Up-to-Date
                  </span>
                )
              ) : (
                <span className="badge badge-gray">Disabled</span>
              )}
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.5rem' }}>
              Last Checked: {status?.last_check_time ? new Date(status.last_check_time).toLocaleTimeString() : 'Never'}
            </div>
          </div>

          <div>
            <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase' }}>
              Managed Records
            </div>
            <div style={{ fontSize: '1.5rem', fontWeight: 700, color: 'var(--accent-cyan)', marginTop: '0.25rem' }}>
              {recordsList.length} A-Records
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.25rem' }}>
              {autoSyncProxies ? 'Auto-syncing all active proxy domains' : 'Manual domain list only'}
            </div>
          </div>
        </div>

        {status?.last_error && (
          <div style={{
            marginTop: '1.25rem',
            padding: '0.75rem 1rem',
            borderRadius: 'var(--radius-md)',
            background: '#2e141a',
            border: '1px solid #7f1d1d',
            color: '#f87171',
            fontSize: '0.8125rem',
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
          }}>
            <ShieldAlert size={16} />
            <span>{status.last_error}</span>
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1fr)', gap: '2rem', alignItems: 'start' }}>
        {/* Left: Configuration Form */}
        <div className="card">
          <div className="card-header">
            <div className="card-title">
              <Key size={18} color="var(--accent-cyan)" />
              Cloudflare API Configuration
            </div>
            <label className="switch">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              <span className="switch-slider" />
            </label>
          </div>

          {saveSuccess && (
            <div style={{
              background: '#0f291e',
              border: '1px solid #165b3d',
              borderRadius: 'var(--radius-md)',
              padding: '0.75rem 1rem',
              color: '#34d399',
              fontSize: '0.875rem',
              marginBottom: '1.25rem',
            }}>
              Cloudflare DDNS configuration saved successfully!
            </div>
          )}

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

          <form onSubmit={handleSaveConfig}>
            {/* Auth Method Tabs */}
            <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1.25rem' }}>
              <button
                type="button"
                onClick={() => setAuthMethod('token')}
                className={`btn ${authMethod === 'token' ? 'btn-outline-cyan' : 'btn-secondary'}`}
                style={{ fontSize: '0.8125rem', padding: '0.4rem 0.85rem' }}
              >
                API Token (Recommended)
              </button>
              <button
                type="button"
                onClick={() => setAuthMethod('key')}
                className={`btn ${authMethod === 'key' ? 'btn-outline-cyan' : 'btn-secondary'}`}
                style={{ fontSize: '0.8125rem', padding: '0.4rem 0.85rem' }}
              >
                Global API Key
              </button>
            </div>

            {authMethod === 'token' ? (
              <div className="form-group">
                <label className="form-label">Cloudflare API Token</label>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <input
                    type="password"
                    className="input input-mono"
                    placeholder="Enter your Cloudflare Bearer API token"
                    value={apiToken}
                    onChange={(e) => setApiToken(e.target.value)}
                  />
                  <button
                    type="button"
                    onClick={handleTestToken}
                    disabled={isTestingToken || !apiToken}
                    className="btn btn-secondary"
                    style={{ whiteSpace: 'nowrap', fontSize: '0.8125rem' }}
                  >
                    {isTestingToken ? 'Verifying...' : 'Verify Token'}
                  </button>
                </div>
                <div className="form-helper" style={{ marginTop: '0.4rem', lineHeight: '1.4' }}>
                  <span>
                    💡 <strong>Account ID is not required.</strong> Only the API Token is needed. Use Cloudflare's built-in <strong>"Edit zone DNS"</strong> template with <code>Zone:DNS:Edit</code> and <code>Zone:Zone:Read</code> permissions for your target zones.
                  </span>
                </div>
                {tokenVerifyMessage && (
                  <div style={{
                    marginTop: '0.75rem',
                    padding: '0.6rem 0.85rem',
                    borderRadius: 'var(--radius-sm)',
                    background: tokenVerifyMessage.success ? '#0f291e' : '#2e141a',
                    border: `1px solid ${tokenVerifyMessage.success ? '#165b3d' : '#7f1d1d'}`,
                    color: tokenVerifyMessage.success ? '#34d399' : '#f87171',
                    fontSize: '0.8125rem',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '0.25rem',
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', fontWeight: 600 }}>
                      {tokenVerifyMessage.success ? <CheckCircle2 size={14} /> : <AlertTriangle size={14} />}
                      <span>{tokenVerifyMessage.message}</span>
                    </div>
                    {!tokenVerifyMessage.success && (
                      <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.2rem' }}>
                        Ensure you created an <strong>API Token</strong> (not an Account ID or Global API Key) at <em>dash.cloudflare.com/profile/api-tokens</em>, and that Zone Resources includes your domain.
                      </div>
                    )}
                  </div>
                )}
              </div>
            ) : (
              <>
                <div className="form-group">
                  <label className="form-label">Global API Key</label>
                  <input
                    type="password"
                    className="input input-mono"
                    placeholder="Enter your Cloudflare Global API Key"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                  />
                </div>
                <div className="form-group">
                  <label className="form-label">Cloudflare Account Email</label>
                  <input
                    type="email"
                    className="input"
                    placeholder="email@example.com"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                </div>
              </>
            )}

            {/* Zone Selector */}
            <div className="form-group">
              <label className="form-label">Cloudflare Target Zone</label>
              {availableZones.length > 0 ? (
                <select
                  className="select"
                  value={zoneId}
                  onChange={(e) => {
                    setZoneId(e.target.value)
                    const found = availableZones.find((z) => z.id === e.target.value)
                    if (found) setZoneName(found.name)
                  }}
                >
                  {availableZones.map((z) => (
                    <option key={z.id} value={z.id}>
                      {z.name} ({z.id.slice(0, 8)}...)
                    </option>
                  ))}
                </select>
              ) : (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.5rem' }}>
                  <input
                    type="text"
                    className="input input-mono"
                    placeholder="Zone ID (optional, auto-detected)"
                    value={zoneId}
                    onChange={(e) => setZoneId(e.target.value)}
                  />
                  <input
                    type="text"
                    className="input"
                    placeholder="Zone Name (e.g. makinson.me)"
                    value={zoneName}
                    onChange={(e) => setZoneName(e.target.value)}
                  />
                </div>
              )}
              <div className="form-helper">Leave blank to automatically discover the zone based on domains.</div>
            </div>

            {/* Sync Settings */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', marginBottom: '1.25rem' }}>
              <div>
                <label className="form-label">Check Interval</label>
                <select
                  className="select"
                  value={intervalSec}
                  onChange={(e) => setIntervalSec(Number(e.target.value))}
                >
                  <option value={60}>Every 1 Minute</option>
                  <option value={300}>Every 5 Minutes (Default)</option>
                  <option value={900}>Every 15 Minutes</option>
                  <option value={1800}>Every 30 Minutes</option>
                  <option value={3600}>Every 1 Hour</option>
                </select>
              </div>

              <div>
                <label className="form-label">Cloudflare Proxy (Orange Cloud)</label>
                <div style={{ display: 'flex', alignItems: 'center', height: '42px', gap: '0.75rem' }}>
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={proxied}
                      onChange={(e) => setProxied(e.target.checked)}
                    />
                    <span className="switch-slider" />
                  </label>
                  <span style={{ fontSize: '0.8125rem', color: proxied ? 'var(--accent-cyan)' : 'var(--text-muted)' }}>
                    {proxied ? 'Proxied (CDN/WAF)' : 'DNS Only'}
                  </span>
                </div>
              </div>
            </div>

            {/* Auto-Sync Proxies Checkbox */}
            <div style={{
              background: '#0e131d',
              border: '1px solid var(--border-subtle)',
              borderRadius: 'var(--radius-md)',
              padding: '1rem',
              marginBottom: '1.25rem',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div>
                  <div style={{ fontSize: '0.875rem', fontWeight: 600, color: '#fff' }}>
                    Auto-Sync Proxy Targets
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.2rem' }}>
                    Automatically create and update Cloudflare DNS records for all configured reverse proxies
                  </div>
                </div>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={autoSyncProxies}
                    onChange={(e) => setAutoSyncProxies(e.target.checked)}
                  />
                  <span className="switch-slider" />
                </label>
              </div>
            </div>

            {/* Additional Custom Subdomains */}
            <div className="form-group">
              <label className="form-label">Additional Explicit Domains (Comma separated)</label>
              <input
                type="text"
                className="input input-mono"
                placeholder="vpn.makinson.me, home.makinson.me"
                value={customDomains}
                onChange={(e) => setCustomDomains(e.target.value)}
              />
            </div>

            <button
              id="btn-save-ddns"
              type="submit"
              disabled={isSaving}
              className="btn btn-primary"
              style={{ width: '100%' }}
            >
              {isSaving ? 'Saving Configuration...' : 'Save Cloudflare Settings'}
            </button>
          </form>
        </div>

        {/* Right: Managed Records & Sync History */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
          {/* Managed DNS Records */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <Layers size={18} color="var(--accent-cyan)" />
                Managed Cloudflare Records
              </div>
              <span className="badge badge-cyan">{recordsList.length} Active</span>
            </div>

            {recordsList.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '2rem 1rem', color: 'var(--text-muted)' }}>
                <Cloud size={32} style={{ margin: '0 auto 0.75rem', opacity: 0.3 }} />
                <p style={{ fontSize: '0.875rem' }}>No DNS records synced yet. Save credentials and click Sync Now.</p>
              </div>
            ) : (
              <div className="table-container" style={{ border: 'none' }}>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Domain</th>
                      <th>Content IP</th>
                      <th>Proxy</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {recordsList.map((rec) => (
                      <tr key={rec.domain}>
                        <td>
                          <span style={{ fontWeight: 600, color: '#fff', fontFamily: 'monospace', fontSize: '0.8125rem' }}>
                            {rec.domain}
                          </span>
                        </td>
                        <td>
                          <span style={{ fontFamily: 'monospace', fontSize: '0.8125rem', color: '#cbd5e1' }}>
                            {rec.content}
                          </span>
                        </td>
                        <td>
                          {rec.proxied ? (
                            <span className="badge badge-amber" style={{ fontSize: '0.7rem' }}>Proxied</span>
                          ) : (
                            <span className="badge badge-gray" style={{ fontSize: '0.7rem' }}>DNS Only</span>
                          )}
                        </td>
                        <td>
                          {rec.status === 'synced' ? (
                            <span className="badge badge-emerald" style={{ fontSize: '0.7rem' }}>Synced</span>
                          ) : rec.status === 'updated' ? (
                            <span className="badge badge-cyan" style={{ fontSize: '0.7rem' }}>Updated</span>
                          ) : (
                            <span className="badge badge-rose" style={{ fontSize: '0.7rem' }} title={rec.message}>
                              Error
                            </span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>

          {/* Sync History Timeline */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <Clock size={18} color="var(--accent-cyan)" />
                Recent Sync Activity
              </div>
            </div>

            {status?.history && status.history.length > 0 ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', maxHeight: '320px', overflowY: 'auto' }}>
                {status.history.slice(-8).reverse().map((item, idx) => (
                  <div
                    key={idx}
                    style={{
                      padding: '0.75rem',
                      borderRadius: 'var(--radius-md)',
                      background: '#0e131d',
                      border: '1px solid var(--border-subtle)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      gap: '0.5rem',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.65rem' }}>
                      {item.success ? (
                        <CheckCircle2 size={16} color="var(--accent-emerald)" />
                      ) : (
                        <AlertTriangle size={16} color="var(--accent-rose)" />
                      )}
                      <div>
                        <div style={{ fontSize: '0.8125rem', color: '#fff', fontWeight: 500 }}>
                          {item.message}
                        </div>
                        <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', fontFamily: 'monospace' }}>
                          IP: {item.ip}
                        </div>
                      </div>
                    </div>
                    <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                      {new Date(item.timestamp).toLocaleTimeString()}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div style={{ textAlign: 'center', padding: '1.5rem 1rem', color: 'var(--text-muted)', fontSize: '0.8125rem' }}>
                No sync events recorded yet.
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
