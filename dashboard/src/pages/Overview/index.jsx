import { useState, useEffect } from 'react'
import { LayoutDashboard, Activity } from 'lucide-react'
import { api } from '../../lib/api'
import { cn, formatRelativeTime, formatCurrency, statusLabel } from '../../lib/utils'
import { deriveOverviewStats } from './deriveStats'

const STATUS_COLOR = {
  CONFIRMED:     'text-status-green',
  FAILED:        'text-status-red',
  INDETERMINATE: 'text-status-yellow',
  MISMATCH:      'text-status-yellow',
  PENDING:       'text-text-muted',
  VERIFYING:     'text-text-muted',
  REFUNDED:      'text-text-secondary',
}

function StatCard({ label, value, sub, warn, hint }) {
  return (
    <div title={hint} className="bg-bg-surface border border-bg-border rounded-xl px-4 py-3">
      <p className="text-xs text-text-muted mb-1">{label}</p>
      <p className={cn('text-2xl font-mono font-medium', warn ? 'text-status-red' : 'text-text-primary')}>
        {value ?? '—'}
      </p>
      {sub && <p className="text-xs text-text-muted mt-0.5">{sub}</p>}
    </div>
  )
}

export default function Overview() {
  const [overviewStats,  setOverviewStats]  = useState(null)
  const [mismatchStats,  setMismatchStats]  = useState(null)
  const [deliveryStats,  setDeliveryStats]  = useState(null)
  const [recentTxns,     setRecentTxns]     = useState([])
  const [loading,        setLoading]        = useState(true)

  useEffect(() => {
    async function load() {
      try {
        const [ov, mm, del, txns] = await Promise.all([
          api.getOverviewStats(),
          api.getMismatchStats(),
          api.getDeliveryStats(),
          api.getTransactions({ limit: 5 }),
        ])
        setOverviewStats(ov)
        setMismatchStats(mm)
        setDeliveryStats(del)
        setRecentTxns(txns?.data ?? [])
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [])

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="h-16 bg-bg-surface rounded-xl animate-pulse" />
        <div className="grid grid-cols-3 gap-4">
          {[1, 2, 3, 4, 5, 6].map(i => (
            <div key={i} className="h-20 bg-bg-surface rounded-xl animate-pulse" />
          ))}
        </div>
        <div className="h-48 bg-bg-surface rounded-xl animate-pulse" />
      </div>
    )
  }

  // Derive numbers from overview stats
  const {
    activeHolds,
    pendingDel,
    exhaustedDel,
    rejectedHooks,
    mismatchCount,
    deliveredCount,
    deliveryRate,
    hasDeliveryIssues,
  } = deriveOverviewStats(overviewStats, mismatchStats, deliveryStats)

  const stats = [
    {
      label: 'Payments in progress',
      value: activeHolds,
      warn: activeHolds >= 20,
      hint: 'Customers currently checking out, awaiting confirmation.',
    },
    {
      label: 'Confirmations pending',
      value: pendingDel,
      warn: pendingDel > 50,
      hint: 'Verified results queued to notify your app.',
    },
    {
      label: "Couldn't reach your app",
      value: exhaustedDel,
      warn: exhaustedDel > 0,
      hint: 'Confirmations we tried to deliver but your app never accepted.',
    },
    {
      label: 'Blocked signals',
      value: rejectedHooks,
      warn: rejectedHooks > 5,
      hint: 'Suspicious webhooks that failed signature verification.',
      sub: '/ 1 hr',
    },
    {
      label: 'Webhook contradictions',
      value: mismatchCount,
      warn: false,
      hint: 'Transactions where the first webhook claimed one outcome but verification confirmed the opposite.',
      sub: 'last 7 days',
    },
    {
      label: 'Delivery success rate',
      value: `${deliveryRate}%`,
      warn: deliveryRate < 95,
      hint: 'Ratio of successfully delivered callbacks to total attempted.',
      sub: `${deliveredCount} delivered`,
    },
  ]

  return (
    <div className="space-y-6">

      {/* Page header */}
      <div className="flex items-center gap-2">
        <LayoutDashboard size={18} strokeWidth={1.5} className="text-text-muted" />
        <h1 className="text-base font-medium text-text-primary">Overview</h1>
      </div>

      {/* Delivery status banner */}
      <div className={cn(
        'rounded-xl border px-5 py-4',
        hasDeliveryIssues
          ? 'bg-bg-surface border-status-red/30'
          : 'bg-bg-surface border-bg-border'
      )}>
        <div className="flex items-center gap-3">
          <span className={cn(
            'h-2.5 w-2.5 rounded-full flex-shrink-0',
            hasDeliveryIssues ? 'bg-status-red animate-pulse' : 'bg-status-green'
          )} />
          <span className="text-base font-medium text-text-primary">
            {hasDeliveryIssues
              ? `Delivery issues detected — ${exhaustedDel > 0 ? `${exhaustedDel} exhausted deliveries need attention` : `${rejectedHooks} blocked webhooks in the last hour`}`
              : 'No delivery issues. Callbacks are being sent and webhooks accepted normally.'
            }
          </span>
        </div>
        {hasDeliveryIssues && (
          <p className="text-sm text-text-muted mt-1 pl-[22px]">
            {exhaustedDel > 0 && 'Check Health → replay exhausted deliveries. '}
            {rejectedHooks > 5 && 'High webhook rejection rate may indicate a secret mismatch.'}
          </p>
        )}
      </div>

      {/* Stats grid — 3 columns × 2 rows */}
      <div className="grid grid-cols-3 gap-3">
        {stats.map(s => (
          <StatCard key={s.label} {...s} />
        ))}
      </div>

      {/* Recent transactions */}
      <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-bg-border flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Activity size={14} strokeWidth={1.5} className="text-text-muted" />
            <h2 className="text-sm font-medium text-text-primary">Recent transactions</h2>
          </div>
          <a
            href="/dashboard/transactions"
            className="text-xs text-text-muted hover:text-text-primary transition-colors"
          >
            View all →
          </a>
        </div>

        {recentTxns.length === 0 ? (
          <div className="px-5 py-8 text-center text-sm text-text-muted">No transactions found</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-bg-border">
                {['TXN ID', 'Gateway', 'Amount', 'Status', 'Time'].map(h => (
                  <th key={h} className="text-left text-xs text-text-muted px-4 py-2.5 font-normal">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {recentTxns.map(txn => (
                <tr key={txn.txn_id} className="border-b border-bg-border last:border-0 hover:bg-bg-elevated">
                  <td className="px-4 py-3 text-xs font-mono text-text-primary">{txn.txn_id?.slice(0, 16)}</td>
                  <td className="px-4 py-3 text-xs text-text-secondary">{txn.gateway}</td>
                  <td className="px-4 py-3 text-xs font-mono text-text-primary">{formatCurrency(txn.amount)}</td>
                  <td className="px-4 py-3 text-xs font-mono">
                    <span className={cn(STATUS_COLOR[txn.status] ?? 'text-text-muted')}>
                      {statusLabel(txn.status)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs text-text-muted font-mono">
                    {formatRelativeTime(txn.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

    </div>
  )
}
