import React, { useState } from 'react'
import {
  Globe,
  Copy,
  Check,
  LogOut,
  RefreshCw,
  User,
  Shield,
} from 'lucide-react'

interface NavbarProps {
  publicIP: string
  ddnsEnabled: boolean
  username: string
  onLogout: () => void
  onRefresh: () => void
  isRefreshing?: boolean
}

export const Navbar: React.FC<NavbarProps> = ({
  publicIP,
  ddnsEnabled,
  username,
  onLogout,
  onRefresh,
  isRefreshing,
}) => {
  const [copied, setCopied] = useState(false)

  const copyIP = () => {
    if (!publicIP) return
    navigator.clipboard.writeText(publicIP)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <header className="top-navbar">
      {/* Left side: External IP pill */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
        {publicIP ? (
          <div
            id="public-ip-pill"
            onClick={copyIP}
            title="Click to copy public IP"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.5rem',
              padding: '0.35rem 0.75rem',
              borderRadius: 'var(--radius-md)',
              background: '#0c283d',
              border: '1px solid #0369a1',
              cursor: 'pointer',
              fontSize: '0.8125rem',
              color: 'var(--accent-cyan)',
              fontFamily: "'JetBrains Mono', monospace",
              transition: 'background-color 0.15s',
            }}
          >
            <Globe size={14} />
            <span>Public IP: {publicIP}</span>
            {copied ? <Check size={13} color="var(--accent-emerald)" /> : <Copy size={13} style={{ opacity: 0.7 }} />}
          </div>
        ) : (
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.8125rem', color: 'var(--text-muted)' }}>
            <Globe size={14} />
            <span>Detecting IP...</span>
          </div>
        )}

        {ddnsEnabled && (
          <span className="badge badge-cyan" title="Cloudflare DDNS background sync active">
            <span className="status-dot status-dot-healthy" style={{ width: '6px', height: '6px' }} />
            Cloudflare DDNS Active
          </span>
        )}
      </div>

      {/* Right side: Actions & User */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
        <button
          id="btn-refresh-global"
          onClick={onRefresh}
          className="btn btn-secondary btn-icon"
          title="Refresh system status"
        >
          <RefreshCw size={16} className={isRefreshing ? 'spin' : ''} />
        </button>

        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.65rem',
          padding: '0.35rem 0.75rem',
          borderRadius: 'var(--radius-md)',
          background: '#161e2b',
          border: '1px solid var(--border-subtle)',
          fontSize: '0.8125rem',
          color: 'var(--text-primary)',
        }}>
          <div style={{
            width: '22px',
            height: '22px',
            borderRadius: '50%',
            background: 'var(--accent-blue)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}>
            <User size={13} color="#fff" />
          </div>
          <span style={{ fontWeight: 600 }}>{username || 'Admin'}</span>
          <span title="Admin verified" style={{ display: 'inline-flex' }}>
            <Shield size={12} color="var(--accent-emerald)" />
          </span>
        </div>

        <button
          id="btn-logout"
          onClick={onLogout}
          className="btn btn-secondary btn-icon"
          title="Log out of admin session"
          style={{ color: 'var(--accent-rose)' }}
        >
          <LogOut size={16} />
        </button>
      </div>
    </header>
  )
}
