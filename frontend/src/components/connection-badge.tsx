import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'

import { cn } from '@/lib/utils'
import { getWAStatus, statusLabels, type WAStatus } from '@/lib/wa'

// Colour carries meaning here, so the label is always present too: a dot alone
// would be unreadable for anyone who cannot distinguish the hues.
const dotClass: Record<WAStatus, string> = {
  connected: 'bg-emerald-500',
  pairing: 'bg-amber-500',
  connecting: 'bg-amber-500',
  disconnected: 'bg-muted-foreground/50',
  logged_out: 'bg-muted-foreground/50',
  error: 'bg-destructive',
}

/**
 * Persistent link state in the topbar. Whether the machine is currently reading
 * anything is the single most important piece of context in this app, so it
 * stays visible on every page rather than only on the pairing screen.
 */
export function ConnectionBadge() {
  const { data, isPending } = useQuery({
    queryKey: ['wa', 'status'],
    queryFn: getWAStatus,
    refetchInterval: 10_000,
  })

  if (isPending || !data) {
    return (
      <span className="hidden items-center gap-2 text-xs text-muted-foreground sm:inline-flex">
        <span className="size-1.5 animate-pulse rounded-full bg-muted-foreground/40" />
        Memuat status
      </span>
    )
  }

  const { status } = data.connection

  return (
    <Link
      to="/pairing"
      className="inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-xs transition-colors hover:bg-accent"
      title={`${data.allowlist_active} nomor aktif dibaca`}
    >
      <span className={cn('size-1.5 shrink-0 rounded-full', dotClass[status])} />
      <span className="font-medium">{statusLabels[status]}</span>
      <span className="hidden text-muted-foreground sm:inline">
        · {data.allowlist_active} nomor
      </span>
    </Link>
  )
}
