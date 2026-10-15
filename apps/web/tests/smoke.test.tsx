import { createElement, type ReactNode } from "react";
import { useState } from "react";
import { renderToString } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigProvider } from "antd";
import { describe, expect, it } from "vitest";

import { AuthProvider } from "../components/auth";
import ApprovalsPage from "../app/(workspace)/approvals/page";
import ChatPage from "../app/(workspace)/chat/page";
import DocumentsPage from "../app/(workspace)/documents/page";
import LoginPage from "../app/login/page";
import ReportsPage from "../app/(workspace)/reports/page";

function TestProviders({ children }: { children: ReactNode }) {
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
  );
  return (
    <QueryClientProvider client={client}>
      <ConfigProvider theme={{ token: { colorPrimary: "#125872" } }}>
        <AuthProvider>{children}</AuthProvider>
      </ConfigProvider>
    </QueryClientProvider>
  );
}

function renderPage(node: ReactNode): string {
  return renderToString(createElement(TestProviders, null, node));
}

describe("web smoke (antd + tanstack query)", () => {
  it("renders the login page (split-screen design)", () => {
    const html = renderPage(createElement(LoginPage));
    expect(html).toContain("Selamat datang kembali");
    expect(html).toContain("admin@dev.local");
  });

  it("renders the chat page shell", () => {
    const html = renderPage(createElement(ChatPage));
    expect(html).toContain("Chat Baru");
    expect(html).toContain("chat-compose");
  });

  it("renders the documents page", () => {
    const html = renderPage(createElement(DocumentsPage));
    expect(html).toContain("Knowledge Base");
    expect(html).toContain("Unggah Dokumen");
  });

  it("renders reports with real data sections", () => {
    const html = renderPage(createElement(ReportsPage));
    expect(html).toContain("Distribusi status");
  });

  it("renders placeholder modules without faking features", () => {
    const approvals = renderPage(createElement(ApprovalsPage));
    expect(approvals).toContain("membutuhkan backend");
  });
});
