// 配置定义 API 已迁移到 @/features/workspace/config/config-definition-api。
// 这里保留 re-export，避免 project-config-tab / project-config-row 等现有引用一次性改完。
export {
  configSchemaPath,
  listConfigSchema,
  type ConfigSchemaDefinition,
} from "@/features/workspace/config/config-definition-api"
