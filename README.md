<p align="center">
  <img src="apps/web/public/kawandok-wordmark.png" alt="Kawandok" width="420" />
</p>

<p align="center">
  <strong>Workspace pengetahuan berbasis dokumen yang aman, dapat ditelusuri, dan siap dikembangkan.</strong>
</p>

<p align="center">
  <a href="#mulai-cepat">Mulai cepat</a> ·
  <a href="#konfigurasi-model-ai">Model AI</a> ·
  <a href="#arsitektur">Arsitektur</a> ·
  <a href="#keamanan-dan-batasan">Keamanan</a>
</p>

> Status: siap untuk pengembangan lokal dan evaluasi. **Belum siap produksi.**
> OIDC, kebijakan retensi, persetujuan penyedia AI, dan lisensi open-source
> masih perlu diputuskan sebelum publikasi atau penggunaan data nyata.

## Tentang Kawandok

Kawandok adalah workspace pengetahuan internal untuk mengunggah dokumen,
mengelola hak akses, mencari isi dokumen, dan melakukan tanya jawab berbasis
sumber. Setiap akses dokumen dan bukti jawaban diperiksa di server. Data XLSX
keuangan diproses melalui perhitungan SQL tervalidasi—bukan dihitung oleh LLM.

Fitur utama:

- Unggah PDF, DOCX, dan XLSX dengan klasifikasi, departemen, serta ACL
  *deny-by-default*.
- Retrieval hybrid: pencarian teks PostgreSQL dan pencarian semantik vector.
- Chat streaming dengan sitasi dokumen, halaman, atau bagian sumber.
- Impor XLSX finansial dengan validasi format dan agregasi terparameterisasi.
- Provider LLM dan embedding yang dapat diganti: `mock`, satu endpoint
  OpenAI-compatible, atau beberapa endpoint dalam mode round-robin + failover.
- Infrastruktur lokal berbasis Docker Compose: PostgreSQL + pgvector, Redis,
  dan SeaweedFS (S3-compatible).

## Arsitektur

```mermaid
flowchart LR
    U[Pengguna] --> W[Web Next.js]
    W --> A[API Go]
    A --> DB[(PostgreSQL + pgvector)]
    A --> S3[(SeaweedFS / S3)]
    A --> Q[Redis Streams]
    Q --> WK[Worker Python]
    WK --> S3
    WK --> DB
    A --> L[LLM / Embedding Provider]
```

## Alur dokumen dan jawaban

```mermaid
sequenceDiagram
    participant Admin as Admin
    participant API as API
    participant Store as Object Storage
    participant Worker as Worker
    participant DB as PostgreSQL
    participant User as Pengguna

    Admin->>API: Unggah PDF/DOCX/XLSX + ACL
    API->>Store: Simpan objek dengan kunci UUID
    API->>DB: Simpan metadata dan job ingest
    API->>Worker: Kirim job melalui Redis
    Worker->>Store: Ambil dokumen
    Worker->>Worker: Scan opsional, parse, chunk, embedding
    Worker->>DB: Simpan chunk/vector atau transaksi finansial
    User->>API: Ajukan pertanyaan
    API->>DB: Periksa ACL dan cari bukti relevan
    API->>User: Jawaban + sitasi sumber
```

## Prasyarat

- Docker Desktop
- Go 1.22 atau lebih baru
- Python 3.10 atau lebih baru
- Node.js 20 dan npm
- Git
- Opsional: GNU Make + Git Bash/WSL. Pengguna PowerShell dapat menjalankan
  perintah Docker/Go/Python/npm langsung.

## Mulai cepat

### 1. Clone dan siapkan konfigurasi

```sh
git clone <URL_REPOSITORY_ANDA> kawandok
cd kawandok
cp .env.example .env
```

Pada Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

File `.env` sudah memakai provider `mock` sehingga tidak membutuhkan akun AI,
Cloudflare R2, atau API key untuk menjalankan simulasi pertama. Jangan commit
file `.env`.

### 2. Instal dependency aplikasi

macOS/Linux/Git Bash:

