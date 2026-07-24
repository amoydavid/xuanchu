"use client"

import * as React from "react"

import { cn } from "@/lib/utils"

function Table({
  className,
  /**
   * 是否渲染为独立卡片表格容器（带圆角边框 + 卡片底）。
   * 默认 true：表格自带 `overflow-x-auto rounded-lg border bg-card` 外壳，
   * 页面无需再手写包裹 div；横向可滚动以容纳宽表。设为 false 时退化为纯滚动容器，
   * 用于嵌入既有卡片内部。
   */
  card = true,
  /**
   * 透传给容器 div 的额外 class（如响应式可见性 `hidden md:block`）。
   * 卡片样式由 card 控制，这里只放布局/可见性修饰，避免再手写包裹 div。
   */
  containerClassName,
  ...props
}: React.ComponentProps<"table"> & {
  card?: boolean
  containerClassName?: string
}) {
  return (
    <div
      data-slot="table-container"
      className={cn(
        "relative w-full",
        card
          ? "overflow-x-auto rounded-lg border bg-card"
          : "overflow-x-auto",
        containerClassName
      )}
    >
      <table
        data-slot="table"
        className={cn("w-full caption-bottom text-sm", className)}
        {...props}
      />
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        className
      )}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted",
        className
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "h-10 px-3 text-left align-middle text-xs font-semibold tracking-wide text-muted-foreground uppercase whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "h-10 px-3 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
