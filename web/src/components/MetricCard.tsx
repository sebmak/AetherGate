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
      bg: '#0c283d',
      border: '#0369a1',
      text: '#38bdf8',
    },
    emerald: {
      bg: '#0f291e',
      border: '#165b3d',
      text: '#34d399',
    },
    amber: {
      bg: '#2c200e',
      border: '#78350f',
      text: '#fbbf24',
    },
    rose: {
      bg: '#2e141a',
      border: '#7f1d1d',
      text: '#f87171',
    },
    purple: {
      bg: '#231536',
      border: '#6b21a8',
      text: '#c084fc',
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
