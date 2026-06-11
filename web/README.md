# Xuanchu Web Admin Console

这是 Xuanchu v0.4.0 的嵌入式 Web Admin Console 前端源码。

技术栈：

- pnpm
- Vite 8
- React
- TypeScript
- shadcn/ui
- i18next

常用命令：

```bash
pnpm dev
pnpm test
pnpm typecheck
pnpm build
```

`pnpm build` 会把产物输出到 `../internal/webconsole/dist`，由 Go `embed` 打入单个 `xuanchu` 二进制。

## Adding components

To add components to your app, run the following command:

```bash
npx shadcn@latest add button
```

This will place the ui components in the `src/components` directory.

## Using components

To use the components in your app, import them as follows:

```tsx
import { Button } from "@/components/ui/button"
```
