import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  ChevronDown,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  User,
  UserPlus,
  Users,
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { id as idLocale } from 'date-fns/locale'
import { toast } from 'sonner'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { ApiError } from '@/lib/api'
import { formatPhone } from '@/components/wa-profile'
import {
  addContact,
  deleteContact,
  getContacts,
  rejectionLabels,
  setContactActive,
  type ContactRow,
  type Rejection,
} from '@/lib/wa'

const errorMessages: Record<string, string> = {
  phone_required: 'Nomor wajib diisi.',
  phone_too_short: 'Nomor terlalu pendek.',
  phone_invalid_characters: 'Nomor hanya boleh berisi angka dan pemisah.',
  encryption_key_not_configured: 'ENCRYPTION_KEY belum diatur di .env.',
  contact_not_found: 'Kontak tidak ditemukan.',
}

function describeError(err: unknown): string {
  if (err instanceof ApiError) return errorMessages[err.code] ?? err.code
  return err instanceof Error ? err.message : 'Terjadi kesalahan.'
}

const EMPTY_ROWS: ContactRow[] = []

/**
 * Two letters from the name, or null when there is no name. Falling back to the
 * last two digits of the number produced meaningless labels like "44".
 */
function initials(row: ContactRow): string | null {
  const source = row.label || row.name
  const parts = source.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return null
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

function relativeTime(iso: string | null | undefined): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return formatDistanceToNow(d, { addSuffix: true, locale: idLocale })
}

