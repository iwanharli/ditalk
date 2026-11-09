import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { CheckCircle2, FileArchive, FileText, FolderOpen, Loader2, Upload } from 'lucide-react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { PageContainer, PageHeader, PageSections } from '@/components/page-shell'
import { cn } from '@/lib/utils'
import { ApiError } from '@/lib/api'
import { formatPhone } from '@/components/wa-profile'
import {
  getContacts,
  getImportFiles,
  previewImport,
  runImport,
  type ImportPreview,
  type ImportResult,
} from '@/lib/wa'

const errorMessages: Record<string, string> = {
  file_required: 'Pilih berkas dulu.',
  only_txt_or_zip_supported: 'Hanya berkas .txt atau .zip dari Export Chat.',
  zip_without_chat_text: 'Zip ini tidak berisi _chat.txt.',
  zip_unreadable: 'Zip tidak bisa dibaca.',
  import_file_unreadable: 'Berkas tidak bisa dibaca dari folder impor.',
  self_name_required: 'Pilih dulu nama mana yang mewakili Anda.',
  parse_failed: 'Isi berkas tidak dikenali sebagai Export Chat.',
  encryption_key_not_configured: 'ENCRYPTION_KEY belum diatur di .env.',
}

function describeError(err: unknown): string {
  if (err instanceof ApiError) return errorMessages[err.code] ?? err.code
  return err instanceof Error ? err.message : 'Terjadi kesalahan.'
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(0)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

export function ImportPage() {
  const [fileName, setFileName] = useState('')
  const [upload, setUpload] = useState<File | null>(null)
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [selfName, setSelfName] = useState('')
  const [chatKey, setChatKey] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [result, setResult] = useState<ImportResult | null>(null)

  const files = useQuery({ queryKey: ['imports', 'files'], queryFn: getImportFiles })
  const contacts = useQuery({ queryKey: ['wa', 'contacts'], queryFn: getContacts })

  const reset = () => {
    setPreview(null)
    setSelfName('')
    setResult(null)
  }

  const doPreview = useMutation({
    mutationFn: () => previewImport(upload ? { file: upload } : { fileName }),
    onSuccess: (p) => {
      setPreview(p)
      setResult(null)
      // The sender with the most messages is usually the account owner, but it is
      // only a starting guess — picking the wrong side silently inverts every
      // per-person statistic, so the choice stays explicit.
      const names = Object.entries(p.senders).sort((a, b) => b[1] - a[1])
      if (names.length > 0) setSelfName(names[0][0])
    },
    onError: (err) => toast.error(describeError(err)),
  })

  const doImport = useMutation({
    mutationFn: () =>
      runImport({
        ...(upload ? { file: upload } : { fileName }),
        selfName,
        displayName,
        chatKey,
      }),
    onSuccess: (r) => {
      setResult(r)
      toast.success(`${r.inserted.toLocaleString('id-ID')} pesan tersimpan.`)
    },
    onError: (err) => toast.error(describeError(err)),
  })

  const selected = upload?.name ?? fileName
  const registered = (contacts.data?.contacts ?? []).filter((c) => c.registered)

  return (
    <PageContainer>
      <PageHeader
        title="Impor Chat"
        description="Masukkan riwayat percakapan dari fitur Export Chat WhatsApp. Berkas .txt maupun .zip dengan media sama-sama diterima."
      />

      <PageSections>
        <Alert>
          <FolderOpen className="size-4" />
          <AlertTitle>Berkas besar tidak perlu diunggah</AlertTitle>
          <AlertDescription>
            Ekspor dengan media bisa ratusan megabyte. Taruh berkasnya di folder{' '}
            <code className="rounded bg-muted px-1 py-0.5 text-xs">
              {files.data?.dir ?? 'backend/data-chat'}
            </code>{' '}
            lalu pilih dari daftar — transkripnya dibaca langsung dari dalam zip
            tanpa menyalin apa pun.
          </AlertDescription>
        </Alert>

        <Card>
          <CardHeader>
            <CardTitle className="font-heading">1. Pilih berkas</CardTitle>
            <CardDescription>
              Dari folder impor, atau unggah langsung bila berkasnya kecil.
            </CardDescription>
          </CardHeader>

          <CardContent className="space-y-4">
            {files.isPending ? (
              <Skeleton className="h-20 w-full" />
            ) : (files.data?.files.length ?? 0) === 0 ? (
              <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
                Belum ada berkas di folder impor.
              </p>
            ) : (
              <ul className="divide-y rounded-lg border">
                {files.data?.files.map((f) => (
                  <li key={f.name}>
                    <button
                      type="button"
                      onClick={() => {
                        setFileName(f.name)
                        setUpload(null)
                        reset()
                      }}
                      className={cn(
                        'flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors',
                        fileName === f.name && !upload
                          ? 'bg-primary/[0.06]'
                          : 'hover:bg-accent/50',
                      )}
                    >
                      {f.is_archive ? (
                        <FileArchive className="size-4 shrink-0 text-muted-foreground" />
                      ) : (
                        <FileText className="size-4 shrink-0 text-muted-foreground" />
                      )}
                      <span className="min-w-0 flex-1 truncate text-sm">{f.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {formatSize(f.size_bytes)}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}

            <div className="flex flex-wrap items-center gap-3">
              <Label
                htmlFor="upload"
                className="inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm hover:bg-accent"
              >
                <Upload className="size-4" />
                Unggah berkas
              </Label>
              <input
                id="upload"
                type="file"
                accept=".txt,.zip"
                className="sr-only"
                onChange={(e) => {
                  const f = e.target.files?.[0] ?? null
                  setUpload(f)
                  setFileName('')
                  reset()
                }}
              />
              {upload && (
                <span className="text-sm text-muted-foreground">
                  {upload.name} · {formatSize(upload.size)}
                </span>
              )}
            </div>

            <Button
              onClick={() => doPreview.mutate()}
              disabled={!selected || doPreview.isPending}
            >
              {doPreview.isPending && <Loader2 className="size-4 animate-spin" />}
              Baca isinya
            </Button>
            <p className="text-xs text-muted-foreground">
              Membaca tidak menyimpan apa pun — aman diulang.
            </p>
          </CardContent>
        </Card>

        {preview && (
          <Card>
            <CardHeader>
              <CardTitle className="font-heading">2. Periksa dan tentukan</CardTitle>
              <CardDescription>
                {preview.total_messages.toLocaleString('id-ID')} pesan
                {preview.first_at && preview.last_at && (
                  <>
                    {' '}
                    · {preview.first_at.slice(0, 10)} sampai {preview.last_at.slice(0, 10)}
                  </>
                )}
              </CardDescription>
            </CardHeader>

            <CardContent className="space-y-5">
              <div className="space-y-2">
                <Label className="text-sm">Mana yang Anda?</Label>
                <p className="text-xs text-muted-foreground">
                  Salah pilih akan membalik seluruh statistik per pihak.
                </p>
                <div className="flex flex-wrap gap-2 pt-1">
                  {Object.entries(preview.senders).map(([name, count]) => (
                    <button
                      key={name}
                      type="button"
                      onClick={() => setSelfName(name)}
                      className={cn(
                        'rounded-md border px-3 py-2 text-left text-sm transition-colors',
                        selfName === name
                          ? 'border-primary bg-primary/10'
                          : 'hover:bg-accent/50',
                      )}
                    >
                      <span className="font-medium">{name}</span>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {count.toLocaleString('id-ID')} pesan
                      </span>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <Label className="text-sm">Percakapan ini dengan siapa?</Label>
                <p className="text-xs text-muted-foreground">
                  Memilih kontak membuat impor menyatu dengan sinkronisasi live ke
                  satu percakapan, bukan dua.
                </p>
                <div className="flex flex-wrap gap-2 pt-1">
                  {registered.map((c) => (
                    <button
                      key={c.phone}
                      type="button"
                      onClick={() => {
                        setChatKey(c.phone)
                        setDisplayName(c.label || c.name || c.phone)
                      }}
                      className={cn(
                        'rounded-md border px-3 py-2 text-left text-sm transition-colors',
                        chatKey === c.phone
                          ? 'border-primary bg-primary/10'
                          : 'hover:bg-accent/50',
                      )}
                    >
                      <span className="font-medium">{c.label || c.name || 'Tanpa nama'}</span>
                      <span className="ml-2 font-mono text-xs text-muted-foreground">
                        {formatPhone(c.phone)}
                      </span>
                    </button>
                  ))}
                </div>
                <div className="flex flex-wrap items-end gap-3 pt-2">
                  <div className="min-w-[190px] flex-1 space-y-1.5">
                    <Label htmlFor="chatkey" className="text-xs">
                      Atau ketik nomornya
                    </Label>
                    <Input
                      id="chatkey"
                      value={chatKey}
                      onChange={(e) => setChatKey(e.target.value)}
                      placeholder="0812 3456 7890"
                      inputMode="tel"
                    />
                  </div>
                  <div className="min-w-[160px] flex-1 space-y-1.5">
                    <Label htmlFor="dispname" className="text-xs">
                      Label (opsional)
                    </Label>
                    <Input
                      id="dispname"
                      value={displayName}
                      onChange={(e) => setDisplayName(e.target.value)}
                      placeholder="mis. Kakak"
                    />
                  </div>
                </div>
              </div>

              {(preview.warnings?.length ?? 0) > 0 && (
                <div className="space-y-1 rounded-md border border-dashed p-3">
                  <p className="text-sm font-medium">Peringatan dari pembacaan</p>
                  <ul className="list-inside list-disc text-xs text-muted-foreground">
                    {preview.warnings?.slice(0, 5).map((wn) => (
                      <li key={wn}>{wn}</li>
                    ))}
                  </ul>
                </div>
              )}

              <Button
                onClick={() => doImport.mutate()}
                disabled={!selfName || !chatKey || doImport.isPending}
              >
                {doImport.isPending && <Loader2 className="size-4 animate-spin" />}
                Simpan ke database
              </Button>
            </CardContent>
          </Card>
        )}

        {result && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 font-heading">
                <CheckCircle2 className="size-4 text-primary" />
                Selesai
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-2">
              <Badge variant="secondary">
                {result.inserted.toLocaleString('id-ID')} tersimpan
              </Badge>
              <Badge variant="secondary">
                {result.skipped.toLocaleString('id-ID')} dilewati
              </Badge>
              <Badge variant="secondary">
                {result.sessions.toLocaleString('id-ID')} sesi
              </Badge>
              <p className="w-full pt-2 text-xs text-muted-foreground">
                Yang dilewati adalah pesan sistem dan pesan yang sudah tersimpan.
                Mengimpor berkas yang sama lagi tidak akan menggandakan apa pun.
              </p>
            </CardContent>
          </Card>
        )}
      </PageSections>
    </PageContainer>
  )
}
