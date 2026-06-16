import { useTranslation } from "react-i18next"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"

// scope 分组定义。与后端 internal/auth/scope.go scopeRegistry 对齐。
// resource: 该组前缀；actions: 该组拥有的 action；i18nKey: 分组名。
type ScopeGroup = {
  resource: string
  actions: string[]
  i18nKey: string
}

const SCOPE_GROUPS: ScopeGroup[] = [
  { resource: "task", actions: ["read", "write"], i18nKey: "token.scopeGroup.task" },
  { resource: "project", actions: ["read", "write"], i18nKey: "token.scopeGroup.project" },
  { resource: "context", actions: ["read", "write"], i18nKey: "token.scopeGroup.context" },
  { resource: "config", actions: ["read", "write"], i18nKey: "token.scopeGroup.config" },
  { resource: "workspace", actions: ["read", "write"], i18nKey: "token.scopeGroup.workspace" },
  { resource: "audit", actions: ["read"], i18nKey: "token.scopeGroup.audit" },
  { resource: "token", actions: ["read", "write"], i18nKey: "token.scopeGroup.token" },
  { resource: "hook", actions: ["read", "write"], i18nKey: "token.scopeGroup.hook" },
  { resource: "notification", actions: ["read", "write"], i18nKey: "token.scopeGroup.notification" },
  { resource: "reminder", actions: ["read", "write"], i18nKey: "token.scopeGroup.reminder" },
]

type ScopeEditorProps = {
  value: string[]
  onChange: (scopes: string[]) => void
  canImpersonate?: boolean
}

export function ScopeEditor({
  value,
  onChange,
  canImpersonate = false,
}: ScopeEditorProps) {
  const { t } = useTranslation()
  const valueSet = new Set(value)

  const toggle = (scope: string, checked: boolean) => {
    const next = new Set(valueSet)
    if (checked) {
      next.add(scope)
    } else {
      next.delete(scope)
    }
    onChange(Array.from(next).sort())
  }

  const toggleGroup = (group: ScopeGroup, checked: boolean) => {
    const next = new Set(valueSet)
    for (const action of group.actions) {
      const scope = `${group.resource}:${action}`
      if (checked) {
        next.add(scope)
      } else {
        next.delete(scope)
      }
    }
    onChange(Array.from(next).sort())
  }

  return (
    <div className="space-y-3 rounded-none border p-3">
      {SCOPE_GROUPS.map((group) => {
        const groupScopes = group.actions.map(
          (action) => `${group.resource}:${action}`
        )
        const allChecked = groupScopes.every((s) => valueSet.has(s))
        return (
          <div key={group.resource} className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs text-muted-foreground">
                {t(group.i18nKey)}
              </Label>
              <button
                className="text-xs text-muted-foreground underline-offset-2 hover:underline"
                onClick={() => toggleGroup(group, !allChecked)}
                type="button"
              >
                {allChecked ? t("token.clearAll") : t("token.selectAll")}
              </button>
            </div>
            <div className="flex flex-wrap gap-4 pl-1">
              {groupScopes.map((scope) => (
                <label
                  key={scope}
                  className="flex items-center gap-2 text-sm"
                >
                  <Checkbox
                    checked={valueSet.has(scope)}
                    onCheckedChange={(checked) =>
                      toggle(scope, checked === true)
                    }
                  />
                  <span>{scope.split(":")[1]}</span>
                </label>
              ))}
            </div>
          </div>
        )
      })}
      {canImpersonate ? (
        <div className="space-y-2 border-t pt-3">
          <Label className="text-xs text-muted-foreground">
            {t("token.scopeGroup.impersonate")}
          </Label>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={valueSet.has("impersonate")}
              onCheckedChange={(checked) =>
                toggle("impersonate", checked === true)
              }
            />
            <span>impersonate</span>
          </label>
        </div>
      ) : null}
    </div>
  )
}
