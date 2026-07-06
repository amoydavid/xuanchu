type OrSeparatorProps = {
  label?: string
}

/**
 * 登录页 SSO 与凭证登录之间的「或」分隔线。
 * 参考 better-auth-ui 的 OR 分隔线:左右两条细横线,中间夹文字。
 */
export function OrSeparator({ label = "或" }: OrSeparatorProps) {
  return (
    <div className="relative my-4" aria-hidden>
      <div className="absolute inset-0 flex items-center">
        <span className="w-full border-t" />
      </div>
      <div className="relative flex justify-center">
        <span className="bg-card px-2 text-xs uppercase text-muted-foreground">
          {label}
        </span>
      </div>
    </div>
  )
}
