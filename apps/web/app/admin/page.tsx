"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import {
  ApiError,
  Department,
  DocumentRow,
  downloadDocument,
  getDocument,
  getVersionStatus,
  isLoggedIn,
  listDepartments,
  listDocuments,
  reindexDocument,
  uploadDocument,
} from "../../lib/api";

type Grants = { departmentName: string; read: boolean; download: boolean };

const STATUS_OPTIONS = ["", "uploaded", "queued", "processing", "ready", "failed"] as const;

export default function AdminPage() {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [documents, setDocuments] = useState<DocumentRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [query, setQuery] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [title, setTitle] = useState("");
  const [classification, setClassification] = useState("internal");
  const [file, setFile] = useState<File | null>(null);
  const [departmentId, setDepartmentId] = useState("");
  const [grants, setGrants] = useState<Grants>({ departmentName: "Finance", read: true, download: false });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [detail, setDetail] = useState<DocumentRow | null>(null);
  const [detailBusy, setDetailBusy] = useState(false);

  useEffect(() => {
    if (!isLoggedIn()) {
      window.location.href = "/login";
    }
  }, []);

  const refresh = useCallback(
    async (q: string, status: string, pageNumber: number, append: boolean) => {
      try {
        const result = await listDocuments(q, status, pageNumber, 20);
        setDocuments((prev) => (append ? [...prev, ...result.items] : result.items));
        setTotal(result.total);
        setPage(pageNumber);
      } catch (err) {
        setError(describe(err));
      }
    },
    []
  );

  useEffect(() => {
    if (!isLoggedIn()) return;
    listDepartments()
      .then((depts) => {
        setDepartments(depts);
        setDepartmentId((current) => current || depts[0]?.id || "");
      })
      .catch(() => setDepartments([]));
    void refresh("", "", 1, false);
  }, [refresh]);

  useEffect(() => {
    const pending = documents.some((d) => d.status !== "ready" && d.status !== "failed");
    if (!pending) return;
    const timer = setInterval(() => void refresh(appliedQuery, statusFilter, 1, false), 2000);
    return () => clearInterval(timer);
  }, [documents, appliedQuery, statusFilter, refresh]);

  const changeFilter = (status: string) => {
    setStatusFilter(status);
    void refresh(appliedQuery, status, 1, false);
  };

  const search = () => {
    setAppliedQuery(query);
    void refresh(query, statusFilter, 1, false);
  };

  const loadMore = () => {
    void refresh(appliedQuery, statusFilter, page + 1, true);
  };

  const openDetail = async (doc: DocumentRow) => {
    setDetail(doc);
    setDetailBusy(true);
    try {
      const fresh = await getDocument(doc.id);
      setDetail(fresh);
    } catch (err) {
      setError(describe(err));
    } finally {
      setDetailBusy(false);
    }
  };

  const refreshDetailStatus = async () => {
    if (!detail) return;
    setDetailBusy(true);
    try {
      const status = await getVersionStatus(detail.id, detail.version_id);
      setDetail({ ...detail, status });
      await refresh(appliedQuery, statusFilter, 1, false);
    } catch (err) {
      setError(describe(err));
    } finally {
      setDetailBusy(false);
    }
  };

  const submitUpload = async () => {
    if (!file) {
      setError("Pilih file PDF/DOCX/XLSX terlebih dahulu.");
      return;
    }
    if (!title.trim()) {
      setError("Judul wajib diisi.");
      return;
    }
    const departmentIdValue = departmentId || departments[0]?.id || "";
    if (!departmentIdValue) {
      setError("Daftar departemen belum tersedia.");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const grantsJson = buildGrantsJson(grants);
      const result = await uploadDocument({
        file,
        title: title.trim(),
        classification,
        departmentId: departmentIdValue,
        grantsJson,
      });
      setNotice(`Terkirim: ${result.document_id} (status: ${result.status})`);
      setTitle("");
      setFile(null);
      const input = document.getElementById("file-input") as HTMLInputElement | null;
      if (input) input.value = "";
      await refresh(appliedQuery, statusFilter, 1, false);
    } catch (err) {
      setError(describe(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="admin">
      <section className="card">
        <h2>Unggah Dokumen</h2>
        <div className="grid">
          <label>
            Judul
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Kebijakan cuti 2026" />
          </label>
          <label>
            Klasifikasi
            <select value={classification} onChange={(e) => setClassification(e.target.value)}>
              <option value="internal">internal</option>
              <option value="confidential">confidential</option>
              <option value="restricted">restricted</option>
            </select>
          </label>
          <label>
            Departemen pemilik
            <select value={departmentId} onChange={(e) => setDepartmentId(e.target.value)}>
              {departments.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            File (PDF/DOCX/XLSX)
            <input
              id="file-input"
              type="file"
              accept=".pdf,.docx,.xlsx"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </label>
        </div>
        <fieldset className="grants">
          <legend>Beri akses (deny-by-default)</legend>
          <label>
            Departemen penerima
            <input
              value={grants.departmentName}
              onChange={(e) => setGrants({ ...grants, departmentName: e.target.value })}
            />
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={grants.read}
              onChange={(e) => setGrants({ ...grants, read: e.target.checked })}
            />
            baca
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={grants.download}
              onChange={(e) => setGrants({ ...grants, download: e.target.checked })}
            />
            unduh
          </label>
        </fieldset>
        <button type="button" onClick={() => void submitUpload()} disabled={busy}>
          {busy ? "Mengunggah…" : "Unggah"}
        </button>
        {notice ? <p className="ok">{notice}</p> : null}
        {error ? <p className="error">{error}</p> : null}
      </section>

      <section className="card">
        <h2>Daftar Dokumen{total ? ` (${total})` : ""}</h2>
        <div className="searchrow">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Cari judul / nama file…"
            onKeyDown={(e) => {
              if (e.key === "Enter") search();
            }}
          />
          <select value={statusFilter} onChange={(e) => changeFilter(e.target.value)}>
            {STATUS_OPTIONS.map((s) => (
              <option key={s || "all"} value={s}>
                {s === "" ? "semua status" : s}
              </option>
            ))}
          </select>
          <button type="button" onClick={search}>
            Cari
          </button>
        </div>
        <table>
          <thead>
            <tr>
              <th>Judul</th>
              <th>File</th>
              <th>Status</th>
              <th>Aksi</th>
            </tr>
          </thead>
          <tbody>
            {documents.length === 0 ? (
              <tr>
                <td colSpan={4} className="muted">
                  Belum ada dokumen yang dapat Anda akses.
                </td>
              </tr>
            ) : null}
            {documents.map((d) => (
              <tr key={d.id}>
                <td>
                  <button type="button" className="linklike" onClick={() => void openDetail(d)}>
                    {d.title}
                  </button>
                </td>
                <td>{d.filename}</td>
                <td>
                  <span className={`badge ${d.status}`}>{d.status}</span>
                </td>
                <td>
                  <button
                    type="button"
                    onClick={() => void downloadDocument(d.id, d.filename)}
                    disabled={d.status !== "ready"}
                  >
                    Unduh
                  </button>{" "}
                  <button type="button" onClick={() => void reindexDocument(d.id)}>
                    Reindex
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {documents.length < total ? (
          <button type="button" className="wide" onClick={loadMore}>
            Muat lebih banyak ({documents.length}/{total})
          </button>
        ) : null}
      </section>

      {detail ? (
        <div className="modal-backdrop" onClick={() => setDetail(null)}>
          <div className="modal card" onClick={(e) => e.stopPropagation()}>
            <h3>Detail Dokumen</h3>
            {detailBusy ? <p className="muted">memuat…</p> : null}
            <dl className="detail">
              <dt>Judul</dt>
              <dd>{detail.title}</dd>
              <dt>File</dt>
              <dd>{detail.filename}</dd>
              <dt>Klasifikasi</dt>
              <dd>{detail.classification}</dd>
              <dt>Status versi aktif</dt>
              <dd>
                <span className={`badge ${detail.status}`}>{detail.status}</span>{" "}
                <button type="button" className="ghost" onClick={() => void refreshDetailStatus()} disabled={detailBusy}>
                  Perbarui status
                </button>
              </dd>
              <dt>ID Dokumen</dt>
              <dd className="mono">{detail.id}</dd>
              <dt>ID Versi</dt>
              <dd className="mono">{detail.version_id}</dd>
              <dt>Ukuran</dt>
              <dd>{formatBytes(detail.size_bytes)}</dd>
              <dt>Dibuat</dt>
              <dd>{new Date(detail.created_at).toLocaleString("id-ID")}</dd>
            </dl>
            <div className="modal-actions">
              <button
                type="button"
                onClick={() => void downloadDocument(detail.id, detail.filename)}
                disabled={detail.status !== "ready"}
              >
                Unduh
              </button>
              <button type="button" className="ghost" onClick={() => setDetail(null)}>
                Tutup
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <p className="muted pad">
        Kembali ke <Link href="/chat">Chat</Link>.
      </p>
    </main>
  );
}

function buildGrantsJson(grants: Grants): string {
  const entries: Array<Record<string, string>> = [];
  const dept = grants.departmentName.trim();
  if (dept && grants.read) entries.push({ principal_type: "department", principal_name: dept, action: "read" });
  if (dept && grants.download) entries.push({ principal_type: "department", principal_name: dept, action: "download" });
  return JSON.stringify(entries);
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}
