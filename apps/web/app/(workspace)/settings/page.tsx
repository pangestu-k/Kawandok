"use client";

import { Button, Typography } from "antd";
import Link from "next/link";

import { PlaceholderModule } from "../../../components/PlaceholderModule";

export default function SettingsPage() {
  return (
    <PlaceholderModule
      title="Settings"
      description="Pengaturan workspace, tim, dan integrasi memerlukan endpoint manajemen (admin) yang belum tersedia."
      icon={<Typography.Title level={2}>⚙</Typography.Title>}
    >
      <Link href="/chat">
        <Button type="primary">Kembali ke Chat</Button>
      </Link>
    </PlaceholderModule>
  );
}
