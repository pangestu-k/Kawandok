import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";

import AdminPage from "../app/admin/page";
import HomePage from "../app/page";
import LoginPage from "../app/login/page";

describe("web smoke", () => {
  it("renders the home page", () => {
    const html = renderToString(createElement(HomePage));
    expect(html).toContain("Office AI Assistant");
  });

  it("renders the login page", () => {
    const html = renderToString(createElement(LoginPage));
    expect(html).toContain("Login");
  });

  it("renders the admin page", () => {
    const html = renderToString(createElement(AdminPage));
    expect(html).toContain("Admin");
  });
});
