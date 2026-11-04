# Berkontribusi ke Kawandok

Terima kasih telah membantu mengembangkan Kawandok. Dokumen ini menjelaskan
alur kontribusi agar perubahan tetap aman, mudah ditinjau, dan dapat diuji.

## Sebelum mulai

1. Baca [README.md](README.md) untuk menyiapkan environment lokal.
2. Baca [baseline keamanan](docs/05-SECURITY.md), terutama aturan akses
   dokumen, data finansial, dan data sintetis.
3. Periksa issue yang ada sebelum membuat pekerjaan baru atau perubahan besar.

Jangan commit `.env`, API key, token, dokumen perusahaan, data pelanggan, atau
data finansial nyata. Gunakan fixture sintetis pada test dan contoh.

## Alur kontribusi

1. Fork repository lalu buat branch dari `main`:

   ```sh
   git checkout -b feat/nama-perubahan
   ```

2. Buat perubahan sekecil dan sejelas mungkin. Hindari mencampur refactor
   besar dengan perubahan fitur yang tidak terkait.
3. Tambahkan atau perbarui test bila perilaku berubah.
4. Jalankan pemeriksaan yang relevan:

   ```sh
   make test
   make lint
   make secret-scan
   ```

   `make test` membuat database test terpisah dan tidak boleh diarahkan ke
   database development yang berisi data simulasi Anda.

5. Perbarui dokumentasi, konfigurasi contoh, atau migration bila perubahan
   memang membutuhkannya.
6. Push branch lalu buat pull request ke `main`.

## Standar pull request

Jelaskan secara ringkas:

- masalah atau kebutuhan yang diselesaikan;
- perubahan utama dan dampaknya;
- perintah test yang dijalankan beserta hasilnya;
- migration, perubahan environment variable, atau tindakan operator yang
  diperlukan;
- risiko keamanan atau batasan yang tersisa.

Pull request yang mengubah otorisasi, penyimpanan dokumen, retrieval, chat,
atau data finansial wajib menyertakan test negatif untuk akses yang ditolak.
Jangan membiarkan model menjalankan SQL bebas atau mengubah catatan finansial.

## Konvensi proyek

- `apps/api`: Go; pemilik logika domain, autentikasi, ACL, kontrak API, dan
  query finansial.
- `apps/worker`: Python; parsing berkas, chunking, embedding, dan ingestion.
- `apps/web`: Next.js; tidak boleh memiliki kredensial cloud.
- `infra/db/migrations`: satu-satunya tempat untuk perubahan skema database.
- `contracts`: kontrak API bersama; perbarui saat route atau payload berubah.

Gunakan provider mock untuk test. Adapter provider cloud harus tetap berada di
belakang interface provider dan tidak boleh diperlukan pada test default.

## Commit dan review

Gunakan pesan commit ringkas dengan pola Conventional Commits, misalnya:

```text
feat: tambah filter status dokumen
fix: cegah citation tanpa hak akses
docs: perjelas konfigurasi round-robin
```

Reviewer akan memprioritaskan ketepatan ACL, integritas data finansial,
penanganan error/retry, test, dan dokumentasi dibanding ukuran fitur.

## Pelaporan kerentanan

Jangan membuka issue publik yang memuat credential, dokumen, langkah eksploit,
atau data sensitif. Hubungi maintainer repository melalui kanal privat yang
tertera pada profil organisasi/repository, sertakan dampak dan langkah
reproduksi yang disanitasi.

## Lisensi

Dengan mengirim kontribusi, Anda menyetujui agar kontribusi didistribusikan
dengan lisensi [MIT](LICENSE).
