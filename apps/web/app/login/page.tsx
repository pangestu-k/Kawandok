"use client";

import { FormEvent, useEffect, useState } from "react";

import { ApiError, isLoggedIn, loginAs } from "../../lib/api";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (isLoggedIn()) {
      window.location.href = "/chat";
    }
  }, []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const value = email.trim().toLowerCase();
    if (!value || !value.includes("@")) {
      setError("Masukkan email development yang valid (contoh: admin@dev.local).");
      return;
    }
    try {
      await loginAs(value);
      window.location.href = "/chat";
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Tidak dapat masuk.");
    }
  };

  return (
    <main className="center">
      <form className="card login" onSubmit={submit}>
        <h1>Masuk</h1>
        <p className="muted">
          Mode development: token JWT diterbitkan untuk email seeded user
          (admin@dev.local, siti@dev.local, budi@dev.local, rian@dev.local).
        </p>
        <label htmlFor="email">Email</label>
        <input
          id="email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="admin@dev.local"
          autoComplete="username"
        />
        {error ? <p className="error">{error}</p> : null}
        <button type="submit">Masuk</button>
      </form>
    </main>
  );
}
