import { useState, useEffect, useRef } from 'react'
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, LineChart, Line, Area, ReferenceLine } from 'recharts'
import { Shield, Truck, AlertTriangle, ShieldAlert } from 'lucide-react'
import StatTile from '../../components/shared/StatTile'
import { api } from '../../lib/api'
import { cn, formatTimestamp } from '../../lib/utils'

function ChartTooltip({ active, payload, label }) {
  if (!active || !payload) return null
  return (
    <div className="bg-bg-elevated border border-bg-border rounded-lg px-3 py-2 shadow-lg">
      <p className="text-xs text-text-secondary mb-1 font-mono">{label}</p>
      {payload.map((entry, i) => (
        <p key={i} className="text-xs font-mono" style={{ color: entry.color }}>
          {entry.name}: {entry.value}
        </p>
      ))}
    </div>
  )
}

function MismatchTooltip({ active, payload, label }) {
  if (!active || !payload?.[0]) return null
  return (
    <div className="bg-bg-elevated border border-bg-border rounded-lg px-3 py-2 shadow-lg">
      <p className="text-xs text-text-secondary mb-1 font-mono">{label}</p>
      <p className="text-xs font-mono text-neon-orange">
        Mismatch Rate: {payload[0].value}%
      </p>
      {payload[0].payload.count != null && (
        <p className="text-xs font-mono text-text-muted">
          Count: {payload[0].payload.count}
        </p>
      )}
    </div>
  )
}

