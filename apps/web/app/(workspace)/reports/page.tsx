"use client";

import { Card, Col, Progress, Row, Statistic, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";

import { listDocuments } from "../../../lib/api";

const { Title, Text } = Typography;

export default function ReportsPage() {
  const documentsQuery = useQuery({
    queryKey: ["documents", "reports"],
    queryFn: () => listDocuments("", "", 1, 100),
  });

  const items = documentsQuery.data?.items ?? [];
  const total = documentsQuery.data?.total ?? 0;
  const byStatus = ["ready", "queued", "processing", "uploaded", "failed"].map(
    (status) => ({
      status,
      count: items.filter((d) => d.status === status).length,
    })
  );
  const sum = byStatus.reduce((acc, s) => acc + s.count, 0) || 1;

  return (
    <div>
      <Title level={3} style={{ marginBottom: 4 }}>
        Reports
      </Title>
      <Text type="secondary">
        Ringkasan status dokumen yang dapat Anda akses (data nyata dari API).
      </Text>

      <Row gutter={[16, 16]} style={{ marginTop: 20 }}>
        <Col xs={12} md={6}>
          <Card variant="borderless">
            <Statistic title="Total dokumen" value={total} />
          </Card>
        </Col>
        {byStatus
          .filter((s) => s.status === "ready" || s.status === "failed")
          .map((s) => (
            <Col xs={12} md={6} key={s.status}>
              <Card variant="borderless">
                <Statistic
                  title={s.status === "ready" ? "Siap dicari" : "Gagal indexing"}
                  value={s.count}
                  valueStyle={{
                    color: s.status === "ready" ? "#16845b" : "#be434d",
                  }}
                />
              </Card>
            </Col>
          ))}
      </Row>

      <Card variant="borderless" style={{ marginTop: 20 }}>
        <Title level={5}>Distribusi status indexing</Title>
        {byStatus.map((s) => (
          <div key={s.status} style={{ marginBottom: 10 }}>
            <Text style={{ fontSize: 12 }}>{s.status}</Text>
            <Progress
              percent={Math.round((s.count / sum) * 100)}
              size="small"
              format={() => `${s.count}`}
              strokeColor={
                s.status === "ready"
                  ? "#16845b"
                  : s.status === "failed"
                    ? "#be434d"
                    : "#1265aa"
              }
            />
          </div>
        ))}
        <Text type="secondary" style={{ fontSize: 11 }}>
          Statistik dihitung dari maksimum 100 dokumen pertama dalam cakupan
          akses Anda.
        </Text>
      </Card>
    </div>
  );
}
