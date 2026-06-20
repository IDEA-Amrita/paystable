import { useState, useEffect } from 'react'
import { Eye, EyeOff, RotateCcw, Clock, CheckCircle, AlertTriangle } from 'lucide-react'
import { api } from '../../lib/api'
import { cn, formatDate, formatDuration } from '../../lib/utils'

export default function Config() {
  const [config, setConfig] = useState(null)
  const [rotationStatus, setRotationStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // Rotation form state
  const [showRotationForm, setShowRotationForm] = useState(false)
  const [newSecret, setNewSecret] = useState('')
  const [showSecret, setShowSecret] = useState(false)
  const [windowHours, setWindowHours] = useState(24)
  const [rotateLoading, setRotateLoading] = useState(false)
  const [rotateError, setRotateError] = useState(null)

  useEffect(() => {
    async function fetchData() {
      try {
        const [cfgData, rotData] = await Promise.all([
          api.getConfig(),
          api.getRotationStatus(),
        ])
        setConfig(cfgData)
        setRotationStatus(rotData)
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }, [])

  const handleRotate = async () => {
    if (!newSecret.trim()) {
      setRotateError('Secret cannot be empty')
      return
    }
    setRotateLoading(true)
    setRotateError(null)
    try {
      await api.rotateSecret({ new_secret: newSecret, window_hours: windowHours })
      setShowRotationForm(false)
      setNewSecret('')
      // Refresh rotation status
      const rotData = await api.getRotationStatus()
      setRotationStatus(rotData)
    } catch (err) {
      setRotateError(err.message)
    } finally {
      setRotateLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-semibold text-text-primary tracking-tight">Config</h1>
        <div className="h-96 bg-bg-surface rounded-xl animate-pulse shadow-card" />
        <div className="h-48 bg-bg-surface rounded-xl animate-pulse shadow-card" />
      </div>
    )
  }

  if (error && !config) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-semibold text-text-primary tracking-tight">Config</h1>
        <div className="bg-bg-surface border border-bg-border rounded-xl p-8 shadow-card text-center">
          <AlertTriangle size={40} className="text-neon-red mx-auto mb-3" />
          <p className="text-sm text-text-secondary">
            Could not load runtime config. Is the Go server running?
          </p>
          <p className="text-xs text-text-muted mt-1">{error}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-text-primary tracking-tight">Config</h1>

      {/* Runtime Configuration */}
      <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
        <div className="px-5 py-4 border-b border-bg-border">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            Runtime Configuration
          </h2>
          <p className="text-xs text-text-muted mt-0.5">
            Read-only. Reflects values loaded at binary startup.
          </p>
        </div>

        <div>
          {config && config.map((item, i) => (
            <div
              key={item.key}
              className={cn(
                'flex items-center justify-between px-5 py-3.5',
                i % 2 === 0 ? 'bg-bg-surface' : 'bg-bg-elevated'
              )}
            >
              <span className="text-xs font-mono text-text-secondary uppercase tracking-wide">
                {item.key}
              </span>
              <span className="text-right">
                {item.is_secret ? (
                  <span className="flex items-center gap-2">
                    <span className="text-xs font-mono text-text-muted tracking-wider">████████████</span>
                    {item.is_set ? (
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium
                                       bg-neon-green/10 text-neon-green border border-neon-green/20">
                        SET
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium
                                       bg-neon-red/10 text-neon-red border border-neon-red/20">
                        NOT SET
                      </span>
                    )}
                  </span>
                ) : (
                  <span className="text-sm font-mono text-neon-blue">{item.value}</span>
                )}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Secret Rotation Panel */}
      <div className="bg-bg-surface border border-bg-border rounded-xl shadow-card overflow-hidden">
        <div className="px-5 py-4 border-b border-bg-border">
          <h2 className="text-xs font-medium text-text-secondary uppercase tracking-widest">
            PayU Webhook Secret Rotation
          </h2>
        </div>

        <div className="p-5">
          {rotationStatus?.is_active ? (
            /* Active rotation */
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <RotateCcw size={16} className="text-neon-yellow animate-spin" style={{ animationDuration: '3s' }} />
                  <span className="text-sm font-semibold text-neon-yellow">ROTATION IN PROGRESS</span>
                </div>
                <div className="flex items-center gap-1.5 text-xs text-neon-yellow font-mono">
                  <Clock size={12} />
                  {rotationStatus.hours_remaining}h left
                </div>
              </div>

              <div className="space-y-2 text-sm text-text-secondary">
                <p className="flex items-center gap-2">
                  <span className="text-text-muted">Old key:</span>
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium
                                   bg-neon-green/10 text-neon-green border border-neon-green/20">SET</span>
                  <span className="text-xs text-text-muted">
                    — accepted until: {rotationStatus.window_ends_at ? formatDate(rotationStatus.window_ends_at) : '—'}
                  </span>
                </p>
                <p className="flex items-center gap-2">
                  <span className="text-text-muted">New key:</span>
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium
                                   bg-neon-green/10 text-neon-green border border-neon-green/20">SET</span>
                  <span className="text-xs text-text-muted">— active now</span>
                </p>
              </div>

              <div className="bg-bg-muted border border-bg-border rounded-lg p-3 space-y-1">
                <p className="text-xs text-text-secondary">
                  Both keys are being accepted simultaneously.
                </p>
                <p className="text-xs text-text-secondary">
                  Old key will be dropped automatically when window ends.
                </p>
                <p className="text-xs text-text-muted mt-1">
                  Warning at T-1h will fire to Slack/Telegram.
                </p>
              </div>
            </div>
          ) : showRotationForm ? (
            /* Rotation form */
            <div className="space-y-4">
              <h3 className="text-sm font-semibold text-text-primary">Start Secret Rotation</h3>

              <div>
                <label className="block text-xs text-text-secondary mb-1.5">New PayU Webhook Secret</label>
                <div className="relative">
                  <input
                    type={showSecret ? 'text' : 'password'}
                    value={newSecret}
                    onChange={(e) => setNewSecret(e.target.value)}
                    placeholder="Paste new secret here..."
                    className="w-full bg-bg-muted border border-bg-border rounded-lg px-4 py-2.5
                               text-sm font-mono text-text-primary placeholder:text-text-muted
                               focus:outline-none focus:ring-2 focus:ring-neon-green/30 focus:border-neon-green/40
                               transition-all duration-150 pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowSecret(!showSecret)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary
                               transition-colors duration-150"
                  >
                    {showSecret ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs text-text-secondary mb-2">Overlap Window Duration</label>
                <div className="flex gap-3">
                  {[1, 24, 48].map((hours) => (
                    <label
                      key={hours}
                      className={cn(
                        'flex items-center gap-2 px-4 py-2 rounded-lg border cursor-pointer',
                        'transition-all duration-150 text-sm',
                        windowHours === hours
                          ? 'border-neon-green/40 bg-neon-green/5 text-neon-green'
                          : 'border-bg-border text-text-secondary hover:border-text-muted'
                      )}
                    >
                      <input
                        type="radio"
                        name="window"
                        value={hours}
                        checked={windowHours === hours}
                        onChange={() => setWindowHours(hours)}
                        className="sr-only"
                      />
                      <span className={cn(
                        'h-3 w-3 rounded-full border-2',
                        windowHours === hours ? 'border-neon-green bg-neon-green' : 'border-text-muted'
                      )} />
                      {hours} hour{hours !== 1 ? 's' : ''}
                    </label>
                  ))}
                </div>
              </div>

              <div className="bg-bg-muted border border-bg-border rounded-lg p-3">
                <p className="text-xs text-text-secondary">
                  During the overlap window, both the old and new secrets will be accepted.
                  When the window closes, the old secret is permanently dropped.
                </p>
              </div>

              {rotateError && (
                <p className="text-xs text-neon-red">{rotateError}</p>
              )}

              <div className="flex items-center justify-between pt-2">
                <button
                  onClick={() => { setShowRotationForm(false); setRotateError(null) }}
                  className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary
                             transition-colors duration-150"
                >
                  Cancel
                </button>
                <button
                  onClick={handleRotate}
                  disabled={rotateLoading}
                  className={cn(
                    'px-5 py-2 rounded-lg text-sm font-semibold',
                    'transition-all duration-150',
                    rotateLoading
                      ? 'bg-neon-green/50 text-bg-base cursor-wait'
                      : 'bg-neon-green text-bg-base hover:bg-neon-green/90'
                  )}
                >
                  {rotateLoading ? (
                    <span className="flex items-center gap-2">
                      <span className="h-4 w-4 border-2 border-bg-base/30 border-t-bg-base rounded-full animate-spin" />
                      Rotating…
                    </span>
                  ) : (
                    'Begin Rotation →'
                  )}
                </button>
              </div>
            </div>
          ) : (
            /* Default state */
            <div className="space-y-3">
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between">
                  <span className="text-text-muted">Last rotated:</span>
                  <span className="text-text-secondary font-mono text-xs">
                    {rotationStatus?.last_rotated_at ? formatDate(rotationStatus.last_rotated_at) : 'Never'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-text-muted">Rotation window:</span>
                  <span className="text-text-secondary text-xs">Not active</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-text-muted">Current key:</span>
                  <span className="flex items-center gap-2">
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium
                                     bg-neon-green/10 text-neon-green border border-neon-green/20">SET</span>
                    <span className="text-xs font-mono text-text-muted tracking-wider">████████████</span>
                  </span>
                </div>
              </div>

              <div className="flex justify-end pt-2">
                <button
                  onClick={() => setShowRotationForm(true)}
                  className="px-4 py-2 rounded-lg text-sm font-medium text-text-secondary
                             border border-bg-border hover:text-text-primary hover:border-text-muted
                             transition-all duration-150"
                >
                  Start Rotation →
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
