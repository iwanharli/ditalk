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

## Menjalankan

```bash
docker compose up -d                      # postgres+pgvector, redis, minio

cd backend && go run ./cmd/api            # :8080
cd frontend && npm run dev                # :5173
cd services/wa-connector && npm run dev
cd services/ai-media && uvicorn app.main:app --reload --port 8000
```

## Batasan

Aplikasi ini tidak mengirim pesan, tidak melakukan auto-reply, tidak melakukan
face recognition, dan tidak menghasilkan diagnosis. Baileys adalah integrasi
tidak resmi terhadap WhatsApp Web — tinjau ketentuan layanan sebelum digunakan.
