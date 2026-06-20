import { useState, useEffect } from 'react'
import { Search, SearchX } from 'lucide-react'
import DataTable from '../../components/shared/DataTable'
import StatusBadge from '../../components/shared/StatusBadge'
import TimelineDrawer from './TimelineDrawer'
import { api } from '../../lib/api'
import { cn, formatCurrency, formatRelativeTime, formatDuration, truncate } from '../../lib/utils'

const STATUSES = ['all', 'PENDING', 'VERIFYING', 'CONFIRMED', 'FAILED', 'INDETERMINATE', 'REFUNDED']

const STATUS_CHIP_COLORS = {
  all:           { active: 'border-text-secondary text-text-primary', inactive: '' },
  PENDING:       { active: 'border-neon-yellow/60 text-neon-yellow bg-neon-yellow/5', inactive: '' },
  VERIFYING:     { active: 'border-neon-blue/60 text-neon-blue bg-neon-blue/5', inactive: '' },
  CONFIRMED:     { active: 'border-neon-green/60 text-neon-green bg-neon-green/5', inactive: '' },
  FAILED:        { active: 'border-neon-red/60 text-neon-red bg-neon-red/5', inactive: '' },
  INDETERMINATE: { active: 'border-neon-purple/60 text-neon-purple bg-neon-purple/5', inactive: '' },
  REFUNDED:      { active: 'border-neon-cyan/60 text-neon-cyan bg-neon-cyan/5', inactive: '' },
}

const GATEWAY_COLORS = {
  PayU:      'bg-neon-yellow',
  Razorpay:  'bg-neon-blue',
  Cashfree:  'bg-neon-cyan',
  PhonePe:   'bg-neon-purple',
}

export default function Transactions() {
  const [transactions, setTransactions] = useState([])
  const [statusCounts, setStatusCounts] = useState({})
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [selectedTxn, setSelectedTxn] = useState(null)
  const [loadingDetail, setLoadingDetail] = useState(false)

  const fetchTransactions = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await api.getTransactions({
        status: statusFilter === 'all' ? undefined : statusFilter,
        search: search || undefined,
        page,
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

  useEffect(() => {
    fetchTransactions()
  }, [statusFilter, page])

  // Debounced search
  useEffect(() => {
    const timeout = setTimeout(() => {
      setPage(1)
      fetchTransactions()
    }, 300)
    return () => clearTimeout(timeout)
  }, [search])

  const handleRowClick = async (row) => {
    setDrawerOpen(true)
    setLoadingDetail(true)
    try {
      const detail = await api.getTransaction(row.txn_id)
      setSelectedTxn(detail)
    } catch (err) {
      console.error('Failed to load transaction:', err)
    } finally {
      setLoadingDetail(false)
    }
  }

  const columns = [
    {
      key: 'txn_id',
      label: 'TXN ID',
      sortable: true,
      mono: true,
      render: (val) => (
        <span className="font-mono text-sm" title={val}>
          {truncate(val, 14)}
        </span>
      ),
    },
    {
      key: 'gateway',
      label: 'Gateway',
      sortable: true,
      render: (val) => (
        <span className="flex items-center gap-2 text-sm">
          <span className={cn('h-2 w-2 rounded-full', GATEWAY_COLORS[val] || 'bg-text-muted')} />
          {val}
        </span>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (val) => <StatusBadge status={val} />,
    },
    {
      key: 'amount',
      label: 'Amount',
      sortable: true,
      align: 'right',
      render: (val) => (
        <span className="font-mono tabular-nums">{formatCurrency(val)}</span>
      ),
    },
    {
      key: 'created_at',
      label: 'Created',
      sortable: true,
      render: (val) => (
        <span className="text-xs text-text-secondary" title={val}>
          {formatRelativeTime(val)}
        </span>
      ),
    },
    {
      key: 'resolve_duration_ms',
      label: 'Time to Resolve',
      sortable: true,
      align: 'right',
      render: (val) => {
        if (val == null) return <span className="text-xs text-text-muted">—</span>
        const isLong = val > 300000 // > 5 min
        return (
          <span className={cn('font-mono text-xs tabular-nums', isLong ? 'text-neon-red' : 'text-text-secondary')}>
            {formatDuration(val)}
          </span>
        )
      },
    },
  ]

  const totalPages = Math.ceil(total / 25)

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-text-primary tracking-tight">Transactions</h1>

      {/* Search + Filter Bar */}
      <div className="space-y-3">
        <div className="relative">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
          <input
            type="text"
            placeholder="Search by txn_id, gateway, amount..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full bg-bg-muted border border-bg-border rounded-lg pl-10 pr-4 py-2.5
                       text-sm text-text-primary placeholder:text-text-muted
                       focus:outline-none focus:ring-2 focus:ring-neon-green/30 focus:border-neon-green/40
                       transition-all duration-150"
          />
        </div>

        <div className="flex gap-2 flex-wrap">
          {STATUSES.map((status) => {
            const isActive = statusFilter === status
            const count = status === 'all' ? total : (statusCounts[status] || 0)
            const chipColors = STATUS_CHIP_COLORS[status]

            return (
              <button
                key={status}
                onClick={() => { setStatusFilter(status); setPage(1) }}
                className={cn(
                  'px-3 py-1.5 rounded-full text-xs font-medium font-mono border',
                  'transition-all duration-150',
                  isActive
                    ? chipColors.active
                    : 'bg-bg-muted text-text-secondary border-bg-border hover:border-text-muted hover:text-text-primary'
                )}
              >
                {status === 'all' ? 'All' : status}
                <span className="ml-1.5 text-text-muted">({count})</span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Error state */}
      {error && (
        <div className="text-sm text-neon-red bg-neon-red/5 border border-neon-red/20 rounded-lg px-4 py-3">
          Error: {error}
        </div>
      )}

      {/* Transactions Table */}
      <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
        <DataTable
          columns={columns}
          data={transactions}
          loading={loading}
          onRowClick={handleRowClick}
          selectedId={selectedTxn?.txn_id}
          emptyMessage="No transactions match your filters. Try widening the date range or clearing status filters."
          emptyIcon={SearchX}
        />

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-bg-border">
            <span className="text-xs text-text-secondary">
              Page {page} of {totalPages} · {total} total
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page <= 1}
                className={cn(
                  'px-3 py-1.5 text-xs font-medium rounded-lg border',
                  'transition-all duration-150',
                  page <= 1
                    ? 'text-text-muted border-bg-border cursor-not-allowed'
                    : 'text-text-secondary border-bg-border hover:text-text-primary hover:border-text-muted'
                )}
              >
                Previous
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className={cn(
                  'px-3 py-1.5 text-xs font-medium rounded-lg border',
                  'transition-all duration-150',
                  page >= totalPages
                    ? 'text-text-muted border-bg-border cursor-not-allowed'
                    : 'text-text-secondary border-bg-border hover:text-text-primary hover:border-text-muted'
                )}
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Timeline Drawer */}
      <TimelineDrawer
        open={drawerOpen}
        onClose={() => { setDrawerOpen(false); setSelectedTxn(null) }}
        transaction={selectedTxn}
      />
    </div>
  )
}
