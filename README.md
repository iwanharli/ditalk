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

Override target dengan `GOOSE_DBSTRING`, mis. saat memakai docker-compose:

```bash
GOOSE_DBSTRING="postgres://ditalk:ditalk@localhost:5432/db_ditalk?sslmode=disable" make migrate-up
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

```bash
docker compose up -d                      # postgres+pgvector, redis, minio

cd backend && make migrate-up && go run ./cmd/api   # :8080
cd frontend && npm run dev                # :5173
cd services/wa-connector && npm run dev
cd services/ai-media && uvicorn app.main:app --reload --port 8000
```

## Batasan

Aplikasi ini tidak mengirim pesan, tidak melakukan auto-reply, tidak melakukan
face recognition, dan tidak menghasilkan diagnosis. Baileys adalah integrasi
tidak resmi terhadap WhatsApp Web — tinjau ketentuan layanan sebelum digunakan.
