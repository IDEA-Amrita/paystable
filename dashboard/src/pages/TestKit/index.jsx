import { useState, useEffect, useRef } from 'react'
import { cn, formatRelativeTime } from '../../lib/utils'

const GATEWAY_URL  = import.meta.env.VITE_GATEWAY_URL  || 'http://localhost:9090'
const MERCHANT_URL = import.meta.env.VITE_MERCHANT_URL || 'http://localhost:9091'
const PAYSTABLE_URL = ''

const SCENARIOS = [
  {
    id: 'happy-path',
    label: 'Happy path',
    description: 'Gateway says success, verification confirms success. Should reach Paid quickly.',
    status: 'success',
    failUntilS: 0,
    amount: 49900,
  },
  {
    id: 'false-failure',
    label: 'False failure',
    description: 'Gateway fires a FAILURE webhook, but is actually successful after 25s. paystable should still reach Paid.',
    status: 'success',
    failUntilS: 25,
    webhookStatus: 'failure',
    amount: 49900,
  },
  {
    id: 'genuine-failure',
    label: 'Genuine failure',
    description: 'Gateway fires failure, verification confirms failure. Should reach Failed.',
    status: 'failed',
    failUntilS: 0,
    webhookStatus: 'failure',
    amount: 49900,
  },
  {
    id: 'amount-mismatch',
    label: 'Amount mismatch',
    description: 'Gateway reports success but with wrong amount. Should reach Needs attention.',
    status: 'success',
    failUntilS: 0,
    amount: 25000,
    holdAmount: 49900,
  },
  {
    id: 'duplicate-webhook',
    label: 'Duplicate webhook',
    description: 'Same success webhook fired 3 times. Should produce a single Paid result.',
    status: 'success',
    failUntilS: 0,
    amount: 49900,
    duplicate: true,
  },
]

function LogLine({ line }) {
  const colors = {
    info: 'text-text-secondary',
    success: 'text-status-green',
    error: 'text-status-red',
    status: 'text-status-blue',
  }
  return (
    <div className={cn('text-xs font-mono', colors[line.type] || 'text-text-muted')}>
      <span className="text-text-muted mr-2">{line.time}</span>{line.msg}
    </div>
  )
}

