import React from 'react'
import type { LucideIcon } from 'lucide-react'

interface MetricCardProps {
  title: string
  value: string | number
  detail?: string
  icon: LucideIcon
  color?: 'cyan' | 'emerald' | 'amber' | 'rose' | 'purple'
}

export const MetricCard: React.FC<MetricCardProps> = ({
  title,
  value,
  detail,
  icon: Icon,
  color = 'cyan',
}) => {
  const colorMap = {
    cyan: {
      bg: 'rgba(56, 189, 248, 0.12)',
      border: 'rgba(56, 189, 248, 0.25)',
      text: 'var(--accent-cyan)',
    },
    emerald: {
      bg: 'rgba(16, 185, 129, 0.12)',
      border: 'rgba(16, 185, 129, 0.25)',
      text: 'var(--accent-emerald)',
    },
    amber: {
      bg: 'rgba(245, 158, 11, 0.12)',
      border: 'rgba(245, 158, 11, 0.25)',
      text: 'var(--accent-amber)',
    },
    rose: {
      bg: 'rgba(244, 63, 94, 0.12)',
      border: 'rgba(244, 63, 94, 0.25)',
      text: 'var(--accent-rose)',
    },
    purple: {
      bg: 'rgba(139, 92, 246, 0.12)',
      border: 'rgba(139, 92, 246, 0.25)',
      text: 'var(--accent-purple)',
    },
  }

  const activeColor = colorMap[color]

  return (
    <div className="metric-card">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem' }}>
        <span className="metric-label">{title}</span>
        <div
          className="metric-icon-wrap"
          style={{
            background: activeColor.bg,
            border: `1px solid ${activeColor.border}`,
            color: activeColor.text,
            marginBottom: 0,
          }}
        >
          <Icon size={20} />
        </div>
      </div>
      <div className="metric-value">{value}</div>
      {detail && <div className="metric-detail">{detail}</div>}
    </div>
  )
}
