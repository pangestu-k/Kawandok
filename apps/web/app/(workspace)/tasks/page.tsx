"use client";

import { Button, Typography } from "antd";
import Link from "next/link";

import { PlaceholderModule } from "../../../components/PlaceholderModule";

export default function TasksPage() {
  return (
    <PlaceholderModule
      title="Tasks"
      description="Manajemen tugas memerlukan penyimpanan tugas lintas perangkat di backend."
      icon={<Typography.Title level={2}>≡</Typography.Title>}
    >
      <Link href="/chat">
        <Button type="primary">Kembali ke Chat</Button>
      </Link>
    </PlaceholderModule>
  );
}
