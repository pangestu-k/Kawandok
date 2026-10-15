"use client";

import { Button, Typography } from "antd";
import Link from "next/link";

import { PlaceholderModule } from "../../../components/PlaceholderModule";

export default function ApprovalsPage() {
  return (
    <PlaceholderModule
      title="Approvals"
      description="Persetujuan internal (cuti, pembelian, reimbursment) memerlukan entitas workflow di backend."
      icon={<Typography.Title level={2}>✓</Typography.Title>}
    >
      <Link href="/chat">
        <Button type="primary">Kembali ke Chat</Button>
      </Link>
    </PlaceholderModule>
  );
}
