"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  ApiError,
  ChatMessage,
  Citation,
  createSession,
  fetchMessages,
  isLoggedIn,
  streamChatMessage,
} from "../lib/api";

type Draft = { text: string; citations: Citation[] };

function getSessionId(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem("office-ai-session");
}

function setSessionId(id: string) {
  window.localStorage.setItem("office-ai-session", id);
}

function mergeCitation(existing: Citation[], citation: Citation): Citation[] {
  if (existing.some((c) => c.ref === citation.ref)) return existing;
  return [...existing, citation];
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}

export default function ChatPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState<Draft>({ text: "", citations: [] });
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [statusStage, setStatusStage] = useState("");
  const [error, setError] = useState("");
  const [loggedIn, setLoggedIn] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);
  const draftRef = useRef("");

  draftRef.current = draft.text;

  useEffect(() => {
    if (!isLoggedIn()) {
      setLoggedIn(false);
      window.location.href = "/login";
      return;
    }
    const sessionId = getSessionId();
    if (!sessionId) return;
    fetchMessages(sessionId)
      .then(setMessages)
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) {
          setSessionId("");
        } else {
          setError(describe(err));
        }
      });
  }, []);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, draft]);

  const send = async () => {
    const text = input.trim();
    if (!text || streaming) return;
    setError("");
    setInput("");

    let sessionId = getSessionId();
    try {
      if (!sessionId) {
        sessionId = await createSession();
        setSessionId(sessionId);
      }
    } catch (err) {
      setError(describe(err));
      return;
    }

    setMessages((prev) => [
      ...prev,
      {
        id: `local-${Date.now()}`,
        role: "user",
        body: text,
        created_at: new Date().toISOString(),
      },
    ]);
    setStreaming(true);
    setDraft({ text: "", citations: [] });
    draftRef.current = "";

    const clientRequestId =
      typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : `req-${Date.now()}`;

    try {
      await streamChatMessage(sessionId, text, clientRequestId, {
        onStatus: (stage) => setStatusStage(stage),
        onDelta: (delta) => setDraft((prev) => ({ ...prev, text: prev.text + delta })),
        onCitation: (citation) =>
          setDraft((prev) => ({ ...prev, citations: mergeCitation(prev.citations, citation) })),
        onError: (code, message) => setError(`${code}: ${message}`),
        onDone: (citations) => {
          setMessages((prev) => [
            ...prev,
            {
              id: `assistant-${Date.now()}`,
              role: "assistant",
              body: draftRef.current || "(tanpa jawaban)",
              citations,
              created_at: new Date().toISOString(),
            },
          ]);
          setDraft({ text: "", citations: [] });
          setStatusStage("");
        },
      });
    } catch (err) {
      setError(describe(err));
    } finally {
      setStreaming(false);
      setStatusStage("");
    }
  };

  if (!loggedIn) {
    return <main className="center muted">Mengalihkan ke halaman masuk…</main>;
  }

  return (
    <main className="chat">
      <nav className="topnav">
        <strong>Office AI Assistant</strong>
        <span className="spacer" />
        <Link href="/admin">Admin Dokumen</Link>
        <Link href="/login">Ganti User</Link>
      </nav>

      <div className="messages">
        {messages.length === 0 && !streaming ? (
          <p className="muted">
            Ajukan pertanyaan tentang dokumen yang dapat Anda akses, atau minta
            ringkasan keuangan dengan tanggal (contoh: &quot;Berapa total
            pendapatan departemen Finance 2026-01-01 2026-02-28?&quot;).
          </p>
        ) : null}
        {messages.map((m) => (
          <div key={m.id} className={`bubble ${m.role}`}>
            <div className="body">{m.body}</div>
            {m.citations && m.citations.length > 0 ? (
              <div className="citations">
                {m.citations.map((c) => (
                  <span key={c.ref} className="citation" title={c.snippet}>
                    [{c.ref}] {c.title}
                    {c.page_start ? ` (hlm. ${c.page_start})` : ""}
                    {c.section_ref ? ` (${c.section_ref})` : ""}
                  </span>
                ))}
              </div>
            ) : null}
          </div>
        ))}
        {streaming ? (
          <div className="bubble assistant streaming">
            {statusStage ? <div className="status">menyiapkan: {statusStage}…</div> : null}
            {draft.text ? <div className="body">{draft.text}</div> : null}
            {draft.citations.map((c) => (
              <div key={c.ref} className="citations">
                <span className="citation">
                  [{c.ref}] {c.title}
                </span>
              </div>
            ))}
          </div>
        ) : null}
        <div ref={bottomRef} />
      </div>

      {error ? <p className="error pad">{error}</p> : null}

      <form
        className="composer"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Tulis pertanyaan Anda…"
          disabled={streaming}
        />
        <button type="submit" disabled={streaming || !input.trim()}>
          Kirim
        </button>
      </form>
    </main>
  );
}
