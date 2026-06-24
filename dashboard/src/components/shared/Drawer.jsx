import { useEffect, useCallback } from 'react'
import { X } from 'lucide-react'
import { cn } from '../../lib/utils'

export default function Drawer({ open, onClose, children }) {
  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Escape' && open) onClose()
  }, [open, onClose])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
    return () => { document.body.style.overflow = '' }
  }, [open])

  return (
    <>
      {/* Overlay */}
      <div
        className={cn(
          'fixed inset-0 bg-black/50 z-40 transition-opacity duration-200',
          open ? 'opacity-100' : 'opacity-0 pointer-events-none'
        )}
        onClick={onClose}
      />

      {/* Drawer Panel */}
      <div
        className={cn(
          'fixed top-0 right-0 h-full w-[480px] max-w-full bg-bg-surface shadow-drawer z-50',
          'transition-transform duration-200 ease-out',
          'rounded-l-xl overflow-y-auto',
          open ? 'translate-x-0' : 'translate-x-full'
        )}
      >
        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-4 right-4 p-1.5 text-text-muted hover:text-text-primary
                     hover:bg-bg-elevated rounded-lg transition-all duration-150 z-10"
          aria-label="Close drawer"
        >
          <X size={18} />
        </button>

        {children}
      </div>
    </>
  )
}
