// Realistic mock data for all Paystable dashboard endpoints
// Every screen works without the Go server

const NOW = new Date()
const ago = (ms) => new Date(NOW.getTime() - ms).toISOString()
const hoursAgo = (h) => ago(h * 3600000)
const minsAgo = (m) => ago(m * 60000)
const secsAgo = (s) => ago(s * 1000)
const daysAgo = (d) => ago(d * 86400000)

// --- Seeded random for consistent-looking data ---
let _seed = 42
function seededRandom() {
  _seed = (_seed * 16807 + 0) % 2147483647
  return (_seed & 0x7fffffff) / 0x7fffffff
}

function randomId() {
  return Array.from({ length: 16 }, () => '0123456789abcdef'[Math.floor(seededRandom() * 16)]).join('')
}

// --- Transactions ---
const GATEWAYS = ['PayU']
const STATUSES = ['PENDING', 'VERIFYING', 'CONFIRMED', 'FAILED', 'INDETERMINATE', 'MISMATCH', 'REFUNDED']
const STATUS_WEIGHTS = [0.05, 0.08, 0.55, 0.16, 0.05, 0.03, 0.08]

function pickWeighted(items, weights) {
  const r = seededRandom()
  let cum = 0
  for (let i = 0; i < items.length; i++) {
    cum += weights[i]
    if (r <= cum) return items[i]
  }
  return items[items.length - 1]
}

const AMOUNTS = [9900, 14900, 24900, 49900, 99900, 199900, 499900, 999900, 1499900, 2499900]

function generateTransactions(count = 80) {
  const txns = []
  for (let i = 0; i < count; i++) {
    const id = `TXN-${randomId().slice(0, 12)}`
    const status = pickWeighted(STATUSES, STATUS_WEIGHTS)
    const gateway = GATEWAYS[Math.floor(seededRandom() * GATEWAYS.length)]
    const amount = AMOUNTS[Math.floor(seededRandom() * AMOUNTS.length)]
    const createdAt = hoursAgo(seededRandom() * 48)
    const resolveMs = status === 'PENDING' || status === 'VERIFYING'
      ? null
      : Math.floor(seededRandom() * 360000) + 3000 // 3s to 6min
    
    txns.push({
      id,
      txn_id: id,
      gateway,
      status,
      amount,
      created_at: createdAt,
      resolved_at: resolveMs ? new Date(new Date(createdAt).getTime() + resolveMs).toISOString() : null,
      resolve_duration_ms: resolveMs,
      merchant_order_id: `ORD-${randomId().slice(0, 8)}`,
      callback_url: `https://merchant-${Math.floor(seededRandom() * 3) + 1}.app/webhooks/paystable`,
    })
  }
  return txns.sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
}

const ALL_TRANSACTIONS = generateTransactions(80)

// --- Transaction Detail (ledger events for timeline) ---
function generateLedgerEvents(txn) {
  const events = []
  const baseTime = new Date(txn.created_at)
  let t = baseTime.getTime()

  events.push({
    type: 'hold_created',
    timestamp: new Date(t).toISOString(),
    source: 'API',
    detail: `Hold created for ${txn.gateway}`,
    data: { amount: txn.amount, gateway: txn.gateway, merchant_order_id: txn.merchant_order_id }
  })

  t += 2000 + Math.floor(seededRandom() * 5000) // 2-7s later
  const webhookClaim = txn.status === 'CONFIRMED' && seededRandom() > 0.7 ? 'payment.failed' : 
                       txn.status === 'FAILED' ? 'payment.failed' : 'payment.success'
  events.push({
    type: 'webhook_received',
    timestamp: new Date(t).toISOString(),
    source: txn.gateway,
    detail: `${webhookClaim} (${(txn.amount / 100).toFixed(2)})`,
    data: {
      event: webhookClaim,
      gateway: txn.gateway,
      payload_hash: randomId(),
      raw_status: webhookClaim === 'payment.failed' ? 'failed' : 'captured',
    }
  })

  if (txn.status !== 'PENDING') {
    const pollCount = txn.status === 'INDETERMINATE' ? 8 : Math.floor(seededRandom() * 3) + 2
    for (let p = 0; p < pollCount; p++) {
      t += 5000 + Math.floor(seededRandom() * 15000) // 5-20s between polls
      const pollSuccess = txn.status === 'CONFIRMED' || (txn.status === 'INDETERMINATE' && seededRandom() > 0.5)
      events.push({
        type: 'poll_completed',
        timestamp: new Date(t).toISOString(),
        source: 'stabilizer',
        detail: `${pollSuccess ? 'success' : 'failed'} · ₹${(txn.amount / 100).toFixed(2)}`,
        attempt: p + 1,
        data: {
          gateway_status: pollSuccess ? 'captured' : 'failed',
          amount: pollSuccess ? txn.amount : 0,
          attempt: p + 1,
        }
      })
    }

    if (txn.status !== 'VERIFYING') {
      t += 1000
      const prevState = 'VERIFYING'
      events.push({
        type: 'state_transition',
        timestamp: new Date(t).toISOString(),
        source: 'stabilizer',
        detail: `${prevState} → ${txn.status}`,
        data: { from: prevState, to: txn.status }
      })
    }

    if (txn.status === 'CONFIRMED' || txn.status === 'FAILED') {
      t += 500 + Math.floor(seededRandom() * 2000)
      events.push({
        type: 'callback_delivered',
        timestamp: new Date(t).toISOString(),
        source: 'delivery',
        detail: `200 OK · POST ${txn.callback_url}`,
        data: { status_code: 200, url: txn.callback_url, method: 'POST' }
      })
    }
  }

  return events
}

