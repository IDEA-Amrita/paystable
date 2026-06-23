import { useState, useEffect } from 'react'
import { Eye, EyeOff, RefreshCw, Check, X } from 'lucide-react'
import { api } from '../../lib/api'
import { cn, formatRelativeTime, formatDate, truncate } from '../../lib/utils'

function ReplayButton({ deliveryId, onReplay }) {
  const [state, setState] = useState('idle')

  const handleClick = async (e) => {
    e.stopPropagation()
    setState('loading')
    try {
      await api.replayDelivery(deliveryId)
      setState('success')
      onReplay?.(deliveryId)
    } catch {
      setState('error')
      setTimeout(() => setState('idle'), 3000)
    }
  }

  if (state === 'success') return (
    <span className="text-xs text-status-green flex items-center gap-1">
      <Check size={11} /> Queued
    </span>
  )
  if (state === 'error') return (
    <span className="text-xs text-status-red flex items-center gap-1">
      <X size={11} /> Failed
    </span>
  )

  return (
    <button
      onClick={handleClick}
      disabled={state === 'loading'}
      className="text-xs text-text-secondary hover:text-text-primary border border-bg-border hover:border-bg-border
                 px-2.5 py-1 rounded transition-colors duration-100 flex items-center gap-1.5"
    >
      <RefreshCw size={11} className={state === 'loading' ? 'animate-spin' : ''} />
      Replay
    </button>
  )
}

