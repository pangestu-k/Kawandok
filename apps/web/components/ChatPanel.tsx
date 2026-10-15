"use client";

import {
  Avatar,
  Button,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { PaperClipOutlined, SendOutlined, UserOutlined } from "@ant-design/icons";
import { useEffect, useRef, useState } from "react";

import {
  ApiError,
  Citation,
  ChatMessage,
  clearSessionId,
  createSession,
  fetchMessages,
  getSessionId,
  isLoggedIn,
  newRequestId,
  setSessionId,
  streamChatMessage,
} from "../lib/api";

const { Text } = Typography;

type Draft = { text: string; citations: Citation[] };

const PROMPTS = [
  "Ringkas dokumen kebijakan yang tersedia",
  "Berapa total pendapatan departemen Finance bulan ini?",
  "Apa isi SOP pengajuan cuti?",
  "Bandingkan pendapatan dua bulan terakhir",
];

export function ChatPanel() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState<Draft>({ text: "", citations: [] });
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [statusStage, setStatusStage] = useState("");
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [hasSession, setHasSession] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const draftRef = useRef("");
  const composerRef = useRef<HTMLTextAreaElement>(null);

  draftRef.current = draft.text;

  useEffect(() => {
    if (!isLoggedIn()) {
      window.location.href = "/login";
      return;
    }
    const sessionId = getSessionId();
    if (!sessionId) {
      setReady(true);
      return;
    }
    setHasSession(true);
    fetchMessages(sessionId)
      .then((history) => {
        setMessages(history);
        setReady(true);
      })
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) {
          clearSessionId();
          setHasSession(false);
        } else {
          setError(describe(err));
        }
        setReady(true);
      });
  }, []);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, draft]);

  const startNewSession = () => {
    clearSessionId();
    setHasSession(false);
    setMessages([]);
    setDraft({ text: "", citations: [] });
    setError("");
    composerRef.current?.focus();
  };

  const send = async (raw?: string) => {
    const text = (raw ?? input).trim();
    if (!text || streaming) return;
    setError("");
    setInput("");

    let sessionId = getSessionId();
    try {
      if (!sessionId) {
        sessionId = await createSession();
        setSessionId(sessionId);
        setHasSession(true);
      }
    } catch (err) {
      setError(describe(err));
      return;
    }

    const now = new Date().toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit" });
    setMessages((prev) => [
      ...prev,
      { id: `local-${Date.now()}`, role: "user", body: text, created_at: now },
    ]);
    setStreaming(true);
    setDraft({ text: "", citations: [] });
    draftRef.current = "";

    try {
      await streamChatMessage(sessionId, text, newRequestId(), {
        onStatus: (stage) => setStatusStage(stage),
        onDelta: (delta) => setDraft((prev) => ({ ...prev, text: prev.text + delta })),
        onCitation: (citation) =>
          setDraft((prev) => ({
            ...prev,
            citations: prev.citations.some((c) => c.ref === citation.ref)
              ? prev.citations
              : [...prev.citations, citation],
          })),
        onError: (code, msg) => setError(`${code}: ${msg}`),
        onDone: (citations) => {
          setMessages((prev) => [
            ...prev,
            {
              id: `assistant-${Date.now()}`,
              role: "assistant",
              body: draftRef.current || "(tanpa jawaban)",
              citations,
              created_at: now,
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

  return (
    <div className="chat-layout">
      <section className="chat-center">
        <div className="chat-head">
          <div className="row">
            <Avatar shape="square" style={{ background: "linear-gradient(140deg,#0b4a68,#0d8890)" }}>
              ✦
            </Avatar>
            <div>
              <Typography.Title level={4} style={{ margin: 0 }}>
                {hasSession ? "Percakapan aktif" : "Chat Baru"}
              </Typography.Title>
              <Text type="secondary" style={{ fontSize: 12 }}>
                Diskusi dengan AI Assistant · jawaban dilengkapi sitasi dokumen
              </Text>
            </div>
          </div>
          {hasSession ? (
            <Button onClick={startNewSession} type="primary" ghost>
              Percakapan baru
            </Button>
          ) : null}
        </div>

        <div className="chat-scroll" ref={scrollRef}>
          {!ready ? (
            <Text type="secondary">Memuat…</Text>
          ) : messages.length === 0 && !streaming ? (
            <div className="empty-chat">
              <div className="ai-avatar" style={{ margin: "0 auto 14px", width: 52, height: 52 }}>
                ✦
              </div>
              <Typography.Title level={3}>Apa yang ingin Anda kerjakan?</Typography.Title>
              <p>
                Tanyakan isi dokumen yang dapat Anda akses, atau minta ringkasan
                keuangan dengan tanggal (contoh: 2026-01-01 2026-02-28).
              </p>
              <div className="prompt-grid">
                {PROMPTS.map((p) => (
                  <button
                    key={p}
                    type="button"
                    className="prompt-tile"
                    onClick={() => void send(p)}
                  >
                    {p} →
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <>
              {messages.map((m) => (
                <article key={m.id} className={`message ${m.role === "user" ? "user" : ""}`}>
                  {m.role === "user" ? (
                    <Avatar icon={<UserOutlined />} style={{ background: "#125872" }} />
                  ) : (
                    <div className="ai-avatar">✦</div>
                  )}
                  <div className="message-content">
                    <div className="message-head">
                      {m.role === "user" ? "Anda" : "AI Assistant"}{" "}
                      <time>{formatTime(m.created_at)}</time>
                    </div>
                    <div className="bubble">{m.body}</div>
                    {m.citations && m.citations.length > 0 ? (
                      <div className="citations">
                        {m.citations.map((c) => (
                          <Tooltip key={c.ref} title={c.snippet}>
                            <Tag color="blue">
                              [{c.ref}] {c.title}
                              {c.page_start ? ` · hlm. ${c.page_start}` : ""}
                              {c.section_ref ? ` · ${c.section_ref}` : ""}
                            </Tag>
                          </Tooltip>
                        ))}
                      </div>
                    ) : null}
                  </div>
                </article>
              ))}
              {streaming ? (
                <article className="message">
                  <div className="ai-avatar">✦</div>
                  <div className="message-content">
                    <div className="message-head">
                      AI Assistant {statusStage ? <time>· {statusStage}…</time> : null}
                    </div>
                    <div className="bubble">
                      {draft.text || <Text type="secondary">menyusun jawaban…</Text>}
                    </div>
                    {draft.citations.map((c) => (
                      <div className="citations" key={c.ref}>
                        <Tag color="blue">
                          [{c.ref}] {c.title}
                        </Tag>
                      </div>
                    ))}
                  </div>
                </article>
              ) : null}
            </>
          )}
        </div>

        {error ? <p className="error pad">{error}</p> : null}

        <form
          className="chat-compose"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <div className="composer">
            <Tooltip title="Lampiran (belum tersedia)">
              <Button type="text" icon={<PaperClipOutlined />} disabled />
            </Tooltip>
            <textarea
              ref={composerRef}
              rows={1}
              value={input}
              disabled={streaming}
              placeholder="Ketik pertanyaan Anda… (Enter untuk kirim)"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void send();
                }
              }}
            />
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={() => void send()}
              loading={streaming}
              disabled={!input.trim()}
            />
          </div>
          <div className="tiny muted" style={{ marginTop: 7, textAlign: "center" }}>
            Jawaban selalu dilengkapi sumber dokumen; angka keuangan dihitung dari
            data tervalidasi.
          </div>
        </form>
      </section>

      <aside className="chat-rail">
        <div className="right-card">
          <h3>Cara memakai</h3>
          <p className="small">
            Ajukan pertanyaan berbahasa sehari-hari. Untuk angka keuangan,
            sertakan tanggal berformat YYYY-MM-DD dan nama departemen bila
            diperlukan.
          </p>
        </div>
        <div className="right-card">
          <h3>Sitasi</h3>
          <p className="small">
            Setiap jawaban berbasis dokumen mencantumkan sumbernya. Klik tag
            sitasi untuk melihat cuplikan aslinya.
          </p>
        </div>
        <div className="right-card">
          <h3>Keamanan</h3>
          <p className="small">
            Anda hanya akan melihat dokumen dan angka yang berada dalam cakupan
            akses Anda; percobaan di luar itu ditolak dan dicatat.
          </p>
        </div>
      </aside>
    </div>
  );
}

function formatTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit" });
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}
