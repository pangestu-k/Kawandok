import type { Metadata } from "next";
import type { ReactNode } from "react";

import Nav from "./Nav";

import "./globals.css";

export const metadata: Metadata = {
  title: "Office AI Assistant",
  description: "Permission-aware internal company assistant",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="id">
      <body>
        <Nav />
        {children}
      </body>
    </html>
  );
}
