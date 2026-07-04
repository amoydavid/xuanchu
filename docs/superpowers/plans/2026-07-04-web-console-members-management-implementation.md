# Web Console Members Management Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox syntax for tracking; completed steps are marked with `- [x]`.

**Goal:** Implement the `/members` workspace member-management spec: owner/admin can add, edit display names, change roles, and remove members through shadcn-based dialogs while member/viewer retain a read-only roster.

**Architecture:** Keep membership rules in `internal/app` and storage mutations in `internal/storage`; HTTP only decodes requests and delegates to app services. The Web Console uses existing React Query + shadcn UI primitives (`Dialog`, `AlertDialog`, `Select`, `DropdownMenu`, `Input`, `Badge`, `Alert`, `Table`) and derives UX permissions from `/api/v1/credentials/current`.

**Tech Stack:** Go 1.25, GORM, chi/Huma HTTP bridge, React 19, TanStack Query, shadcn/Radix UI, Vitest, Go tests.

---

## Implementation Status

状态：已实现。

本计划已经完成 backend membership contract、HTTP API、browser session capability、Web Console `/members` 主页面、`/members/:userRef` 次级成员详情页、README/ROADMAP 同步和完整验证。当前实现刻意不扩展 CLI/MCP 的 member delete；CLI 文档继续说明没有 `member delete`，Web Console/HTTP API 支持移出 workspace membership 且不删除 user。

完成验证：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web test
pnpm --dir web build
git diff --check
```

## Chunk 1: Backend Membership Contract

### Task 1: Storage and App Membership Mutations

**Files:**
- Modify: `internal/storage/member_repo.go`
- Modify: `internal/app/workspace.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1: Write failing app tests**

Add tests covering:

```go
func TestRemoveMemberRespectsOwnerRules(t *testing.T) {
    // owner can remove non-owner; admin can remove non-owner;
    // admin cannot remove owner; owner cannot remove last owner.
}

func TestModifyMemberDisplayNameRespectsOwnerRules(t *testing.T) {
    // admin edits non-owner display_name; admin cannot edit owner;
    // member cannot edit anyone.
}

func TestAddMemberCanCreateUserAtomically(t *testing.T) {
    // AddMember with NewUser creates user + membership in one service call.
}
```

- [x] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/app -run 'Test(RemoveMemberRespectsOwnerRules|ModifyMemberDisplayNameRespectsOwnerRules|AddMemberCanCreateUserAtomically)' -count=1
```

Expected: FAIL because `RemoveMember`, `ModifyMemberDisplayName`, `NewUser` input support, or repository delete does not exist.

- [x] **Step 3: Implement storage/app**

Implement:

- `MemberRepository.Delete(userID, workspaceID string) error`
- `AddMemberInput.NewUser *AddMemberUserInput`
- `ModifyMemberInput { WorkspaceRef, UserRef string; Role *Role; DisplayName *string }`
- `Service.ModifyMember(input ModifyMemberInput) (MemberView, error)`
- `Service.RemoveMember(workspaceRef, userRef string) error`

Rules:

- Existing `AddMember` keeps `UserRef` behavior.
- `NewUser` path creates user and membership in one `withAudit` transaction.
- `ModifyMember` applies role and/or `display_name`, using `requireMemberManagement(actorRole, targetRole, currentRole)`.
- Admin can manage non-owner members only.
- Owner can manage owner members but cannot remove or downgrade the last owner.
- `RemoveMember` deletes only membership, never user.
- Audit actions: `member.add`, `member.role`, `member.remove`; display-name-only changes may use `member.profile.modify` with payload `display_name`.

- [x] **Step 4: Run app tests to verify GREEN**

Run:

```bash
go test ./internal/app -run 'Test(RemoveMemberRespectsOwnerRules|ModifyMemberDisplayNameRespectsOwnerRules|AddMemberCanCreateUserAtomically|AddMemberAndChangeMemberRoleRespectOwnerRules|ChangeMemberRoleProtectsLastOwner)' -count=1
```

Expected: PASS.

### Task 2: HTTP API for Member Patch/Delete/Create-New-User

**Files:**
- Modify: `internal/httpapi/workspaces.go`
- Modify: `internal/httpapi/huma_routes.go`
- Test: `internal/httpapi/auth_test.go` or `internal/httpapi/workspaces_test.go` if present

- [x] **Step 1: Write failing HTTP tests**

Add tests covering:

```go
func TestHTTPMemberPatchCanUpdateDisplayNameAndRole(t *testing.T) {}
func TestHTTPMemberDeleteRemovesNonOwnerAndProtectsOwner(t *testing.T) {}
func TestHTTPMemberAddCanCreateNewUser(t *testing.T) {}
```

- [x] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/httpapi -run 'TestHTTPMember(PatchCanUpdateDisplayNameAndRole|DeleteRemovesNonOwnerAndProtectsOwner|AddCanCreateNewUser)' -count=1
```

Expected: FAIL because HTTP handlers do not yet support the new request shapes or DELETE route.

- [x] **Step 3: Implement HTTP handlers**

Implement:

- Request body supports `user`, `new_user`, `role`, `display_name`.
- `handleMemberAdd` delegates to `Service.AddMember` with either `UserRef` or `NewUser`.
- Replace `handleMemberRole` internals with `Service.ModifyMember`.
- Add `handleMemberDelete`.
- Add Huma route:

```go
{Method: http.MethodDelete, Path: "/api/v1/workspaces/{workspace}/members/{user}", Tag: "Members", Summary: "Remove a workspace member.", Handler: s.handleMemberDelete}
```

- Keep auth requirement `member:write` + `PermissionMemberManage`; app layer performs owner-specific checks.

