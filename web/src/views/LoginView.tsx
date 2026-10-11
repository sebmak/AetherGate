import React, { useState } from 'react'
import { Lock, User, ArrowRight, ShieldAlert, FileCode, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '../services/api'

interface LoginViewProps {
  isSetup: boolean
  onAuthenticated: (username: string) => void
}

export const LoginView: React.FC<LoginViewProps> = ({ isSetup, onAuthenticated }) => {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [legacyConfig, setLegacyConfig] = useState('')
  const [showLegacyImport, setShowLegacyImport] = useState(false)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!username.trim() || !password) {
      setError('Please provide username and password')
      return
    }

    if (isSetup) {
      if (password !== confirmPassword) {
        setError('Passwords do not match')
        return
      }
      if (password.length < 6) {
        setError('Password must be at least 6 characters')
        return
      }

      setIsLoading(true)
      try {
        const res = await api.performSetup(username.trim(), password, legacyConfig)
        onAuthenticated(res.username)
      } catch (err: any) {
        setError(err.message || 'Setup failed')
      } finally {
        setIsLoading(false)
      }
    } else {
      setIsLoading(true)
      try {
        const res = await api.login(username.trim(), password)
        onAuthenticated(res.username)
      } catch (err: any) {
        setError(err.message || 'Login failed')
      } finally {
        setIsLoading(false)
      }
    }
  }

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      backgroundColor: 'var(--bg-primary)',
      padding: '1.5rem',
    }}>
      <div style={{
        width: '100%',
        maxWidth: '420px',
        background: 'var(--bg-card)',
        border: '1px solid var(--border-card)',
        borderRadius: 'var(--radius-xl)',
        boxShadow: 'var(--shadow-md)',
        padding: '2.25rem',
      }}>
        {/* Brand Header */}
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <img
            src="/icon.png"
            alt="ProxyServer Logo"
            style={{
              width: '60px',
              height: '60px',
              borderRadius: '12px',
              marginBottom: '1rem',
              border: '1px solid var(--border-subtle)',
              objectFit: 'cover',
            }}
          />
          <h1 style={{ fontSize: '1.45rem', fontWeight: 700, color: '#fff', letterSpacing: '-0.02em', marginBottom: '0.35rem' }}>
            {isSetup ? 'Initial Security Setup' : 'ProxyServer Gateway'}
          </h1>
          <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
            {isSetup
              ? 'Create your administrator account to secure the proxy'
              : 'Sign in to access reverse proxies & Cloudflare DDNS'}
          </p>
        </div>

        {error && (
          <div style={{
            background: '#2e141a',
            border: '1px solid #7f1d1d',
            borderRadius: 'var(--radius-md)',
            padding: '0.75rem 1rem',
            color: '#f87171',
            fontSize: '0.8125rem',
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            marginBottom: '1.25rem',
          }}>
            <ShieldAlert size={16} />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label className="form-label">Administrator Username</label>
            <div style={{ position: 'relative' }}>
              <User size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
              <input
                id="login-username"
                type="text"
                className="input"
                style={{ paddingLeft: '2.5rem' }}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                autoFocus
                required
              />
            </div>
          </div>

          <div className="form-group">
            <label className="form-label">Password</label>
            <div style={{ position: 'relative' }}>
              <Lock size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
              <input
                id="login-password"
                type="password"
                className="input"
                style={{ paddingLeft: '2.5rem' }}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
              />
            </div>
          </div>

          {isSetup && (
            <>
              <div className="form-group">
                <label className="form-label">Confirm Password</label>
                <div style={{ position: 'relative' }}>
                  <Lock size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
                  <input
                    id="login-confirm-password"
                    type="password"
                    className="input"
                    style={{ paddingLeft: '2.5rem' }}
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    placeholder="••••••••"
                    required
                  />
                </div>
              </div>

              <div style={{ marginTop: '0.25rem', marginBottom: '1rem' }}>
                <button
                  id="btn-toggle-legacy-import"
                  type="button"
                  onClick={() => setShowLegacyImport(!showLegacyImport)}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: 'var(--accent-cyan)',
                    fontSize: '0.8125rem',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '0.4rem',
                    padding: 0,
                    fontWeight: 500,
                  }}
                >
                  <FileCode size={14} />
                  <span>{showLegacyImport ? 'Hide Existing Config Import' : 'Import Existing servers.json (Optional)'}</span>
                  {showLegacyImport ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                </button>

                {showLegacyImport && (
                  <div style={{
                    marginTop: '0.65rem',
                    padding: '0.85rem',
                    background: 'var(--bg-input)',
                    borderRadius: 'var(--radius-md)',
                    border: '1px solid var(--border-subtle)',
                  }}>
                    <label className="form-label" style={{ fontSize: '0.75rem', display: 'flex', justifyContent: 'space-between', marginBottom: '0.35rem' }}>
                      <span>Paste Legacy servers.json</span>
                      <span style={{ color: 'var(--text-muted)' }}>JSON payload</span>
                    </label>
                    <textarea
                      id="login-legacy-config"
                      className="input input-mono"
                      style={{
                        width: '100%',
                        minHeight: '120px',
                        fontSize: '0.75rem',
                        lineHeight: '1.4',
                        resize: 'vertical',
                      }}
                      value={legacyConfig}
                      onChange={(e) => setLegacyConfig(e.target.value)}
                      placeholder={`{\n  "targets": {\n    "app.home.example.com": {\n      "host": "192.168.1.100:8080",\n      "scheme": "http",\n      "force_ssl": true\n    }\n  }\n}`}
                    />
                    <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)', marginTop: '0.4rem' }}>
                      Paste your existing config here to automatically migrate your reverse proxy routes during setup.
                    </div>
                  </div>
                )}
              </div>
            </>
          )}

          <button
            id="btn-login-submit"
            type="submit"
            disabled={isLoading}
            className="btn btn-primary"
            style={{ width: '100%', padding: '0.75rem', marginTop: '0.75rem' }}
          >
            {isLoading ? (
              'Processing...'
            ) : isSetup ? (
              <>
                Initialize Admin Account <ArrowRight size={16} />
              </>
            ) : (
              <>
                Sign In to Gateway <ArrowRight size={16} />
              </>
            )}
          </button>
        </form>

        <div style={{ textAlign: 'center', marginTop: '1.75rem', fontSize: '0.75rem', color: 'var(--text-muted)' }}>
          Self-hosted • Go 1.27 • Automated Cloudflare DDNS
        </div>
      </div>
    </div>
  )
}
