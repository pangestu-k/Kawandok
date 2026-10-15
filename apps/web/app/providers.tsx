"use client";

import { ConfigProvider, theme as antdTheme, App as AntApp } from "antd";
import idID from "antd/locale/id_ID";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { useState, type ReactNode } from "react";

import { AuthProvider } from "../components/auth";

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            refetchOnWindowFocus: false,
            staleTime: 15_000,
          },
        },
      })
  );

  return (
    <AntdRegistry>
      <ConfigProvider
        locale={idID}
        theme={{
          algorithm: antdTheme.defaultAlgorithm,
          token: {
            colorPrimary: "#125872",
            colorInfo: "#1265aa",
            colorLink: "#176eb6",
            colorBgLayout: "#f5f7fa",
            colorText: "#13243c",
            colorTextSecondary: "#718097",
            colorBorder: "#dce4ed",
            colorBorderSecondary: "#e5eaf0",
            borderRadius: 8,
            fontFamily:
              "'DM Sans', -apple-system, 'Segoe UI', sans-serif",
          },
          components: {
            Layout: {
              siderBg: "transparent",
              headerBg: "#ffffff",
              bodyBg: "#f5f7fa",
            },
            Menu: {
              darkItemBg: "transparent",
              darkSubMenuItemBg: "transparent",
              darkItemSelectedBg: "#204a63",
              darkItemSelectedColor: "#7ce0f3",
              darkItemColor: "#d2ddeb",
              darkItemHoverBg: "#263e55",
            },
            Table: {
              headerBg: "#f6f8fa",
              headerColor: "#607188",
              rowHoverBg: "#f5f9fc",
            },
          },
        }}
      >
        <AntApp>
          <QueryClientProvider client={queryClient}>
            <AuthProvider>{children}</AuthProvider>
          </QueryClientProvider>
        </AntApp>
      </ConfigProvider>
    </AntdRegistry>
  );
}