export default function Overview() {
  const [stats, setStats] = useState(null)
  const [volume, setVolume] = useState(null)
  const [mismatchRate, setMismatchRate] = useState(null)
  const [feed, setFeed] = useState([])
  const [loading, setLoading] = useState(true)
  const [refreshPulse, setRefreshPulse] = useState(false)
  const feedRef = useRef(null)

  useEffect(() => {
    async function fetchAll() {
      try {
        const [s, v, m, f] = await Promise.all([
          api.getOverviewStats(),
          api.getVolumeChart(),
          api.getMismatchRateChart(),
          api.getLedgerFeed(),
        ])
        setStats(s)
        setVolume(v)
        setMismatchRate(m)
        setFeed(f)
      } catch (err) {
        console.error('Overview fetch error:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchAll()
  }, [])

  // Live feed refresh every 5s
  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const f = await api.getLedgerFeed()
        setFeed(f)
        setRefreshPulse(true)
        setTimeout(() => setRefreshPulse(false), 500)
      } catch (err) {
        console.error('Feed refresh error:', err)
      }
    }, 5000)
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-semibold text-text-primary tracking-tight">Overview</h1>
        <div className="grid grid-cols-4 gap-6">
          {[1, 2, 3, 4].map(i => (
            <div key={i} className="h-32 bg-bg-surface rounded-xl animate-pulse shadow-card" />
          ))}
        </div>
        <div className="grid grid-cols-2 gap-6">
          <div className="h-72 bg-bg-surface rounded-xl animate-pulse shadow-card" />
          <div className="h-72 bg-bg-surface rounded-xl animate-pulse shadow-card" />
        </div>
        <div className="h-80 bg-bg-surface rounded-xl animate-pulse shadow-card" />
      </div>
    )
  }

  const getStatus = (key) => {
    if (!stats) return 'normal'
    return stats[key]?.status || 'normal'
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-text-primary tracking-tight">Overview</h1>

      {/* Health Tiles */}
      <div className="grid grid-cols-4 gap-6">
        <StatTile
          label="Active Holds"
          value={stats?.active_holds?.value ?? '—'}
          status={getStatus('active_holds')}
          icon={Shield}
        />
        <StatTile
          label="Pending Deliveries"
          value={stats?.pending_deliveries?.value ?? '—'}
          status={getStatus('pending_deliveries')}
          icon={Truck}
        />
        <StatTile
          label="Exhausted Deliveries"
          value={stats?.exhausted_deliveries?.value ?? '—'}
          status={getStatus('exhausted_deliveries')}
          icon={AlertTriangle}
        />
        <StatTile
          label="Rejected Webhooks"
          value={stats?.rejected_webhooks?.value ?? '—'}
          subtext={stats?.rejected_webhooks?.subtext}
          status={getStatus('rejected_webhooks')}
          icon={ShieldAlert}
        />
      </div>

      {/* Charts Row */}
      <div className="grid grid-cols-2 gap-6">
        {/* 24h Volume Chart */}
        <div className="bg-bg-surface border border-bg-border rounded-xl p-5 shadow-card">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest mb-4">
            Transaction Volume — Last 24h
          </h2>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={volume} barCategoryGap="20%">
              <CartesianGrid strokeDasharray="3 3" stroke="#21262d" vertical={false} />
              <XAxis
                dataKey="hour"
                tick={{ fontSize: 10, fill: '#8b949e' }}
                axisLine={{ stroke: '#21262d' }}
                tickLine={false}
                interval={1}
              />
              <YAxis
                tick={{ fontSize: 10, fill: '#8b949e' }}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip content={<ChartTooltip />} />
              <Bar dataKey="confirmed" stackId="a" fill="#22d3a5" name="Confirmed" isAnimationActive={false} radius={[0, 0, 0, 0]} />
              <Bar dataKey="failed" stackId="a" fill="#f43f5e" name="Failed" isAnimationActive={false} />
              <Bar dataKey="indeterminate" stackId="a" fill="#a78bfa" name="Indeterminate" isAnimationActive={false} radius={[2, 2, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>

        {/* Mismatch Rate Chart */}
        <div className="bg-bg-surface border border-bg-border rounded-xl p-5 shadow-card">
          <div className="mb-4">
            <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
              Mismatch Rate
            </h2>
            <p className="text-xs text-text-muted mt-0.5">gateway claimed vs verified truth</p>
          </div>
          <ResponsiveContainer width="100%" height={220}>
            <LineChart data={mismatchRate}>
              <CartesianGrid strokeDasharray="3 3" stroke="#21262d" vertical={false} />
              <XAxis
                dataKey="date"
                tick={{ fontSize: 10, fill: '#8b949e' }}
                axisLine={{ stroke: '#21262d' }}
                tickLine={false}
                interval={1}
              />
              <YAxis
                tick={{ fontSize: 10, fill: '#8b949e' }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v) => `${v}%`}
              />
              <Tooltip content={<MismatchTooltip />} />
              <ReferenceLine y={0} stroke="#22d3a5" strokeDasharray="3 3" label={{ value: 'Goal', position: 'right', fill: '#22d3a5', fontSize: 10 }} />
              <defs>
                <linearGradient id="mismatchGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#fb923c" stopOpacity={0.15} />
                  <stop offset="100%" stopColor="#fb923c" stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area type="monotone" dataKey="rate" stroke="none" fill="url(#mismatchGrad)" isAnimationActive={false} />
              <Line
                type="monotone"
                dataKey="rate"
                stroke="#fb923c"
                strokeWidth={2}
                dot={{ r: 3, fill: '#fb923c', stroke: '#0d1117', strokeWidth: 2 }}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Live Activity Feed */}
      <div className="bg-bg-surface border border-bg-border rounded-xl p-5 shadow-card">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2.5">
            <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
              Live Ledger Feed
            </h2>
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-green opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-neon-green" />
            </span>
          </div>
          <div className="flex items-center gap-2 text-xs text-text-muted">
            <span className={cn(
              'inline-block h-1.5 w-1.5 rounded-full transition-colors duration-300',
              refreshPulse ? 'bg-neon-green' : 'bg-text-muted'
            )} />
            Refreshing every 5s
          </div>
        </div>

        <div ref={feedRef} className="max-h-[400px] overflow-y-auto space-y-0">
          {feed.map((entry, i) => (
            <div
              key={`${entry.timestamp}-${i}`}
              className={cn(
                'grid grid-cols-[90px_180px_100px_1fr] gap-4 px-3 py-2.5 rounded-lg',
                'hover:bg-bg-elevated transition-colors duration-100',
                i === 0 && 'animate-slide-in'
              )}
            >
              <span className="text-xs font-mono text-text-muted tabular-nums">
                {formatTimestamp(entry.timestamp)}
              </span>
              <span className="text-sm font-mono text-text-primary">
                {entry.event_type}
              </span>
              <span className="text-xs text-text-secondary">
                {entry.actor}
              </span>
              <span className="text-xs text-text-secondary truncate">
                {entry.detail}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
