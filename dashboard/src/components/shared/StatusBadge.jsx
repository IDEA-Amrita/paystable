import { cn, statusLabel, statusMeaning } from '../../lib/utils'

const STATUS_CONFIG = {
  PENDING:       { color: 'text-status-yellow', bg: 'bg-status-yellow/10', border: 'border-status-yellow/30' },
  VERIFYING:     { color: 'text-status-blue',   bg: 'bg-status-blue/10',   border: 'border-status-blue/30' },
  CONFIRMED:     { color: 'text-status-green',  bg: 'bg-status-green/10',  border: 'border-status-green/30' },
  FAILED:        { color: 'text-status-red',    bg: 'bg-status-red/10',    border: 'border-status-red/30' },
  INDETERMINATE: { color: 'text-status-purple', bg: 'bg-status-purple/10', border: 'border-status-purple/30' },
  MISMATCH:      { color: 'text-status-purple', bg: 'bg-status-purple/10', border: 'border-status-purple/30' },
  REFUNDED:      { color: 'text-status-cyan',   bg: 'bg-status-cyan/10',   border: 'border-status-cyan/30' },
}

export default function StatusBadge({ status, className, raw = false }) {
  const config = STATUS_CONFIG[status] || STATUS_CONFIG.PENDING

  return (
    <span
      title={statusMeaning(status)}
      className={cn(
        'inline-flex items-center gap-1.5 border rounded-full px-2.5 py-0.5 text-xs font-medium',
        config.color, config.bg, config.border,
        className
      )}
    >
      <span className="text-[8px]">●</span>
      {raw ? status : statusLabel(status)}
    </span>
  )
}
