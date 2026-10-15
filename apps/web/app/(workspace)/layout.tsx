"use client";

import { RequireAuth } from "../../components/RequireAuth";

export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  return <RequireAuth>{children}</RequireAuth>;
}
