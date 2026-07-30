# ditalk

Sistem analisis percakapan WhatsApp pribadi berbasis AI multimodal. Read-only,
local-first, evidence-first. Rancangan lengkap ada di
[docs/](docs/Konsep_Sistem_Analisis_Percakapan_WhatsApp_AI_Multimodal_Revisi_v1.3.pdf).

## Struktur

| Path | Stack | Peran |
| --- | --- | --- |
| `backend/` | Go | API, auth, orchestrator, database, deletion |
| `frontend/` | React + TypeScript (Vite) | Dashboard, timeline, search, privacy center |
| `services/wa-connector/` | Node.js + Baileys | Linked Device, event ingestion (read-only) |
| `services/ai-media/` | Python + FastAPI | Prosodi, OCR lokal, feature extraction |

Baileys hanya tersedia sebagai library Node.js, dan prosodi/OCR paling matang di
Python — karena itu keduanya berdiri sebagai service terpisah di belakang Go.

## Library yang dipakai

**Backend (Go)** — `pgx/v5` (PostgreSQL), `goose/v3` (migrasi, embedded ke
binary), `asynq` + Redis (job queue), `openai-go/v3` (AI), `golang-jwt/v5` +
`argon2id` (auth), `validator/v10`, `rs/cors`, `godotenv`. Routing HTTP memakai
`net/http.ServeMux` bawaan Go 1.22+, tanpa framework tambahan.

**Frontend (React 19)** — Tailwind v4 + shadcn/ui (Radix, preset Nova) dengan 25
komponen di `src/components/ui/`, `react-router-dom`, `@tanstack/react-query`,
`recharts` (chart), `zod` (validasi kontrak), `lucide-react`, `date-fns`,
`next-themes`, `sonner`.

Komponen shadcn adalah **kode di repo ini**, bukan dependency — bebas diedit.

**Prasyarat**: PostgreSQL + pgvector, Redis, Node 20+, Go 1.24+, Python 3.11+.
Semua jalan native di host, tanpa container.

```bash
cp .env.example .env    # lalu isi OPENAI_API_KEY, ENCRYPTION_KEY, JWT_SECRET
```

## Sambungkan WhatsApp (Linked Device)

Jalankan `./run.sh`, buka `http://localhost:5173/pairing`, lalu pindai QR dengan
WhatsApp di ponsel (Perangkat Tertaut → Tautkan perangkat).

**QR dibuat oleh connector, bukan oleh backend.** `./run.sh` menjalankan
connector secara default; tanpa proses itu tidak ada yang bisa menghasilkan QR.
Bila connector mati, dashboard mengatakannya secara eksplisit dan tombol "Minta
QR baru" dinonaktifkan, bukan melaporkan sukses palsu.

Untuk menjalankan connector sendiri:

```bash
cd services/wa-connector && npm run dev
```

**Hanya nomor yang Anda daftarkan yang dibaca.** Percakapan dari nomor di luar
daftar dibuang, dan **grup selalu ditolak** tanpa kecuali — grup berisi pihak
ketiga yang tidak pernah memberi persetujuan (bab 19.1). Filter berjalan di dua
tempat: di connector, sehingga isi pesan yang tidak berhak tidak pernah keluar
dari proses itu, dan diperiksa ulang di backend. Keduanya *fail closed* — apa pun
yang tidak positif cocok dengan kontak aktif akan ditolak, termasuk saat
allowlist belum termuat.

Format lokal dan internasional dianggap nomor yang sama: `0812…`, `+62 812…`,
dan `62812…` semuanya ternormalisasi ke `6281234567890`.

Menjeda kontak (`is_active=false`) menghentikan pembacaan tanpa kehilangan
konfigurasi. Menghapus nomor menghentikan pembacaan baru; menghapus riwayat yang
sudah tersimpan adalah tindakan terpisah di halaman Privasi.

`INTERNAL_TOKEN` wajib diisi. Tanpa itu endpoint event akan menerima pesan
palsu dari apa pun yang bisa menjangkau port tersebut.

## Import Export Chat

Jalur ingestion pertama, tanpa menyentuh API tidak resmi (bab 30, Keputusan 1).

