"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import {
  ApiError,
  Department,
  DocumentRow,
  downloadDocument,
  isLoggedIn,
  listDepartments,
  listDocuments,
  reindexDocument,
  uploadDocument,
} from "../../lib/api";

type Grants = { departmentName: string; read: boolean; download: boolean };

export default function AdminPage() {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [documents, setDocuments] = useState<DocumentRow[]>([]);
  const [query, setQuery] = useState("");
  const [title, setTitle] = useState("");
  const [classification, setClassification] = useState("internal");
  const [file, setFile] = useState<File | null>(null);
  const [departmentId, setDepartmentId] = useState("");
  const [grants, setGrants] = useState<Grants>({ departmentName: "Finance", read: true, download: false });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (!isLoggedIn()) {
      window.location.href = "/login";
    }
  }, []);

  const refresh = useCallback(async (q: string) => {
    try {
      const result = await listDocuments(q, "");
      setDocuments(result.items);
    } catch (err) {
      setError(describe(err));
    }
  }, []);

  useEffect(() => {
    if (!isLoggedIn()) return;
    listDepartments()
      .then((depts) => {
        setDepartments(depts);
        setDepartmentId((current) => current || depts[0]?.id || "");
      })
      .catch(() => setDepartments([]));
    void refresh("");
  }, [refresh]);

  useEffect(() => {
    const pending = documents.some((d) => d.status !== "ready" && d.status !== "failed");
    if (!pending) return;
    const timer = setInterval(() => void refresh(query), 2000);
    return () => clearInterval(timer);
  }, [documents, query, refresh]);

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
      await refresh(query);
    } catch (err) {
      setError(describe(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="admin">
      <nav className="topnav">
        <strong>Admin Dokumen</strong>
        <span className="spacer" />
        <Link href="/chat">Chat</Link>
        <Link href="/login">Ganti User</Link>
      </nav>

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
        <h2>Daftar Dokumen</h2>
        <div className="searchrow">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Cari judul / nama file…"
          />
          <button type="button" onClick={() => void refresh(query)}>
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
                <td>{d.title}</td>
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
      </section>
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

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}
