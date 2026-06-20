import { cn } from '../../lib/utils'

const STATUS_CONFIG = {
  PENDING:       { color: 'text-neon-yellow', bg: 'bg-neon-yellow/10', border: 'border-neon-yellow/30', dot: '●' },
  VERIFYING:     { color: 'text-neon-blue',   bg: 'bg-neon-blue/10',   border: 'border-neon-blue/30',   dot: '◎' },
  CONFIRMED:     { color: 'text-neon-green',  bg: 'bg-neon-green/10',  border: 'border-neon-green/30',  dot: '●' },
  FAILED:        { color: 'text-neon-red',    bg: 'bg-neon-red/10',    border: 'border-neon-red/30',    dot: '●' },
  INDETERMINATE: { color: 'text-neon-purple', bg: 'bg-neon-purple/10', border: 'border-neon-purple/30', dot: '◌' },
  REFUNDED:      { color: 'text-neon-cyan',   bg: 'bg-neon-cyan/10',   border: 'border-neon-cyan/30',   dot: '↩' },
}

export default function StatusBadge({ status, className }) {
  const config = STATUS_CONFIG[status] || STATUS_CONFIG.PENDING

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 border rounded-full px-2.5 py-0.5 text-xs font-medium font-mono',
        config.color,
        config.bg,
        config.border,
        className
      )}
    >
      <span className="text-[10px]">{config.dot}</span>
      {status}
    </span>
  )
}
