import { useState, useEffect } from 'react'
import { Download } from 'lucide-react'
import { api } from '../../lib/api'
import { cn, formatDuration, formatRelativeTime, truncate, downloadCSV } from '../../lib/utils'

const GATEWAY_DOT = {
  PayU:     'bg-status-yellow',
  Razorpay: 'bg-status-blue',
  Cashfree: 'bg-status-cyan',
  PhonePe:  'bg-status-purple',
}

export default function Mismatches() {
  const [mismatches, setMismatches] = useState([])
  const [stats, setStats]           = useState(null)
  const [loading, setLoading]       = useState(true)
  const [error, setError]           = useState(null)

  useEffect(() => {
    async function load() {
      try {
        const [m, s] = await Promise.all([api.getMismatches({}), api.getMismatchStats()])
        setMismatches(m.data)
        setStats(s)
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [])

  const handleExport = () => {
    const headers = ['TXN ID', 'Gateway', 'Webhook claimed', 'Verified truth', 'Detected at', 'Time saved']
    const rows = mismatches.map(m => [
      m.txn_id, m.gateway, m.webhook_claimed, m.verified_truth,
      m.detected_at, formatDuration(m.time_saved_ms),
    ])
    downloadCSV(headers, rows, 'paystable-mismatches.csv')
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="h-16 bg-bg-surface rounded-xl animate-pulse" />
        <div className="h-64 bg-bg-surface rounded-xl animate-pulse" />
      </div>
    )
  }

  if (error) {
    return <div className="text-sm text-status-red p-4">{error}</div>
  }

  const count7d = stats?.last_7_days ?? 0

  return (
    <div className="space-y-5">

      {/* Count + export — that's all the header needs */}
      <div className="flex items-end justify-between">
        <div>
          <p className="text-3xl font-mono font-medium text-text-primary">{count7d}</p>
          <p className="text-sm text-text-muted mt-0.5">
            times the gateway told us a payment failed when it had actually succeeded — caught in the last 7 days
          </p>
        </div>
        {mismatches.length > 0 && (
          <button
            onClick={handleExport}
            className="flex items-center gap-2 px-3 py-2 text-xs text-text-secondary border border-bg-border
                       rounded-lg hover:text-text-primary hover:border-bg-border transition-colors"
          >
            <Download size={13} />
            Export CSV
          </button>
        )}
      </div>

      {/* Table */}
      {mismatches.length === 0 ? (
        <div className="bg-bg-surface border border-bg-border rounded-xl py-16 text-center">
          <p className="text-sm text-text-muted">No mismatches yet.</p>
          <p className="text-xs text-text-muted mt-1 max-w-sm mx-auto">
            When a gateway reports a failure that paystable verifies as a success, it will appear here.
          </p>
        </div>
      ) : (
        <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-bg-border">
                {['TXN ID', 'Gateway', 'Webhook claimed', 'Verified truth', 'Detected', 'Time saved'].map(h => (
                  <th key={h} className="text-left text-xs text-text-muted px-4 py-2.5 font-normal">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {mismatches.map((m, i) => (
                <tr key={i} className="border-b border-bg-border last:border-0 hover:bg-bg-elevated transition-colors">
                  <td className="px-4 py-3 text-xs font-mono text-text-primary">{truncate(m.txn_id, 16)}</td>
                  <td className="px-4 py-3">
                    <span className="flex items-center gap-1.5 text-xs text-text-secondary">
                      <span className={cn('h-1.5 w-1.5 rounded-full', GATEWAY_DOT[m.gateway] || 'bg-text-muted')} />
                      {m.gateway}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs font-mono text-status-red">{m.webhook_claimed}</td>
                  <td className="px-4 py-3 text-xs font-mono text-status-green">{m.verified_truth}</td>
                  <td className="px-4 py-3 text-xs text-text-muted">{formatRelativeTime(m.detected_at)}</td>
                  <td className="px-4 py-3 text-xs font-mono text-text-secondary">
                    {formatDuration(m.time_saved_ms)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

    </div>
  )
}
