"use client";

import { Button, Card, Form, Input } from "antd";
import { useEffect, useState } from "react";

import { useAuth } from "../../components/auth";
import { ApiError } from "../../lib/api";

type LoginValues = { email: string };

const FEATURES = [
  {
    title: "Tanya dokumen perusahaan",
    body: "Jawaban berbasis sumber: setiap respons mencantumkan dokumen dan halaman asalnya.",
  },
  {
    title: "Laporan keuangan yang terukur",
    body: "Angka dihitung dari data tervalidasi di database — bukan ditebak oleh model.",
  },
  {
    title: "Keamanan berlapis",
    body: "Akses dokumen dan angka keuangan selalu diperiksa ulang per pengguna.",
  },
];

export default function LoginPage() {
  const { status, login } = useAuth();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (status === "authed") window.location.href = "/chat";
  }, [status]);

  const submit = async (values: LoginValues) => {
    setSubmitting(true);
    setError("");
    try {
      await login(values.email.trim().toLowerCase());
      window.location.href = "/chat";
    } catch (err) {
      setError(
        err instanceof ApiError
          ? `${err.code}: ${err.message}`
          : "Tidak dapat masuk — pastikan API berjalan dan email terdaftar."
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="login-page">
      <section className="login-art">
        <div className="login-brand">
          <span className="brand-mark">K</span>
          <span>
            KantorHub <span className="muted">/ Workspace</span>
          </span>
        </div>
        <h1>
          Asisten kantor yang memahami dokumen dan angka perusahaan Anda.
        </h1>
        <p>
          Satu tempat untuk bertanya pada dokumen internal, menelusuri
          knowledge base, dan meminta ringkasan keuangan yang terverifikasi.
        </p>
        <div className="feature-list">
          {FEATURES.map((f) => (
            <div className="feature" key={f.title}>
              <span className="feature-icon">✦</span>
              <div>
                <b>{f.title}</b>
                <p>{f.body}</p>
              </div>
            </div>
          ))}
        </div>
        <div className="login-art-bottom">
          KantorHub · Office AI Assistant (development)
        </div>
      </section>

      <section className="login-right">
        <Card className="login-card" variant="borderless">
          <div className="row">
            <span className="brand-mark" style={{ background: "#125872" }}>
              K
            </span>
            <div>
              <b>PT. Nusantara Karya</b>
              <p className="small" style={{ margin: 0 }}>
                Workspace
              </p>
            </div>
          </div>
          <h1 style={{ fontSize: 26, margin: "24px 0 6px" }}>
            Selamat datang kembali
          </h1>
          <p style={{ marginBottom: 24 }}>
            Masuk menggunakan email workspace terdaftar.
          </p>

          <Form<LoginValues> layout="vertical" onFinish={submit} requiredMark={false}>
            <Form.Item
              name="email"
              label="Email perusahaan"
              rules={[
                { required: true, message: "Email wajib diisi" },
                { type: "email", message: "Format email tidak valid" },
              ]}
            >
              <Input
                size="large"
                placeholder="nama@perusahaan.co.id"
                autoComplete="username"
              />
            </Form.Item>
            {error ? <p className="error">{error}</p> : null}
            <Button
              type="primary"
              htmlType="submit"
              block
              size="large"
              loading={submitting}
            >
              Masuk
            </Button>
          </Form>

          <p className="tiny muted" style={{ marginTop: 18 }}>
            Mode development: masuk memakai email seeded user — admin@dev.local,
            siti@dev.local (finance), budi@dev.local, rian@dev.local.
          </p>
        </Card>
      </section>
    </div>
  );
}
