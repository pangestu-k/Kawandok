export const TOKEN_KEY = "office-ai-token";
export const SESSION_KEY = "office-ai-session";

export type Citation = {
  ref: string;
  document_id: string;
  version_id: string;
  title: string;
  page_start?: number | null;
  page_end?: number | null;
  section_ref?: string | null;
  snippet: string;
};

export type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  body: string;
  citations?: Citation[];
  created_at: string;
};

export type DocumentRow = {
  id: string;
  title: string;
  classification: string;
  department_id: string;
  created_at: string;
  version_id: string;
  status: string;
  filename: string;
  size_bytes: number;
};

export type Department = { id: string; name: string };

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TOKEN_KEY);
}

export function loginAs(email: string): void {
  window.localStorage.setItem(TOKEN_KEY, `dev-${email.trim()}`);
}

export function logout(): void {
  window.localStorage.removeItem(TOKEN_KEY);
  window.localStorage.removeItem(SESSION_KEY);
}

export function isLoggedIn(): boolean {
  return getToken() !== null;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function parseError(resp: Response): Promise<ApiError> {
  let code = "REQUEST_FAILED";
  let message = `HTTP ${resp.status}`;
  try {
    const body = await resp.json();
    if (body?.error?.code) code = body.error.code;
    if (body?.error?.message) message = body.error.message;
  } catch {
    // keep defaults
  }
  return new ApiError(resp.status, code, message);
}

async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const resp = await fetch(`/api${path}`, {
    ...init,
    headers: { ...authHeaders(), ...(init?.headers ?? {}) },
  });
  if (!resp.ok) throw await parseError(resp);
  return resp;
}

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await apiFetch(path, init);
  return (await resp.json()) as T;
}

export async function listDepartments(): Promise<Department[]> {
  const body = await jsonFetch<{ departments: Department[] }>("/v1/departments");
  return body.departments ?? [];
}

export async function createSession(title?: string): Promise<string> {
  const body = await jsonFetch<{ session_id: string }>("/v1/chat/sessions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title: title ?? "" }),
  });
  return body.session_id;
}

export async function fetchMessages(sessionId: string): Promise<ChatMessage[]> {
  const body = await jsonFetch<{ messages: ChatMessage[] }>(
    `/v1/chat/sessions/${sessionId}/messages`
  );
  return body.messages ?? [];
}

export async function listDocuments(query: string, status: string): Promise<{ items: DocumentRow[]; total: number }> {
  const params = new URLSearchParams();
  if (query) params.set("query", query);
  if (status) params.set("status", status);
  return jsonFetch(`/v1/documents?${params.toString()}`);
}

export async function uploadDocument(input: {
  file: File;
  title: string;
  classification: string;
  departmentId: string;
  grantsJson: string;
}): Promise<{ document_id: string; version_id: string; status: string }> {
  const form = new FormData();
  form.append("file", input.file);
  form.append("title", input.title);
  form.append("classification", input.classification);
  form.append("department_id", input.departmentId);
  if (input.grantsJson.trim()) form.append("grants", input.grantsJson.trim());
  return jsonFetch("/v1/documents", { method: "POST", body: form });
}

export async function getVersionStatus(
  documentId: string,
  versionId: string
): Promise<string> {
  const body = await jsonFetch<{ status: string }>(
    `/v1/documents/${documentId}/versions/${versionId}/status`
  );
  return body.status;
}

export async function reindexDocument(documentId: string): Promise<void> {
  await apiFetch(`/v1/documents/${documentId}/reindex`, { method: "POST" });
}

export async function downloadDocument(documentId: string, filename: string): Promise<void> {
  const resp = await apiFetch(`/v1/documents/${documentId}/download`);
  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

export interface StreamHandlers {
  onDelta: (text: string) => void;
  onCitation: (citation: Citation) => void;
  onStatus: (stage: string) => void;
  onError: (code: string, message: string) => void;
  onDone: (citations: Citation[], replayed: boolean) => void;
}

export async function streamChatMessage(
  sessionId: string,
  message: string,
  clientRequestId: string,
  handlers: StreamHandlers
): Promise<void> {
  const resp = await fetch(`/api/v1/chat/sessions/${sessionId}/messages`, {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify({ message, client_request_id: clientRequestId }),
  });

  if (!resp.ok || !resp.body) {
    throw await parseError(resp);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  const dispatch = (frame: string) => {
    let eventType = "message";
    let dataRaw = "";
    for (const line of frame.split("\n")) {
      if (line.startsWith("event: ")) eventType = line.slice(7).trim();
      else if (line.startsWith("data: ")) dataRaw = line.slice(6);
    }
    if (!dataRaw) return;
    let data: Record<string, unknown>;
    try {
      data = JSON.parse(dataRaw);
    } catch {
      return;
    }
    switch (eventType) {
      case "text_delta":
        handlers.onDelta(String(data.delta ?? ""));
        break;
      case "citation":
        handlers.onCitation(data as unknown as Citation);
        break;
      case "tool_status":
        handlers.onStatus(String(data.stage ?? ""));
        break;
      case "error":
        handlers.onError(String(data.code ?? "UNKNOWN"), String(data.message ?? ""));
        break;
      case "done":
        handlers.onDone((data.citations as Citation[]) ?? [], Boolean(data.replayed));
        break;
    }
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      if (frame.trim()) dispatch(frame);
    }
  }
  if (buffer.trim()) dispatch(buffer);
}
