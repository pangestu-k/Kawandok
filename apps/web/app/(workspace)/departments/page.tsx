"use client";

import { Card, Table, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";

import { listDepartments } from "../../../lib/api";

const { Title, Text } = Typography;

export default function DepartmentsPage() {
  const departmentsQuery = useQuery({
    queryKey: ["departments"],
    queryFn: listDepartments,
  });

  const departments = departmentsQuery.data ?? [];

  return (
    <div>
      <Title level={3} style={{ marginBottom: 4 }}>
        Departemen
      </Title>
      <Text type="secondary">Daftar departemen perusahaan beserta jumlah anggota.</Text>

      <Card variant="borderless" styles={{ body: { padding: 0 } }} style={{ marginTop: 20 }}>
        <Table
          rowKey="id"
          loading={departmentsQuery.isLoading}
          dataSource={departments}
          pagination={false}
          columns={[
            { title: "Nama Departemen", dataIndex: "name" },
            {
              title: "Jumlah Anggota",
              dataIndex: "user_count",
              width: 180,
            },
            {
              title: "ID",
              dataIndex: "id",
              render: (v: string) => (
                <Typography.Text code copyable style={{ fontSize: 11 }}>
                  {v}
                </Typography.Text>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
