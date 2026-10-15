"use client";

import {
  App,
  Avatar,
  Button,
  Input,
  Modal,
  Popconfirm,
  Tooltip,
  Typography,
} from "antd";
import {
  DeleteOutlined,
  EditOutlined,
  MessageOutlined,
  PlusOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import {
  ApiError,
  clearSessionId,
  createSession,
  deleteSession,
  getSessionId,
  listSessions,
  renameSession,
  setSessionId,
} from "../lib/api";

const { Text } = Typography;

type SessionActions = {
  onSelected: (sessionId: string) => void;
  activeSessionId: string | null;
};

export function SessionSidebar({
  onSelected,
  activeSessionId,
}: SessionActions) {
  const { message: toast } = App.useApp();
  const queryClient = useQueryClient();
  const [renaming, setRenaming] = useState<{ id: string; title: string } | null>(null);
  const [renameTitle, setRenameTitle] = useState("");

  const sessionsQuery = useQuery({
    queryKey: ["chat-sessions"],
    queryFn: listSessions,
  });

  useEffect(() => {
    const onChangeSession = () => {
      void queryClient.invalidateQueries({ queryKey: ["chat-sessions"] });
    };
    window.addEventListener("office-ai:sessions-changed", onChangeSession);
    return () => window.removeEventListener("office-ai:sessions-changed", onChangeSession);
  }, [queryClient]);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["chat-sessions"] });
    window.dispatchEvent(new Event("office-ai:sessions-changed"));
  };

  const createMutation = useMutation({
    mutationFn: async () => {
      const id = await createSession();
      setSessionId(id);
      return id;
    },
    onSuccess: (id) => {
      invalidate();
      onSelected(id);
    },
    onError: (err) => toast.error(describe(err)),
  });

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      renameSession(id, title),
    onSuccess: () => {
      setRenaming(null);
      invalidate();
      toast.success("Judul percakapan diperbarui");
    },
    onError: (err) => toast.error(describe(err)),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSession(id),
    onSuccess: (_data, id) => {
      if (getSessionId() === id) {
        clearSessionId();
        onSelected("");
      }
      invalidate();
      toast.success("Percakapan dihapus");
    },
    onError: (err) => toast.error(describe(err)),
  });

  const sessions = sessionsQuery.data ?? [];

  return (
    <aside className="session-rail">
      <Button
        type="primary"
        block
        icon={<PlusOutlined />}
        loading={createMutation.isPending}
        onClick={() => createMutation.mutate()}
      >
        Sesi Baru
      </Button>

      <div className="session-list">
        {sessionsQuery.isLoading ? (
          <Text type="secondary" style={{ fontSize: 12 }}>
            memuat…
          </Text>
        ) : null}
        {!sessionsQuery.isLoading && sessions.length === 0 ? (
          <Text type="secondary" style={{ fontSize: 12 }}>
            Belum ada percakapan.
          </Text>
        ) : null}
        {sessions.map((s) => {
          const active = s.session_id === activeSessionId;
          return (
            <div
              key={s.session_id}
              className={`session-item ${active ? "active" : ""}`}
              onClick={() => onSelected(s.session_id)}
            >
              <MessageOutlined style={{ marginTop: 3, flexShrink: 0 }} />
              <div className="session-copy">
                <Text strong ellipsis style={{ fontSize: 12 }}>
                  {s.title || "Tanpa judul"}
                </Text>
                <Text type="secondary" style={{ fontSize: 10 }}>
                  {s.messages} pesan
                </Text>
              </div>
              <span className="session-actions" onClick={(e) => e.stopPropagation()}>
                <Tooltip title="Ganti judul">
                  <Button
                    type="text"
                    size="small"
                    icon={<EditOutlined />}
                    onClick={() => {
                      setRenaming({ id: s.session_id, title: s.title || "" });
                      setRenameTitle(s.title || "");
                    }}
                  />
                </Tooltip>
                <Popconfirm
                  title="Hapus percakapan ini?"
                  okText="Hapus"
                  okButtonProps={{ danger: true }}
                  cancelText="Batal"
                  onConfirm={() => deleteMutation.mutate(s.session_id)}
                >
                  <Button type="text" size="small" danger icon={<DeleteOutlined />} />
                </Popconfirm>
              </span>
            </div>
          );
        })}
      </div>

      <Modal
        open={renaming !== null}
        title="Ganti judul percakapan"
        okText="Simpan"
        cancelText="Batal"
        onCancel={() => setRenaming(null)}
        onOk={() => {
          if (renaming && renameTitle.trim()) {
            renameMutation.mutate({ id: renaming.id, title: renameTitle.trim() });
          }
        }}
        destroyOnClose
      >
        <Input
          value={renameTitle}
          onChange={(e) => setRenameTitle(e.target.value)}
          maxLength={200}
          placeholder="Judul percakapan"
        />
      </Modal>
    </aside>
  );
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}
