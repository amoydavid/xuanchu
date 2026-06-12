import { useTranslation } from "react-i18next"

import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

type ProductLogoProps = {
  className?: string
  markClassName?: string
  showWordmark?: boolean
}

const assetBase = import.meta.env.BASE_URL

export function ProductLogo({
  className,
  markClassName,
  showWordmark = true,
}: ProductLogoProps) {
  const { theme } = useTheme()
  const { t } = useTranslation()
  const logo =
    theme === "dark" ? "xuanchu-logo-dark.svg" : "xuanchu-logo-light.svg"

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <span
        className={cn(
          "relative block size-7 shrink-0 overflow-hidden",
          markClassName
        )}
      >
        <img
          alt=""
          aria-hidden="true"
          className="block h-full w-full object-contain"
          height="28"
          src={`${assetBase}${logo}`}
          width="28"
        />
      </span>
      {showWordmark ? (
        <span className="text-sm font-medium tracking-normal">
          {t("app.brand")}
        </span>
      ) : null}
    </div>
  )
}
