import { useMemo, useState } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import type { ConfigValueType } from "./config-definition-api"

type ConfigValueControlProps = {
  definition: Pick<ConfigSchemaControlDefinition, "value_type" | "enum_values" | "secret">
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
}

// 只依赖 schema 的少量字段，便于 project-config-row 等复用。
type ConfigSchemaControlDefinition = {
  value_type: ConfigValueType | string
  enum_values: string[]
  secret: boolean
}

export function ConfigValueControl({
  definition,
  value,
  onChange,
  disabled,
  id,
}: ConfigValueControlProps) {
  const { t } = useTranslation()
  const enumValues = definition.enum_values ?? []

  // secret reveal 状态只在控件内部维护；外部 value 始终是明文。
  const [revealed, setRevealed] = useState(false)

  // enum 优先级最高
  if (enumValues.length > 0) {
    return (
      <Select disabled={disabled} onValueChange={onChange} value={value}>
        <SelectTrigger className="w-full" id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {enumValues.map((v) => (
            <SelectItem key={v} value={v}>
              {v}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  if (definition.value_type === "boolean") {
    const checked = value === "true"
    return (
      <Switch
        aria-label={t("configDefinitions.value")}
        checked={checked}
        disabled={disabled}
        id={id}
        onCheckedChange={(next) => onChange(next ? "true" : "false")}
      />
    )
  }

  if (definition.value_type === "json") {
    return (
      <JsonControl
        disabled={disabled}
        id={id}
        invalidHint={t("configDefinitions.jsonInvalid")}
        onChange={onChange}
        value={value}
      />
    )
  }

  const isSecret = definition.secret === true
  if (isSecret) {
    return (
      <div className="flex items-center gap-1">
        <Input
          disabled={disabled}
          id={id}
          onChange={(e) => onChange(e.target.value)}
          type={revealed ? "text" : "password"}
          value={value}
        />
        <Button
          aria-label={
            revealed
              ? t("configDefinitions.hideSecret")
              : t("configDefinitions.revealSecret")
          }
          onClick={() => setRevealed((v) => !v)}
          size="icon-sm"
          type="button"
          variant="ghost"
        >
          {revealed ? <EyeOffIcon /> : <EyeIcon />}
        </Button>
      </div>
    )
  }

  return (
    <Input
      disabled={disabled}
      id={id}
      onChange={(e) => onChange(e.target.value)}
      type={definition.value_type === "number" ? "number" : "text"}
      value={value}
    />
  )
}

// JsonControl 只做 parse 提示，不阻止输入。
function JsonControl(props: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
  invalidHint: string
}) {
  const { value, onChange, disabled, id, invalidHint } = props
  const isInvalid = useMemo(() => isInvalidJson(value), [value])
  return (
    <div className="space-y-1">
      <Textarea
        className="font-mono text-xs"
        disabled={disabled}
        id={id}
        onChange={(e) => onChange(e.target.value)}
        rows={4}
        value={value}
      />
      {isInvalid ? <p className="text-xs text-destructive">{invalidHint}</p> : null}
    </div>
  )
}

function isInvalidJson(value: string): boolean {
  if (value.trim() === "") return false
  try {
    JSON.parse(value)
    return false
  } catch {
    return true
  }
}