export default function TestKit() {
  const [running, setRunning]           = useState(null)
  const [log, setLog]                   = useState([])
  const [merchantOffline, setMerchantOffline] = useState(false)
  const [activeTxn, setActiveTxn]       = useState(null)
  const [currentStatus, setCurrentStatus] = useState(null)
  const pollRef = useRef(null)
  const logRef  = useRef(null)

  const addLog = (msg, type = 'info') => {
    const time = new Date().toLocaleTimeString('en-IN', { hour12: false })
    setLog(prev => [...prev, { msg, type, time }])
  }

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [log])

  useEffect(() => () => clearInterval(pollRef.current), [])

  const pollStatus = (txnID, token) => {
    clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const res = await fetch(`/api/v1/transactions/${txnID}/status?token=${token}`)
        const data = await res.json()
        const s = data.status
        setCurrentStatus(s)
        addLog(`status: ${s}`, ['CONFIRMED','FAILED','INDETERMINATE'].includes(s) ? (s === 'CONFIRMED' ? 'success' : 'error') : 'status')
        if (['CONFIRMED','FAILED','INDETERMINATE'].includes(s)) {
          clearInterval(pollRef.current)
          setRunning(null)
          addLog(`done: ${s}`, s === 'CONFIRMED' ? 'success' : 'error')
        }
      } catch (e) {
        addLog('poll error: ' + e.message, 'error')
      }
    }, 2000)
  }

  const runScenario = async (scenario) => {
    setRunning(scenario.id)
    setLog([])
    setCurrentStatus(null)
    setActiveTxn(null)
    const txnID = `${scenario.id}-${Date.now()}`
    const holdAmount = scenario.holdAmount || scenario.amount

    try {
      addLog(`starting: ${scenario.label}`)

      addLog('creating payment hold...')
      const holdRes = await fetch(`${PAYSTABLE_URL}/api/v1/hold`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer test-admin-key' },
        body: JSON.stringify({
          txn_id: txnID, gateway: 'payu', amount: holdAmount, currency: 'INR',
          ttl_seconds: 300, callback_url: `${MERCHANT_URL}/callback`,
          metadata: { scenario: scenario.id },
        }),
      })
      const hold = await holdRes.json()
      if (!hold.read_token) {
        addLog('failed to create hold: ' + JSON.stringify(hold), 'error')
        setRunning(null)
        return
      }
      setActiveTxn({ txnID, token: hold.read_token })
      addLog(`hold created: ${txnID}`, 'success')

      addLog('scripting mock gateway...')
      await fetch(`${GATEWAY_URL}/script`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          txn_id: txnID, status: scenario.status, amount: scenario.amount,
          fail_until_s: scenario.failUntilS || 0,
          product_info: 'test-ticket', firstname: 'tester', email: 'test@paystable.dev',
        }),
      })
      addLog(`gateway scripted: ${scenario.status}${scenario.failUntilS ? ` (failing for ${scenario.failUntilS}s)` : ''}`)

      const webhookStatus = scenario.webhookStatus || 'success'
      const fires = scenario.duplicate ? 3 : 1
      for (let i = 0; i < fires; i++) {
        await fetch(`${GATEWAY_URL}/fire-webhook`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ txn_id: txnID, status: webhookStatus }),
        })
        addLog(`webhook fired${fires > 1 ? ` (${i+1}/${fires})` : ''}: ${webhookStatus}`)
      }

      addLog('polling for status...')
      pollStatus(txnID, hold.read_token)
    } catch (e) {
      addLog('error: ' + e.message, 'error')
      setRunning(null)
    }
  }

  const toggleMerchant = async () => {
    try {
      const res = await fetch(`${MERCHANT_URL}/toggle-offline`, { method: 'POST' })
      const data = await res.json()
      setMerchantOffline(data.state === 'offline')
      addLog(`merchant is now ${data.state}`, data.state === 'offline' ? 'error' : 'success')
    } catch (e) {
      addLog('could not reach mock merchant: ' + e.message, 'error')
    }
  }

  const statusColor = {
    CONFIRMED: 'text-status-green',
    FAILED: 'text-status-red',
    INDETERMINATE: 'text-status-yellow',
    VERIFYING: 'text-status-blue',
    PENDING: 'text-text-muted',
  }

  return (
    <div className="space-y-5">
      <div>
        <p className="text-sm text-text-muted">
          Runs against the mock gateway at{' '}
          <span className="font-mono text-text-secondary">{GATEWAY_URL}</span>.
          Start the test environment first: <span className="font-mono text-text-secondary">docker compose -f docker-compose.testkit.yml up</span>
        </p>
      </div>

      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-text-primary">Scenarios</h2>
        <button
          onClick={toggleMerchant}
          className={cn(
            'text-xs px-3 py-1.5 rounded border transition-colors',
            merchantOffline
              ? 'border-status-red/40 text-status-red bg-status-red/5 hover:bg-status-red/10'
              : 'border-bg-border text-text-secondary hover:text-text-primary'
          )}
        >
          {merchantOffline ? 'Merchant is offline — click to bring back' : 'Take merchant offline'}
        </button>
      </div>

      <div className="grid grid-cols-1 gap-2">
        {SCENARIOS.map(s => (
          <div key={s.id} className={cn(
            'bg-bg-surface border border-bg-border rounded-xl px-5 py-4 flex items-start justify-between gap-4',
            running === s.id && 'border-status-blue/30'
          )}>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-text-primary">{s.label}</p>
              <p className="text-xs text-text-muted mt-0.5">{s.description}</p>
            </div>
            <button
              onClick={() => runScenario(s)}
              disabled={!!running}
              className={cn(
                'flex-shrink-0 text-xs px-4 py-2 rounded-lg border transition-colors',
                running === s.id
                  ? 'border-status-blue/40 text-status-blue bg-status-blue/5'
                  : running
                  ? 'border-bg-border text-text-muted cursor-not-allowed'
                  : 'border-bg-border text-text-secondary hover:text-text-primary hover:border-text-muted'
              )}
            >
              {running === s.id ? 'Running...' : 'Run'}
            </button>
          </div>
        ))}
      </div>

      {(log.length > 0 || currentStatus) && (
        <div className="bg-bg-surface border border-bg-border rounded-xl overflow-hidden">
          {currentStatus && (
            <div className="px-5 py-3 border-b border-bg-border flex items-center gap-3">
              <span className="text-xs text-text-muted">Current status:</span>
              <span className={cn('text-sm font-mono font-medium', statusColor[currentStatus] || 'text-text-primary')}>
                {currentStatus}
              </span>
              {activeTxn && (
                <span className="text-xs text-text-muted font-mono ml-auto">{activeTxn.txnID}</span>
              )}
            </div>
          )}
          <div ref={logRef} className="px-5 py-3 space-y-0.5 max-h-64 overflow-y-auto">
            {log.map((line, i) => <LogLine key={i} line={line} />)}
            {running && <div className="text-xs text-text-muted font-mono animate-pulse">waiting...</div>}
          </div>
        </div>
      )}
    </div>
  )
}
