import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { Menu, MessageSquareLock } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { ConnectionBadge } from '@/components/connection-badge'
import { ThemeToggle } from '@/components/theme-toggle'
import { cn } from '@/lib/utils'
import { findNavItem, navGroups } from '@/lib/nav'

function Brand() {
  return (
    <div className="flex items-center gap-2.5">
      <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground">
        <MessageSquareLock className="size-4" />
      </span>
      <span className="min-w-0">
        <span className="block font-heading text-sm font-semibold leading-tight">
          ditalk
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          Analisis percakapan pribadi
        </span>
      </span>
    </div>
  )
}

function NavList({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <nav className="space-y-6">
      {navGroups.map((group) => (
        <div key={group.label}>
          <p className="mb-2 px-3 text-[0.6875rem] font-semibold uppercase tracking-wider text-muted-foreground/80">
            {group.label}
          </p>
          <ul className="space-y-0.5">
            {group.items.map(({ path, label, icon: Icon }) => (
              <li key={path}>
                <NavLink
                  to={path}
                  end={path === '/'}
                  onClick={onNavigate}
                  className={({ isActive }) =>
                    cn(
                      'flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors',
                      'focus-visible:ring-ring/50 outline-none focus-visible:ring-2',
                      isActive
                        ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
                        : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground',
                    )
                  }
                >
                  {({ isActive }) => (
                    <>
                      <Icon
                        className={cn(
                          'size-4 shrink-0',
                          isActive ? 'text-primary' : 'text-muted-foreground',
                        )}
                      />
                      <span className="truncate">{label}</span>
                    </>
                  )}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  )
}

export function AppLayout() {
  const location = useLocation()
  const [mobileOpen, setMobileOpen] = useState(false)
  const current = findNavItem(location.pathname)

  // Navigating should close the drawer even when the click came from a link
  // inside the page rather than from the nav list.
  useEffect(() => setMobileOpen(false), [location.pathname])

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r bg-sidebar lg:flex">
        <div className="flex h-14 items-center border-b px-4">
          <Brand />
        </div>
        <div className="flex-1 overflow-y-auto px-3 py-5">
          <NavList />
        </div>
        <div className="border-t px-4 py-3">
          <p className="text-[0.6875rem] leading-relaxed text-muted-foreground">
            Read-only. Tidak mengirim pesan, tidak membalas otomatis.
          </p>
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b bg-background/80 px-4 backdrop-blur-sm sm:px-6">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon" className="lg:hidden" aria-label="Buka menu">
                <Menu className="size-4" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-72 p-0">
              <SheetTitle className="sr-only">Navigasi</SheetTitle>
              <div className="flex h-14 items-center border-b px-4">
                <Brand />
              </div>
              <div className="overflow-y-auto px-3 py-5">
                <NavList onNavigate={() => setMobileOpen(false)} />
              </div>
            </SheetContent>
          </Sheet>

          <p className="min-w-0 flex-1 truncate text-sm font-medium">
            {current?.title ?? current?.label ?? 'ditalk'}
          </p>

          <div className="flex items-center gap-2">
            <ConnectionBadge />
            <ThemeToggle />
          </div>
        </header>

        <main>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
