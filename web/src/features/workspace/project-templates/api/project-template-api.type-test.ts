import type { CapturePreview } from "./project-template-api"

// 编译期契约夹具：阻断 Capture Preview 必须能显式返回 snapshot: null。
export const blockingCapturePreviewContract: CapturePreview = {
  selection: {
    task_refs: [],
    series_refs: [],
    config_keys: [],
    automation_rule_ids: [],
  },
  required_config_keys: [],
  source_hash: "source-hash",
  counts: { tasks: 0, series: 0, configs: 0, automations: 0 },
  blocking_issues: [
    { code: "project_template_dependency_missing", message: "missing" },
  ],
  warnings: [],
  snapshot: null,
}
