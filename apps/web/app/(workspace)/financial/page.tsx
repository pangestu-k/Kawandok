"use client";

import {
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Row,
  Select,
  Statistic,
  Table,
  Typography,
} from "antd";
import { useQuery } from "@tanstack/react-query";
import type { Dayjs } from "dayjs";

import {
  ApiError,
  financeCompare,
  financeSummary,
  listDepartments,
  type FinanceComparison,
  type FinanceSummary,
} from "../../../lib/api";

const { Title, Text } = Typography;

type FormValues = {
  department: string;
  summaryRange: [Dayjs, Dayjs];
  compareA: [Dayjs, Dayjs];
  compareB: [Dayjs, Dayjs];
};

function formatAmount(currency: string, minor: number): string {
  if (!currency) return "-";
  const sign = minor < 0 ? "-" : "";
  const abs = Math.abs(minor);
  const major = Math.floor(abs / 100)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return `${sign}${currency} ${major},${String(abs % 100).padStart(2, "0")}`;
}

export default function FinancialPage() {
  const [form] = Form.useForm<FormValues>();
  const departmentsQuery = useQuery({ queryKey: ["departments"], queryFn: listDepartments });
  const department = Form.useWatch("department", form);
  const summaryRange = Form.useWatch("summaryRange", form);
  const compareA = Form.useWatch("compareA", form);
  const compareB = Form.useWatch("compareB", form);

  const dept = department ?? "";
  const summaryQuery = useQuery({
    queryKey: ["finance-summary", dept, summaryRange?.[0]?.valueOf(), summaryRange?.[1]?.valueOf()],
    queryFn: () =>
      financeSummary({
        department: dept,
        start: summaryRange![0].format("YYYY-MM-DD"),
        end: summaryRange![1].format("YYYY-MM-DD"),
      }),
    enabled: Boolean(summaryRange && summaryRange[0] && summaryRange[1]),
  });

  const compareQuery = useQuery({
    queryKey: [
      "finance-compare",
      dept,
      compareA?.[0]?.valueOf(),
      compareA?.[1]?.valueOf(),
      compareB?.[0]?.valueOf(),
      compareB?.[1]?.valueOf(),
    ],
    queryFn: () =>
      financeCompare({
        department: dept,
        aStart: compareA![0].format("YYYY-MM-DD"),
        aEnd: compareA![1].format("YYYY-MM-DD"),
        bStart: compareB![0].format("YYYY-MM-DD"),
        bEnd: compareB![1].format("YYYY-MM-DD"),
      }),
    enabled: Boolean(
      compareA && compareA[0] && compareA[1] && compareB && compareB[0] && compareB[1]
    ),
  });

  const summary = summaryQuery.data;
  const comparison = compareQuery.data;
  const financeError =
    summaryQuery.error instanceof ApiError
      ? (summaryQuery.error as ApiError)
      : compareQuery.error instanceof ApiError
        ? (compareQuery.error as ApiError)
        : null;

  return (
    <div>
      <Title level={3} style={{ marginBottom: 4 }}>
        Financial
      </Title>
      <Text type="secondary">
        Angka dihitung dari transaksi tervalidasi di database (read-only,
        ter-audit, dibatasi hak akses departemen Anda).
      </Text>

      <Card variant="borderless" style={{ marginTop: 20 }}>
        <Form form={form} layout="inline" initialValues={{ department: "" }}>
          <Form.Item name="department" label="Cakupan">
            <Select
              style={{ minWidth: 220 }}
              loading={departmentsQuery.isLoading}
              options={[
                { value: "", label: "Departemen saya" },
                { value: "all", label: "Semua departemen" },
                ...(departmentsQuery.data ?? []).map((d) => ({
                  value: d.name,
                  label: d.name,
                })),
              ]}
            />
          </Form.Item>
        </Form>
        {financeError ? (
          <p className="error" style={{ marginTop: 12 }}>
            {financeError.code}: {financeError.message}
          </p>
        ) : null}
      </Card>

      <Card variant="borderless" title="Ringkasan periode" style={{ marginTop: 20 }}>
        <Form form={form} layout="inline">
          <Form.Item name="summaryRange" label="Periode">
            <DatePicker.RangePicker />
          </Form.Item>
        </Form>

        {summaryQuery.isFetching ? (
          <Text type="secondary" style={{ display: "block", marginTop: 12 }}>
            memuatâ€¦
          </Text>
        ) : null}
        {summary ? (
          <>
            <Row gutter={16} style={{ marginTop: 12 }}>
              <Col xs={12} md={8}>
                <Statistic
                  title={`Total (${summary.currency || "-"})`}
                  value={formatAmount(summary.currency, summary.total_minor_units)}
                />
              </Col>
            </Row>
            <Table
              style={{ marginTop: 16 }}
              rowKey="category"
              size="small"
              pagination={false}
              dataSource={summary.by_category.map((c) => ({ ...c, key: c.category }))}
              columns={[
                { title: "Kategori", dataIndex: "category" },
                {
                  title: "Total",
                  dataIndex: "total_minor_units",
                  render: (v: number) => formatAmount(summary.currency, v),
                },
              ]}
            />
          </>
        ) : null}
        {!summary && !summaryQuery.isFetching ? (
          <Text type="secondary" style={{ display: "block", marginTop: 12 }}>
            Pilih periode untuk melihat ringkasan.
          </Text>
        ) : null}
      </Card>

      <Card variant="borderless" title="Perbandingan dua periode" style={{ marginTop: 20 }}>
        <Form form={form} layout="inline">
          <Form.Item name="compareA" label="Periode A">
            <DatePicker.RangePicker />
          </Form.Item>
          <Form.Item name="compareB" label="Periode B">
            <DatePicker.RangePicker />
          </Form.Item>
        </Form>

        {compareQuery.isFetching ? (
          <Text type="secondary" style={{ display: "block", marginTop: 12 }}>
            memuatâ€¦
          </Text>
        ) : null}
        {comparison ? (
          <Row gutter={16} style={{ marginTop: 12 }}>
            <Col xs={12} md={6}>
              <Statistic
                title="Periode A"
                value={formatAmount(comparison.currency, comparison.first_period_minor_units)}
              />
            </Col>
            <Col xs={12} md={6}>
              <Statistic
                title="Periode B"
                value={formatAmount(comparison.currency, comparison.second_period_minor_units)}
              />
            </Col>
            <Col xs={12} md={6}>
              <Statistic
                title="Selisih"
                value={formatAmount(comparison.currency, comparison.delta_minor_units)}
              />
            </Col>
            <Col xs={12} md={6}>
              <Statistic
                title="Persentase"
                value={
                  comparison.delta_percent != null
                    ? `${comparison.delta_percent > 0 ? "+" : ""}${comparison.delta_percent}%`
                    : "-"
                }
              />
            </Col>
          </Row>
        ) : null}
        {!comparison && !compareQuery.isFetching ? (
          <Text type="secondary" style={{ display: "block", marginTop: 12 }}>
            Pilih dua periode untuk membandingkan.
          </Text>
        ) : null}
      </Card>

      <Text type="secondary" style={{ fontSize: 11, display: "block", marginTop: 12 }}>
        Sumber angka: agregat SQL predefined atas financial_transactions
        tervalidasi â€” model AI tidak pernah menghitung angka keuangan.
      </Text>
    </div>
  );
}