export default function Health() {
  const [stats, setStats]               = useState(null)
  const [exhausted, setExhausted]       = useState([])
  const [deliveryStats, setDeliveryStats] = useState(null)
  const [mismatchStats, setMismatchStats] = useState(null)
  const [config, setConfig]             = useState(null)
  const [rotationStatus, setRotationStatus] = useState(null)
  const [loading, setLoading]           = useState(true)
  const [replayedIds, setReplayedIds]   = useState(new Set())

  const [showRotationForm, setShowRotationForm] = useState(false)
  const [newSecret, setNewSecret]       = useState('')
  const [showSecret, setShowSecret]     = useState(false)
  const [windowHours, setWindowHours]   = useState(24)
  const [rotateLoading, setRotateLoading] = useState(false)
  const [rotateError, setRotateError]   = useState(null)

  useEffect(() => {
    async function load() {
      try {
        const [s, del, delStats, mm, cfg, rot] = await Promise.all([
          api.getOverviewStats(),
          api.getDeliveries({ status: 'exhausted' }),
          api.getDeliveryStats(),
          api.getMismatchStats(),
          api.getConfig(),
          api.getRotationStatus(),
        ])
        setStats(s)
        setExhausted(del.data)
        setDeliveryStats(delStats)
        setMismatchStats(mm)
        setConfig(cfg)
        setRotationStatus(rot)
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [])

  const handleRotate = async () => {
    if (!newSecret.trim()) { setRotateError('Secret cannot be empty'); return }
    setRotateLoading(true)
    setRotateError(null)
    try {
      await api.rotateSecret({ new_secret: newSecret, window_hours: windowHours })
      setShowRotationForm(false)
      setNewSecret('')
      const rot = await api.getRotationStatus()
      setRotationStatus(rot)
    } catch (err) {
      setRotateError(err.message)
    } finally {
      setRotateLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="h-20 bg-bg-surface rounded-xl animate-pulse" />
        <div className="grid grid-cols-3 gap-4">
          {[1,2,3].map(i => <div key={i} className="h-24 bg-bg-surface rounded-xl animate-pulse" />)}
        </div>
      </div>
    )
  }

  const exhaustedCount = deliveryStats?.exhausted ?? 0
  const mismatchCount  = mismatchStats?.last_7_days ?? 0
  const stuckCount     = (stats?.active_holds?.value ?? 0)
  const pendingDel     = deliveryStats?.pending ?? 0

  const allGood = exhaustedCount === 0 && stuckCount < 20

  return (
    <div className="space-y-6">

      {/* Status headline — the most important thing on this page */}
      <div className={cn(
        'rounded-xl border px-5 py-4',
        allGood
          ? 'bg-bg-surface border-bg-border'
          : 'bg-bg-surface border-status-red/30'
      )}>
        <div className="flex items-center gap-3">
          <span className={cn(
            'h-2.5 w-2.5 rounded-full flex-shrink-0',
            allGood ? 'bg-status-green' : 'bg-status-red animate-pulse'
          )} />
          <span className="text-base font-medium text-text-primary">
            {allGood
              ? 'Everything looks normal. All payments are being verified and confirmed.'
              : `${exhaustedCount > 0 ? `${exhaustedCount} customers may have paid without your app knowing.` : ''} ${stuckCount >= 20 ? `${stuckCount} payments still in progress.` : ''}`.trim()
            }
          </span>
        </div>
        {!allGood && (
          <p className="text-sm text-text-muted mt-1 pl-[22px]">
            {exhaustedCount > 0 && 'Replay the failed confirmations below so your app gets notified. '}
            {stuckCount >= 20 && 'A high number still in progress may mean verification is running slow.'}
          </p>
        )}
      </div>

      {/* Four compact tiles, plain language */}
      <div className="grid grid-cols-4 gap-3">
        {[
          { label: 'Payments in progress', value: stuckCount,     warn: stuckCount >= 20, hint: 'Customers currently checking out, awaiting confirmation.' },
          { label: 'Confirmations to send', value: pendingDel,     warn: pendingDel > 50, hint: 'Verified results queued to notify your app.' },
          { label: "Couldn't reach your app", value: exhaustedCount, warn: exhaustedCount > 0, hint: 'Confirmations we tried to deliver but your app never accepted.' },
          { label: 'Blocked signals',  value: stats?.rejected_webhooks?.value ?? 0, warn: (stats?.rejected_webhooks?.value ?? 0) > 5, hint: 'Suspicious webhooks that failed signature verification.' },
        ].map(({ label, value, warn, hint }) => (
          <div key={label} title={hint} className="bg-bg-surface border border-bg-border rounded-xl px-4 py-3">
            <p className="text-xs text-text-muted mb-1">{label}</p>
            <p className={cn('text-2xl font-mono font-medium', warn ? 'text-status-red' : 'text-text-primary')}>
              {value}
            </p>
          </div>
        ))}
      </div>

      {/* Mismatch hero */}
      <div className="bg-bg-surface border border-bg-border rounded-xl px-5 py-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs text-text-muted mb-1">False signals caught</p>
            <p className="text-3xl font-mono font-medium text-text-primary">{mismatchCount}</p>
            <p className="text-xs text-text-secondary mt-1">
              gateway webhooks that were wrong — prevented by paystable this week
            </p>
          </div>
          <a href="/dashboard/mismatches" className="text-xs text-text-muted hover:text-text-primary transition-colors">
            View all →
          </a>
        </div>
      </div>

      {/* Exhausted deliveries */}
      {exhaustedCount > 0 && (
        <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
          <div className="px-5 py-3 border-b border-bg-border">
            <h2 className="text-sm font-medium text-text-primary">Customers charged, but your app wasn't told</h2>
            <p className="text-xs text-text-muted mt-0.5">
              We confirmed these payments and tried to notify your app 8 times over 24 hours. It never accepted them. Replay to try again.
            </p>
          </div>
          <table className="w-full">
            <thead>
              <tr className="border-b border-bg-border">
                {['TXN ID', 'Event', 'Attempts', 'Last error', 'Last attempt', ''].map(h => (
                  <th key={h} className="text-left text-xs text-text-muted px-4 py-2.5 font-normal">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {exhausted.map((d) => (
                <tr key={d.id} className={cn('border-b border-bg-border last:border-0 hover:bg-bg-elevated', replayedIds.has(d.id) && 'opacity-50')}>
                  <td className="px-4 py-3 text-xs font-mono text-text-primary">{truncate(d.txn_id, 14)}</td>
                  <td className="px-4 py-3 text-xs font-mono text-text-secondary">{d.event_type}</td>
                  <td className="px-4 py-3 text-xs font-mono text-status-red">{d.attempts}/{d.max_attempts}</td>
                  <td className="px-4 py-3 text-xs text-text-muted max-w-[180px] truncate" title={d.last_error}>{d.last_error}</td>
                  <td className="px-4 py-3 text-xs text-text-muted font-mono">{formatRelativeTime(d.last_attempt_at)}</td>
                  <td className="px-4 py-3">
                    {replayedIds.has(d.id)
                      ? <span className="text-xs text-status-green">Replaying</span>
                      : <ReplayButton deliveryId={d.id} onReplay={(id) => setReplayedIds(prev => new Set([...prev, id]))} />
                    }
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Secret rotation */}
      <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-bg-border flex items-center justify-between">
          <h2 className="text-sm font-medium text-text-primary">Webhook secret rotation</h2>
          {rotationStatus?.is_active && (
            <span className="text-xs text-status-yellow font-mono">ROTATION ACTIVE · {rotationStatus.hours_remaining}h left</span>
          )}
        </div>
        <div className="p-5">
          {rotationStatus?.is_active ? (
            <p className="text-sm text-text-secondary">
              Both old and new keys are being accepted. Old key drops automatically when the window closes on{' '}
              <span className="font-mono text-text-primary">{rotationStatus.window_ends_at ? formatDate(rotationStatus.window_ends_at) : '—'}</span>.
            </p>
          ) : showRotationForm ? (
            <div className="space-y-4">
              <div>
                <label className="text-xs text-text-muted block mb-1.5">New PayU webhook secret</label>
                <div className="relative">
                  <input
                    type={showSecret ? 'text' : 'password'}
                    value={newSecret}
                    onChange={e => setNewSecret(e.target.value)}
                    placeholder="Paste new secret..."
                    className="w-full bg-bg-muted border border-bg-border rounded-lg px-3 py-2.5
                               text-sm font-mono text-text-primary placeholder:text-text-muted
                               focus:outline-none focus:border-bg-border pr-9"
                  />
                  <button onClick={() => setShowSecret(!showSecret)}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary">
                    {showSecret ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
              </div>
              <div className="flex gap-2">
                {[1, 24, 48].map(h => (
                  <button key={h} onClick={() => setWindowHours(h)}
                    className={cn(
                      'px-3 py-1.5 rounded-lg text-xs border transition-colors',
                      windowHours === h
                        ? 'border-text-muted text-text-primary bg-bg-elevated'
                        : 'border-bg-border text-text-muted hover:border-text-muted'
                    )}>
                    {h}h window
                  </button>
                ))}
              </div>
              {rotateError && <p className="text-xs text-status-red">{rotateError}</p>}
              <div className="flex gap-3 pt-1">
                <button onClick={() => { setShowRotationForm(false); setRotateError(null) }}
                  className="text-sm text-text-muted hover:text-text-primary transition-colors">
                  Cancel
                </button>
                <button onClick={handleRotate} disabled={rotateLoading}
                  className="px-4 py-2 rounded-lg text-sm bg-text-primary text-bg-base font-medium
                             hover:opacity-90 transition-opacity disabled:opacity-50">
                  {rotateLoading ? 'Rotating…' : 'Begin rotation'}
                </button>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-between">
              <div className="text-sm text-text-muted">
                Last rotated:{' '}
                <span className="text-text-secondary">
                  {rotationStatus?.last_rotated_at ? formatDate(rotationStatus.last_rotated_at) : 'never'}
                </span>
              </div>
              <button onClick={() => setShowRotationForm(true)}
                className="text-xs text-text-secondary hover:text-text-primary border border-bg-border
                           px-3 py-1.5 rounded transition-colors">
                Rotate secret
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Runtime config — stripped to 3 meaningful values */}
      <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-bg-border">
          <h2 className="text-sm font-medium text-text-primary">Runtime</h2>
        </div>
        <div className="divide-y divide-bg-border">
          {(config || []).filter(c => !c.is_secret || ['GATEWAY', 'STABILIZATION_N', 'LOG_LEVEL'].includes(c.key)).slice(0, 6).map(item => (
            <div key={item.key} className="flex items-center justify-between px-5 py-2.5">
              <span className="text-xs font-mono text-text-muted">{item.key}</span>
              {item.is_secret
                ? <span className="text-xs font-mono text-text-muted">{'•'.repeat(12)}{' '}<span className={cn('text-[10px]', item.is_set ? 'text-status-green' : 'text-status-red')}>{item.is_set ? 'set' : 'NOT SET'}</span></span>
                : <span className="text-xs font-mono text-text-primary">{item.value}</span>
              }
            </div>
          ))}
        </div>
      </div>

    </div>
  )
}
