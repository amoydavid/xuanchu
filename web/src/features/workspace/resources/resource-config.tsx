import type React from "react"

import type { PageKey } from "@/components/AppShell"
import {
  ResourcePage,
  statusCell,
  textCell,
  userCell,
} from "@/pages/ResourcePage"

type Translate = (key: string) => string

export function resourceConfig(
  page: PageKey,
  t: Translate,
  workspaceSlug?: string
): React.ComponentProps<typeof ResourcePage> {
  switch (page) {
    case "workspaces":
      return {
        title: t("page.workspaces"),
        path: "/api/v1/workspaces",
        columns: [
          { key: "slug", header: t("resource.slug"), render: textCell("slug") },
          { key: "name", header: t("common.name"), render: textCell("name") },
          {
            key: "role",
            header: t("resource.role"),
            render: statusCell("role"),
          },
        ],
      }
    case "members":
      return {
        enabled: Boolean(workspaceSlug),
        title: t("page.members"),
        path: `/api/v1/workspaces/${workspaceSlug ?? ""}/members`,
        columns: [
          {
            key: "name",
            header: t("common.actor"),
            render: (row) => {
              const name = String(row["name"] ?? "")
              const email = row["email"] ? String(row["email"]) : ""
              return email ? `${name} (${email})` : name
            },
          },
          {
            key: "role",
            header: t("resource.role"),
            render: statusCell("role"),
          },
        ],
      }
    case "tokens":
      return {
        title: t("page.tokens"),
        path: "/api/v1/tokens",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          {
            key: "type",
            header: t("resource.type"),
            render: statusCell("type"),
          },
          {
            key: "prefix",
            header: t("resource.prefix"),
            render: textCell("prefix"),
          },
        ],
      }
    case "hooks":
      return {
        title: t("page.hooks"),
        path: "/api/v1/hooks",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          {
            key: "scope_type",
            header: t("overview.scope"),
            render: statusCell("scope_type"),
          },
          {
            key: "enabled",
            header: t("resource.enabled"),
            render: statusCell("enabled"),
          },
        ],
      }
    case "notifications":
      return {
        title: t("page.notifications"),
        path: "/api/v1/notification-sinks",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          {
            key: "type",
            header: t("resource.type"),
            render: statusCell("type"),
          },
          {
            key: "enabled",
            header: t("resource.enabled"),
            render: statusCell("enabled"),
          },
        ],
      }
    case "audit":
      return {
        title: t("page.audit"),
        path: "/api/v1/audit?limit=50",
        columns: [
          {
            key: "action",
            header: t("overview.action"),
            render: textCell("action"),
          },
          {
            key: "actor",
            header: t("common.actor"),
            render: userCell("actor"),
          },
          {
            key: "target_type",
            header: t("overview.target"),
            render: textCell("target_type"),
          },
        ],
      }
    case "settings":
      return {
        title: t("page.settings"),
        path: "/api/v1/config",
        columns: [
          { key: "key", header: t("resource.key"), render: textCell("key") },
          {
            key: "value",
            header: t("resource.value"),
            render: textCell("value"),
          },
        ],
      }
    case "overview":
      return {
        title: t("page.overview"),
        path: "/api/v1/config",
        columns: [
          { key: "key", header: t("resource.key"), render: textCell("key") },
          {
            key: "value",
            header: t("resource.value"),
            render: textCell("value"),
          },
        ],
      }
  }
}
