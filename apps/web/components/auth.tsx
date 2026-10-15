"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import { getToken, loginAs, logout as clearAuth } from "../lib/api";

export type AuthStatus = "loading" | "authed" | "anon";

type AuthContextValue = {
  status: AuthStatus;
  email: string | null;
  login: (email: string) => Promise<void>;
  logout: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const EMAIL_KEY = "office-ai-email";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [email, setEmail] = useState<string | null>(null);

  useEffect(() => {
    const token = getToken();
    const stored = window.localStorage.getItem(EMAIL_KEY);
    if (token && stored) {
      setEmail(stored);
      setStatus("authed");
    } else {
      setStatus("anon");
    }
  }, []);

  const login = useCallback(async (value: string) => {
    await loginAs(value);
    window.localStorage.setItem(EMAIL_KEY, value.trim().toLowerCase());
    setEmail(value.trim().toLowerCase());
    setStatus("authed");
  }, []);

  const logout = useCallback(() => {
    clearAuth();
    window.localStorage.removeItem(EMAIL_KEY);
    setEmail(null);
    setStatus("anon");
    window.location.href = "/login";
  }, []);

  const value = useMemo(
    () => ({ status, email, login, logout }),
    [status, email, login, logout]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