```sh
cd apps/worker
python -m venv .venv
. .venv/bin/activate
python -m pip install -e ".[dev]"

cd ../web
npm ci
cd ../..
```

Windows PowerShell:

```powershell
py -m venv apps\worker\.venv
.\apps\worker\.venv\Scripts\python.exe -m pip install -e ".\apps\worker[dev]"
npm --prefix apps\web ci
```

### 3. Jalankan infrastruktur Docker dan migrasi

Dengan Make:

```sh
make up
make migrate
```

Tanpa Make:

```sh
docker compose -f infra/docker-compose.yml up -d
go -C apps/api run ./cmd/migrate -dir ../../infra/db/migrations -database "postgres://migrator:dev_migrator_password@127.0.0.1:15432/office_ai?sslmode=disable" up
```

Docker Compose hanya menjalankan infrastruktur lokal:

| Layanan | Port host | Fungsi |
|---|---:|---|
| PostgreSQL + pgvector | `15432` | metadata, ACL, chunk, vector, transaksi |
| Redis | `16379` | antrian ingestion |
| SeaweedFS S3 | `19000` | objek dokumen lokal |

Port non-standar tersebut memungkinkan Kawandok berjalan bersamaan dengan
project lain yang memakai PostgreSQL `5432` atau Redis `6379`.

### 4. Jalankan tiga aplikasi

Jalankan masing-masing pada terminal berbeda:

```sh
make run-go       # API: http://127.0.0.1:18080
make run-worker   # worker ingestion
make run-web      # web: http://127.0.0.1:3000
```

Atau tanpa Make:

```sh
go run ./apps/api/cmd/api
cd apps/worker && python -m worker.main
cd apps/web && npm run dev
```

API dan worker memuat `.env` terdekat secara otomatis. Variabel environment
yang sudah ada pada sistem selalu menang atas isi file `.env`.

### 5. Coba alur pertama

1. Buka `http://localhost:3000`.
2. Masuk sebagai `admin@dev.local`.
3. Buka **Knowledge Base**, unggah dokumen sintetis PDF, DOCX, atau XLSX.
4. Tentukan departemen dan grant akses pada dokumen.
5. Tunggu status dokumen menjadi `ready`, lalu ajukan pertanyaan melalui chat.

Akun development lain: `budi@dev.local` (employee, General),
`siti@dev.local` (finance, Finance), dan `rian@dev.local` (employee, Finance).
Unggah hanya tersedia untuk admin; akses dokumen selalu ditolak sampai grant
yang sesuai diberikan.

## Konfigurasi model AI

Kawandok memisahkan **LLM** (menyusun jawaban) dari **embedding** (mencari
potongan dokumen yang relevan). Keduanya dapat menggunakan provider yang
berbeda.

### Mode mock — default, tanpa biaya

```env
API_LLM_PROVIDER=mock
API_EMBEDDING_PROVIDER=mock
WORKER_EMBEDDING_PROVIDER=mock
```

Mode ini cocok untuk menguji upload, ACL, ingestion, sitasi, dan alur aplikasi.
Embedding mock deterministik tetapi tidak memahami makna bahasa secara penuh;
pencarian kata penuh tetap tersedia.

### Satu model OpenAI-compatible

Gunakan untuk Ollama lokal, OpenAI-compatible gateway, atau provider cloud
yang mendukung endpoint chat-completions dan embeddings.

```env
# LLM: satu model untuk menjawab chat
API_LLM_PROVIDER=openai
API_LLM_BASE_URL=https://provider.example/v1
API_LLM_API_KEY=ganti_dengan_kunci_anda
API_LLM_MODEL=nama-model-chat

# Embedding: API dan worker WAJIB sama
API_EMBEDDING_PROVIDER=openai
API_EMBEDDING_BASE_URL=https://provider.example/v1
API_EMBEDDING_API_KEY=ganti_dengan_kunci_anda
API_EMBEDDING_MODEL=nama-model-embedding
API_EMBEDDING_DIM=1536

WORKER_EMBEDDING_PROVIDER=openai
WORKER_EMBEDDING_BASE_URL=https://provider.example/v1
WORKER_EMBEDDING_API_KEY=ganti_dengan_kunci_anda
WORKER_EMBEDDING_MODEL=nama-model-embedding
WORKER_EMBEDDING_DIM=1536
```

