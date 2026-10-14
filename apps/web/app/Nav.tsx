"use client";

import Link from "next/link";

import { isLoggedIn, logout } from "../lib/api";

export default function Nav() {
  const signOut = () => {
    logout();
    window.location.href = "/login";
  };

  return (
    <nav className="topnav">
      <strong>Office AI Assistant</strong>
      <span className="spacer" />
      <Link href="/chat">Chat</Link>
      <Link href="/admin">Admin Dokumen</Link>
      {isLoggedIn() ? (
        <button type="button" className="link" onClick={signOut}>
          Keluar
        </button>
      ) : (
        <Link href="/login">Masuk</Link>
      )}
    </nav>
  );
}
