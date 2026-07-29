import { NavLink, Outlet } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { navigation } from '@/router'

export function AppLayout() {
  return (
    <div className="flex min-h-svh bg-background text-foreground">
      <aside className="hidden w-56 shrink-0 border-r p-4 md:block">
        <div className="mb-6 px-2">
          <p className="font-heading text-lg font-semibold">ditalk</p>
          <p className="text-xs text-muted-foreground">Analisis percakapan pribadi</p>
        </div>
        <nav className="flex flex-col gap-1">
          {navigation.map(({ path, label }) => (
            <NavLink
              key={path}
              to={path}
              end={path === '/'}
              className={({ isActive }) =>
                cn(
                  'rounded-md px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-accent font-medium text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
                )
              }
            >
              {label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <main className="min-w-0 flex-1 p-6">
        <Outlet />
      </main>
    </div>
  )
}