- [x] **Step 4: Run HTTP tests to verify GREEN**

Run:

```bash
go test ./internal/httpapi -run 'TestHTTPMember(PatchCanUpdateDisplayNameAndRole|DeleteRemovesNonOwnerAndProtectsOwner|AddCanCreateNewUser)|TestTenantTokenCanManageHTTPUsersAndMembers' -count=1
```

Expected: PASS.

### Task 3: Browser Session Capability

**Files:**
- Modify: `internal/httpapi/middleware.go`
- Test: `internal/httpapi/middleware_dual_test.go`

- [x] **Step 1: Write failing capability test**

Add/extend a test proving cookie browser sessions expose `member:write` in `/api/v1/credentials/current.capabilities` and still omit `token:write` and `impersonate`.

- [x] **Step 2: Run test to verify RED**

Run:

```bash
go test ./internal/httpapi -run 'TestCookieCredentialsCurrent' -count=1
```

Expected: FAIL if `member:write` is absent.

- [x] **Step 3: Add capability**

Add `auth.ScopeMemberWrite` to `browserSessionScopes()` only. Do not add `token:write` or `impersonate`.

- [x] **Step 4: Run test to verify GREEN**

Run:

```bash
go test ./internal/httpapi -run 'TestCookieCredentialsCurrent' -count=1
```

Expected: PASS.

## Chunk 2: Web Console Members UX

### Task 4: Members API Client

**Files:**
- Modify: `web/src/features/workspace/members/members-api.ts`
- Test: `web/src/features/workspace/members/members-page.test.tsx`

- [x] **Step 1: Write failing API usage tests through page**

Extend page tests to assert the UI calls:

- `POST /api/v1/workspaces/{workspace}/members` with existing `user`.
- `POST /api/v1/workspaces/{workspace}/members` with `new_user`.
- `PATCH /api/v1/workspaces/{workspace}/members/{user}` with `role` and/or `display_name`.
- `DELETE /api/v1/workspaces/{workspace}/members/{user}`.

- [x] **Step 2: Run tests to verify RED**

Run:

```bash
pnpm --dir web test -- members-page.test.tsx
```

Expected: FAIL because page controls/API helpers do not exist.

- [x] **Step 3: Implement API helpers**

Add:

- `addWorkspaceMember(workspaceSlug, input)`
- `modifyWorkspaceMember(workspaceSlug, userID, input)`
- `removeWorkspaceMember(workspaceSlug, userID)`

Keep `listWorkspaceMembers`. Stop using `PATCH /api/v1/users/{id}` from the members page.

- [x] **Step 4: Re-run focused tests**

Run:

```bash
pnpm --dir web test -- members-page.test.tsx
```

Expected: Remaining failures only from missing UI, not API helper import/type errors.

### Task 5: shadcn Members Page Interactions

**Files:**
- Modify: `web/src/features/workspace/members/members-page.tsx`
- Modify: `web/src/routes/workspace/MembersRoute.tsx`
- Create: `web/src/routes/workspace/MemberDetailRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Test: `web/src/features/workspace/members/members-page.test.tsx`

- [x] **Step 1: Add failing UI tests**

Cover:

- Owner sees counts, filters, add button, row action menu.
- Owner adds existing user through shadcn `Dialog`.
- Owner creates user + member through the same dialog.
- Admin cannot manage owner row but can manage member row.
- Member sees read-only roster and no add/action controls.
- Removing a member uses shadcn `AlertDialog`.
- Role owner promotion/downgrade uses shadcn confirmation before PATCH.
- Row menu links to `/members/:userRef`.
- `/members/:userRef` renders identity snapshot, membership metadata, external IDs, token summary link, recent member audit, and the same shadcn management dialogs when allowed.

- [x] **Step 2: Run focused tests to verify RED**

Run:

```bash
pnpm --dir web test -- members-page.test.tsx
```

Expected: FAIL because controls are not implemented.

- [x] **Step 3: Implement page UX**

Use shadcn components only for dialogs and controls:

- `Dialog` for add member and edit display name.
- `AlertDialog` or existing `DestructiveConfirmDialog` for remove/owner-risk role changes.
- `Select` for role and source mode.
- `DropdownMenu` for row actions.
- `Input` for search and form fields.
- `Badge`, `Alert`, `Table`, `Skeleton`, `Button`, `Label` for the rest.
- Add `MembersDetailPage` and `MemberDetailRoute` for the secondary page. Keep low-frequency information there: identity snapshot, current membership, external identity summary, token page jump, and recent member audit.

Keep visual style quiet and operational: dense table, compact toolbar, no nested cards, no marketing copy.

- [x] **Step 4: Run focused web tests to verify GREEN**

Run:

```bash
pnpm --dir web test -- members-page.test.tsx
```

Expected: PASS.

## Chunk 3: Documentation and Verification

### Task 6: Docs Sync

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md` only if roadmap status/feature boundary needs explicit update

- [x] **Step 1: Check doc deltas needed**

Search README/ROADMAP for stale `member delete` wording.

- [x] **Step 2: Update user-visible docs**

At minimum update README member permission text from “当前没有 `member delete`” to the new remove-member behavior and Web Console capability.

- [x] **Step 3: Run doc diff check**

Run:

```bash
git diff --check
```

Expected: PASS.

### Task 7: Full Verification

**Files:** all changed files.

- [x] **Step 1: Run Go verification**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS.

- [x] **Step 2: Run web verification**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

Expected: PASS.

- [x] **Step 3: Final worktree check**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; changed files limited to plan/spec/docs/backend/frontend plus any pre-existing dirty files explicitly called out.
