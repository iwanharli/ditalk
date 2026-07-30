import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/**
 * One container for every page, so padding and measure never drift between
 * screens. Pages must not set their own max-width or outer padding.
 *
 * The cap is deliberately generous: this is a dense analysis tool with wide
 * evidence tables and side-by-side panels, not an article. Prose blocks limit
 * their own measure with max-w-* instead of the whole page being narrowed.
 */
export function PageContainer({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6', className)}>
      {children}
    </div>
  )
}

/** Consistent title block. `actions` sits inline on wide screens, below on narrow. */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 space-y-1.5">
        <h1 className="font-heading text-2xl font-semibold tracking-tight text-balance">
          {title}
        </h1>
        {description && (
          <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </header>
  )
}

/** Vertical rhythm between blocks within a page. */
export function PageSections({ children }: { children: ReactNode }) {
  return <div className="space-y-6">{children}</div>
}
