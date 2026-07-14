// Breadcrumb 是项目工作台的多级面包屑。前 N-1 级可点击，最后一级为当前页（纯文本）。
export type BreadcrumbItem = {
  label: string
  href?: string
}

export function Breadcrumb({ items }: { items: BreadcrumbItem[] }) {
  return (
    <nav className="text-xs text-muted-foreground" aria-label="breadcrumb">
      {items.map((item, index) => {
        const isLast = index === items.length - 1
        return (
          <span key={index}>
            {index > 0 ? " / " : null}
            {isLast || !item.href ? (
              <span className={isLast ? "text-foreground" : undefined}>
                {item.label}
              </span>
            ) : (
              <a className="hover:text-foreground" href={item.href}>
                {item.label}
              </a>
            )}
          </span>
        )
      })}
    </nav>
  )
}
