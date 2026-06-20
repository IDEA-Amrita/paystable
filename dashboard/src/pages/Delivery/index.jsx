import { useState, useEffect } from 'react'
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, ReferenceLine, Cell } from 'recharts'
import { CheckCircle, Truck, AlertTriangle, RefreshCw, Check, X } from 'lucide-react'
import StatTile from '../../components/shared/StatTile'
import { api } from '../../lib/api'
import { cn, formatRelativeTime, truncate } from '../../lib/utils'

const LATENCY_COLORS = ['#22d3a5', '#22d3a5', '#60a5fa', '#f59e0b', '#fb923c', '#f43f5e']

function LatencyTooltip({ active, payload, label }) {
  if (!active || !payload?.[0]) return null
  return (
    <div className="bg-bg-elevated border border-bg-border rounded-lg px-3 py-2 shadow-lg">
      <p className="text-xs font-mono text-text-secondary">{label}</p>
      <p className="text-xs font-mono text-text-primary">{payload[0].value} deliveries</p>
    </div>
  )
}

function ReplayButton({ deliveryId, onReplay }) {
  const [state, setState] = useState('idle') // idle | loading | success | error

  const handleClick = async (e) => {
    e.stopPropagation()
    setState('loading')
    try {
      await api.replayDelivery(deliveryId)
      setState('success')
      onReplay?.(deliveryId)
    } catch (err) {
      setState('error')
      setTimeout(() => setState('idle'), 3000)
    }
  }

  if (state === 'success') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-neon-green px-3 py-1.5">
        <Check size={12} /> Queued
      </span>
    )
  }

  if (state === 'error') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-neon-red px-3 py-1.5">
        <X size={12} /> Failed
      </span>
    )
  }

  return (
    <button
      onClick={handleClick}
      disabled={state === 'loading'}
      className={cn(
        'inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium',
        'border transition-all duration-150',
        state === 'loading'
          ? 'bg-neon-green/5 border-neon-green/20 text-neon-green/60 cursor-wait'
          : 'bg-neon-green/10 border-neon-green/40 text-neon-green hover:bg-neon-green/20 hover:border-neon-green/60'
      )}
    >
      {state === 'loading' ? (
        <>
          <span className="h-3 w-3 border-2 border-neon-green/30 border-t-neon-green rounded-full animate-spin" />
          Replaying…
        </>
      ) : (
        <>
          <RefreshCw size={12} />
          Replay
        </>
      )}
    </button>
  )
}

