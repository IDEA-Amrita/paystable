import { NavLink } from 'react-router-dom'
import { Hexagon, LayoutDashboard, ArrowLeftRight, Zap, Send, Settings } from 'lucide-react'
import { cn } from '../../lib/utils'

const navItems = [
  { to: '/overview',     label: 'Overview',      icon: LayoutDashboard },
  { to: '/transactions', label: 'Transactions',  icon: ArrowLeftRight },
  { to: '/mismatches',   label: 'Mismatches',    icon: Zap },
  { to: '/delivery',     label: 'Delivery',      icon: Send },
  { to: '/config',       label: 'Config',        icon: Settings },
]

export default function Sidebar() {
  return (
    <aside className="fixed left-0 top-0 h-screen w-[220px] bg-bg-surface border-r border-bg-border
                      flex flex-col z-30">
      {/* Logo Section */}
      <div className="px-5 pt-5 pb-4 border-b border-bg-border">
        <div className="flex items-center gap-2.5">
          <Hexagon size={22} className="text-accent-brand" strokeWidth={2} />
          <span className="text-text-primary font-semibold text-base tracking-tight">paystable</span>
        </div>
        <span className="text-xs text-text-muted font-mono mt-1 block pl-[34px]">v0.1.0</span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 py-3 px-2">
        <ul className="space-y-0.5">
          {navItems.map(({ to, label, icon: Icon }) => (
            <li key={to}>
              <NavLink
                to={to}
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-3 px-4 py-2.5 rounded-lg text-sm font-medium',
                    'transition-all duration-150',
                    isActive
                      ? 'text-neon-green bg-accent-glow border-r-2 border-neon-green'
                      : 'text-text-secondary hover:text-text-primary hover:bg-bg-elevated'
                  )
                }
              >
                <Icon size={16} />
                {label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>

      {/* Live Indicator */}
      <div className="px-5 py-4 border-t border-bg-border">
        <div className="flex items-center gap-2">
          <span className="relative flex h-2.5 w-2.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-green opacity-75" />
            <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-neon-green" />
          </span>
          <span className="text-xs font-mono text-neon-green font-medium">LIVE</span>
        </div>
        <span className="text-xs text-text-muted mt-0.5 block pl-[18px]">Connected</span>
      </div>
    </aside>
  )
}
