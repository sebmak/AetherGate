import React, { useState, useEffect } from 'react'
import {
  Settings,
  Shield,
  ShieldCheck,
  Download,
  Lock,
  Server,
  FileCode,
  Upload,
  ChevronDown,
  ChevronRight,
  RefreshCw,
  AlertTriangle,
  AlertCircle,
} from 'lucide-react'
import type { SystemSettings, CertificateInfo } from '../types'
import { api } from '../services/api'

interface SettingsViewProps {
  onRefresh: () => void
}

export const SettingsView: React.FC<SettingsViewProps> = ({ onRefresh }) => {
  const [settings, setSettings] = useState<SystemSettings | null>(null)
  const [tlsMode, setTlsMode] = useState<'autocert' | 'custom' | 'none'>('autocert')
  const [tlsEmail, setTlsEmail] = useState('')
  const [certPath, setCertPath] = useState('')
  const [keyPath, setKeyPath] = useState('')

  // Certificate status & renewal state
  const [certs, setCerts] = useState<CertificateInfo[]>([])
  const [loadingCerts, setLoadingCerts] = useState(false)
  const [renewingDomain, setRenewingDomain] = useState<string | null>(null)
  const [isRenewingAll, setIsRenewingAll] = useState(false)
  const [certActionMsg, setCertActionMsg] = useState<{ success: boolean; message: string } | null>(null)

  // Password change state
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passError, setPassError] = useState('')
  const [passSuccess, setPassSuccess] = useState(false)
  const [isChangingPass, setIsChangingPass] = useState(false)

  // Status banners
  const [tlsSuccess, setTlsSuccess] = useState(false)
  const [tlsError, setTlsError] = useState('')
  const [isSavingTLS, setIsSavingTLS] = useState(false)
  const [legacySuccess, setLegacySuccess] = useState<{ count: number } | null>(null)
  const [legacyError, setLegacyError] = useState('')
  const [legacyPayload, setLegacyPayload] = useState('')
  const [showLegacyPaste, setShowLegacyPaste] = useState(false)
  const [isImportingLegacy, setIsImportingLegacy] = useState(false)

  const loadCerts = async () => {
    setLoadingCerts(true)
    try {
      const res = await api.getTLSCerts()
      setCerts(res.certs || [])
    } catch {
      // ignore
    } finally {
      setLoadingCerts(false)
    }
  }

  useEffect(() => {
    api.getSettings().then((s) => {
      setSettings(s)
      if (s.tls) {
        setTlsMode(s.tls.mode || 'autocert')
        setTlsEmail(s.tls.email || '')
        setCertPath(s.tls.cert_path || '')
        setKeyPath(s.tls.key_path || '')
      }
    }).catch(() => {})
    loadCerts()
  }, [])

  const handleRenewSingle = async (domain: string) => {
    setRenewingDomain(domain)
    setCertActionMsg(null)
    try {
      const proxies = await api.getProxies()
      const target = proxies.find((p) => p.domain.toLowerCase() === domain.toLowerCase())
      if (target) {
        const res = await api.renewCertificate(target.id)
        if (res.success) {
          setCertActionMsg({
            success: true,
            message: `Certificate for ${domain} renewed successfully (${res.cert?.days_remaining ?? 0} days left).`,
          })
        } else {
          setCertActionMsg({
            success: false,
            message: res.error || `Failed to renew certificate for ${domain}`,
          })
        }
      } else {
        setCertActionMsg({ success: false, message: `No active proxy route matching ${domain}` })
      }
      await loadCerts()
      onRefresh()
    } catch (err: any) {
      setCertActionMsg({ success: false, message: err.message || `Renewal failed for ${domain}` })
      await loadCerts()
    } finally {
      setRenewingDomain(null)
      setTimeout(() => setCertActionMsg(null), 8000)
    }
  }

  const handleRenewAll = async () => {
    setIsRenewingAll(true)
    setCertActionMsg(null)
    try {
      const res = await api.renewAllCertificates()
      if (res.success) {
        setCertActionMsg({
          success: true,
          message: 'All registered domain certificates were regenerated and renewed successfully!',
        })
      } else {
        setCertActionMsg({
          success: false,
          message: res.error || 'Some domain certificates encountered errors during renewal.',
        })
      }
      await loadCerts()
      onRefresh()
    } catch (err: any) {
      setCertActionMsg({ success: false, message: err.message || 'Bulk certificate renewal request failed.' })
      await loadCerts()
    } finally {
      setIsRenewingAll(false)
      setTimeout(() => setCertActionMsg(null), 8000)
    }
  }

  const handleSaveTLS = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSavingTLS(true)
    setTlsError('')
    setTlsSuccess(false)

    const trimmedEmail = tlsEmail.trim()
    if (trimmedEmail && /@(example\.(com|org|net)|test\.(com|org)|localhost|invalid)/i.test(trimmedEmail)) {
      setTlsError('Please enter a real email or leave blank. Domains like example.com are rejected by Let\'s Encrypt.')
      setIsSavingTLS(false)
      return
    }

    try {
      await api.updateSettings({
        tls: {
          mode: tlsMode,
          email: trimmedEmail,
          cert_path: certPath.trim(),
          key_path: keyPath.trim(),
        },
      })
      setTlsSuccess(true)
      setTimeout(() => setTlsSuccess(false), 3000)
    } catch (err: any) {
      setTlsError(err.message || 'Failed to update TLS settings')
    } finally {
      setIsSavingTLS(false)
    }
  }

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    setPassError('')
    setPassSuccess(false)

    if (newPassword !== confirmPassword) {
      setPassError('New passwords do not match')
      return
    }
    if (newPassword.length < 6) {
      setPassError('New password must be at least 6 characters')
      return
    }

    setIsChangingPass(true)
    try {
      await api.changePassword(currentPassword, newPassword)
      setPassSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setTimeout(() => setPassSuccess(false), 3000)
    } catch (err: any) {
      setPassError(err.message || 'Failed to update password')
    } finally {
      setIsChangingPass(false)
    }
  }

  const handleImportLegacy = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!legacyPayload.trim()) {
      setLegacyError('Please paste a legacy JSON payload')
      return
    }
    setIsImportingLegacy(true)
    setLegacyError('')
    setLegacySuccess(null)
    try {
      const res = await api.importLegacy(legacyPayload.trim())
      setLegacySuccess({ count: res.imported_count })
      setLegacyPayload('')
      setShowLegacyPaste(false)
      onRefresh()
      setTimeout(() => setLegacySuccess(null), 5000)
    } catch (err: any) {
      setLegacyError(err.message || 'Import failed')
    } finally {
      setIsImportingLegacy(false)
    }
  }

  return (
    <div>
      {/* Header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            <Settings size={28} color="var(--accent-cyan)" />
            System & Security Settings
          </h1>
          <p className="page-subtitle">Configure ports, TLS certificates, administrator security, and migrations</p>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1fr)', gap: '2rem', alignItems: 'start' }}>
        {/* Left Column: System & TLS */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
          {/* Server Info */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <Server size={18} color="var(--accent-cyan)" />
                Runtime Architecture & Ports
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '1rem', marginBottom: '1.25rem' }}>
              <div style={{ background: '#111a2e', padding: '0.85rem', borderRadius: '8px', border: '1px solid var(--border-subtle)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Admin Web UI & API</div>
                <div style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-cyan)', fontFamily: 'monospace' }}>
                  :{settings?.admin_port || 8080}
                </div>
              </div>
              <div style={{ background: '#0e131d', padding: '0.85rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-subtle)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>HTTP Reverse Proxy</div>
                <div style={{ fontSize: '1.25rem', fontWeight: 700, color: '#fff', fontFamily: 'monospace' }}>
                  :{settings?.http_port || 9080}
                </div>
              </div>
              <div style={{ background: '#0e131d', padding: '0.85rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-subtle)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>HTTPS Reverse Proxy</div>
                <div style={{ fontSize: '1.25rem', fontWeight: 700, color: '#34d399', fontFamily: 'monospace' }}>
                  :{settings?.https_port || 9443}
                </div>
              </div>
            </div>

            <div style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              Data Directory: <code style={{ color: 'var(--accent-cyan)' }}>{settings?.data_dir || './data'}</code>
            </div>
          </div>

          {/* TLS / ACME Config */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <Shield size={18} color="var(--accent-cyan)" />
                TLS & Automated Certificates (ACME)
              </div>
            </div>

            {tlsSuccess && (
              <div style={{ background: '#0f291e', border: '1px solid #165b3d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#34d399', fontSize: '0.85rem', marginBottom: '1rem' }}>
                TLS settings updated successfully!
              </div>
            )}
            {tlsError && (
              <div style={{ background: '#2e141a', border: '1px solid #7f1d1d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#f87171', fontSize: '0.85rem', marginBottom: '1rem' }}>
                {tlsError}
              </div>
            )}

            <form
              onSubmit={handleSaveTLS}
              data-1p-ignore="true"
              data-lpignore="true"
              data-bwignore="true"
            >
              <div className="form-group">
                <label className="form-label">Certificate Mode</label>
                <select
                  className="select"
                  value={tlsMode}
                  onChange={(e) => setTlsMode(e.target.value as any)}
                >
                  <option value="autocert">Let's Encrypt ACME Autocert (Automatic HTTPS)</option>
                  <option value="custom">Custom Certificate & Key Files</option>
                  <option value="none">Self-Signed / External Edge SSL (e.g. Cloudflare Origin)</option>
                </select>
              </div>

              {tlsMode === 'autocert' && (
                <div className="form-group">
                  <label className="form-label">Let's Encrypt Registration Email</label>
                  <input
                    type="email"
                    className="input"
                    value={tlsEmail}
                    onChange={(e) => setTlsEmail(e.target.value)}
                    placeholder="yourname@gmail.com (or leave blank)"
                  />
                  <div className="form-helper">
                    Optional. Used for Let's Encrypt certificate renewal notices. If left blank, certificates are issued anonymously. Do not use placeholder domains like <em>example.com</em>.
                  </div>
                </div>
              )}

              {tlsMode === 'custom' && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', marginBottom: '1rem' }}>
                  <div>
                    <label className="form-label">Certificate File Path (.crt / .pem)</label>
                    <input
                      type="text"
                      className="input input-mono"
                      value={certPath}
                      onChange={(e) => setCertPath(e.target.value)}
                      placeholder="/data/certs/cert.pem"
                    />
                  </div>
                  <div>
                    <label className="form-label">Private Key File Path (.key)</label>
                    <input
                      type="text"
                      className="input input-mono"
                      value={keyPath}
                      onChange={(e) => setKeyPath(e.target.value)}
                      placeholder="/data/certs/key.pem"
                    />
                  </div>
                </div>
              )}

              <button type="submit" disabled={isSavingTLS} className="btn btn-primary">
                {isSavingTLS ? 'Saving TLS...' : 'Save TLS Configuration'}
              </button>
            </form>
          </div>

          {/* Active Domain Certificates & Renewal Management */}
          <div className="card">
            <div className="card-header" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '0.75rem' }}>
              <div className="card-title">
                <ShieldCheck size={18} color="var(--accent-cyan)" />
                Managed Domain Certificates
              </div>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <button
                  type="button"
                  onClick={loadCerts}
                  disabled={loadingCerts}
                  className="btn btn-secondary"
                  style={{ fontSize: '0.75rem', padding: '0.35rem 0.65rem' }}
                  title="Reload certificates from disk"
                >
                  <RefreshCw size={12} className={loadingCerts ? 'spin' : ''} />
                  Refresh
                </button>
                <button
                  type="button"
                  onClick={handleRenewAll}
                  disabled={isRenewingAll || certs.length === 0}
                  className="btn btn-primary"
                  style={{ fontSize: '0.75rem', padding: '0.35rem 0.75rem' }}
                  title="Regenerate all certificates with Let's Encrypt"
                >
                  <RefreshCw size={12} className={isRenewingAll ? 'spin' : ''} />
                  {isRenewingAll ? 'Regenerating All...' : 'Regenerate All Certificates'}
                </button>
              </div>
            </div>

            {certActionMsg && (
              <div style={{
                background: certActionMsg.success ? '#0f291e' : '#2e141a',
                border: `1px solid ${certActionMsg.success ? '#165b3d' : '#7f1d1d'}`,
                padding: '0.75rem 1rem',
                borderRadius: 'var(--radius-md)',
                color: certActionMsg.success ? '#34d399' : '#f87171',
                fontSize: '0.85rem',
                marginBottom: '1rem',
              }}>
                {certActionMsg.message}
              </div>
            )}

            {certs.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '2rem 1rem', color: 'var(--text-muted)', fontSize: '0.875rem' }}>
                No active proxy routes registered. Configure reverse proxies to manage SSL/TLS certificates.
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                {certs.map((c) => {
                  const isRenewing = renewingDomain === c.domain
                  return (
                    <div
                      key={c.domain}
                      style={{
                        background: '#0d1320',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: 'var(--radius-md)',
                        padding: '0.85rem 1rem',
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.5rem', flexWrap: 'wrap', gap: '0.5rem' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                          <span style={{ fontWeight: 600, fontFamily: 'monospace', color: '#fff', fontSize: '0.875rem' }}>
                            {c.domain}
                          </span>
                          {c.status === 'valid' && (
                            <span className="badge badge-emerald"><ShieldCheck size={11} /> Valid • {c.days_remaining}d left</span>
                          )}
                          {c.status === 'expiring_soon' && (
                            <span className="badge badge-amber"><AlertTriangle size={11} /> Expiring • {c.days_remaining}d left</span>
                          )}
                          {c.status === 'expired' && (
                            <span className="badge badge-rose"><AlertCircle size={11} /> Expired</span>
                          )}
                          {c.status === 'self_signed' && (
                            <span className="badge badge-cyan"><Shield size={11} /> Dev Self-Signed</span>
                          )}
                          {c.status === 'error' && (
                            <span className="badge badge-rose"><AlertCircle size={11} /> Cert Error</span>
                          )}
                          {c.status === 'renewing' && (
                            <span className="badge badge-cyan"><RefreshCw size={11} className="spin" /> Renewing...</span>
                          )}
                        </div>

                        <button
                          type="button"
                          onClick={() => handleRenewSingle(c.domain)}
                          disabled={isRenewing || isRenewingAll}
                          className="btn btn-secondary"
                          style={{ fontSize: '0.75rem', padding: '0.3rem 0.6rem' }}
                          title={`Force Let's Encrypt regeneration for ${c.domain}`}
                        >
                          <RefreshCw size={12} className={isRenewing ? 'spin' : ''} />
                          {isRenewing ? 'Regenerating...' : 'Regenerate'}
                        </button>
                      </div>

                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: '0.5rem', fontSize: '0.75rem' }}>
                        <div>
                          <span style={{ color: 'var(--text-muted)' }}>Issuer: </span>
                          <span style={{ color: '#fff' }}>{c.issuer || 'Let\'s Encrypt'}</span>
                        </div>
                        <div>
                          <span style={{ color: 'var(--text-muted)' }}>Days Left: </span>
                          <span style={{ color: c.days_remaining > 15 ? '#34d399' : '#fbbf24', fontWeight: 600 }}>
                            {c.days_remaining} days
                          </span>
                        </div>
                        <div>
                          <span style={{ color: 'var(--text-muted)' }}>Renewed: </span>
                          <span style={{ color: 'var(--text-secondary)' }}>
                            {c.issued_at ? new Date(c.issued_at).toLocaleDateString() : 'N/A'}
                          </span>
                        </div>
                        <div>
                          <span style={{ color: 'var(--text-muted)' }}>Expires: </span>
                          <span style={{ color: 'var(--text-secondary)' }}>
                            {c.expires_at ? new Date(c.expires_at).toLocaleDateString() : 'N/A'}
                          </span>
                        </div>
                      </div>

                      {c.last_error && (
                        <div style={{ marginTop: '0.5rem', background: '#2e141a', border: '1px solid #7f1d1d', padding: '0.45rem 0.65rem', borderRadius: 'var(--radius-sm)', color: '#f87171', fontSize: '0.75rem' }}>
                          <strong>Last Renewal Error:</strong> {c.last_error}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Security & Migrations */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
          {/* Change Admin Password */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <Lock size={18} color="var(--accent-cyan)" />
                Administrator Security
              </div>
            </div>

            {passSuccess && (
              <div style={{ background: '#0f291e', border: '1px solid #165b3d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#34d399', fontSize: '0.85rem', marginBottom: '1rem' }}>
                Password changed successfully!
              </div>
            )}
            {passError && (
              <div style={{ background: '#2e141a', border: '1px solid #7f1d1d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#f87171', fontSize: '0.85rem', marginBottom: '1rem' }}>
                {passError}
              </div>
            )}

            <form
              onSubmit={handleChangePassword}
              data-1p-ignore="true"
              data-lpignore="true"
              data-bwignore="true"
              autoComplete="off"
            >
              <div className="form-group">
                <label className="form-label">Current Password</label>
                <input
                  id="settings-cur-password"
                  name="cur_pwd_ignore"
                  type="password"
                  className="input"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  autoComplete="off"
                  data-1p-ignore="true"
                  data-lpignore="true"
                  data-bwignore="true"
                  data-form-type="other"
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">New Password</label>
                <input
                  id="settings-new-password"
                  name="new_pwd_ignore"
                  type="password"
                  className="input"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  autoComplete="new-password"
                  data-1p-ignore="true"
                  data-lpignore="true"
                  data-bwignore="true"
                  data-form-type="other"
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">Confirm New Password</label>
                <input
                  id="settings-conf-password"
                  name="conf_pwd_ignore"
                  type="password"
                  className="input"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  data-1p-ignore="true"
                  data-lpignore="true"
                  data-bwignore="true"
                  data-form-type="other"
                  required
                />
              </div>
              <button type="submit" disabled={isChangingPass} className="btn btn-primary">
                {isChangingPass ? 'Updating...' : 'Update Password'}
              </button>
            </form>
          </div>

          {/* Configuration & Legacy Import */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <FileCode size={18} color="var(--accent-cyan)" />
                Configuration & Legacy Import
              </div>
            </div>

            <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginBottom: '1.25rem' }}>
              Import routes directly by pasting your legacy <code>servers.json</code> payload, or export a complete configuration backup.
            </p>

            {legacySuccess && (
              <div style={{ background: '#0f291e', border: '1px solid #165b3d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#34d399', fontSize: '0.85rem', marginBottom: '1rem' }}>
                Successfully imported {legacySuccess.count} route(s) into ProxyServer!
              </div>
            )}
            {legacyError && (
              <div style={{ background: '#2e141a', border: '1px solid #7f1d1d', padding: '0.75rem', borderRadius: 'var(--radius-md)', color: '#f87171', fontSize: '0.85rem', marginBottom: '1rem' }}>
                {legacyError}
              </div>
            )}

            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <button
                type="button"
                onClick={() => {
                  setShowLegacyPaste(!showLegacyPaste)
                  setLegacyError('')
                }}
                className="btn btn-outline-cyan"
                style={{ justifyContent: 'space-between' }}
              >
                <span style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Upload size={16} /> Paste & Import Legacy servers.json
                </span>
                {showLegacyPaste ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
              </button>

              {showLegacyPaste && (
                <form
                  onSubmit={handleImportLegacy}
                  data-1p-ignore="true"
                  data-lpignore="true"
                  data-bwignore="true"
                  style={{ background: '#0e131d', padding: '1rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-subtle)', marginTop: '0.25rem' }}
                >
                  <label className="form-label" style={{ fontSize: '0.75rem' }}>Legacy servers.json JSON payload</label>
                  <textarea
                    id="settings-legacy-payload"
                    className="input input-mono"
                    style={{ width: '100%', minHeight: '120px', fontSize: '0.75rem', resize: 'vertical', marginBottom: '0.75rem' }}
                    value={legacyPayload}
                    onChange={(e) => setLegacyPayload(e.target.value)}
                    placeholder={`{\n  "targets": {\n    "service.home.makinson.me": {\n      "host": "192.168.1.104:9990",\n      "scheme": "http",\n      "force_ssl": true\n    }\n  }\n}`}
                    required
                  />
                  <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
                    <button
                      type="button"
                      onClick={() => {
                        setShowLegacyPaste(false)
                        setLegacyError('')
                      }}
                      className="btn btn-secondary"
                      style={{ padding: '0.4rem 0.75rem', fontSize: '0.8125rem' }}
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={isImportingLegacy}
                      className="btn btn-primary"
                      style={{ padding: '0.4rem 0.75rem', fontSize: '0.8125rem' }}
                    >
                      {isImportingLegacy ? 'Importing...' : 'Import Routes'}
                    </button>
                  </div>
                </form>
              )}

              <a
                href="/api/settings/export"
                download="proxyserver-config.json"
                className="btn btn-secondary"
                style={{ justifyContent: 'flex-start' }}
              >
                <Download size={16} /> Export Live Configuration JSON
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