function generatePolls(txn, events) {
  return events
    .filter(e => e.type === 'poll_completed')
    .map((e, i, arr) => ({
      attempt: e.attempt,
      timestamp: e.timestamp,
      gateway_response: e.data.gateway_status === 'captured' ? 'success' : 'failure',
      amount: e.data.amount,
      delta_from_prev: i === 0 ? null : new Date(e.timestamp) - new Date(arr[i - 1].timestamp),
    }))
}

// --- Overview Stats ---
const overviewStats = {
  active_holds: { value: 24, status: 'normal' },
  pending_deliveries: { value: 12, status: 'normal' },
  exhausted_deliveries: { value: 3, status: 'warning' },
  rejected_webhooks: { value: 1, subtext: '/ 1hr', status: 'normal' },
}

// --- Volume Chart (24h) ---
function generateVolumeChart() {
  const hours = []
  for (let h = 23; h >= 0; h--) {
    const hour = new Date(NOW.getTime() - h * 3600000)
    const label = hour.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', hour12: false })
    const base = 15 + Math.floor(seededRandom() * 35)
    const confirmed = Math.floor(base * (0.5 + seededRandom() * 0.3))
    const failed = Math.floor(base * (0.1 + seededRandom() * 0.15))
    const indeterminate = Math.max(0, base - confirmed - failed)
    hours.push({ hour: label, confirmed, failed, indeterminate })
  }
  return hours
}

// --- Mismatch Rate Chart (7d) ---
function generateMismatchRateChart() {
  const points = []
  for (let d = 13; d >= 0; d--) {
    const date = new Date(NOW.getTime() - d * 86400000)
    const label = date.toLocaleDateString('en-IN', { month: 'short', day: 'numeric' })
    const rate = Math.max(0, 2 + seededRandom() * 8 - (13 - d) * 0.3)
    const count = Math.floor(rate * (3 + seededRandom() * 2))
    points.push({ date: label, rate: Math.round(rate * 10) / 10, count })
  }
  return points
}

// --- Ledger Feed ---
function generateLedgerFeed() {
  const eventTypes = ['state_transition', 'webhook_received', 'hold_created', 'poll_completed', 'callback_delivered']
  const feed = []

  for (let i = 0; i < 10; i++) {
    const type = eventTypes[Math.floor(seededRandom() * eventTypes.length)]
    const actor = type === 'webhook_received' ? GATEWAYS[Math.floor(seededRandom() * GATEWAYS.length)] :
                  type === 'hold_created' ? 'API' :
                  type === 'callback_delivered' ? 'delivery' : 'stabilizer'
    let detail
    
    if (type === 'state_transition') {
      const from = ['VERIFYING', 'PENDING'][Math.floor(seededRandom() * 2)]
      const to = ['CONFIRMED', 'FAILED'][Math.floor(seededRandom() * 2)]
      detail = `${from} → ${to}`
    } else if (type === 'webhook_received') {
      const evt = seededRandom() > 0.5 ? 'payment.success' : 'payment.failed'
      const amt = AMOUNTS[Math.floor(seededRandom() * AMOUNTS.length)]
      detail = `${evt} (₹${(amt / 100).toFixed(0)})`
    } else if (type === 'hold_created') {
      detail = `TXN-${randomId().slice(0, 7)}`
    } else if (type === 'poll_completed') {
      detail = seededRandom() > 0.3 ? 'success · ₹499' : 'failed'
    } else {
      detail = '200 OK'
    }

    feed.push({
      timestamp: secsAgo(i * 5 + Math.floor(seededRandom() * 3)),
      event_type: type,
      actor,
      detail,
    })
  }

  return feed
}

