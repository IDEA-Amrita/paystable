import { cn, formatTimestamp, formatDuration, formatCurrency } from '../../lib/utils'
import StatusBadge from '../../components/shared/StatusBadge'
import JsonBlock from '../../components/shared/JsonBlock'
import Drawer from '../../components/shared/Drawer'

const EVENT_COLORS = {
  hold_created:       'bg-neon-blue',
  webhook_received:   'bg-neon-yellow',
  poll_completed:     'bg-neon-green',
  state_transition:   'bg-neon-cyan',
  callback_delivered: 'bg-neon-green',
  quarantined:        'bg-neon-red',
}

const STATE_COLORS = {
  PENDING:       'text-neon-yellow',
  VERIFYING:     'text-neon-blue',
  CONFIRMED:     'text-neon-green',
  FAILED:        'text-neon-red',
  INDETERMINATE: 'text-neon-purple',
  REFUNDED:      'text-neon-cyan',
}

export default function TimelineDrawer({ open, onClose, transaction }) {
  if (!transaction) return <Drawer open={open} onClose={onClose}><div /></Drawer>

  const txn = transaction
  const events = txn.events || []
  const polls = txn.polls || []

  return (
    <Drawer open={open} onClose={onClose}>
      {/* Header */}
      <div className="sticky top-0 z-10 bg-bg-surface border-b border-bg-border px-6 py-5">
        <div className="flex items-start justify-between pr-8">
          <div>
            <h2 className="text-lg font-semibold text-text-primary font-mono">{txn.txn_id}</h2>
            <p className="text-sm text-text-secondary mt-1">
              {txn.gateway} · {formatCurrency(txn.amount)} · Created {formatTimestamp(txn.created_at)}
            </p>
          </div>
          <StatusBadge status={txn.status} className="text-sm" />
        </div>
      </div>

      <div className="px-6 py-5 space-y-6">
        {/* Event Timeline */}
        <div>
          <h3 className="text-xs font-medium text-text-secondary uppercase tracking-widest mb-4">
            Event Timeline
          </h3>

          <div className="relative ml-3">
            {/* Vertical rail line */}
            <div className="absolute left-[3px] top-2 bottom-2 w-[2px] bg-bg-border" />

            <div className="space-y-0">
              {events.map((event, i) => {
                const prevEvent = events[i - 1]
                const deltaMs = prevEvent
                  ? new Date(event.timestamp) - new Date(prevEvent.timestamp)
                  : null

                // Determine dot color variant for failed polls
                let dotColor = EVENT_COLORS[event.type] || 'bg-text-muted'
                if (event.type === 'poll_completed' && event.data?.gateway_status !== 'captured') {
                  dotColor = 'bg-neon-red'
                }

                return (
                  <div key={i} className="relative pl-7 pb-5">
                    {/* Dot */}
                    <div className={cn(
                      'absolute left-0 top-1.5 h-2 w-2 rounded-full ring-2 ring-bg-surface',
                      dotColor
                    )} />

                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-mono text-text-muted tabular-nums">
                          {formatTimestamp(event.timestamp)}
                        </span>
                        <span className="text-sm font-medium font-mono text-text-primary">
                          {event.type}
                        </span>
                        {event.attempt && (
                          <span className="text-xs text-text-muted font-mono">[attempt {event.attempt}]</span>
                        )}
                      </div>

                      <p className="text-xs text-text-secondary mt-0.5">
                        {event.source} → {event.detail}
                      </p>

                      {/* State transition special rendering */}
                      {event.type === 'state_transition' && event.data && (
                        <div className="flex items-center gap-2 mt-1">
                          <span className={cn('text-xs font-semibold font-mono', STATE_COLORS[event.data.from])}>
                            {event.data.from}
                          </span>
                          <span className="text-text-muted text-xs">→</span>
                          <span className={cn('text-xs font-semibold font-mono', STATE_COLORS[event.data.to])}>
                            {event.data.to}
                          </span>
                          <span className="text-[10px] font-mono text-text-muted ml-1">resolved</span>
                        </div>
                      )}

                      {deltaMs != null && (
                        <span className="text-xs text-text-muted font-mono mt-0.5 block">
                          Δ +{formatDuration(deltaMs)} from previous
                        </span>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Verification Polls Table */}
        {polls.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-text-secondary uppercase tracking-widest mb-3">
              Gateway Verification Polls
            </h3>

            <div className="border border-bg-border rounded-xl overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="bg-bg-muted border-b border-bg-border">
                    <th className="text-xs text-text-secondary uppercase tracking-widest py-2.5 px-3 text-left font-medium">Attempt</th>
                    <th className="text-xs text-text-secondary uppercase tracking-widest py-2.5 px-3 text-left font-medium">Timestamp</th>
                    <th className="text-xs text-text-secondary uppercase tracking-widest py-2.5 px-3 text-left font-medium">Response</th>
                    <th className="text-xs text-text-secondary uppercase tracking-widest py-2.5 px-3 text-right font-medium">Amount</th>
                    <th className="text-xs text-text-secondary uppercase tracking-widest py-2.5 px-3 text-right font-medium">Δ from prev</th>
                  </tr>
                </thead>
                <tbody>
                  {polls.map((poll, i) => (
                    <tr key={i} className="border-b border-bg-border last:border-0">
                      <td className="px-3 py-2.5 text-sm font-mono text-text-primary">{poll.attempt}</td>
                      <td className="px-3 py-2.5 text-xs font-mono text-text-secondary">
                        {formatTimestamp(poll.timestamp)}
                      </td>
                      <td className="px-3 py-2.5 text-xs font-mono">
                        {poll.gateway_response === 'success' ? (
                          <span className="text-neon-green">✓ success</span>
                        ) : (
                          <span className="text-neon-red">✗ failure</span>
                        )}
                      </td>
                      <td className={cn(
                        'px-3 py-2.5 text-sm font-mono text-right',
                        poll.amount !== txn.amount ? 'text-neon-orange' : 'text-text-primary'
                      )}>
                        {poll.amount !== txn.amount && <span className="mr-1">⚠</span>}
                        {formatCurrency(poll.amount)}
                      </td>
                      <td className="px-3 py-2.5 text-xs font-mono text-text-muted text-right">
                        {poll.delta_from_prev ? `+${formatDuration(poll.delta_from_prev)}` : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {polls.length >= 3 && polls.every(p => p.gateway_response === 'success') && (
              <div className="mt-2 text-xs text-neon-green font-mono flex items-center gap-1.5">
                <span>●</span>
                Stabilized after {polls.length} agreeing polls
              </div>
            )}
          </div>
        )}

        {/* Raw Payloads */}
        <div className="space-y-3">
          <h3 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            Raw Payloads
          </h3>
          {txn.raw_webhook && (
            <JsonBlock title={`Raw Webhook Payload (${txn.gateway})`} data={txn.raw_webhook} />
          )}
          {txn.raw_verification && (
            <JsonBlock title="Raw Gateway Verification Response" data={txn.raw_verification} />
          )}
        </div>
      </div>
    </Drawer>
  )
}
