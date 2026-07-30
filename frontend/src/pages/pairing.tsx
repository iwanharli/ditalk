import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, ShieldCheck, Trash2, Unplug } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { QRCanvas } from '@/components/qr-canvas'
import { PageContainer, PageHeader, PageSections } from '@/components/page-shell'
import { ApiError } from '@/lib/api'
import {
  addContact,
  deleteContact,
  getAllowlist,
  getWAStatus,
  logoutDevice,
  rejectionLabels,
  requestPairing,
  setContactActive,
  statusLabels,
  type AllowedContact,
  type Rejection,
  type WAStatus,
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

function statusVariant(status: WAStatus): 'default' | 'secondary' | 'destructive' {
  switch (status) {
    case 'connected':
      return 'default'
    case 'error':
    case 'logged_out':
      return 'destructive'
    default:
      return 'secondary'
  }
}

export function PairingPage() {
  const qc = useQueryClient()

  const status = useQuery({
    queryKey: ['wa', 'status'],
    queryFn: getWAStatus,
    // A QR code rotates every ~20s, so the screen has to keep up with it.
    refetchInterval: 3000,
  })

  const allowlist = useQuery({ queryKey: ['wa', 'allowlist'], queryFn: getAllowlist })

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['wa'] })
  }

  const pair = useMutation({
    mutationFn: requestPairing,
    onSuccess: () => toast.success('Meminta QR baru. Tunggu beberapa detik.'),
    onError: (err) => toast.error(describeError(err)),
  })

  const logout = useMutation({
    mutationFn: logoutDevice,
    onSuccess: () => {
      toast.success('Perangkat dilepas.')
      invalidate()
    },
    onError: (err) => toast.error(describeError(err)),
  })

  const connection = status.data?.connection

  return (
    <PageContainer>
      <PageHeader
        title="Sambungkan WhatsApp"
        description="Tautkan akun Anda sendiri sebagai Linked Device. Aplikasi hanya membaca — tidak mengirim pesan, tidak membalas otomatis."
      />

      <PageSections>
        <Alert>
          <ShieldCheck className="size-4" />
          <AlertTitle>Hanya nomor yang Anda daftarkan yang dibaca</AlertTitle>
          <AlertDescription>
            Percakapan dari nomor di luar daftar, termasuk semua grup, dibuang sebelum
            dianalisis. Filter berjalan di connector dan diperiksa ulang di backend.
          </AlertDescription>
        </Alert>

        <div className="grid gap-6 md:grid-cols-[auto_1fr]">
          <Card className="md:w-[340px]">
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-2 font-heading">
                Status
                {connection && (
                  <Badge variant={statusVariant(connection.status)}>
                    {statusLabels[connection.status]}
                  </Badge>
                )}
              </CardTitle>
              <CardDescription>
                {connection?.self_phone
                  ? `Tertaut ke +${connection.self_phone}`
                  : 'Belum ada akun tertaut.'}
              </CardDescription>
            </CardHeader>

            <CardContent className="space-y-4">
              {status.isPending ? (
                <Skeleton className="h-[264px] w-full" />
              ) : connection?.qr ? (
                <div className="space-y-3">
                  <QRCanvas value={connection.qr} />
                  <ol className="list-inside list-decimal space-y-1 text-xs text-muted-foreground">
                    <li>Buka WhatsApp di ponsel Anda.</li>
                    <li>Pilih Perangkat Tertaut, lalu Tautkan perangkat.</li>
                    <li>Pindai kode ini. Kode berganti otomatis.</li>
                  </ol>
                </div>
              ) : connection?.status === 'connected' ? (
                <p className="text-sm text-muted-foreground">
                  Sudah tersambung. Tidak perlu memindai QR.
                </p>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Belum ada QR. Pastikan connector berjalan
                  <code className="mx-1 rounded bg-muted px-1 py-0.5 text-xs">
                    npm run dev
                  </code>
                  di services/wa-connector, lalu minta QR baru.
                </p>
              )}

              {connection?.last_error && (
                <p className="text-sm text-destructive">{connection.last_error}</p>
              )}

              <div className="flex flex-wrap gap-2">
                <Button
                  onClick={() => pair.mutate()}
                  disabled={pair.isPending || connection?.status === 'connected'}
                  size="sm"
                >
                  {pair.isPending && <Loader2 className="size-4 animate-spin" />}
                  Minta QR baru
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => logout.mutate()}
                  disabled={logout.isPending}
                >
                  <Unplug className="size-4" />
                  Lepas perangkat
                </Button>
              </div>
            </CardContent>
          </Card>

          <AllowlistCard
            isPending={allowlist.isPending}
            contacts={allowlist.data?.contacts ?? []}
            rejections={allowlist.data?.rejections ?? []}
            onChanged={invalidate}
          />
        </div>
      </PageSections>
    </PageContainer>
  )
}

function AllowlistCard({
  isPending,
  contacts,
  rejections,
  onChanged,
}: {
  isPending: boolean
  contacts: AllowedContact[]
  rejections: Rejection[]
  onChanged: () => void
}) {
  const [phone, setPhone] = useState('')
  const [label, setLabel] = useState('')

  const add = useMutation({
    mutationFn: () => addContact({ phone, label }),
    onSuccess: (c) => {
      toast.success(`+${c.phone} ditambahkan.`)
      setPhone('')
      setLabel('')
      onChanged()
    },
    onError: (err) => toast.error(describeError(err)),
  })

  const toggle = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      setContactActive(id, active),
    onSuccess: onChanged,
    onError: (err) => toast.error(describeError(err)),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteContact(id),
    onSuccess: () => {
      toast.success('Kontak dihapus dari daftar.')
      onChanged()
    },
    onError: (err) => toast.error(describeError(err)),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="font-heading">Nomor yang terdaftar</CardTitle>
        <CardDescription>
          Hanya percakapan dengan nomor ini yang diproses. Menghapus nomor
          menghentikan pembacaan baru; riwayat yang sudah tersimpan dihapus terpisah
          dari halaman Privasi.
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-5">
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate()
          }}
        >
          <div className="min-w-[180px] flex-1 space-y-1.5">
            <Label htmlFor="phone">Nomor WhatsApp</Label>
            <Input
              id="phone"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              placeholder="0812 3456 7890"
              inputMode="tel"
              autoComplete="off"
            />
          </div>
          <div className="min-w-[140px] flex-1 space-y-1.5">
            <Label htmlFor="label">Label (opsional)</Label>
            <Input
              id="label"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="mis. Kakak"
              autoComplete="off"
            />
          </div>
          <Button type="submit" disabled={add.isPending || phone.trim() === ''}>
            {add.isPending ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
            Tambah
          </Button>
        </form>

        <p className="text-xs text-muted-foreground">
          Format lokal maupun internasional diterima: 0812…, +62 812…, dan 62812…
          dianggap nomor yang sama.
        </p>

        {isPending ? (
          <Skeleton className="h-24 w-full" />
        ) : contacts.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            Belum ada nomor terdaftar. Selama daftar kosong, tidak ada percakapan yang
            diproses.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Nomor</TableHead>
                  <TableHead>Label</TableHead>
                  <TableHead>Aktif</TableHead>
                  <TableHead className="text-right">Aksi</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {contacts.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-mono text-sm">+{c.phone}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {c.label || '—'}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={c.is_active}
                        onCheckedChange={(active) => toggle.mutate({ id: c.id, active })}
                        aria-label={`Aktifkan pembacaan untuk ${c.phone}`}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => remove.mutate(c.id)}
                        disabled={remove.isPending}
                        aria-label={`Hapus ${c.phone}`}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

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
