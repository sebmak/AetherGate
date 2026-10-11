import React, { useState } from 'react'
import { Lock, User, ArrowRight, ShieldAlert } from 'lucide-react'
import { api } from '../services/api'

interface LoginViewProps {
  isSetup: boolean
  onAuthenticated: (username: string) => void
}

export const LoginView: React.FC<LoginViewProps> = ({ isSetup, onAuthenticated }) => {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
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
        const res = await api.performSetup(username.trim(), password)
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
      background: 'radial-gradient(circle at 50% 20%, rgba(37, 99, 235, 0.15) 0%, transparent 60%), #080c14',
      padding: '1.5rem',
    }}>
      <div style={{
        width: '100%',
        maxWidth: '420px',
        background: 'rgba(15, 23, 42, 0.85)',
        border: '1px solid rgba(56, 189, 248, 0.2)',
        borderRadius: 'var(--radius-xl)',
        backdropFilter: 'blur(20px)',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.7), 0 0 40px rgba(56, 189, 248, 0.12)',
        padding: '2.5rem',
      }}>
        {/* Brand Header */}
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <img
            src="/icon.png"
            alt="ProxyServer Logo"
            style={{
              width: '64px',
              height: '64px',
              borderRadius: '16px',
              marginBottom: '1rem',
              boxShadow: '0 0 32px rgba(56, 189, 248, 0.45)',
              border: '1px solid rgba(56, 189, 248, 0.3)',
              objectFit: 'cover',
            }}
          />
          <h1 style={{ fontSize: '1.5rem', fontWeight: 700, color: '#fff', letterSpacing: '-0.02em', marginBottom: '0.35rem' }}>
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
            background: 'rgba(244, 63, 94, 0.12)',
            border: '1px solid rgba(244, 63, 94, 0.3)',
            borderRadius: 'var(--radius-md)',
            padding: '0.75rem 1rem',
            color: '#fca5a5',
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
