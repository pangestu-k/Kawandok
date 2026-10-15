"use client";

import { Card, Table, Tabs, Tag, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";

import { listRoles, listUsers } from "../../../lib/api";

const { Title, Text } = Typography;

function statusTag(status: string) {
  return (
    <Tag color={status === "active" ? "green" : status === "disabled" ? "red" : "gold"}>
      {status}
    </Tag>
  );
}

function roleTag(role: string) {
  const color =
    role === "admin" ? "red" : role === "finance" ? "purple" : role === "employee" ? "blue" : "gray";
  return <Tag color={color}>{role}</Tag>;
}

export default function UsersPage() {
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: listUsers });
  const rolesQuery = useQuery({ queryKey: ["roles"], queryFn: listRoles });

  const users = usersQuery.data ?? [];
  const roles = rolesQuery.data ?? [];

  return (
    <div>
      <Title level={3} style={{ marginBottom: 4 }}>
        Users &amp; Roles
      </Title>
      <Text type="secondary">
        Kelola visibilitas anggota workspace, peran, dan departemen mereka
        (hanya admin; hanya-baca pada fase ini).
      </Text>

      <Card variant="borderless" style={{ marginTop: 20 }}>
        <Tabs
          items={[
            {
              key: "users",
              label: `Users (${users.length})`,
              children: (
                <Table
                  rowKey="id"
                  loading={usersQuery.isLoading}
                  dataSource={users}
                  pagination={{ pageSize: 10 }}
                  columns={[
                    {
                      title: "Nama",
                      dataIndex: "display_name",
                      render: (v: string, row) => (
                        <div>
                          <Text strong>{v}</Text>
                          <br />
                          <Text type="secondary" style={{ fontSize: 11 }}>
                            {row.email}
                          </Text>
                        </div>
                      ),
                    },
                    {
                      title: "Departemen",
                      dataIndex: "department",
                    },
                    {
                      title: "Roles",
                      dataIndex: "roles",
                      render: (roles: string[]) => (
                        <span>
                          {(roles ?? []).map((r) => (
                            <span key={r}>{roleTag(r)} </span>
                          ))}
                          {(roles ?? []).length === 0 ? <Text type="secondary">-</Text> : null}
                        </span>
                      ),
                    },
                    { title: "Status", dataIndex: "status", render: statusTag },
                  ]}
                />
              ),
            },
            {
              key: "roles",
              label: `Roles (${roles.length})`,
              children: (
                <Table
                  rowKey="id"
                  loading={rolesQuery.isLoading}
                  dataSource={roles}
                  pagination={false}
                  columns={[
                    { title: "Role", dataIndex: "name", render: (v: string) => roleTag(v) },
                    { title: "Jumlah Pengguna", dataIndex: "user_count", width: 200 },
                  ]}
                />
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
