import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, PlugZap, ShieldCheck, Unplug } from 'lucide-react'
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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { QRCanvas } from '@/components/qr-canvas'
import { WAProfile } from '@/components/wa-profile'
import { ContactPicker } from '@/components/contact-picker'
import { PageContainer, PageHeader, PageSections } from '@/components/page-shell'
import { ApiError } from '@/lib/api'
import {
  getAllowlist,
  getWAStatus,
  logoutDevice,
  requestPairing,
  statusLabels,
  type WAStatus,
} from '@/lib/wa'

const errorMessages: Record<string, string> = {
  connector_offline: 'Connector tidak berjalan, jadi tidak ada yang bisa membuat QR.',
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

        {/* items-start keeps the status card at its natural height instead of
            stretching to match the taller allowlist panel beside it. */}
        <div className="grid items-start gap-6 md:grid-cols-[auto_1fr]">
          <Card className="md:w-[340px]">
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-2 font-heading">
                Status
                {connection && (
                  // Matches the topbar badge: two different labels for the same
                  // state would read as two different problems.
                  <Badge
                    variant={
                      !connection.connector_online && connection.status !== 'connected'
                        ? 'secondary'
                        : statusVariant(connection.status)
                    }
                  >
                    {!connection.connector_online && connection.status !== 'connected'
                      ? 'Connector mati'
                      : statusLabels[connection.status]}
                  </Badge>
                )}
              </CardTitle>
              <CardDescription>
                {/* The number is not repeated here: the profile below already
                    shows it, formatted. */}
                {connection?.status === 'connected'
                  ? 'Akun tertaut sebagai Linked Device.'
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
                <WAProfile
                  connection={connection}
                  allowlistActive={status.data?.allowlist_active ?? 0}
                />
              ) : connection && !connection.connector_online ? (
                <ConnectorOffline />
              ) : (
                <p className="text-sm text-muted-foreground">
                  Menunggu QR dari connector. Kode biasanya muncul dalam beberapa
                  detik.
                </p>
              )}

              {connection?.last_error && (
                <p className="text-sm text-destructive">{connection.last_error}</p>
              )}

              <div className="flex flex-wrap gap-2">
                <Button
                  onClick={() => pair.mutate()}
                  disabled={
                    pair.isPending ||
                    connection?.status === 'connected' ||
                    // Nothing would honour the request; a disabled button is
                    // honest, a success toast would not be.
                    connection?.connector_online === false
                  }
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

          <ContactPicker
            rejections={allowlist.data?.rejections ?? []}
            connected={connection?.status === 'connected'}
          />
        </div>
      </PageSections>
    </PageContainer>
  )
}

/**
 * The QR comes from Baileys inside the connector process. When that process is
 * not running there is nothing to wait for, so say so plainly and give the exact
 * command rather than leaving an empty panel.
 */
function ConnectorOffline() {
  return (
    <div className="space-y-3 rounded-lg border border-dashed p-4">
      <div className="flex items-start gap-2">
        <PlugZap className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-500" />
        <div className="space-y-1">
          <p className="text-sm font-medium">Connector tidak berjalan</p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            QR dibuat oleh connector, bukan oleh backend. Jalankan salah satu:
          </p>
        </div>
      </div>
      <pre className="overflow-x-auto rounded-md bg-muted px-3 py-2 text-xs">
        <code>./run.sh</code>
      </pre>
      <p className="text-xs leading-relaxed text-muted-foreground">
        <code className="rounded bg-muted px-1 py-0.5">./run.sh</code> menjalankan
        connector sekaligus. Atau jalankan sendiri dari{' '}
        <code className="rounded bg-muted px-1 py-0.5">services/wa-connector</code>{' '}
        dengan <code className="rounded bg-muted px-1 py-0.5">npm run dev</code>.
        Setelah hidup, QR muncul sendiri di sini.
      </p>
    </div>
  )
}
