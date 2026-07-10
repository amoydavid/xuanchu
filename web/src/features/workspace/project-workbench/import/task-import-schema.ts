export const TASK_IMPORT_JSON_SCHEMA = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  $id: "https://xuanchu.local/schemas/task-import.schema.json",
  title: "Xuanchu task import JSON",
  description:
    "任务导入 JSON 可以是任务数组，也可以是包含 tasks 数组的对象。description 默认按 Markdown 内容编写，技术上仍按字符串保存。",
  oneOf: [
    {
      type: "array",
      description: "直接上传任务数组。",
      items: { $ref: "#/$defs/task" },
    },
    {
      type: "object",
      description: "上传包含 tasks 数组的对象，便于后续扩展批次级元数据。",
      required: ["tasks"],
      properties: {
        tasks: {
          type: "array",
          description: "本次要导入的任务列表。整批预检通过后才会提交。",
          items: { $ref: "#/$defs/task" },
        },
      },
      additionalProperties: false,
    },
  ],
  $defs: {
    task: {
      type: "object",
      description:
        "单个任务。未列出的 top-level 字段会作为 UDA 保留；推荐自定义字段使用 uda.<name>。",
      required: ["title"],
      properties: {
        id: {
          type: "string",
          description:
            "导入文件内的临时引用 ID，可选；只要求在本批次内不重复，不要求 UUID 格式。不会作为任务 ID 入库，可被 blocked_by 引用。",
        },
        import_id: {
          type: "string",
          description:
            "id 的同义字段；当同时填写 id 与 import_id 时使用 import_id。只要求在本批次内不重复的字符串。",
        },
        uuid: {
          type: "string",
          description:
            "uuid 是最终任务标识的候选值。未填写时由浏览器预检生成新 UUID；若填写，会作为导入后任务的 uuid 并可被本批次其他任务的 blocked_by 引用。普通导入建议使用 id/import_id 做临时引用。",
        },
        title: {
          type: "string",
          minLength: 1,
          description: "任务标题，必填。用于列表中的短任务名称。",
        },
        description: {
          type: ["string", "null"],
          description:
            "任务详细描述，可空；默认按 Markdown 编写和展示，技术上按普通字符串保存。",
        },
        status: {
          type: "string",
          enum: ["pending", "completed", "deleted", "waiting", "recurring"],
          default: "pending",
          description: "任务状态。未填写时默认为 pending。",
        },
        priority: {
          type: ["string", "null"],
          enum: ["H", "M", "L", null],
          description: "任务优先级：H 高、M 中、L 低；可留空。",
        },
        tags: {
          oneOf: [
            { type: "string", description: "逗号或换行分隔的标签。" },
            {
              type: "array",
              description: "标签数组。",
              items: { type: "string" },
            },
          ],
          description: "任务标签。XLSX 中通常用逗号分隔。",
        },
        assignees: {
          oneOf: [
            { type: "string", description: "逗号或换行分隔的指派人引用。" },
            {
              type: "array",
              description:
                "指派人引用数组；元素可以是用户 ID、姓名、邮箱，或包含 id/name/email 的对象。",
              items: {
                oneOf: [
                  { type: "string" },
                  {
                    type: "object",
                    properties: {
                      id: {
                        type: "string",
                        description: "用户 ID。",
                      },
                      name: {
                        type: "string",
                        description: "稳定用户引用名；用于服务端解析用户。",
                      },
                      display_name: {
                        type: "string",
                        description:
                          "展示姓名或昵称；用于预检创建缺失用户和 UI 展示，不作为唯一身份凭证。",
                      },
                      email: {
                        type: ["string", "null"],
                        description: "用户邮箱。",
                      },
                      external_ids: {
                        type: "array",
                        description: "外部身份标识列表。",
                        items: {
                          type: "object",
                          properties: {
                            provider: { type: "string", description: "外部 ID 提供方，如 feishu、wecom。" },
                            user_type: { type: "string", description: "ID 种类，如 user_id、open_id、union_id。" },
                            external_id: { type: "string", description: "外部 ID 值。" },
                          },
                        },
                      },
                    },
                    additionalProperties: true,
                  },
                ],
              },
            },
          ],
          description:
            "任务指派人。普通姓名或邮箱必须能解析为当前 workspace 成员；预检可选择创建/加入缺失成员。",
        },
        blocked_by: {
          oneOf: [
            { type: "string", description: "逗号或换行分隔的阻塞任务引用。" },
            {
              type: "array",
              description: "阻塞任务引用数组。",
              items: { type: "string" },
            },
          ],
          description:
            "当前任务被哪些任务阻塞。可引用本导入文件内的 id/import_id，也可引用当前项目已有任务 UUID。",
        },
        due: {
          type: ["string", "null"],
          description: "截止时间。推荐 YYYY-MM-DD 或 RFC3339 date-time。",
        },
        wait: {
          type: ["string", "null"],
          description: "等待到该时间后任务才进入可处理状态。推荐 YYYY-MM-DD 或 RFC3339 date-time。",
        },
        scheduled: {
          type: ["string", "null"],
          description: "计划开始处理时间。推荐 YYYY-MM-DD 或 RFC3339 date-time。",
        },
        until: {
          type: ["string", "null"],
          description: "任务有效期截止时间。推荐 YYYY-MM-DD 或 RFC3339 date-time。",
        },
        start: {
          type: ["string", "null"],
          description: "任务开始时间。推荐 RFC3339 date-time。",
        },
        end: {
          type: ["string", "null"],
          description: "任务完成或终止时间。推荐 RFC3339 date-time。",
        },
        entry: {
          type: ["string", "null"],
          description: "任务创建时间。未填写时由导入解析阶段填入当前时间。",
        },
        modified: {
          type: ["string", "null"],
          description: "任务修改时间。未填写时由导入解析阶段填入当前时间。",
        },
        recur: {
          type: ["string", "null"],
          description:
            "重复规则字符串；当前导入流程仅原样保存，不在浏览器端解析或展开循环任务。普通一次性任务请留空。",
        },
        parent: {
          type: ["string", "null"],
          description: "父任务引用。当前仅作为导入字段透传。",
        },
        task_slug: {
          type: ["string", "null"],
          description: "任务 slug，可选；用于已有系统内的人类可读任务引用。",
        },
        annotations: {
          type: "array",
          description: "任务注解列表。",
          items: {
            type: "object",
            required: ["description"],
            properties: {
              id: {
                type: "string",
                description: "注解 ID，可选。",
              },
              entry: {
                type: ["string", "null"],
                description: "注解时间。未填写时使用任务 entry。",
              },
              description: {
                type: "string",
                description: "注解内容。",
              },
            },
            additionalProperties: false,
          },
        },
        links: {
          type: "array",
          description:
            "任务关联链接，按服务端 link 结构透传；常见字段包括 type、url、title。",
          items: {
            type: "object",
            additionalProperties: true,
          },
        },
        "uda.estimate": {
          type: ["string", "number", "null"],
          description:
            "示例 UDA 字段。实际可使用任意 uda.<name> 字段，导入时会转换为字符串保存。",
        },
      },
      patternProperties: {
        "^uda\\.[A-Za-z0-9_.-]+$": {
          type: ["string", "number", "boolean", "object", "array", "null"],
          description:
            "自定义 UDA 字段，字段名格式为 uda.<name>；非字符串值导入时会转换为字符串或 JSON 字符串。",
        },
      },
      additionalProperties: {
        type: ["string", "number", "boolean", "object", "array", "null"],
        description:
          "未列出的 top-level 字段会作为 orphan UDA 保留；建议新字段显式写成 uda.<name>。",
      },
    },
  },
} as const

export const TASK_IMPORT_JSON_SCHEMA_TEXT = JSON.stringify(
  TASK_IMPORT_JSON_SCHEMA,
  null,
  2
)