// --- Mismatches ---
function generateMismatches() {
  const mismatches = []
  const claimedOptions = ['payment.failed', 'payment.pending', 'payment.expired']
  const truthOptions = ['payment.success', 'payment.captured']

  for (let i = 0; i < 23; i++) {
    const gateway = GATEWAYS[Math.floor(seededRandom() * GATEWAYS.length)]
    const amount = AMOUNTS[Math.floor(seededRandom() * AMOUNTS.length)]
    const timeSavedMs = Math.floor(seededRandom() * 720000) + 3000

    mismatches.push({
      txn_id: `TXN-${randomId().slice(0, 12)}`,
      gateway,
      webhook_claimed: claimedOptions[Math.floor(seededRandom() * claimedOptions.length)],
      verified_truth: truthOptions[Math.floor(seededRandom() * truthOptions.length)],
      detected_at: hoursAgo(seededRandom() * 168), // last 7 days
      time_saved_ms: timeSavedMs,
      amount,
    })
  }

  return mismatches.sort((a, b) => new Date(b.detected_at) - new Date(a.detected_at))
}

const ALL_MISMATCHES = generateMismatches()

const mismatchStats = {
  last_7_days: ALL_MISMATCHES.length,
  rate_7d: 4.7,
  amount_mismatches: 3,
  trend: Array.from({ length: 14 }, () => Math.floor(seededRandom() * 8) + 1),
  rate_trend: Array.from({ length: 14 }, () => Math.round((seededRandom() * 6 + 1) * 10) / 10),
  per_gateway: {
    PayU: { total: 489, mismatches: 23, rate: 4.7 },
  }
}

const amountMismatches = [
  {
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    gateway: 'PayU',
    hold_amount: 49900,
    gateway_reported: 44900,
    difference: 5000,
    detected_at: hoursAgo(6),
  },
  {
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    gateway: 'PayU',
    hold_amount: 99900,
    gateway_reported: 89900,
    difference: 10000,
    detected_at: hoursAgo(18),
  },
  {
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    gateway: 'PayU',
    hold_amount: 14900,
    gateway_reported: 12400,
    difference: 2500,
    detected_at: daysAgo(2),
  },
]

// --- Delivery ---
const exhaustedDeliveries = [
  {
    id: 'del-001',
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    event_type: 'state_transition',
    attempts: 8,
    max_attempts: 8,
    last_error: 'HTTP 502 Bad Gateway: upstream connect error or disconnect/reset before headers',
    last_attempt_at: minsAgo(12),
    callback_url: 'https://merchant-app.example.com/webhooks/paystable/v2/handler',
    status: 'exhausted',
  },
  {
    id: 'del-002',
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    event_type: 'callback_delivered',
    attempts: 8,
    max_attempts: 8,
    last_error: 'connection refused: dial tcp 10.0.2.15:443: connect: connection refused',
    last_attempt_at: minsAgo(45),
    callback_url: 'https://payments.startup.io/hooks/paystable',
    status: 'exhausted',
  },
  {
    id: 'del-003',
    txn_id: `TXN-${randomId().slice(0, 12)}`,
    event_type: 'state_transition',
    attempts: 8,
    max_attempts: 8,
    last_error: 'HTTP 500 Internal Server Error: {"error":"database_unavailable"}',
    last_attempt_at: hoursAgo(2),
    callback_url: 'https://api.bigcorp.in/v1/payment-callbacks',
    status: 'exhausted',
  },
]

const deliveryStats = {
  delivered_today: 847,
  pending: 12,
  exhausted: 3,
  latency_histogram: [
    { bucket: '< 1s', count: 312 },
    { bucket: '1–2s', count: 245 },
    { bucket: '2–5s', count: 178 },
    { bucket: '5–15s', count: 67 },
    { bucket: '15–60s', count: 34 },
    { bucket: '> 60s', count: 11 },
  ],
}

// --- Config ---
const runtimeConfig = [
  { key: 'DATABASE_URL', value: null, is_secret: true, is_set: true },
  { key: 'GATEWAY', value: 'PayU', is_secret: false, is_set: true },
  { key: 'PAYU_MERCHANT_KEY', value: null, is_secret: true, is_set: true },
  { key: 'PAYU_SALT', value: null, is_secret: true, is_set: true },
  { key: 'DASHBOARD_ENABLED', value: 'true', is_secret: false, is_set: true },
  { key: 'DASHBOARD_PORT', value: '8080', is_secret: false, is_set: true },
  { key: 'STABILIZER_MIN_POLLS', value: '3', is_secret: false, is_set: true },
  { key: 'STABILIZER_BACKOFF_BASE', value: '5s', is_secret: false, is_set: true },
  { key: 'DELIVERY_MAX_ATTEMPTS', value: '8', is_secret: false, is_set: true },
  { key: 'DELIVERY_BACKOFF_BASE', value: '30s', is_secret: false, is_set: true },
  { key: 'CALLBACK_TIMEOUT', value: '10s', is_secret: false, is_set: true },
  { key: 'WEBHOOK_SECRET', value: null, is_secret: true, is_set: true },
  { key: 'API_TOKEN', value: null, is_secret: true, is_set: false },
]

