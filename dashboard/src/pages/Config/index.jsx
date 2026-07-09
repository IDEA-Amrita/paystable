import { useState, useEffect } from 'react'
import { Settings, Eye, EyeOff, Check, X, RefreshCw, ShieldCheck } from 'lucide-react'
import { api } from '../../lib/api'
import { cn, formatDate } from '../../lib/utils'

const GATEWAY_OPTIONS = ['payu']

export default function Config() {
  const [config,          setConfig]          = useState(null)
  const [rotationStatus,  setRotationStatus]  = useState(null)
  const [loading,         setLoading]         = useState(true)

  // Rotation form
  const [showForm,        setShowForm]        = useState(false)
  const [gateway,         setGateway]         = useState('payu')
  const [newSecret,       setNewSecret]       = useState('')
  const [showSecret,      setShowSecret]      = useState(false)
  const [windowHours,     setWindowHours]     = useState(24)
  const [rotateLoading,   setRotateLoading]   = useState(false)
  const [rotateError,     setRotateError]     = useState(null)
  const [rotateSuccess,   setRotateSuccess]   = useState(false)

  useEffect(() => {
    async function load() {
      try {
        const [cfg, rot] = await Promise.all([
          api.getConfig(),
          api.getRotationStatus(),
        ])
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
    setRotateSuccess(false)
    try {
      await api.rotateSecret({ gateway, new_secret: newSecret, window_hours: windowHours })
      setRotateSuccess(true)
      setNewSecret('')
      setShowForm(false)
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
        <div className="h-12 bg-bg-surface rounded-xl animate-pulse" />
        <div className="h-64 bg-bg-surface rounded-xl animate-pulse" />
        <div className="h-40 bg-bg-surface rounded-xl animate-pulse" />
      </div>
    )
  }

  const secretRows  = (config || []).filter(c => c.is_secret)
  const unsetSecrets = secretRows.filter(c => !c.is_set)

  return (
    <div className="space-y-6">

      {/* Page header */}
      <div className="flex items-center gap-2">
        <Settings size={18} strokeWidth={1.5} className="text-text-muted" />
        <h1 className="text-base font-medium text-text-primary">Config</h1>
      </div>

      {unsetSecrets.length > 0 && (
        <div className="rounded-xl border border-status-yellow/30 bg-bg-surface px-5 py-4">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-status-yellow flex-shrink-0" />
            <span className="text-sm font-medium text-text-primary">
              {unsetSecrets.length} required secret{unsetSecrets.length > 1 ? 's are' : ' is'} not set
            </span>
          </div>
          <p className="text-xs text-text-muted mt-1 pl-4">
            {unsetSecrets.map(s => s.key).join(', ')}
          </p>
        </div>
      )}

      {/* Environment config table */}
      <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-bg-border flex items-center justify-between">
          <div>
            <h2 className="text-sm font-medium text-text-primary">Environment variables</h2>
            <p className="text-xs text-text-muted mt-0.5">
              Runtime configuration loaded from environment. Read-only — these values were fixed at
              process startup; edit .env and restart to change them. Secrets show only their set status.
            </p>
          </div>
        </div>

        <table className="w-full">
          <thead>
            <tr className="border-b border-bg-border">
              {['Variable', 'Type', 'Value', 'Status'].map(h => (
                <th key={h} className="text-left text-xs text-text-muted px-4 py-2.5 font-normal">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {(config || []).map(item => (
              <tr key={item.key} className="border-b border-bg-border last:border-0 hover:bg-bg-elevated">
                <td className="px-4 py-2.5 text-xs font-mono text-text-primary">{item.key}</td>
                <td className="px-4 py-2.5">
                  {item.is_secret
                    ? <span className="text-[10px] text-text-muted border border-bg-border rounded px-1.5 py-0.5">secret</span>
                    : <span className="text-[10px] text-text-muted border border-bg-border rounded px-1.5 py-0.5">public</span>
                  }
                </td>
                <td className="px-4 py-2.5 text-xs font-mono">
                  {item.is_secret ? (
                    <span className="text-text-muted tracking-widest">{'•'.repeat(10)}</span>
                  ) : (
                    <span className="text-text-primary">{item.value ?? '—'}</span>
                  )}
                </td>
                <td className="px-4 py-2.5">
                  {item.is_set
                    ? <span className="flex items-center gap-1 text-[10px] text-status-green"><Check size={10} /> set</span>
                    : <span className="flex items-center gap-1 text-[10px] text-status-red"><X size={10} /> NOT SET</span>
                  }
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Secret Rotation section */}
      <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-bg-border flex items-center justify-between">
          <div className="flex items-center gap-2">
            <ShieldCheck size={14} strokeWidth={1.5} className="text-text-muted" />
            <h2 className="text-sm font-medium text-text-primary">Secret rotation</h2>
          </div>
          {rotationStatus?.is_active && (
            <span className="text-xs text-status-yellow font-mono">
              ROTATION ACTIVE · {rotationStatus.hours_remaining}h left
            </span>
          )}
        </div>

        <div className="p-5">
          {rotationStatus?.is_active ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <span className="h-2 w-2 rounded-full bg-status-yellow animate-pulse flex-shrink-0" />
                <p className="text-sm text-text-secondary">
                  Rotation is active. Both old and new keys are being accepted.
                </p>
              </div>
              <p className="text-xs text-text-muted pl-4">
                Old key drops automatically on{' '}
                <span className="font-mono text-text-primary">
                  {rotationStatus.window_ends_at ? formatDate(rotationStatus.window_ends_at) : '—'}
                </span>
              </p>
              <div className="grid grid-cols-2 gap-3 mt-3">
                <div className="bg-bg-muted rounded-lg px-4 py-3">
                  <p className="text-xs text-text-muted">Last rotated</p>
                  <p className="text-xs font-mono text-text-primary mt-1">
                    {rotationStatus.last_rotated_at ? formatDate(rotationStatus.last_rotated_at) : 'never'}
                  </p>
                </div>
                <div className="bg-bg-muted rounded-lg px-4 py-3">
                  <p className="text-xs text-text-muted">Hours remaining</p>
                  <p className="text-xs font-mono text-text-primary mt-1">
                    {rotationStatus.hours_remaining ?? '—'}h
                  </p>
                </div>
              </div>
            </div>
          ) : showForm ? (
            <div className="space-y-4">
              {/* Gateway */}
              <div>
                <label className="text-xs text-text-muted block mb-1.5">Gateway</label>
                <select
                  value={gateway}
                  onChange={e => setGateway(e.target.value)}
                  className="w-full bg-bg-muted border border-bg-border rounded-lg px-3 py-2.5
                             text-sm text-text-primary focus:outline-none focus:border-bg-border"
                >
                  {GATEWAY_OPTIONS.map(g => (
                    <option key={g} value={g}>{g}</option>
                  ))}
                </select>
              </div>

              {/* New secret */}
              <div>
                <label className="text-xs text-text-muted block mb-1.5">New webhook secret</label>
                <div className="relative">
                  <input
                    type={showSecret ? 'text' : 'password'}
                    value={newSecret}
                    onChange={e => setNewSecret(e.target.value)}
                    placeholder="Paste new secret…"
                    className="w-full bg-bg-muted border border-bg-border rounded-lg px-3 py-2.5
                               text-sm font-mono text-text-primary placeholder:text-text-muted
                               focus:outline-none focus:border-bg-border pr-9"
                  />
                  <button
                    onClick={() => setShowSecret(!showSecret)}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
                  >
                    {showSecret ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
              </div>

              {/* Window hours */}
              <div>
                <label className="text-xs text-text-muted block mb-1.5">Overlap window</label>
                <div className="flex gap-2">
                  {[1, 24, 48].map(h => (
                    <button
                      key={h}
                      onClick={() => setWindowHours(h)}
                      className={cn(
                        'px-3 py-1.5 rounded-lg text-xs border transition-colors',
                        windowHours === h
                          ? 'border-text-muted text-text-primary bg-bg-elevated'
                          : 'border-bg-border text-text-muted hover:border-text-muted'
                      )}
                    >
                      {h}h window
                    </button>
                  ))}
                </div>
                <p className="text-xs text-text-muted mt-1.5">
                  Both old and new secrets will be accepted during this window.
                </p>
              </div>

              {rotateError && (
                <p className="text-xs text-status-red flex items-center gap-1">
                  <X size={12} /> {rotateError}
                </p>
              )}

              <div className="flex gap-3 pt-1">
                <button
                  onClick={() => { setShowForm(false); setRotateError(null) }}
                  className="text-sm text-text-muted hover:text-text-primary transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleRotate}
                  disabled={rotateLoading}
                  className="px-4 py-2 rounded-lg text-sm bg-text-primary text-bg-base font-medium
                             hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
                >
                  {rotateLoading && <RefreshCw size={13} className="animate-spin" />}
                  {rotateLoading ? 'Rotating…' : 'Begin rotation'}
                </button>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-between">
              <div>
                {rotateSuccess && (
                  <p className="text-xs text-status-green flex items-center gap-1 mb-1">
                    <Check size={12} /> Rotation initiated successfully
                  </p>
                )}
                <p className="text-sm text-text-muted">
                  Last rotated:{' '}
                  <span className="text-text-secondary">
                    {rotationStatus?.last_rotated_at ? formatDate(rotationStatus.last_rotated_at) : 'never'}
                  </span>
                </p>
              </div>
              <button
                onClick={() => { setShowForm(true); setRotateSuccess(false) }}
                className="text-xs text-text-secondary hover:text-text-primary border border-bg-border
                           px-3 py-1.5 rounded transition-colors"
              >
                Rotate secret
              </button>
            </div>
          )}
        </div>
      </div>

    </div>
  )
}
