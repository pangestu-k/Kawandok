"use client";

import {
  Avatar,
  Button,
  Input,
  Layout,
  Menu,
  Popconfirm,
  Typography,
} from "antd";
import {
  BookOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MessageOutlined,
  PieChartOutlined,
  SearchOutlined,
  TeamOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import Image from "next/image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";

import { useAuth, useMe } from "./auth";

const { Sider, Header, Content } = Layout;
const { Text } = Typography;

export function AppShell({ children }: { children: ReactNode }) {
  const { status, logout } = useAuth();
  const me = useMe();
  const pathname = usePathname();
  const router = useRouter();
  const [collapsed, setCollapsed] = useState(false);

  if (status === "loading") {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Text type="secondary">Memuat…</Text>
      </div>
    );
  }

  if (status === "anon") {
    if (typeof window !== "undefined") router.replace("/login");
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Text type="secondary">Mengalihkan ke halaman masuk…</Text>
      </div>
    );
  }

  const isAdmin = me.data?.is_admin === true;

  const navItems = [
    { key: "/chat", icon: <MessageOutlined />, label: link("/chat", "Chat") },
    { key: "/documents", icon: <BookOutlined />, label: link("/documents", "Knowledge Base") },
    { key: "/departments", icon: <TeamOutlined />, label: link("/departments", "Departemen") },
    ...(isAdmin
      ? [{ key: "/users", icon: <TeamOutlined />, label: link("/users", "Users & Roles") }]
      : []),
    { key: "/financial", icon: <WalletOutlined />, label: link("/financial", "Financial") },
    { key: "/reports", icon: <PieChartOutlined />, label: link("/reports", "Reports") },
  ];

  const selected = navItems.find(
    (item) => pathname === item.key || pathname.startsWith(item.key + "/")
  );

  const displayName = me.data?.display_name ?? me.data?.email ?? "…";

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider
        width={248}
        collapsedWidth={69}
        collapsed={collapsed}
        style={{ background: "linear-gradient(170deg, #11273f, #101f33)" }}
      >
        <div
          style={{
            height: "100%",
            display: "flex",
            flexDirection: "column",
            padding: collapsed ? "20px 8px" : "20px 13px",
            overflow: "auto",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              border: "1px solid #314660",
              borderRadius: 10,
              padding: collapsed ? "8px 4px" : 10,
              marginBottom: 22,
              justifyContent: collapsed ? "center" : "flex-start",
            }}
          >
            <Image
              src="/kawandok-mark.png"
              alt="Kawandok"
              width={38}
              height={38}
              priority
              style={{ width: 38, height: 38, objectFit: "contain", flexShrink: 0 }}
            />
            {!collapsed ? (
              <div>
                <div style={{ color: "#fff", fontWeight: 700, fontSize: 12 }}>
                  Kawandok
                </div>
                <div style={{ color: "#afc0d2", fontSize: 11 }}>Knowledge Workspace</div>
              </div>
            ) : null}
          </div>

          <Menu
            theme="dark"
            mode="inline"
            inlineCollapsed={collapsed}
            selectedKeys={selected ? [selected.key] : []}
            items={navItems}
            style={{ background: "transparent", borderInlineEnd: 0 }}
          />

          <div
            style={{
              marginTop: "auto",
              borderTop: "1px solid #31455a",
              paddingTop: 15,
              display: "flex",
              flexDirection: "column",
              gap: 8,
            }}
          >
            <Button
              ghost
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={() => setCollapsed((v) => !v)}
              style={{ textAlign: "left", color: "#cbd7e5", border: 0 }}
            >
              {!collapsed ? "Ciutkan menu" : null}
            </Button>
            <Popconfirm
              title="Keluar dari workspace?"
              okText="Keluar"
              cancelText="Batal"
              onConfirm={logout}
            >
              <Button
                ghost
                icon={<LogoutOutlined />}
                style={{ textAlign: "left", color: "#cbd7e5", border: 0 }}
              >
                {!collapsed ? "Keluar" : null}
              </Button>
            </Popconfirm>
          </div>
        </div>
      </Sider>

      <Layout>
        <Header
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 16,
            padding: "0 25px",
            borderBottom: "1px solid #e5eaf0",
          }}
        >
          <Input
            prefix={<SearchOutlined style={{ color: "#607189" }} />}
            placeholder="Cari dokumen, chat, atau topik…"
            style={{ maxWidth: 610, width: "65%", background: "#f2f5f8" }}
            onPressEnter={(e) => {
              const q = (e.target as HTMLInputElement).value.trim();
              router.push(q ? `/documents?q=${encodeURIComponent(q)}` : "/documents");
            }}
          />
          <div style={{ display: "flex", alignItems: "center", gap: 11 }}>
            <Avatar style={{ background: "#125872" }}>
              {(me.data?.display_name ?? "?").slice(0, 2).toUpperCase()}
            </Avatar>
            <div style={{ lineHeight: 1.2 }}>
              <Text strong style={{ display: "block", fontSize: 12 }}>
                {displayName}
              </Text>
              <Text type="secondary" style={{ fontSize: 10 }}>
                {me.data?.roles.join(", ") || "anggota"}
              </Text>
            </div>
          </div>
        </Header>
        <Content style={{ overflow: "auto", background: "#fff" }}>
          <div style={{ padding: 26, maxWidth: 1600, margin: "0 auto" }}>
            {children}
          </div>
        </Content>
      </Layout>
    </Layout>
  );
}

function link(href: string, label: string): ReactNode {
  return (
    <Link href={href} style={{ color: "inherit", textDecoration: "none" }}>
      {label}
    </Link>
  );
}
