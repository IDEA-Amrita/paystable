import { cn } from '../../lib/utils'
import { ChevronUp, ChevronDown } from 'lucide-react'
import { useState } from 'react'

function SkeletonRows({ columns, count = 3 }) {
  return Array.from({ length: count }).map((_, i) => (
    <tr key={i} className="border-b border-bg-border">
      {columns.map((_, j) => (
        <td key={j} className="px-4 py-3">
          <div className="h-4 bg-bg-elevated/50 animate-pulse rounded w-3/4" />
        </td>
      ))}
    </tr>
  ))
}

export default function DataTable({
  columns,
  data,
  onRowClick,
  loading,
  emptyMessage = 'No data found',
  emptyIcon: EmptyIcon,
  selectedId,
  rowClassName,
}) {
  const [sortKey, setSortKey] = useState(null)
  const [sortDir, setSortDir] = useState('asc')

  const handleSort = (key) => {
    if (!key) return
    if (sortKey === key) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  let sortedData = data || []
  if (sortKey && sortedData.length > 0) {
    sortedData = [...sortedData].sort((a, b) => {
      const aVal = a[sortKey]
      const bVal = b[sortKey]
      if (aVal == null && bVal == null) return 0
      if (aVal == null) return 1
      if (bVal == null) return -1
      if (typeof aVal === 'string') {
        return sortDir === 'asc' ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal)
      }
      return sortDir === 'asc' ? aVal - bVal : bVal - aVal
    })
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr className="bg-bg-surface border-b border-bg-border">
            {columns.map((col) => (
              <th
                key={col.key}
                className={cn(
                  'text-text-secondary text-xs uppercase tracking-widest py-3 px-4 text-left font-medium',
                  col.sortable && 'cursor-pointer select-none hover:text-text-primary transition-colors duration-150',
                  col.align === 'right' && 'text-right'
                )}
                onClick={() => col.sortable && handleSort(col.key)}
              >
                <span className="inline-flex items-center gap-1">
                  {col.label}
                  {col.sortable && sortKey === col.key && (
                    sortDir === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <SkeletonRows columns={columns} />
          ) : sortedData.length === 0 ? (
            <tr>
              <td colSpan={columns.length} className="text-center py-16">
                {EmptyIcon && <EmptyIcon size={40} className="text-text-muted mx-auto mb-3" />}
                <p className="text-sm text-text-secondary">{emptyMessage}</p>
              </td>
            </tr>
          ) : (
            sortedData.map((row, i) => (
              <tr
                key={row.id || row.txn_id || i}
                className={cn(
                  'border-b border-bg-border transition-colors duration-100',
                  onRowClick && 'cursor-pointer hover:bg-bg-elevated',
                  selectedId && (row.id === selectedId || row.txn_id === selectedId) &&
                    'bg-bg-elevated border-l-2 border-l-neon-green',
                  typeof rowClassName === 'function' ? rowClassName(row) : rowClassName
                )}
                onClick={() => onRowClick?.(row)}
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={cn(
                      'px-4 py-3 text-sm text-text-primary',
                      col.align === 'right' && 'text-right',
                      col.mono && 'font-mono'
                    )}
                  >
                    {col.render ? col.render(row[col.key], row) : row[col.key]}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
