"use client";

import {
  Button,
  Card,
  Checkbox,
  Descriptions,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import { DownloadOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import {
  ApiError,
  Department,
  DocumentRow,
  downloadDocument,
  getDocument,
  getVersionStatus,
  listDepartments,
  listDocuments,
  reindexDocument,
  uploadDocument,
} from "../../../lib/api";

const { Text, Title } = Typography;

const STATUS_TABS = [
  { key: "", label: "Semua" },
  { key: "uploaded", label: "Uploaded" },
  { key: "queued", label: "Queued" },
  { key: "processing", label: "Processing" },
  { key: "ready", label: "Ready" },
  { key: "failed", label: "Failed" },
];

function statusTag(status: string) {
  const color =
    status === "ready" ? "green" : status === "failed" ? "red" : "gold";
  return <Tag color={color}>{status}</Tag>;
}

export default function DocumentsPage() {
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [detail, setDetail] = useState<DocumentRow | null>(null);
  const [error, setError] = useState("");

  const documentsQuery = useQuery({
    queryKey: ["documents", query, status],
    queryFn: () => listDocuments(query, status, 1, 50),
  });

  const departmentsQuery = useQuery({
    queryKey: ["departments"],
    queryFn: listDepartments,
    staleTime: 5 * 60_000,
  });

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["documents"] });
  };

  const rows = documentsQuery.data?.items ?? [];
  const total = documentsQuery.data?.total ?? 0;

  return (
    <div>
      <div className="page-heading">
        <div className="row">
          <div>
            <Title level={3} style={{ margin: 0 }}>
              Knowledge Base
            </Title>
            <Text type="secondary">
              Dokumen perusahaan yang dapat Anda akses â€” unggah, telusuri,
              unduh, dan pantau status indexing.
            </Text>
          </div>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => setUploadOpen(true)}
          >
            Unggah Dokumen
          </Button>
        </div>
      </div>

      <Tabs
        activeKey={status}
        items={STATUS_TABS.map((t) => ({ key: t.key, label: t.label }))}
        onChange={(key) => setStatus(key)}
      />

      <Space.Compact style={{ width: "100%", maxWidth: 480, marginBottom: 16 }}>
        <Input
          placeholder="Cari judul atau nama fileâ€¦"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onPressEnter={() => refresh()}
          allowClear
        />
        <Button onClick={() => refresh()}>Cari</Button>
      </Space.Compact>

      {error ? <p className="error">{error}</p> : null}

      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<DocumentRow>
          rowKey="id"
          loading={documentsQuery.isLoading}
          dataSource={rows}
          pagination={{ pageSize: 10, total, showTotal: (t) => `${t} dokumen` }}
          onRow={(record) => ({
            onClick: () => void openDetail(record, setDetail, setError),
            style: { cursor: "pointer" },
          })}
          columns={[
            {
              title: "Dokumen",
              dataIndex: "title",
              render: (title: string, row) => (
                <div>
                  <Text strong>{title}</Text>
                  <br />
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {row.filename}
                  </Text>
                </div>
              ),
            },
            {
              title: "Klasifikasi",
              dataIndex: "classification",
              render: (v: string) => (
                <Tag color={v === "restricted" ? "red" : v === "confidential" ? "purple" : "blue"}>
                  {v}
                </Tag>
              ),
            },
            { title: "Status", dataIndex: "status", render: statusTag },
            {
              title: "Ukuran",
              dataIndex: "size_bytes",
              render: (v: number) => formatBytes(v),
            },
            {
              title: "Dibuat",
              dataIndex: "created_at",
              render: (v: string) => new Date(v).toLocaleString("id-ID"),
            },
            {
              title: "Aksi",
              key: "actions",
              render: (_, row) => (
                <Space onClick={(e) => e.stopPropagation()}>
                  <Button
                    size="small"
                    icon={<DownloadOutlined />}
                    disabled={row.status !== "ready"}
                    onClick={() => void downloadDocument(row.id, row.filename)}
                  >
                    Unduh
                  </Button>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    onClick={async () => {
                      try {
                        await reindexDocument(row.id);
                        refresh();
                      } catch (err) {
                        setError(describe(err));
                      }
                    }}
                  >
                    Reindex
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <UploadModal
        open={uploadOpen}
        departments={departmentsQuery.data ?? []}
        departmentsLoading={departmentsQuery.isLoading || departmentsQuery.isFetching}
        departmentHint={
          departmentsQuery.isError ? (
            <span style={{ color: "#b42318" }}>
              Gagal memuat daftar departemen: {describe(departmentsQuery.error)}. Coba muat ulang
              halaman.
            </span>
          ) : departmentsQuery.isSuccess && (departmentsQuery.data ?? []).length === 0 ? (
            <span style={{ color: "#b42318" }}>
              Belum ada departemen. Isi data dev (jalankan API dengan API_DEV_SEED=true) atau
              minta admin membuat departemen.
            </span>
          ) : undefined
        }
        onClose={() => setUploadOpen(false)}
        onUploaded={() => {
          setUploadOpen(false);
          refresh();
        }}
      />

      <Modal
        open={detail !== null}
        title="Detail Dokumen"
        footer={null}
        onCancel={() => setDetail(null)}
      >
        {detail ? (
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="Judul">{detail.title}</Descriptions.Item>
            <Descriptions.Item label="File">{detail.filename}</Descriptions.Item>
            <Descriptions.Item label="Klasifikasi">
              {detail.classification}
            </Descriptions.Item>
            <Descriptions.Item label="Status">{statusTag(detail.status)}</Descriptions.Item>
            <Descriptions.Item label="Ukuran">{formatBytes(detail.size_bytes)}</Descriptions.Item>
            <Descriptions.Item label="Dibuat">
              {new Date(detail.created_at).toLocaleString("id-ID")}
            </Descriptions.Item>
            <Descriptions.Item label="ID Versi">
              <Text code copyable style={{ fontSize: 11 }}>
                {detail.version_id}
              </Text>
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  );
}

async function openDetail(
  row: DocumentRow,
  setDetail: (d: DocumentRow) => void,
  setError: (e: string) => void
) {
  try {
    setDetail(row);
    const fresh = await getDocument(row.id);
    setDetail(fresh);
  } catch (err) {
    setError(describe(err));
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return `${err.code}: ${err.message}`;
  return String(err);
}

type UploadForm = {
  title: string;
  classification: string;
  departmentId: string;
  grantDepartments: string[];
  grantRead: boolean;
  grantDownload: boolean;
};

function UploadModal({
  open,
  departments,
  departmentsLoading,
  departmentHint,
  onClose,
  onUploaded,
}: {
  open: boolean;
  departments: Department[];
  departmentsLoading?: boolean;
  departmentHint?: React.ReactNode;
  onClose: () => void;
  onUploaded: () => void;
}) {
  const [form] = Form.useForm<UploadForm>();
  const [file, setFile] = useState<File | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      form.resetFields();
      // The owning department defaults to the first one so the field is never
      // submitted empty; the user can still pick another department.
      form.setFieldsValue({
        classification: "internal",
        grantRead: true,
        grantDownload: false,
        departmentId: departments[0]?.id,
        grantDepartments: [],
      });
      setFile(null);
      setError("");
    }
  }, [open, form, departments]);

  const submit = async () => {
    const values = await form.validateFields();
    if (!file) {
      setError("Pilih file PDF/DOCX/XLSX terlebih dahulu.");
      return;
    }
    const departmentId = values.departmentId || departments[0]?.id || "";
    if (!departmentId) {
      setError("Daftar departemen belum tersedia.");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const grants: Array<Record<string, string>> = [];
      for (const dept of values.grantDepartments ?? []) {
        const name = dept.trim();
        if (!name) continue;
        if (values.grantRead)
          grants.push({ principal_type: "department", principal_name: name, action: "read" });
        if (values.grantDownload)
          grants.push({ principal_type: "department", principal_name: name, action: "download" });
      }
      await uploadDocument({
        file,
        title: values.title.trim(),
        classification: values.classification,
        departmentId,
        grantsJson: JSON.stringify(grants),
      });
      onUploaded();
    } catch (err) {
      setError(describe(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      title="Unggah Dokumen"
      okText="Unggah"
      cancelText="Batal"
      okButtonProps={{ loading: submitting }}
      onOk={() => void submit()}
      onCancel={onClose}
      destroyOnClose
    >
      <Form<UploadForm> form={form} layout="vertical">
        <Form.Item
          name="title"
          label="Judul"
          rules={[{ required: true, message: "Judul wajib diisi" }]}
        >
          <Input placeholder="Kebijakan cuti 2026" maxLength={512} />
        </Form.Item>
        <Form.Item name="classification" label="Klasifikasi">
          <Select
            options={[
              { value: "internal", label: "internal" },
              { value: "confidential", label: "confidential" },
              { value: "restricted", label: "restricted" },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="departmentId"
          label="Departemen pemilik"
          extra={departmentHint}
          rules={[{ required: true, message: "Pilih departemen pemilik" }]}
        >
          <Select
            loading={departmentsLoading}
            placeholder={
              departmentsLoading ? "Memuat departemen..." : "Pilih departemen pemilik"
            }
            options={departments.map((d) => ({
              value: d.id,
              label: `${d.name} (${d.user_count} anggota)`,
            }))}
            notFoundContent={
              departmentsLoading ? "Memuat..." : "Belum ada departemen"
            }
          />
        </Form.Item>
        <Form.Item label="File (PDF/DOCX/XLSX, maks 25 MB)">
          <input
            id="file-input"
            type="file"
            accept=".pdf,.docx,.xlsx"
            style={{ width: "100%" }}
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </Form.Item>
        <fieldset className="grants">
          <legend className="small muted">Beri akses (deny-by-default)</legend>
          <Form.Item
            name="grantDepartments"
            label="Departemen penerima"
            extra="Boleh kosong. Kosong berarti hanya departemen pemilik yang bisa membaca."
          >
            <Select
              mode="multiple"
              allowClear
              loading={departmentsLoading}
              placeholder="Pilih departemen yang boleh membaca"
              options={departments.map((d) => ({ value: d.name, label: d.name }))}
              optionFilterProp="label"
              notFoundContent={
                departmentsLoading ? "Memuat..." : "Belum ada departemen"
              }
            />
          </Form.Item>
          <Space>
            <Form.Item name="grantRead" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Checkbox>baca</Checkbox>
            </Form.Item>
            <Form.Item name="grantDownload" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Checkbox>unduh</Checkbox>
            </Form.Item>
          </Space>
        </fieldset>
        {error ? <p className="error">{error}</p> : null}
      </Form>
    </Modal>
  );
}