export function ContactPicker({
  rejections,
  connected,
}: {
  rejections: Rejection[]
  connected: boolean
}) {
  const qc = useQueryClient()
  const [query, setQuery] = useState('')
  const [manualOpen, setManualOpen] = useState(false)

  const contacts = useQuery({
    queryKey: ['wa', 'contacts'],
    queryFn: getContacts,
    // History sync trickles in, so the list grows for a while after pairing.
    refetchInterval: 5000,
  })

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['wa'] })
  }

  const add = useMutation({
    mutationFn: (input: { phone: string; label?: string }) => addContact(input),
    onSuccess: (c) => {
      toast.success(`${formatPhone(c.phone)} akan dianalisis.`)
      invalidate()
    },
    onError: (err) => toast.error(describeError(err)),
  })

  const toggle = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      setContactActive(id, active),
    onSuccess: invalidate,
    onError: (err) => toast.error(describeError(err)),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteContact(id),
    onSuccess: () => {
      toast.success('Kontak dikeluarkan dari daftar analisis.')
      invalidate()
    },
    onError: (err) => toast.error(describeError(err)),
  })

  // A literal [] fallback would be a new array on every render, which defeats
  // the useMemo below.
  const rows = contacts.data?.contacts ?? EMPTY_ROWS

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    // Search the digits too, so "0812" finds a number stored as 62812…
    const digits = q.replace(/\D/g, '')
    return rows.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.label.toLowerCase().includes(q) ||
        (digits.length > 0 && r.phone.includes(digits.replace(/^0/, '62'))) ||
        (digits.length > 0 && r.phone.includes(digits)),
    )
  }, [rows, query])

  const registeredCount = rows.filter((r) => r.registered && r.is_active).length
  const deviceCount = rows.filter((r) => r.from_device).length

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-3 font-heading">
          Kontak yang dianalisis
          {registeredCount > 0 && (
            <Badge variant="secondary" className="shrink-0 font-normal">
              {registeredCount} dipilih
            </Badge>
          )}
        </CardTitle>
        <CardDescription className="max-w-2xl">
          Pilih dari percakapan yang ada di WhatsApp Anda, atau masukkan nomor manual.
          Hanya kontak yang dipilih yang diproses.
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Cari nama atau nomor"
            className="pl-9"
            aria-label="Cari kontak"
          />
        </div>

        {contacts.isPending ? (
          <div className="space-y-2">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </div>
        ) : rows.length === 0 ? (
          <EmptyState connected={connected} />
        ) : filtered.length === 0 ? (
          <p className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            Tidak ada kontak yang cocok dengan “{query}”.
          </p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {filtered.map((row) => (
              <ContactRowItem
                key={row.phone}
                row={row}
                busy={add.isPending || toggle.isPending || remove.isPending}
                onAdd={() => add.mutate({ phone: row.phone, label: row.name })}
                onToggle={(active) =>
                  row.contact_id && toggle.mutate({ id: row.contact_id, active })
                }
                onRemove={() => row.contact_id && remove.mutate(row.contact_id)}
              />
            ))}
          </ul>
        )}

        {deviceCount > 0 && (
          <p className="text-xs leading-relaxed text-muted-foreground">
            {deviceCount} percakapan terbaca dari perangkat. Nama dan nomornya hanya
            ditampilkan agar Anda bisa memilih — tidak disimpan dan tidak dianalisis
            sampai Anda memilihnya.
          </p>
        )}

        <ManualAdd
          open={manualOpen}
          onOpenChange={setManualOpen}
          pending={add.isPending}
          onSubmit={(phone, label) => add.mutate({ phone, label })}
        />

        {rejections.length > 0 && (
          <div className="space-y-2 border-t pt-4">
            <p className="text-sm font-medium">Yang dibuang oleh filter</p>
            <p className="text-xs text-muted-foreground">
              Hanya jumlah dan alasannya yang dicatat, bukan isi atau nomornya.
            </p>
            <div className="flex flex-wrap gap-2 pt-1">
              {rejections.map((r) => (
                <Badge key={r.reason} variant="secondary">
                  {rejectionLabels[r.reason] ?? r.reason}: {r.count}
                </Badge>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ContactRowItem({
  row,
  busy,
  onAdd,
  onToggle,
  onRemove,
}: {
  row: ContactRow
  busy: boolean
  onAdd: () => void
  onToggle: (active: boolean) => void
  onRemove: () => void
}) {
  const title = row.label || row.name
  const last = relativeTime(row.last_message_at)

  return (
    <li
      className={cn(
        'flex items-center gap-3 px-3 py-2.5 transition-colors',
        row.registered && row.is_active && 'bg-primary/[0.04]',
      )}
    >
      <Avatar className="size-9 shrink-0">
        {/* Pictures exist only for selected contacts; everything else keeps
            initials. See the connector's avatars.js for why. */}
        {row.avatar_version && (
          <AvatarImage
            src={`/api/wa/contacts/${row.phone}/avatar?v=${row.avatar_version}`}
            alt={`Foto profil ${title || row.phone}`}
          />
        )}
        <AvatarFallback
          className={cn(
            'text-xs font-medium',
            row.registered && row.is_active
              ? 'bg-primary/15 text-primary'
              : 'bg-muted text-muted-foreground',
          )}
        >
          {initials(row) ?? <User className="size-4" />}
        </AvatarFallback>
      </Avatar>

      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium">
            {title || (
              <span className="text-muted-foreground">Tanpa nama</span>
            )}
          </p>
          {row.registered && !row.is_active && (
            <Badge variant="outline" className="shrink-0 text-[0.6875rem]">
              dijeda
            </Badge>
          )}
        </div>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {formatPhone(row.phone)}
          {last && <span className="ml-2 font-sans">· {last}</span>}
        </p>

        {row.registered && <StorageLine row={row} />}
      </div>

      {row.registered ? (
        <div className="flex shrink-0 items-center gap-1">
          <Switch
            checked={row.is_active}
            onCheckedChange={onToggle}
            disabled={busy}
            aria-label={`Analisis percakapan dengan ${title || row.phone}`}
          />
          <Button
            variant="ghost"
            size="icon"
            onClick={onRemove}
            disabled={busy}
            aria-label={`Keluarkan ${title || row.phone} dari daftar`}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ) : (
        <Button size="sm" variant="outline" onClick={onAdd} disabled={busy} className="shrink-0">
          <Plus className="size-4" />
          Pilih
        </Button>
      )}
    </li>
  )
}

/**
 * Explains why the list is empty, and names the actual remedy.
 *
 * WhatsApp sends its full history sync only once, right after a device is
 * linked. On a later reconnect it sends nothing, so "wait a moment" would be
 * wrong: without a re-pair or new message activity the list stays empty forever.
 */
/**
 * Shows how much of a selected chat is stored, and whether the engine is still
 * walking its history backwards.
 *
 * Without this the app looks idle during a backfill that can take minutes, and
 * "0 pesan" gives no hint whether that is a failure or simply not started.
 */
function StorageLine({ row }: { row: ContactRow }) {
  if (row.stored === 0) {
    return (
      <p className="mt-0.5 text-xs text-muted-foreground">
        Belum ada pesan tersimpan — menunggu aktivitas pertama di chat ini.
      </p>
    )
  }

  const bf = row.backfill
  return (
    <p className="mt-0.5 flex items-center gap-1.5 text-xs">
      <span className="font-medium">{row.stored.toLocaleString('id-ID')} pesan</span>
      {bf?.running && (
        <span className="inline-flex items-center gap-1 text-muted-foreground">
          <Loader2 className="size-3 animate-spin" />
          mengambil riwayat lama
        </span>
      )}
      {bf?.done && (
        <span className="inline-flex items-center gap-1 text-primary">
          <CheckCircle2 className="size-3" />
          riwayat lengkap
        </span>
      )}
    </p>
  )
}

function EmptyState({ connected }: { connected: boolean }) {
  if (!connected) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center">
        <span className="grid size-10 place-items-center rounded-full bg-muted">
          <Users className="size-5 text-muted-foreground" />
        </span>
        <p className="text-sm font-medium">WhatsApp belum tertaut</p>
        <p className="max-w-sm text-sm text-muted-foreground">
          Pindai QR di panel Status lebih dulu, atau masukkan nomor manual di bawah.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-dashed px-5 py-6">
      <div className="flex items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-amber-500/10">
          <RefreshCw className="size-4 text-amber-600 dark:text-amber-500" />
        </span>
        <div className="space-y-1">
          <p className="text-sm font-medium">Daftar percakapan belum tersedia</p>
          <p className="text-sm leading-relaxed text-muted-foreground">
            WhatsApp hanya mengirim riwayat percakapan sekali, tepat setelah perangkat
            ditautkan. Karena itu daftar ini tidak akan terisi sendiri pada koneksi
            berikutnya.
          </p>
        </div>
      </div>

      <div className="space-y-2 pl-12 text-sm">
        <p className="font-medium">Dua cara mengisinya:</p>
        <ol className="list-outside list-decimal space-y-1.5 pl-4 text-muted-foreground">
          <li>
            Kirim atau terima satu pesan WhatsApp — kontak itu muncul di sini dalam
            beberapa detik.
          </li>
          <li>
            Tautkan ulang untuk mengambil seluruh riwayat sekaligus: tekan{' '}
            <span className="font-medium text-foreground">Lepas perangkat</span> di
            panel Status, lalu pindai QR yang baru. Setelah itu daftarnya tersimpan
            dan tidak hilang saat restart.
          </li>
        </ol>
      </div>

      <p className="pl-12 text-xs text-muted-foreground">
        Atau lewati saja dan masukkan nomor manual di bawah.
      </p>
    </div>
  )
}

function ManualAdd({
  open,
  onOpenChange,
  pending,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  pending: boolean
  onSubmit: (phone: string, label: string) => void
}) {
  const [phone, setPhone] = useState('')
  const [label, setLabel] = useState('')

  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-start px-2">
          <UserPlus className="size-4" />
          Masukkan nomor manual
          <ChevronDown
            className={cn('ml-auto size-4 transition-transform', open && 'rotate-180')}
          />
        </Button>
      </CollapsibleTrigger>

      <CollapsibleContent>
        <form
          className="mt-2 space-y-3 rounded-lg border bg-muted/30 p-3"
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit(phone, label)
            setPhone('')
            setLabel('')
          }}
        >
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-[170px] flex-1 space-y-1.5">
              <Label htmlFor="manual-phone" className="text-xs">
                Nomor WhatsApp
              </Label>
              <Input
                id="manual-phone"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                placeholder="0812 3456 7890"
                inputMode="tel"
                autoComplete="off"
              />
            </div>
            <div className="min-w-[140px] flex-1 space-y-1.5">
              <Label htmlFor="manual-label" className="text-xs">
                Label (opsional)
              </Label>
              <Input
                id="manual-label"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                placeholder="mis. Kakak"
                autoComplete="off"
              />
            </div>
            <Button type="submit" disabled={pending || phone.trim() === ''}>
              {pending ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
              Tambah
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Format lokal maupun internasional diterima: 0812…, +62 812…, dan 62812…
            dianggap nomor yang sama.
          </p>
        </form>
      </CollapsibleContent>
    </Collapsible>
  )
}
