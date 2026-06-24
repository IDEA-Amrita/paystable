export function formatCurrency(paise) {
  const rupees = paise / 100
  return `₹${rupees.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function formatRelativeTime(isoString) {
  const date = new Date(isoString)
  const now = new Date()
  const diffMs = now - date
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHr = Math.floor(diffMin / 60)
  const diffDays = Math.floor(diffHr / 24)

  if (diffSec < 10) return 'just now'
  if (diffSec < 60) return `${diffSec}s ago`
  if (diffMin < 60) return `${diffMin} min ago`
  if (diffHr < 24) return `${diffHr}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString('en-IN', { month: 'short', day: 'numeric' })
}

export function formatDuration(ms) {
  if (ms < 0) ms = 0
  const totalSec = Math.floor(ms / 1000)
  const hours = Math.floor(totalSec / 3600)
  const minutes = Math.floor((totalSec % 3600) / 60)
  const seconds = totalSec % 60

  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

export function formatTimestamp(isoString, mode = 'time') {
  const date = new Date(isoString)
  if (mode === 'time') {
    return date.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  }
  return date.toLocaleDateString('en-IN', { month: 'short', day: 'numeric' }) +
    ' at ' +
    date.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', hour12: false })
}

export function formatDate(isoString) {
  const date = new Date(isoString)
  return date.toLocaleDateString('en-IN', { year: 'numeric', month: 'long', day: 'numeric' }) +
    ' at ' +
    date.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', hour12: false })
}

export function cn(...classes) {
  return classes.filter(Boolean).join(' ')
}

// Two-layer vocabulary: plain label for the surface, raw enum stays available
// underneath for developers debugging against logs and the schema.
const STATUS_LABELS = {
  PENDING:       'Started',
  VERIFYING:     'Checking',
  CONFIRMED:     'Paid',
  FAILED:        'Failed',
  INDETERMINATE: 'Needs attention',
  MISMATCH:      'Amount mismatch',
  REFUNDED:      'Refunded',
}

const STATUS_MEANINGS = {
  PENDING:       'Customer started checkout. No result yet.',
  VERIFYING:     'Confirming with the bank before we trust the result.',
  CONFIRMED:     'Payment went through. The customer was charged.',
  FAILED:        'Payment did not go through. Safe to let the customer retry.',
  INDETERMINATE: 'Something looked wrong (often a mismatched amount). A human should check this.',
  MISMATCH:      'The gateway amount does not match the hold amount.',
  REFUNDED:      'Payment was reversed.',
}

export function statusLabel(status) {
  return STATUS_LABELS[status] || status
}

export function statusMeaning(status) {
  return STATUS_MEANINGS[status] || ''
}

export function truncate(str, len = 12) {
  if (!str) return ''
  if (str.length <= len) return str
  return str.slice(0, len) + '…'
}

export function downloadCSV(headers, rows, filename) {
  const csvContent = [
    headers.join(','),
    ...rows.map(row => row.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(','))
  ].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = filename
  link.click()
  URL.revokeObjectURL(link.href)
}
