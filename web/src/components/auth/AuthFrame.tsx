import type { ReactNode } from "react"

import { ProductLogo } from "@/components/ProductLogo"

type AuthFrameProps = {
  title: string
  description?: string
  children: ReactNode
}

/**
 * 登录类页面共用的卡片壳。
 *
 * 结构:顶部导航条之下,单栏居中卡片(参考 better-auth-ui 范式)。
 * 卡片头部只放裸 logo mark(完整 logo 已在顶部导航条),避免重复展示品牌名。
 */
export function AuthFrame({ title, description, children }: AuthFrameProps) {
  return (
    <div className="mx-auto flex min-h-[calc(100svh-3rem)] max-w-md flex-col justify-center px-6">
      <div className="mb-6">
        <ProductLogo
          className="mb-5"
          markClassName="size-10"
          showWordmark={false}
        />
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description ? (
          <p className="mt-2 text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {children}
    </div>
  )
}
