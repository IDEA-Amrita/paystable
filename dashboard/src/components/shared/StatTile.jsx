import { cn } from '../../lib/utils'
import { AlertTriangle, AlertCircle } from 'lucide-react'

export default function StatTile({ label, value, subtext, status = 'normal', icon: Icon, children }) {
  const statusStyles = {
    normal: 'border-bg-border',
    warning: 'border-neon-yellow/40 bg-neon-yellow/5',
    critical: 'border-neon-red/40 bg-neon-red/5',
  }

  const iconStyles = {
    normal: 'text-neon-blue',
    warning: 'text-neon-yellow',
    critical: 'text-neon-red',
  }

  const indicatorConfig = {
    normal: { text: 'Normal', color: 'text-neon-green', dot: '●' },
    warning: { text: 'Attention', color: 'text-neon-yellow', dot: '⚠' },
    critical: { text: 'Critical', color: 'text-neon-red', dot: '⚠' },
  }

  const indicator = indicatorConfig[status]

  return (
    <div
      className={cn(
        'rounded-xl border p-5 bg-bg-surface shadow-card transition-all duration-150',
        statusStyles[status]
      )}
    >
      <div className="flex items-start justify-between mb-3">
        <span className="text-xs font-medium text-text-secondary uppercase tracking-widest">
          {label}
        </span>
        {Icon && (
          <Icon size={16} className={cn(iconStyles[status])} />
        )}
      </div>

      <div className="mb-2">
        <span className="text-2xl font-semibold text-text-primary font-mono tabular-nums">
          {value}
        </span>
        {subtext && (
          <span className="text-xs text-text-muted ml-2">{subtext}</span>
        )}
      </div>

      {children && (
        <div className="mb-2">{children}</div>
      )}

      <div className={cn('flex items-center gap-1.5 text-xs font-medium', indicator.color)}>
        <span>{indicator.dot}</span>
        <span>{indicator.text}</span>
      </div>
    </div>
  )
}
