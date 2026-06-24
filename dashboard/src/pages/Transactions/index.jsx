import { useState, useRef } from 'react'
import { Search } from 'lucide-react'
import StatusBadge from '../../components/shared/StatusBadge'
import TimelineDrawer from './TimelineDrawer'
import { api } from '../../lib/api'
import { cn, formatCurrency, formatRelativeTime, formatDuration, truncate, statusLabel, statusMeaning } from '../../lib/utils'

const STATUS_COLORS = {
  all:           'text-text-secondary',
  PENDING:       'text-status-yellow',
  VERIFYING:     'text-status-blue',
  CONFIRMED:     'text-status-green',
  FAILED:        'text-status-red',
  INDETERMINATE: 'text-status-purple',
  MISMATCH:      'text-status-purple',
  REFUNDED:      'text-status-cyan',
}

const GATEWAY_DOT = {
  PayU:     'bg-status-yellow',
  Razorpay: 'bg-status-blue',
  Cashfree: 'bg-status-cyan',
  PhonePe:  'bg-status-purple',
}

export default function Transactions() {
  const [query, setQuery]               = useState('')
  const [submitted, setSubmitted]       = useState(false)
  const [transactions, setTransactions] = useState([])
  const [total, setTotal]               = useState(0)
  const [loading, setLoading]           = useState(false)
  const [error, setError]               = useState(null)
  const [statusFilter, setStatusFilter] = useState('all')
  const [statusCounts, setStatusCounts] = useState({})
  const [page, setPage]                 = useState(1)
  const [drawerOpen, setDrawerOpen]     = useState(false)
  const [selectedTxn, setSelectedTxn]   = useState(null)
  const inputRef = useRef(null)

  const search = async (q = query, sf = statusFilter, p = 1) => {
    setLoading(true)
    setError(null)
    setSubmitted(true)
    try {
      const result = await api.getTransactions({
        search: q || undefined,
        status: sf === 'all' ? undefined : sf,
        page: p,
        limit: 25,
      })
      setTransactions(result.data)
      setTotal(result.total)
      setStatusCounts(result.status_counts || {})
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') { setPage(1); search(query, statusFilter, 1) }
  }

  const handleFilterClick = (sf) => {
    setStatusFilter(sf)
    if (submitted) { setPage(1); search(query, sf, 1) }
  }

  const handleRowClick = async (row) => {
    setDrawerOpen(true)
    setSelectedTxn(null)
    try {
      const detail = await api.getTransaction(row.txn_id)
      setSelectedTxn(detail)
    } catch (err) {
      console.error(err)
    }
  }

  const totalPages = Math.ceil(total / 25)
  const statuses = ['all', 'PENDING', 'VERIFYING', 'CONFIRMED', 'FAILED', 'INDETERMINATE', 'MISMATCH', 'REFUNDED']

  return (
    <div className="space-y-4">

      {/* Page context for first-timers */}
      <p className="text-sm text-text-muted">Look up any payment and see exactly what happened to it.</p>

      {/* Search — the front door */}
      <div className="relative">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
        <input
          ref={inputRef}
          autoFocus
          type="text"
          placeholder="Paste a payment ID, or search by gateway / amount — press Enter"
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          className="w-full bg-bg-surface border border-bg-border rounded-xl pl-9 pr-4 py-3
                     text-sm text-text-primary placeholder:text-text-muted
                     focus:outline-none focus:border-bg-border"
        />
      </div>

      {/* Status filter chips — only visible after first search */}
      {submitted && (
        <div className="flex gap-2 flex-wrap">
          {statuses.map(s => (
            <button
              key={s}
              onClick={() => handleFilterClick(s)}
              title={s === 'all' ? '' : statusMeaning(s)}
              className={cn(
                'px-3 py-1 rounded-full text-xs border transition-colors',
                statusFilter === s
                  ? cn('border-bg-border bg-bg-elevated', STATUS_COLORS[s])
                  : 'border-transparent text-text-muted hover:text-text-secondary'
              )}
            >
              {s === 'all' ? 'All' : statusLabel(s)}
              {s === 'all'
                ? <span className="ml-1 text-text-muted">({total})</span>
                : statusCounts[s] > 0 && <span className="ml-1 text-text-muted">({statusCounts[s]})</span>
              }
            </button>
          ))}
        </div>
      )}

      {/* Empty state before any search */}
      {!submitted && (
        <div className="text-center py-20 text-text-muted">
          <Search size={32} className="mx-auto mb-3 opacity-30" />
          <p className="text-sm">Enter a txn_id or filter to look up transactions.</p>
        </div>
      )}

      {/* Error */}
      {error && (
        <div className="text-sm text-status-red bg-bg-surface border border-status-red/20 rounded-xl px-4 py-3">
          {error}
        </div>
      )}

      {/* Results */}
      {submitted && !error && (
        <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
          {loading ? (
            <div className="space-y-0">
              {[1,2,3,4,5].map(i => (
                <div key={i} className="h-12 border-b border-bg-border animate-pulse bg-bg-elevated last:border-0" />
              ))}
            </div>
          ) : transactions.length === 0 ? (
            <div className="text-center py-12 text-text-muted">
              <p className="text-sm">No transactions found.</p>
            </div>
          ) : (
            <>
              <table className="w-full">
                <thead>
                  <tr className="border-b border-bg-border">
                    {['TXN ID', 'Gateway', 'Status', 'Amount', 'Created', 'Resolved in'].map(h => (
                      <th key={h} className="text-left text-xs text-text-muted px-4 py-2.5 font-normal">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {transactions.map(row => (
                    <tr
                      key={row.txn_id}
                      onClick={() => handleRowClick(row)}
                      className="border-b border-bg-border last:border-0 hover:bg-bg-elevated cursor-pointer transition-colors"
                    >
                      <td className="px-4 py-3 text-xs font-mono text-text-primary">{truncate(row.txn_id, 16)}</td>
                      <td className="px-4 py-3">
                        <span className="flex items-center gap-1.5 text-xs text-text-secondary">
                          <span className={cn('h-1.5 w-1.5 rounded-full', GATEWAY_DOT[row.gateway] || 'bg-text-muted')} />
                          {row.gateway}
                        </span>
                      </td>
                      <td className="px-4 py-3"><StatusBadge status={row.status} /></td>
                      <td className="px-4 py-3 text-xs font-mono text-text-primary">{formatCurrency(row.amount)}</td>
                      <td className="px-4 py-3 text-xs text-text-muted">{formatRelativeTime(row.created_at)}</td>
                      <td className="px-4 py-3 text-xs font-mono text-text-muted">
                        {row.resolve_duration_ms != null ? formatDuration(row.resolve_duration_ms) : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {totalPages > 1 && (
                <div className="flex items-center justify-between px-4 py-3 border-t border-bg-border">
                  <span className="text-xs text-text-muted">Page {page} of {totalPages} · {total} total</span>
                  <div className="flex gap-2">
                    <button onClick={() => { const p = Math.max(1, page-1); setPage(p); search(query, statusFilter, p) }}
                      disabled={page <= 1}
                      className="text-xs px-3 py-1.5 border border-bg-border rounded text-text-secondary disabled:opacity-30 hover:text-text-primary transition-colors">
                      Previous
                    </button>
                    <button onClick={() => { const p = Math.min(totalPages, page+1); setPage(p); search(query, statusFilter, p) }}
                      disabled={page >= totalPages}
                      className="text-xs px-3 py-1.5 border border-bg-border rounded text-text-secondary disabled:opacity-30 hover:text-text-primary transition-colors">
                      Next
                    </button>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}

      <TimelineDrawer
        open={drawerOpen}
        onClose={() => { setDrawerOpen(false); setSelectedTxn(null) }}
        transaction={selectedTxn}
      />
    </div>
  )
}
