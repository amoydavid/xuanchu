import { existsSync, readFileSync } from "node:fs"
import path from "node:path"

import { describe, expect, it } from "vitest"

import { zhCN } from "@/locales/zh-CN"

import {
  KNOWN_SCOPES,
  SCOPE_GROUPS,
  SCOPE_IMPERSONATE,
  TENANT_ACCESS_TOKEN_SCOPES,
} from "./scopes"

// 本测试是前后端 scope 一致性的兜底防线。
// 后端的 internal/auth/scope.go 是 scope 定义的唯一真相源（source of truth）。
// 前端 scopes.ts 必须与之完全对齐，否则会出现"后端允许但前端配不出"的漂移
// （曾经导致 tenant token 在 web console 无法勾选 user:read/member:read，
// 进而调用 user_list/member_list 时报 token_scope_denied）。
//
// 后端再加/改 scope 时，本测试会立即红，强制前端同步。

// 从仓库根定位 internal/auth/scope.go，不依赖 cwd。
function resolveScopeGoPath(): string {
  const target = path.join("internal", "auth", "scope.go")
  // 优先用 cwd（vitest 默认 web/，其父目录即仓库根）。
  const candidates = [
    path.resolve(process.cwd(), target), // 从 web/ 出发
    path.resolve(process.cwd(), "..", target), // 兜底：万一 cwd 是仓库根
  ]
  for (const c of candidates) {
    if (existsSync(c)) return c
  }
  throw new Error(`cannot find internal/auth/scope.go from ${process.cwd()}`)
}

const scopeGoPath = resolveScopeGoPath()
const scopeGoSrc = readFileSync(scopeGoPath, "utf8")

/** 解析 scope.go 里的常量声明（如 `ScopeTaskRead = "task:read"`），得到 常量名→字面量 映射。 */
function parseScopeConstants(src: string): Record<string, string> {
  const consts: Record<string, string> = {}
  const re = /(\w+)\s*=\s*"([^"]+)"/g
  let m: RegExpExecArray | null
  while ((m = re.exec(src)) !== null) {
    consts[m[1]] = m[2]
  }
  return consts
}

/**
 * 解析 scope.go 里某个变量（scopeRegistry 切片或 tenantAllowedScopes map）
 * 引用的所有常量名，再解析为字面量 scope 字符串集合。
 *
 * 两种声明形态：
 *   var scopeRegistry = []string{ ScopeTaskRead, ... }                // 切片
 *   var tenantAllowedScopes = map[string]struct{}{ ScopeTaskRead: {} } // map
 *
 * 用「从该 var 声明到下一个顶层声明（^var/^func/^const）之间」界定作用域，
 * 再抓所有 Scope* 常量名。这样对 `map[string]struct{}` 里的花括号免疫。
 */
function resolveScopeSet(src: string, varName: string, consts: Record<string, string>): Set<string> {
  const start = src.indexOf(varName)
  if (start === -1) {
    throw new Error(`variable ${varName} not found in scope.go`)
  }
  // body 从 `=` 之后开始，避免变量名本身（如 tenantAllowedScopes 含 "Scopes"）干扰。
  const eqIdx = src.indexOf("=", start)
  if (eqIdx === -1) {
    throw new Error(`cannot find = for ${varName}`)
  }
  // 结束边界：下一个顶层声明。
  const afterName = src.slice(eqIdx)
  const nextDecl = afterName.slice(1).search(/^var |^func |^const /m)
  const end = nextDecl === -1 ? src.length : eqIdx + 1 + nextDecl
  const body = src.slice(eqIdx, end)

  const names = new Set<string>()
  const re = /Scope\w+/g
  let m: RegExpExecArray | null
  while ((m = re.exec(body)) !== null) {
    names.add(m[0])
  }
  const resolved = new Set<string>()
  for (const n of names) {
    const v = consts[n]
    if (v === undefined) {
      throw new Error(`scope constant ${n} has no string value`)
    }
    resolved.add(v)
  }
  return resolved
}

const consts = parseScopeConstants(scopeGoSrc)
const backendScopes = resolveScopeSet(scopeGoSrc, "scopeRegistry", consts)
const backendTenantScopes = resolveScopeSet(scopeGoSrc, "tenantAllowedScopes", consts)

describe("frontend scope constants vs backend scope.go", () => {
  it("SCOPE_GROUPS covers all backend scopes except impersonate", () => {
    const frontendGrouped = new Set(SCOPE_GROUPS.flatMap((g) => g.scopes))
    const expected = new Set(backendScopes)
    expected.delete(SCOPE_IMPERSONATE)
    expect(frontendGrouped).toEqual(expected)
  })

  it("KNOWN_SCOPES equals backend scopeRegistry", () => {
    expect(new Set(KNOWN_SCOPES)).toEqual(backendScopes)
  })

  it("TENANT_ACCESS_TOKEN_SCOPES equals backend tenantAllowedScopes", () => {
    expect(new Set(TENANT_ACCESS_TOKEN_SCOPES)).toEqual(backendTenantScopes)
  })

  it("SCOPE_GROUPS has no duplicate scope across groups", () => {
    const all = SCOPE_GROUPS.flatMap((g) => g.scopes)
    expect(new Set(all).size).toBe(all.length)
  })

  it("every SCOPE_GROUPS i18nKey exists in zh-CN locale", () => {
    const group = (zhCN as unknown as { token: { scopeGroup: Record<string, string> } }).token.scopeGroup
    for (const g of SCOPE_GROUPS) {
      // i18nKey 形如 "token.scopeGroup.task"，取末段
      const key = g.i18nKey.split(".").pop()!
      expect(group[key], `missing zh-CN translation for ${g.i18nKey}`).toBeTruthy()
    }
  })
})
