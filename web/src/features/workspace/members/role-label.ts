import type { TFunction } from "i18next"

/** 把 membership role（owner/admin/member/viewer）转为本地化文案。 */
export function memberRoleLabel(t: TFunction, role: string): string {
  switch (role) {
    case "owner":
      return t("members.roles.owner")
    case "admin":
      return t("members.roles.admin")
    case "member":
      return t("members.roles.member")
    case "viewer":
      return t("members.roles.viewer")
    default:
      return role
  }
}
