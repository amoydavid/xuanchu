import { useTranslation } from "react-i18next"

import type { NotificationSink } from "../outbound-api"
import {
  selectVars,
  useNotificationTemplateVars,
  type TemplateVarTrigger,
} from "./template-vars"

// 模板变量提示 + sink body 只读预览。
// trigger 决定显示哪些变量（reminder 不显示 event.*，反之同）。
// sink 若是 http_template 类型，额外只读预览其 body_template，把 {{var}} 高亮成 chip。
export function TemplateVarHints({
  trigger,
  sink,
}: {
  trigger: TemplateVarTrigger
  sink: NotificationSink | null
}) {
  const { t } = useTranslation()
  const { data } = useNotificationTemplateVars()
  const bodyVars = selectVars(data, trigger, "body")

  const showBodyPreview = sink?.type === "http_template" && !!sink?.body_template

  return (
    <div className="space-y-2 rounded-md border bg-muted/30 p-3 text-xs">
      <p className="font-medium">
        {trigger === "reminder"
          ? t("outbound.templateVars.titleReminder")
          : t("outbound.templateVars.titleEvent")}
      </p>
      <ul className="flex flex-wrap gap-1.5">
        {bodyVars.map((v) => (
          <li
            key={v.name}
            className="font-mono text-[11px] text-muted-foreground"
            title={v.description}
          >
            <code className="rounded bg-background px-1 py-0.5">{v.name}</code>
            <span className="ml-1">{v.description}</span>
          </li>
        ))}
      </ul>
      {showBodyPreview ? (
        <div className="space-y-1">
          <p className="text-muted-foreground">{t("outbound.templateVars.bodyPreview")}</p>
          <pre className="overflow-x-auto rounded bg-background p-2 font-mono text-[11px]">
            <SinkBodyPreview body={sink!.body_template!} />
          </pre>
        </div>
      ) : null}
    </div>
  )
}

// 把 body 模板里的 {{var}} 渲染成高亮 chip，其余文本原样输出。
function SinkBodyPreview({ body }: { body: string }) {
  const parts = body.split(/(\{\{[^}]+\}\})/g)
  return (
    <>
      {parts.map((part, i) => {
        if (/^\{\{[^}]+\}\}$/.test(part)) {
          return (
            <span key={i} className="rounded bg-primary/15 px-1 text-primary">
              {part}
            </span>
          )
        }
        return <span key={i}>{part}</span>
      })}
    </>
  )
}