```bash
# 1. Lihat isi file dulu tanpa menyimpan apa pun
curl -F "file=@chat.txt" -F "self_name=Nama Anda" \
  http://localhost:8080/imports/preview

# 2. Simpan setelah memastikan nama pengirim benar
curl -F "file=@chat.txt" -F "self_name=Nama Anda" \
     -F "display_name=Nama Lawan Bicara" -F "chat_key=chat-001" \
  http://localhost:8080/imports
```

Parser menangani format Android, iOS, dan 12-jam AS; mendeteksi urutan tanggal
dari isi file (bukan mengasumsikan satu format); menggabungkan pesan multi-baris;
mengenali media, attachment, pesan dihapus/diedit; serta membuang tanda bidi yang
disisipkan WhatsApp. Tanggal mustahil seperti 31 Februari **dibuang dengan
peringatan**, bukan digeser ke 3 Maret.

`import` bersifat idempoten: ID pesan diturunkan dari isinya, jadi mengimpor
ulang file yang tumpang tindih akan melewati baris yang sudah ada. Sesi dihitung
ulang, tidak digandakan.

Tanpa `ENCRYPTION_KEY`, `preview` tetap jalan (tidak menulis) tetapi `import`
ditolak `503` — server tidak akan pernah menulis isi chat sebagai plaintext.

## Database

Nama database: **`db_ditalk`** (PostgreSQL + pgvector). Migrasi memakai
[goose](https://github.com/pressly/goose), ada di `backend/migrations/`.

```bash
createdb db_ditalk
cd backend
make migrate-up        # apply
make migrate-status    # lihat versi
make migrate-down      # rollback satu langkah
make migrate-create name=add_something
```

Override target dengan `GOOSE_DBSTRING` bila Postgres Anda bukan di host default:

```bash
GOOSE_DBSTRING="postgres://user:pass@localhost:5432/db_ditalk?sslmode=disable" make migrate-up
```

| Migrasi | Isi |
| --- | --- |
| `00001_core` | users, wa_sessions, conversations, messages, conversation_sessions, media_assets |
| `00002_analysis` | analyses, analysis_corrections, embeddings (pgvector HNSW) |
| `00003_scoring` | score_versions, aggregate_scores, evidence links, corrections, period_comparisons |
| `00004_knowledge_vault` | people, memories + turunannya, commitments, boundaries, journal, relationship_context |
| `00005_ops` | audit_log, deletion_ledger, processing_jobs, ai_usage |
| `00006_seed_score_version` | bobot baseline GCI `gci-1.0` dari bab 12C-12E |

Beberapa guardrail dokumen ditegakkan di level database, bukan hanya di kode:
GCI wajib `NULL` saat `reliability_status = 'insufficient'` (bab 12E.2), memori
dengan confidence < 60 tidak bisa berstatus `confirmed` (bab 16A.3), dan koordinat
presisi hanya boleh terisi bila `precise_opt_in` true (bab 16A.1).

## Menjalankan

Cara termudah, backend + frontend sekaligus:

```bash
./run.sh                 # backend :8080 + frontend :5173 + connector WhatsApp
./run.sh --worker        # sekalian worker queue
./run.sh --no-connector  # tanpa connector WhatsApp
```

Script memeriksa prasyarat lebih dulu (PostgreSQL, Redis, port bebas), memuat
`.env` tanpa menimpa variabel yang sudah ada di environment, memberi prefix
`[be]`/`[fe]` pada log, dan menghentikan semua proses beserta anaknya saat
Ctrl-C. Override lewat env: `PORT=9000 FRONTEND_PORT=3000 ./run.sh`.

Manual, bila ingin tiap service di terminal terpisah:

```bash
cd backend && go run ./cmd/api      # :8080  API (migrasi jalan otomatis)
cd backend && go run ./cmd/worker   #        worker queue
cd frontend && npm run dev          # :5173  dashboard
cd services/wa-connector && npm run dev
cd services/ai-media && uvicorn app.main:app --reload --port 8000
```

Frontend memproksi `/api/*` ke backend, jadi browser tetap satu origin dan
cookie berperilaku wajar saat development.

Object storage (media asli) baru dibutuhkan di Phase 2-3.

## Batasan

Aplikasi ini tidak mengirim pesan, tidak melakukan auto-reply, tidak melakukan
face recognition, dan tidak menghasilkan diagnosis. Baileys adalah integrasi
tidak resmi terhadap WhatsApp Web — tinjau ketentuan layanan sebelum digunakan.