const rotationStatus = {
  is_active: true,
  last_rotated_at: daysAgo(14),
  window_hours: 24,
  window_ends_at: new Date(NOW.getTime() + 18 * 3600000).toISOString(),
  hours_remaining: 18,
  old_key_set: true,
  new_key_set: true,
}

// --- Raw payloads for JSON viewer ---
const sampleWebhookPayload = {
  event: 'payment.failed',
  payload: {
    payment: {
      entity: {
        id: 'pay_FJ3QHBqwfiY8IJ',
        amount: 49900,
        currency: 'INR',
        status: 'failed',
        method: 'upi',
        description: 'Order #ORD-a8f3c21e',
        bank: null,
        wallet: null,
        vpa: 'user@okhdfcbank',
        email: 'user@example.com',
        contact: '+919876543210',
        fee: 0,
        tax: 0,
        error_code: 'BAD_REQUEST_ERROR',
        error_description: 'Payment was not completed on time.',
        created_at: 1718870700,
      }
    }
  },
  created_at: 1718870703,
}

const sampleVerificationResponse = {
  status: 1,
  result: {
    mihpayid: '18274635287',
    mode: 'UPI',
    status: 'success',
    unmappedstatus: 'captured',
    key: 'MERCHANT_KEY',
    txnid: 'TXN-8f3a2c1d9e04',
    amount: '499.00',
    cardCategory: '',
    discount: '0.00',
    net_amount_debit: '499.00',
    addedon: '2025-06-20 15:05:00',
    productinfo: 'Order',
    firstname: 'Customer',
    email: 'user@example.com',
    phone: '9876543210',
    udf1: '',
    udf2: '',
    udf3: '',
    udf4: '',
    udf5: '',
    field2: '',
    field9: 'Transaction Successful',
    error_code: 'E000',
    bank_ref_num: '418735629134',
  }
}

// --- Export mock API handlers ---
export const mockApi = {
  '/overview/stats': () => overviewStats,
  '/ledger/recent': () => generateLedgerFeed(),
  '/overview/volume': () => generateVolumeChart(),
  '/overview/mismatch-rate': () => generateMismatchRateChart(),

  '/transactions': (params = {}) => {
    let filtered = [...ALL_TRANSACTIONS]
    if (params.status && params.status !== 'all') {
      filtered = filtered.filter(t => t.status === params.status)
    }
    if (params.search) {
      const s = params.search.toLowerCase()
      filtered = filtered.filter(t =>
        t.txn_id.toLowerCase().includes(s) ||
        t.gateway.toLowerCase().includes(s) ||
        String(t.amount).includes(s)
      )
    }
    const page = parseInt(params.page || '1')
    const limit = parseInt(params.limit || '25')
    const start = (page - 1) * limit

    const statusCounts = {}
    ALL_TRANSACTIONS.forEach(t => {
      statusCounts[t.status] = (statusCounts[t.status] || 0) + 1
    })

    return {
      data: filtered.slice(start, start + limit),
      total: filtered.length,
      page,
      limit,
      status_counts: statusCounts,
    }
  },

  '/transactions/:id': (id) => {
    const txn = ALL_TRANSACTIONS.find(t => t.txn_id === id) || ALL_TRANSACTIONS[0]
    const events = generateLedgerEvents(txn)
    const polls = generatePolls(txn, events)
    return {
      ...txn,
      events,
      polls,
      raw_webhook: sampleWebhookPayload,
      raw_verification: sampleVerificationResponse,
    }
  },

  '/mismatches': (params = {}) => {
    let filtered = [...ALL_MISMATCHES]
    if (params.gateway) {
      filtered = filtered.filter(m => m.gateway === params.gateway)
    }
    const page = parseInt(params.page || '1')
    const limit = parseInt(params.limit || '25')
    const start = (page - 1) * limit
    return {
      data: filtered.slice(start, start + limit),
      total: filtered.length,
      page,
      limit,
    }
  },

  '/mismatches/stats': () => ({
    ...mismatchStats,
    amount_mismatches_data: amountMismatches,
  }),

  '/deliveries': (params = {}) => {
    let data = exhaustedDeliveries
    if (params.status === 'exhausted') {
      data = exhaustedDeliveries.filter(d => d.status === 'exhausted')
    }
    return { data, total: data.length }
  },

  '/deliveries/stats': () => deliveryStats,

  '/deliveries/:id/replay': () => ({ success: true, message: 'Delivery queued for replay' }),

  '/config': () => runtimeConfig,

  '/config/rotation-status': () => rotationStatus,

  '/config/rotate-secret': () => ({ success: true, message: 'Secret rotation initiated' }),
}
