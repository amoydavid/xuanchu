import { useTranslation } from "react-i18next"
import { X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"

import {
  KNOWN_SCOPES,
  SCOPE_GROUPS,
  SCOPE_IMPERSONATE,
  TENANT_ACCESS_TOKEN_SCOPES,
  type ScopeGroup,
} from "./scopes"

type ScopeEditorProps = {
  value: string[]
  onChange: (scopes: string[]) => void
  canImpersonate?: boolean
  tenantAccessToken?: boolean
}

export function ScopeEditor({
  value,
  onChange,
  canImpersonate = false,
  tenantAccessToken = false,
}: ScopeEditorProps) {
  const { t } = useTranslation()
  const valueSet = new Set(value)
  const removableScopes = value.filter(
    (s) =>
      !KNOWN_SCOPES.has(s) ||
      (tenantAccessToken && !TENANT_ACCESS_TOKEN_SCOPES.has(s))
  )
  const canSelectScope = (scope: string) =>
    !tenantAccessToken || TENANT_ACCESS_TOKEN_SCOPES.has(scope)
  const scopeDisabled = (scope: string) =>
    !canSelectScope(scope) && !valueSet.has(scope)

  const toggle = (scope: string, checked: boolean) => {
    if (checked && !canSelectScope(scope)) {
      return
    }
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
    for (const scope of group.scopes) {
      if (!canSelectScope(scope)) {
        continue
      }
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
        const selectableScopes = group.scopes.filter(canSelectScope)
        const allChecked =
          selectableScopes.length > 0 &&
          selectableScopes.every((s) => valueSet.has(s))
        return (
          <div key={group.i18nKey} className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs text-muted-foreground">
                {t(group.i18nKey)}
              </Label>
              <Button
                className="text-muted-foreground"
                disabled={selectableScopes.length === 0}
                onClick={() => toggleGroup(group, !allChecked)}
                size="xs"
                type="button"
                variant="ghost"
              >
                {allChecked ? t("token.clearAll") : t("token.selectAll")}
              </Button>
            </div>
            <div className="flex flex-wrap gap-4 pl-1">
              {group.scopes.map((scope) => (
                <label key={scope} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={valueSet.has(scope)}
                    disabled={scopeDisabled(scope)}
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
              checked={valueSet.has(SCOPE_IMPERSONATE)}
              onCheckedChange={(checked) =>
                toggle(SCOPE_IMPERSONATE, checked === true)
              }
            />
            <span>impersonate</span>
          </label>
        </div>
      ) : null}
      {removableScopes.length > 0 ? (
        <div className="space-y-2 border-t pt-3">
          <Label className="text-xs text-muted-foreground">
            {t("token.unknownScopes")}
          </Label>
          <div className="flex flex-wrap gap-2">
            {removableScopes.map((scope) => (
              <span
                className="inline-flex items-center gap-1 rounded-none border bg-muted/50 px-2 py-0.5 text-xs"
                key={scope}
              >
                <code>{scope}</code>
                <Button
                  aria-label={t("token.removeScope")}
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => toggle(scope, false)}
                  size="icon-xs"
                  title={t("token.removeScope")}
                  type="button"
                  variant="ghost"
                >
                  <X className="size-3" />
                </Button>
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}
