import React, { useState, useEffect } from 'react'
import {
  Settings,
  Shield,
  Download,
  RefreshCw,
  Lock,
  Server,
  FileCode,
} from 'lucide-react'
import type { SystemSettings } from '../types'
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
  const [isImportingLegacy, setIsImportingLegacy] = useState(false)

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
  }, [])

  const handleSaveTLS = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSavingTLS(true)
    setTlsError('')
    setTlsSuccess(false)
    try {
      await api.updateSettings({
        tls: {
          mode: tlsMode,
          email: tlsEmail,
          cert_path: certPath,
          key_path: keyPath,
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

  const handleImportLegacy = async () => {
    setIsImportingLegacy(true)
    setLegacySuccess(null)
    try {
      const res = await api.importLegacy()
      setLegacySuccess({ count: res.imported_count })
      onRefresh()
    } catch (err: any) {
      alert(`Import failed: ${err.message}`)
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
              <div style={{ background: '#111a2e', padding: '0.85rem', borderRadius: '8px', border: '1px solid var(--border-subtle)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>HTTP Reverse Proxy</div>
                <div style={{ fontSize: '1.25rem', fontWeight: 700, color: '#fff', fontFamily: 'monospace' }}>
                  :{settings?.http_port || 9080}
                </div>
              </div>
              <div style={{ background: '#111a2e', padding: '0.85rem', borderRadius: '8px', border: '1px solid var(--border-subtle)' }}>
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
              <div style={{ background: 'rgba(16, 185, 129, 0.15)', border: '1px solid rgba(16, 185, 129, 0.3)', padding: '0.75rem', borderRadius: '6px', color: '#6ee7b7', fontSize: '0.85rem', marginBottom: '1rem' }}>
                TLS settings updated successfully!
              </div>
            )}
            {tlsError && (
              <div style={{ background: 'rgba(244, 63, 94, 0.15)', border: '1px solid rgba(244, 63, 94, 0.3)', padding: '0.75rem', borderRadius: '6px', color: '#fca5a5', fontSize: '0.85rem', marginBottom: '1rem' }}>
                {tlsError}
              </div>
            )}

            <form onSubmit={handleSaveTLS}>
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
                    placeholder="admin@makinson.me"
                  />
                  <div className="form-helper">Used for Let's Encrypt certificate renewal notices and TOS acceptance.</div>
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
              <div style={{ background: 'rgba(16, 185, 129, 0.15)', border: '1px solid rgba(16, 185, 129, 0.3)', padding: '0.75rem', borderRadius: '6px', color: '#6ee7b7', fontSize: '0.85rem', marginBottom: '1rem' }}>
                Password changed successfully!
              </div>
            )}
            {passError && (
              <div style={{ background: 'rgba(244, 63, 94, 0.15)', border: '1px solid rgba(244, 63, 94, 0.3)', padding: '0.75rem', borderRadius: '6px', color: '#fca5a5', fontSize: '0.85rem', marginBottom: '1rem' }}>
                {passError}
              </div>
            )}

            <form onSubmit={handleChangePassword}>
              <div className="form-group">
                <label className="form-label">Current Password</label>
                <input
                  type="password"
                  className="input"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">New Password</label>
                <input
                  type="password"
                  className="input"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">Confirm New Password</label>
                <input
                  type="password"
                  className="input"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                />
              </div>
              <button type="submit" disabled={isChangingPass} className="btn btn-primary">
                {isChangingPass ? 'Updating...' : 'Update Password'}
              </button>
            </form>
          </div>

          {/* Legacy Migration & Backup */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">
                <FileCode size={18} color="var(--accent-cyan)" />
                Configuration & Legacy Import
              </div>
            </div>

            <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginBottom: '1.25rem' }}>
              Import routes directly from your legacy <code>proxy_server_old/servers.json</code> or export the live configuration backup.
            </p>

            {legacySuccess && (
              <div style={{ background: 'rgba(16, 185, 129, 0.15)', border: '1px solid rgba(16, 185, 129, 0.3)', padding: '0.75rem', borderRadius: '6px', color: '#6ee7b7', fontSize: '0.85rem', marginBottom: '1rem' }}>
                Imported {legacySuccess.count} new route(s) from legacy servers.json!
              </div>
            )}

            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <button
                type="button"
                onClick={handleImportLegacy}
                disabled={isImportingLegacy}
                className="btn btn-outline-cyan"
                style={{ justifyContent: 'flex-start' }}
              >
                <RefreshCw size={16} className={isImportingLegacy ? 'spin' : ''} />
                Re-import targets from proxy_server_old/servers.json
              </button>

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
