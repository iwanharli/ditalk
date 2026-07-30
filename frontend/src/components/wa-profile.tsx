import { CheckCircle2, Smartphone } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { id as idLocale } from 'date-fns/locale'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Separator } from '@/components/ui/separator'
import type { Connection } from '@/lib/wa'

/** Two letters from the display name, or a phone glyph when there is no name. */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

/**
 * Groups digits so a long number stays readable: 6281234567890 becomes
 * +62 812 3456 7890.
 *
 * Grouping runs right to left so the odd-sized group lands at the front, which
 * is how these numbers are normally written. Left to right would give the
 * lopsided "+62 8124 9442 476".
 *
 * Only Indonesian numbers are grouped. Country codes are 1 to 3 digits, so
 * assuming two would render +1 415 555 2671 as "+14 1 5555 2671" — worse than
 * no grouping, because this number is what the user checks to confirm they
 * linked the right account.
 */
export function formatPhone(phone: string): string {
  const ID_CC = '62'
  if (!phone.startsWith(ID_CC) || phone.length < 8) return `+${phone}`

  let rest = phone.slice(ID_CC.length)
  const groups: string[] = []

  while (rest.length > 4) {
    groups.unshift(rest.slice(-4))
    rest = rest.slice(0, -4)
  }
  groups.unshift(rest)

  return `+${ID_CC} ${groups.join(' ')}`
}

export function WAProfile({
  connection,
  allowlistActive,
}: {
  connection: Connection
  allowlistActive: number
}) {
  const name = connection.self_name?.trim() ?? ''
  const phone = connection.self_phone ?? ''
  const label = initials(name)

  const since = connection.last_connected_at
    ? formatDistanceToNow(new Date(connection.last_connected_at), {
        addSuffix: true,
        locale: idLocale,
      })
    : null

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Avatar className="size-14 border">
          {/* avatar_version busts the cache when the picture changes; the browser
              may keep the image for a few minutes otherwise. */}
          {connection.has_avatar && (
            <AvatarImage
              src={`/api/wa/avatar?v=${connection.avatar_version ?? ''}`}
              alt={name ? `Foto profil ${name}` : 'Foto profil akun tertaut'}
            />
          )}
          <AvatarFallback className="bg-primary/10 text-sm font-medium text-primary">
            {label || <Smartphone className="size-5" />}
          </AvatarFallback>
        </Avatar>

        <div className="min-w-0 space-y-0.5">
          {name ? (
            <p className="truncate font-medium leading-tight">{name}</p>
          ) : (
            <p className="truncate font-medium leading-tight text-muted-foreground">
              Tanpa nama profil
            </p>
          )}
          {phone && (
            <p className="font-mono text-xs text-muted-foreground">
              {formatPhone(phone)}
            </p>
          )}
        </div>
      </div>

      <Separator />

      <dl className="space-y-2 text-xs">
        <div className="flex items-baseline justify-between gap-3">
          <dt className="text-muted-foreground">Tertaut</dt>
          <dd className="text-right font-medium">{since ?? 'baru saja'}</dd>
        </div>
        <div className="flex items-baseline justify-between gap-3">
          <dt className="text-muted-foreground">Nomor dibaca</dt>
          <dd className="text-right font-medium">
            {allowlistActive === 0 ? (
              // The user needs to know why nothing is happening yet.
              <span className="text-amber-600 dark:text-amber-500">belum ada</span>
            ) : (
              `${allowlistActive} nomor`
            )}
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-3">
          <dt className="text-muted-foreground">Mode</dt>
          <dd className="flex items-center gap-1 text-right font-medium">
            <CheckCircle2 className="size-3 text-primary" />
            Read-only
          </dd>
        </div>
      </dl>

      {allowlistActive === 0 && (
        <p className="rounded-md bg-muted/60 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
          Akun sudah tertaut, tetapi belum ada nomor terdaftar — jadi belum ada
          percakapan yang dibaca. Tambahkan nomor di panel sebelah.
        </p>
      )}
    </div>
  )
}
