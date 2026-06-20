import { cn, formatTimestamp, formatDuration, formatCurrency } from '../../lib/utils'
import StatusBadge from '../../components/shared/StatusBadge'
import JsonBlock from '../../components/shared/JsonBlock'
import Drawer from '../../components/shared/Drawer'

const EVENT_COLORS = {
  hold_created:       'bg-status-blue',
  webhook_received:   'bg-status-yellow',
  poll_completed:     'bg-status-green',
  state_transition:   'bg-status-cyan',
  callback_delivered: 'bg-status-green',
  quarantined:        'bg-status-red',
}

const STATE_COLORS = {
  PENDING:       'text-status-yellow',
  VERIFYING:     'text-status-blue',
  CONFIRMED:     'text-status-green',
  FAILED:        'text-status-red',
  INDETERMINATE: 'text-status-purple',
  REFUNDED:      'text-status-cyan',
}

function buildVerdict(txn) {
  const events = txn.events || []
  const webhookEvent = events.find(e => e.type === 'webhook_received')
  const webhookStatus = webhookEvent?.data?.status || webhookEvent?.data?.event_type || null
  const finalStatus = txn.status
  const duration = txn.resolve_duration_ms

  if (finalStatus === 'CONFIRMED') {
    if (webhookStatus && webhookStatus.includes('fail')) {
      return {
        text: `The gateway sent a FAILURE webhook, but paystable verified the payment succeeded.${duration ? ` Confirmed in ${formatDuration(duration)}.` : ''} The merchant was notified.`,
        color: 'text-status-green',
      }
    }
    return {
      text: `Payment confirmed.${duration ? ` Resolved in ${formatDuration(duration)}.` : ''} The merchant was notified.`,
      color: 'text-status-green',
    }
  }
  if (finalStatus === 'FAILED') {
    return {
      text: `Payment failed. Verification confirmed failure across multiple gateway polls.${duration ? ` Resolved in ${formatDuration(duration)}.` : ''}`,
      color: 'text-status-red',
    }
  }
  if (finalStatus === 'INDETERMINATE') {
    return {
      text: 'Amount mismatch or inconclusive verification. Requires manual investigation.',
      color: 'text-status-yellow',
    }
  }
  if (finalStatus === 'VERIFYING') {
    return { text: 'Still verifying with the gateway. Do not act on this yet.', color: 'text-status-blue' }
  }
  return null
}

export default function TimelineDrawer({ open, onClose, transaction }) {
  if (!transaction) return <Drawer open={open} onClose={onClose}><div /></Drawer>

  const txn = transaction
  const events = txn.events || []
  const polls = txn.polls || []
  const verdict = buildVerdict(txn)

  return (
    <Drawer open={open} onClose={onClose}>
      {/* Header */}
      <div className="sticky top-0 z-10 bg-bg-surface border-b border-bg-border px-6 py-4">
        <div className="flex items-start justify-between pr-8">
          <div>
            <h2 className="text-base font-mono text-text-primary">{txn.txn_id}</h2>
            <p className="text-xs text-text-muted mt-0.5">
              {txn.gateway} · {formatCurrency(txn.amount)} · {formatTimestamp(txn.created_at)}
            </p>
          </div>
          <StatusBadge status={txn.status} raw />
        </div>
      </div>

      <div className="px-6 py-5 space-y-6">

        {/* Verdict — the conclusion first */}
        {verdict && (
          <div className="bg-bg-elevated rounded-xl px-4 py-3">
            <p className={cn('text-sm font-medium', verdict.color)}>{verdict.text}</p>
          </div>
        )}

        {/* Event Timeline */}
        <div>
          <h3 className="text-xs text-text-muted uppercase tracking-widest mb-4">Timeline</h3>
          <div className="relative ml-3">
            <div className="absolute left-[3px] top-2 bottom-2 w-px bg-bg-border" />
            <div className="space-y-0">
              {events.map((event, i) => {
                const prevEvent = events[i - 1]
                const deltaMs = prevEvent ? new Date(event.timestamp) - new Date(prevEvent.timestamp) : null
                let dotColor = EVENT_COLORS[event.type] || 'bg-text-muted'
                if (event.type === 'poll_completed' && event.data?.gateway_status !== 'captured') dotColor = 'bg-status-red'

                return (
                  <div key={i} className="relative pl-7 pb-4">
                    <div className={cn('absolute left-0 top-1.5 h-2 w-2 rounded-full ring-2 ring-bg-surface', dotColor)} />
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-mono text-text-muted tabular-nums">{formatTimestamp(event.timestamp)}</span>
                        <span className="text-sm font-mono text-text-primary">{event.type}</span>
                        {event.attempt && <span className="text-xs text-text-muted font-mono">[{event.attempt}]</span>}
                      </div>
                      <p className="text-xs text-text-muted mt-0.5">{event.source} → {event.detail}</p>
                      {event.type === 'state_transition' && event.data && (
                        <div className="flex items-center gap-2 mt-1">
                          <span className={cn('text-xs font-mono font-medium', STATE_COLORS[event.data.from])}>{event.data.from}</span>
                          <span className="text-text-muted text-xs">→</span>
                          <span className={cn('text-xs font-mono font-medium', STATE_COLORS[event.data.to])}>{event.data.to}</span>
                        </div>
                      )}
                      {deltaMs != null && (
                        <span className="text-xs text-text-muted font-mono mt-0.5 block">+{formatDuration(deltaMs)}</span>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Polls */}
        {polls.length > 0 && (
          <div>
            <h3 className="text-xs text-text-muted uppercase tracking-widest mb-3">Verification polls</h3>
            <div className="border border-bg-border rounded-xl overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-bg-border">
                    {['Attempt', 'Timestamp', 'Response', 'Amount', 'Δ'].map(h => (
                      <th key={h} className="text-left text-xs text-text-muted px-3 py-2 font-normal">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {polls.map((poll, i) => (
                    <tr key={i} className="border-b border-bg-border last:border-0">
                      <td className="px-3 py-2 text-xs font-mono text-text-primary">{poll.attempt}</td>
                      <td className="px-3 py-2 text-xs font-mono text-text-muted">{formatTimestamp(poll.timestamp)}</td>
                      <td className="px-3 py-2 text-xs font-mono">
                        {poll.gateway_response === 'success'
                          ? <span className="text-status-green">success</span>
                          : <span className="text-status-red">failure</span>}
                      </td>
                      <td className={cn('px-3 py-2 text-xs font-mono', poll.amount !== txn.amount ? 'text-status-yellow' : 'text-text-primary')}>
                        {poll.amount !== txn.amount && '⚠ '}{formatCurrency(poll.amount)}
                      </td>
                      <td className="px-3 py-2 text-xs font-mono text-text-muted">
                        {poll.delta_from_prev ? `+${formatDuration(poll.delta_from_prev)}` : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Raw payloads */}
        {(txn.raw_webhook || txn.raw_verification) && (
          <div className="space-y-3">
            <h3 className="text-xs text-text-muted uppercase tracking-widest">Raw payloads</h3>
            {txn.raw_webhook && <JsonBlock title={`Webhook (${txn.gateway})`} data={txn.raw_webhook} />}
            {txn.raw_verification && <JsonBlock title="Verification response" data={txn.raw_verification} />}
          </div>
        )}
      </div>
    </Drawer>
  )
}