export default function Delivery() {
  const [deliveryStats, setDeliveryStats] = useState(null)
  const [exhausted, setExhausted] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [replayedIds, setReplayedIds] = useState(new Set())

  useEffect(() => {
    async function fetchData() {
      try {
        const [statsData, delData] = await Promise.all([
          api.getDeliveryStats(),
          api.getDeliveries({ status: 'exhausted' }),
        ])
        setDeliveryStats(statsData)
        setExhausted(delData.data)
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }, [])

  const handleReplay = (id) => {
    setReplayedIds(prev => new Set([...prev, id]))
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-semibold text-text-primary tracking-tight">Delivery</h1>
        <div className="grid grid-cols-3 gap-6">
          {[1, 2, 3].map(i => (
            <div key={i} className="h-32 bg-bg-surface rounded-xl animate-pulse shadow-card" />
          ))}
        </div>
        <div className="h-80 bg-bg-surface rounded-xl animate-pulse shadow-card" />
      </div>
    )
  }

  const exhaustedCount = deliveryStats?.exhausted || 0

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-text-primary tracking-tight">Delivery</h1>

      {error && (
        <div className="text-sm text-neon-red bg-neon-red/5 border border-neon-red/20 rounded-lg px-4 py-3">
          Error: {error}
        </div>
      )}

      {/* Health Tiles */}
      <div className="grid grid-cols-3 gap-6">
        <StatTile
          label="Delivered Today"
          value={deliveryStats?.delivered_today ?? '—'}
          status="normal"
          icon={CheckCircle}
        />
        <StatTile
          label="Pending"
          value={deliveryStats?.pending ?? '—'}
          status="normal"
          icon={Truck}
        />
        <StatTile
          label="Exhausted"
          value={exhaustedCount}
          subtext="needs replay"
          status={exhaustedCount > 10 ? 'critical' : exhaustedCount > 0 ? 'warning' : 'normal'}
          icon={AlertTriangle}
        />
      </div>

      {/* Exhausted Events Table */}
      <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
        <div className="px-5 py-4 border-b border-bg-border">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            Exhausted Events — Require Manual Replay
          </h2>
          <p className="text-xs text-text-muted mt-1">
            These events were attempted 8 times over 24 hours with no successful delivery.
          </p>
        </div>

        {exhausted.length === 0 ? (
          <div className="text-center py-16">
            <CheckCircle size={40} className="text-neon-green mx-auto mb-3" />
            <p className="text-sm text-text-secondary">
              No exhausted events. All deliveries are being received by your endpoint.
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-bg-surface border-b border-bg-border">
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">TXN ID</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Event Type</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Attempts</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Last Error</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Last Attempt</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Callback URL</th>
                  <th className="text-xs text-text-secondary uppercase tracking-widest py-3 px-4 text-left font-medium">Action</th>
                </tr>
              </thead>
              <tbody>
                {exhausted.map((delivery) => {
                  const isReplayed = replayedIds.has(delivery.id)
                  return (
                    <tr
                      key={delivery.id}
                      className={cn(
                        'border-b border-bg-border transition-all duration-300',
                        isReplayed && 'opacity-60'
                      )}
                    >
                      <td className="px-4 py-3 text-sm font-mono text-text-primary">
                        {truncate(delivery.txn_id, 14)}
                      </td>
                      <td className="px-4 py-3 text-sm font-mono text-text-secondary">
                        {delivery.event_type}
                      </td>
                      <td className="px-4 py-3 text-sm font-mono text-neon-red">
                        {delivery.attempts} / {delivery.max_attempts}
                      </td>
                      <td className="px-4 py-3 text-xs font-mono text-text-secondary max-w-[200px]" title={delivery.last_error}>
                        {truncate(delivery.last_error, 40)}
                      </td>
                      <td className="px-4 py-3 text-xs font-mono text-text-muted">
                        {formatRelativeTime(delivery.last_attempt_at)}
                      </td>
                      <td className="px-4 py-3 text-xs font-mono text-text-muted max-w-[180px]" title={delivery.callback_url}>
                        {truncate(delivery.callback_url, 35)}
                      </td>
                      <td className="px-4 py-3">
                        {isReplayed ? (
                          <span className="inline-flex items-center gap-1 text-xs font-mono text-neon-green">
                            <Check size={12} /> Replaying
                          </span>
                        ) : (
                          <ReplayButton deliveryId={delivery.id} onReplay={handleReplay} />
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Delivery Latency Histogram */}
      <div className="bg-bg-surface border border-bg-border rounded-xl p-5 shadow-card">
        <div className="mb-4">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            Delivery Latency Distribution
          </h2>
          <p className="text-xs text-text-muted mt-0.5">
            Time between event creation and first successful delivery
          </p>
        </div>

        <ResponsiveContainer width="100%" height={260}>
          <BarChart data={deliveryStats?.latency_histogram || []} barCategoryGap="20%">
            <CartesianGrid strokeDasharray="3 3" stroke="#21262d" vertical={false} />
            <XAxis
              dataKey="bucket"
              tick={{ fontSize: 11, fill: '#8b949e' }}
              axisLine={{ stroke: '#21262d' }}
              tickLine={false}
            />
            <YAxis
              tick={{ fontSize: 10, fill: '#8b949e' }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip content={<LatencyTooltip />} />
            <ReferenceLine
              x="15–60s"
              stroke="#fb923c"
              strokeDasharray="4 4"
              label={{ value: 'Endpoint may be struggling', position: 'top', fill: '#fb923c', fontSize: 10 }}
            />
            <Bar dataKey="count" isAnimationActive={false} radius={[4, 4, 0, 0]}>
              {(deliveryStats?.latency_histogram || []).map((_, index) => (
                <Cell key={index} fill={LATENCY_COLORS[index] || '#fb923c'} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
