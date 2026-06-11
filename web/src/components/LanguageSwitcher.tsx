import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const languageKey = "xuanchu.console.language"

export function LanguageSwitcher() {
  const { i18n, t } = useTranslation()

  return (
    <Select
      value={i18n.language.startsWith("en") ? "en-US" : "zh-CN"}
      onValueChange={(value) => {
        localStorage.setItem(languageKey, value)
        void i18n.changeLanguage(value)
      }}
    >
      <SelectTrigger aria-label={t("common.language")} size="sm">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="zh-CN">中文</SelectItem>
        <SelectItem value="en-US">English</SelectItem>
      </SelectContent>
    </Select>
  )
}
