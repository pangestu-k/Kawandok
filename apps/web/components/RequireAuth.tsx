"use client";

import { Skeleton } from "antd";
import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";

import { useAuth } from "./auth";
import { AppShell } from "./AppShell";

export function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (status === "anon") router.replace("/login");
  }, [status, router]);

  if (status !== "authed") {
    return <Skeleton active style={{ maxWidth: 720, margin: "40px auto" }} />;
  }

  return <AppShell>{children}</AppShell>;
}