Jangan menaruh key pada frontend atau commit key ke Git. Jika mengganti model
atau dimensi embedding pada database yang sudah ada, buat ulang environment
lokal atau rencanakan re-index/migrasi vector sebelum menjalankan worker.

### Round-robin dan failover multi-model

Mode `robin` memutar model **per request**. Jika satu endpoint gagal sebelum
streaming mengirim token, Kawandok mencoba endpoint berikutnya secara otomatis.
Setiap upaya dicatat di log API.

Aktifkan mode ini:

```env
API_LLM_PROVIDER=robin
```

Opsi yang direkomendasikan adalah grup Groq dan Gemini. Kedua grup harus diisi
bersamaan; model diputar secara selang-seling antar provider.

```env
API_LLM_ROBIN_GROQ_BASE_URL=https://api.groq.com/openai/v1
API_LLM_ROBIN_GROQ_API_KEY=ganti_dengan_kunci_groq
API_LLM_ROBIN_GROQ_MODELS=model-groq-a,model-groq-b

API_LLM_ROBIN_GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
API_LLM_ROBIN_GEMINI_API_KEY=ganti_dengan_kunci_gemini
API_LLM_ROBIN_GEMINI_MODELS=model-gemini-a,model-gemini-b
```

Untuk daftar endpoint bebas, gunakan format eksplisit berikut. Nilai ini
mengambil prioritas di atas konfigurasi grup.

```env
API_LLM_ROBIN=https://provider-a.example/v1|key-a|model-a;https://provider-b.example/v1|key-b|model-b
```

Restart API setelah setiap perubahan konfigurasi LLM. Round-robin hanya berlaku
untuk LLM; embedding harus tetap konsisten antara API dan worker.

## Perintah pengembangan

```sh
make test                         # membuat DB test terpisah lalu menjalankan seluruh test
make lint                         # Go vet, Ruff/Mypy (bila terpasang), ESLint
make secret-scan                  # gitleaks atau pemindai fallback
make migrate-version              # versi schema database aktif
make backup-db                    # backup PostgreSQL lokal terkompresi
make restore-db BACKUP_FILE=...   # restore ke database scratch
make loadtest N=200 C=10          # uji jalur GET terautentikasi
make down                         # hentikan container, volume tetap ada
```

Untuk menghapus seluruh data simulasi lokal, gunakan:

```sh
docker compose -f infra/docker-compose.yml down -v
```

Perintah tersebut menghapus PostgreSQL dan objek dokumen SeaweedFS lokal.

## Keamanan dan batasan

- Dokumen tidak disimpan di folder repository. Saat local development, dokumen
  ada pada volume Docker SeaweedFS dan metadata/vector berada pada volume
  PostgreSQL.
- Jangan memakai data perusahaan nyata sebelum provider, retention, backup,
  identity, dan persetujuan keamanan ditetapkan.
- Scanner ClamAV tersedia pada worker, tetapi nonaktif pada mode development.
  Production worker menuntut mode `clamav`.
- Production API sengaja menolak berjalan sampai autentikasi OIDC tersedia.
- Kawandok dirilis di bawah [Lisensi MIT](LICENSE).

Lihat [deployment notes](docs/DEPLOYMENT.md),
[security review](docs/SECURITY-REVIEW.md), dan
[baseline keamanan](docs/05-SECURITY.md) untuk detail.

## Struktur repository

```text
apps/web       Next.js: antarmuka pengguna
apps/api       Go: domain, autentikasi, ACL, chat, kontrak API
apps/worker    Python: parsing, chunking, embedding, ingestion
infra          Docker Compose, migration database, konfigurasi contoh
docs           keputusan arsitektur dan dokumentasi operasional
```

## Kontribusi

Kontribusi dipersilakan. Baca [CONTRIBUTING.md](CONTRIBUTING.md) untuk setup,
standar test, keamanan, dan alur pull request. Jangan mengunggah `.env`, API
key, data perusahaan, atau dokumen non-sintetis ke repository.
