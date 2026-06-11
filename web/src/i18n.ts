import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import { enUS } from "./locales/en-US"
import { zhCN } from "./locales/zh-CN"

export const supportedLanguages = ["zh-CN", "en-US"] as const
export type SupportedLanguage = (typeof supportedLanguages)[number]

export function normalizeLanguage(value: string | null | undefined): SupportedLanguage {
  if (!value) return "zh-CN"
  const normalized = value.toLowerCase()
  if (normalized.startsWith("en")) return "en-US"
  if (normalized.startsWith("zh")) return "zh-CN"
  return "zh-CN"
}

function preferredLanguage(): SupportedLanguage {
  return normalizeLanguage(
    localStorage.getItem("xuanchu.console.language") || navigator.language
  )
}

i18n.use(initReactI18next).init({
  resources: {
    "zh-CN": { translation: zhCN },
    "en-US": { translation: enUS },
  },
  lng: preferredLanguage(),
  fallbackLng: "zh-CN",
  interpolation: { escapeValue: false },
})

export { i18n }
