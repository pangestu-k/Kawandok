"use client";

import { Result } from "antd";
import type { ReactNode } from "react";

export function PlaceholderModule({
  title,
  description,
  icon,
  children,
}: {
  title: string;
  description: string;
  icon?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Result
      icon={icon}
      title={title}
      subTitle={
        <>
          {description}
          <br />
          Modul ini tersedia di desain workspace namun membutuhkan backend yang
          belum diimplementasikan pada API saat ini — antarmuka tidak dibuat
          semata-mata agar tidak menampilkan fitur palsu.
        </>
      }
      extra={children}
    />
  );
}
