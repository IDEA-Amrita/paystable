import { useState } from 'react'
import { ChevronRight, ChevronDown, Copy, Check } from 'lucide-react'
import { cn } from '../../lib/utils'

function tokenize(jsonString) {
  const tokens = []
  const regex = /("(?:[^"\\]|\\.)*")\s*:/g
  let lastIndex = 0
  let match

  // Simple tokenizer: split JSON into colored segments
  const lines = jsonString.split('\n')
  
  return lines.map((line, lineIdx) => {
    const parts = []
    let remaining = line
    let key = 0

    // Match key: value patterns
    const keyMatch = remaining.match(/^(\s*)"([^"]+)"(:)/)
    if (keyMatch) {
      parts.push(<span key={key++} className="text-text-muted">{keyMatch[1]}</span>)
      parts.push(<span key={key++} className="text-neon-blue">&quot;{keyMatch[2]}&quot;</span>)
      parts.push(<span key={key++} className="text-text-muted">:</span>)
      remaining = remaining.slice(keyMatch[0].length)
    }

    // Process the value part
    if (remaining) {
      // String values
      remaining = remaining.replace(/"([^"]*?)"/g, (match, str) => {
        parts.push(<span key={key++} className="text-neon-green">&quot;{str}&quot;</span>)
        return ''
      })

      // Number values
      remaining = remaining.replace(/\b(\d+\.?\d*)\b/g, (match, num) => {
        parts.push(<span key={key++} className="text-neon-yellow">{num}</span>)
        return ''
      })

      // Boolean / null
      remaining = remaining.replace(/\b(true|false|null)\b/g, (match, val) => {
        parts.push(<span key={key++} className="text-neon-purple">{val}</span>)
        return ''
      })

      // Remaining punctuation
      if (remaining.trim()) {
        parts.push(<span key={key++} className="text-text-muted">{remaining}</span>)
      }
    }

    return (
      <div key={lineIdx} className="leading-5">
        {parts.length > 0 ? parts : <span>{line}</span>}
      </div>
    )
  })
}

export default function JsonBlock({ title, data, defaultOpen = false }) {
  const [isOpen, setIsOpen] = useState(defaultOpen)
  const [copied, setCopied] = useState(false)

  const jsonString = typeof data === 'string' ? data : JSON.stringify(data, null, 2)

  const handleCopy = async (e) => {
    e.stopPropagation()
    try {
      await navigator.clipboard.writeText(jsonString)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Fallback
      const textarea = document.createElement('textarea')
      textarea.value = jsonString
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  return (
    <div className="border border-bg-border rounded-xl overflow-hidden">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center justify-between px-4 py-3 bg-bg-muted
                   hover:bg-bg-elevated transition-colors duration-150 text-left"
      >
        <span className="flex items-center gap-2 text-sm font-medium text-text-secondary">
          {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          {title}
        </span>
        {isOpen && (
          <button
            onClick={handleCopy}
            className="flex items-center gap-1.5 text-xs text-text-muted hover:text-text-primary
                       px-2 py-1 rounded border border-bg-border hover:border-text-muted
                       transition-all duration-150"
          >
            {copied ? (
              <>
                <Check size={12} className="text-neon-green" />
                <span className="text-neon-green">Copied!</span>
              </>
            ) : (
              <>
                <Copy size={12} />
                Copy
              </>
            )}
          </button>
        )}
      </button>

      <div
        className={cn(
          'overflow-hidden transition-all duration-200',
          isOpen ? 'max-h-[300px]' : 'max-h-0'
        )}
      >
        <pre className="bg-bg-base font-mono text-xs p-4 overflow-auto max-h-[300px]">
          <code>{tokenize(jsonString)}</code>
        </pre>
      </div>
    </div>
  )
}
