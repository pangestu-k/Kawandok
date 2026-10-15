import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";

import AdminPage from "../app/admin/page";
import LoginPage from "../app/login/page";
import Nav from "../app/Nav";
import ChatPage from "../app/page";

describe("web smoke", () => {
  it("renders the chat placeholder", () => {
    const html = renderToString(createElement(ChatPage));
    expect(html).toContain("Sesi baru");
    expect(html).toContain("Tulis pertanyaan");
  });

  it("renders the login page", () => {
    const html = renderToString(createElement(LoginPage));
    expect(html).toContain("Masuk");
    expect(html).toContain("admin@dev.local");
  });

  it("renders the admin page", () => {
    const html = renderToString(createElement(AdminPage));
    expect(html).toContain("Unggah Dokumen");
    expect(html).toContain("Daftar Dokumen");
    expect(html).toContain("deny-by-default");
  });

  it("renders nav identically on server and first client render", () => {
    const html = renderToString(createElement(Nav));
    expect(html).toContain("Office AI Assistant");
    expect(html).toContain("Masuk");
  });
});
