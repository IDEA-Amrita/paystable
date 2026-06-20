import { useState, useEffect } from 'react'
import { PieChart, Pie, Cell, ResponsiveContainer } from 'recharts'
import { Download, Zap, AlertTriangle, CheckCircle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import StatTile from '../../components/shared/StatTile'
import Sparkline from '../../components/shared/Sparkline'
import DataTable from '../../components/shared/DataTable'
import { api } from '../../lib/api'
import { cn, formatCurrency, formatRelativeTime, formatDuration, truncate, downloadCSV } from '../../lib/utils'

const GATEWAY_COLORS_MAP = {
  PayU:      '#f59e0b',
  Razorpay:  '#60a5fa',
  Cashfree:  '#22d3ee',
  PhonePe:   '#a78bfa',
}

export default function Mismatches() {
  const navigate = useNavigate()
  const [mismatches, setMismatches] = useState([])
  const [stats, setStats] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    async function fetchData() {
      try {
        const [mData, sData] = await Promise.all([
          api.getMismatches({}),
          api.getMismatchStats(),
        ])
        setMismatches(mData.data)
        setStats(sData)
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }, [])

  const handleExport = () => {
    const headers = ['TXN ID', 'Gateway', 'Webhook Claimed', 'Verified Truth', 'Detected At', 'Time Saved']
    const rows = mismatches.map(m => [
      m.txn_id,
      m.gateway,
      m.webhook_claimed,
      m.verified_truth,
      m.detected_at,
      formatDuration(m.time_saved_ms)
    ])
    downloadCSV(headers, rows, 'paystable-mismatches.csv')
  }

  const getTimeSavedColor = (ms) => {
    if (ms < 30000) return 'text-neon-green'
    if (ms < 120000) return 'text-neon-yellow'
    return 'text-neon-orange'
  }

  const mismatchColumns = [
    {
      key: 'txn_id',
      label: 'TXN ID',
      sortable: true,
      mono: true,
      render: (val) => <span className="font-mono text-sm">{truncate(val, 14)}</span>,
    },
    {
      key: 'gateway',
      label: 'Gateway',
      sortable: true,
      render: (val) => (
        <span className="flex items-center gap-2 text-sm">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: GATEWAY_COLORS_MAP[val] || '#484f58' }} />
          {val}
        </span>
      ),
    },
    {
      key: 'webhook_claimed',
      label: 'Webhook Claimed',
      render: (val) => (
        <span className="font-mono text-sm text-neon-red font-medium">{val}</span>
      ),
    },
    {
      key: 'verified_truth',
      label: 'Verified Truth',
      render: (val) => (
        <span className="font-mono text-sm text-neon-green font-medium">{val}</span>
      ),
    },
    {
      key: 'detected_at',
      label: 'Detected At',
      sortable: true,
      render: (val) => (
        <span className="text-xs text-text-secondary" title={val}>
          {formatRelativeTime(val)}
        </span>
      ),
    },
    {
      key: 'time_saved_ms',
      label: 'Time Saved',
      sortable: true,
      align: 'right',
      render: (val) => (
        <span className={cn('font-mono text-sm tabular-nums', getTimeSavedColor(val))}>
          {formatDuration(val)}
        </span>
      ),
    },
  ]

  const amountMismatchColumns = [
    {
      key: 'txn_id',
      label: 'TXN ID',
      mono: true,
      render: (val) => <span className="font-mono text-sm">{truncate(val, 14)}</span>,
    },
    {
      key: 'gateway',
      label: 'Gateway',
      render: (val) => (
        <span className="flex items-center gap-2 text-sm">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: GATEWAY_COLORS_MAP[val] || '#484f58' }} />
          {val}
        </span>
      ),
    },
    {
      key: 'hold_amount',
      label: 'Hold Amount',
      align: 'right',
      render: (val) => <span className="font-mono text-sm">{formatCurrency(val)}</span>,
    },
    {
      key: 'gateway_reported',
      label: 'Gateway Reported',
      align: 'right',
      render: (val) => <span className="font-mono text-sm text-text-primary">{formatCurrency(val)}</span>,
    },
    {
      key: 'difference',
      label: 'Difference',
      align: 'right',
      render: (val) => (
        <span className="font-mono text-sm text-neon-orange font-medium">
          ⚠ {formatCurrency(val)}
        </span>
      ),
    },
    {
      key: 'detected_at',
      label: 'Detected At',
      render: (val) => (
        <span className="text-xs text-text-secondary">{formatRelativeTime(val)}</span>
      ),
    },
  ]

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-semibold text-text-primary tracking-tight">Mismatches</h1>
        <div className="grid grid-cols-3 gap-6">
          {[1, 2, 3].map(i => (
            <div key={i} className="h-32 bg-bg-surface rounded-xl animate-pulse shadow-card" />
          ))}
        </div>
        <div className="h-80 bg-bg-surface rounded-xl animate-pulse shadow-card" />
      </div>
    )
  }

  const perGateway = stats?.per_gateway || {}

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-text-primary tracking-tight">Mismatches</h1>

      {error && (
        <div className="text-sm text-neon-red bg-neon-red/5 border border-neon-red/20 rounded-lg px-4 py-3">
          Error: {error}
        </div>
      )}

      {/* Top Stats */}
      <div className="grid grid-cols-3 gap-6">
        <StatTile
          label="Mismatches Last 7 Days"
          value={stats?.total_7d ?? '—'}
          status={stats?.total_7d > 50 ? 'critical' : stats?.total_7d > 10 ? 'warning' : 'normal'}
          icon={Zap}
        >
          {stats?.trend && (
            <Sparkline data={stats.trend} color="#fb923c" width={100} height={32} />
          )}
        </StatTile>
        <StatTile
          label="Mismatch Rate (7d)"
          value={stats?.rate_7d != null ? `${stats.rate_7d}%` : '—'}
          status="normal"
          icon={AlertTriangle}
        >
          {stats?.rate_trend && (
            <Sparkline data={stats.rate_trend} color="#fb923c" width={100} height={32} />
          )}
        </StatTile>
        <StatTile
          label="Amount Mismatches"
          value={stats?.amount_mismatches ?? '—'}
          subtext="partial capture"
          status={stats?.amount_mismatches > 0 ? 'warning' : 'normal'}
          icon={AlertTriangle}
        />
      </div>

      {/* Mismatch Table */}
      <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-bg-border">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            Status Mismatches
          </h2>
          <button
            onClick={handleExport}
            className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-text-secondary
                       border border-bg-border rounded-lg hover:text-text-primary hover:border-text-muted
                       transition-all duration-150"
          >
            <Download size={14} />
            Export CSV
          </button>
        </div>

        {mismatches.length === 0 ? (
          <div className="text-center py-16">
            <CheckCircle size={40} className="text-neon-green mx-auto mb-3" />
            <p className="text-sm text-text-secondary">
              No mismatches detected in the selected window.<br />
              Paystable is working correctly.
            </p>
          </div>
        ) : (
          <DataTable
            columns={mismatchColumns}
            data={mismatches}
            onRowClick={(row) => navigate(`/transactions?highlight=${row.txn_id}`)}
          />
        )}
      </div>

      {/* Per-Gateway Breakdown */}
      <div>
        <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest mb-4">
          Reliability by Gateway
        </h2>
        <div className="grid grid-cols-4 gap-6">
          {Object.entries(perGateway).map(([gateway, data]) => {
            const donutData = [
              { name: 'Mismatch', value: data.rate },
              { name: 'Clean', value: 100 - data.rate },
            ]
            const isHighest = Object.entries(perGateway).every(
              ([, d]) => d.rate <= data.rate
            )

            return (
              <div
                key={gateway}
                className={cn(
                  'bg-bg-surface border rounded-xl p-5 shadow-card flex flex-col items-center',
                  isHighest ? 'border-neon-orange/30' : 'border-bg-border'
                )}
              >
                <span className="text-sm font-medium text-text-primary mb-3">{gateway}</span>
                <div className="w-24 h-24 relative">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={donutData}
                        cx="50%"
                        cy="50%"
                        innerRadius={28}
                        outerRadius={40}
                        dataKey="value"
                        isAnimationActive={false}
                        strokeWidth={0}
                      >
                        <Cell fill="#fb923c" />
                        <Cell fill="#21262d" />
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="absolute inset-0 flex items-center justify-center">
                    <span className="text-sm font-bold font-mono text-text-primary">
                      {data.rate}%
                    </span>
                  </div>
                </div>
                <span className="text-xs text-text-muted mt-2 font-mono">
                  {data.mismatches} mismatches
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {/* Amount Mismatches */}
      {stats?.amount_mismatches_data && stats.amount_mismatches_data.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-4">
            <AlertTriangle size={14} className="text-neon-orange" />
            <h2 className="text-xs font-medium text-neon-orange uppercase tracking-widest">
              Amount Mismatches — Potential Partial Capture / Fraud
            </h2>
          </div>

          <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
            <DataTable
              columns={amountMismatchColumns}
              data={stats.amount_mismatches_data}
              rowClassName={() => 'bg-neon-orange/5'}
            />
          </div>
        </div>
      )}
    </div>
  )
}
