import DOMPurify from "dompurify"

// PasteImageCandidate 是粘贴 HTML 中提取出的图片候选。
//
// 候选顺序与 DOM 中 <img> 出现的顺序一致；每个候选替换为同位置的 marker，
// 由编辑器在上传/转存后替换为 attachment 节点。
export type PasteImageCandidate = {
  key: string
  kind: "file" | "data" | "remote"
  alt: string
  file?: File
  sourceURL?: string
}

// SanitizedPaste 是 sanitizeRichPaste 的返回值。
export type SanitizedPaste = {
  html: string
  images: PasteImageCandidate[]
}

const INLINE_IMAGE_MEDIA_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
])

function isSupportedInlineImageMediaType(mediaType: string): boolean {
  return INLINE_IMAGE_MEDIA_TYPES.has(mediaType.toLowerCase())
}

// dataURLToFile 将剪贴板 HTML 中的 data: 图片转换为普通 File，使其与截图走同一
// 受限上传链路；data URL 绝不能进入持久化 Markdown。
export function dataURLToFile(dataURL: string, name: string): File {
  const match = /^data:([^;,]+)?(;base64)?,([\s\S]*)$/i.exec(dataURL)
  if (!match) {
    throw new Error("invalid data image")
  }
  const mediaType = match[1] || "application/octet-stream"
  if (!isSupportedInlineImageMediaType(mediaType)) {
    throw new Error("invalid data image")
  }
  const payload = match[3] || ""
  const decoded = match[2]
    ? Uint8Array.from(atob(payload), (char) => char.charCodeAt(0))
    : new TextEncoder().encode(decodeURIComponent(payload))
  const extension = mediaType.split("/")[1]?.replace(/[^a-z0-9]/gi, "") || "bin"
  const baseName = name.trim() || "image"
  return new File([decoded], `${baseName}.${extension}`, { type: mediaType })
}

const ALLOWED_TAGS = [
  "p", "br", "h1", "h2", "h3", "strong", "b", "em", "i", "s", "del",
  "ul", "ol", "li", "blockquote", "pre", "code",
  "table", "thead", "tbody", "tr", "th", "td",
  "a",
]

const ALLOWED_ATTR = ["href", "title", "colspan", "rowspan", "data-xuanchu-paste-image"]

let candidateCounter = 0

// sanitizeRichPaste 把粘贴的 HTML + clipboard 文件规范化为：
// 1. 安全 HTML（仅白名单标签/属性，去除 img/style/script/iframe/on*）
// 2. 按 DOM 顺序的图片候选（clipboard File 优先；data URL 次之；公网 http(s) 最后）
//
// 每个原始 <img> 替换为 <a data-xuanchu-paste-image="key">alt</a>，
// 由编辑器替换为 attachment 节点。空 alt 用 "image" 兜底。
export function sanitizeRichPaste(input: {
  html: string
  files?: File[]
}): SanitizedPaste {
  const container = document.createElement("div")
  container.innerHTML = input.html

  const fileByIndex = (input.files ?? []).filter((file) =>
    isSupportedInlineImageMediaType(file.type)
  )
  let fileCursor = 0
  const images: PasteImageCandidate[] = []
  const imgs = Array.from(container.querySelectorAll("img"))
  for (const img of imgs) {
    const alt = img.getAttribute("alt") ?? "image"
    // 1) clipboard File 优先
    const file = fileByIndex[fileCursor]
    if (file) {
      fileCursor += 1
      const candidate = makeCandidate("file", alt, { file })
      images.push(candidate)
      replaceWithMarker(img, candidate.key, alt)
      continue
    }
    // 2) data URL
    const src = img.getAttribute("src") ?? ""
    if (src.startsWith("data:")) {
      const mediaType = /^data:([^;,]+)/i.exec(src)?.[1] ?? ""
      if (isSupportedInlineImageMediaType(mediaType)) {
        const candidate = makeCandidate("data", alt, { sourceURL: src })
        images.push(candidate)
        replaceWithMarker(img, candidate.key, alt)
      } else {
        img.remove()
      }
      continue
    }
    // 3) 公网 http(s)
    const remote = pickRemoteImageURL(img)
    if (remote) {
      const candidate = makeCandidate("remote", alt, { sourceURL: remote })
      images.push(candidate)
      replaceWithMarker(img, candidate.key, alt)
      continue
    }
    // 既没有 file 也不是可识别 URL：直接移除该 <img>，避免持久化坏图。
    img.remove()
  }

  const cleaned = DOMPurify.sanitize(container.innerHTML, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: ["style", "script", "iframe", "object", "embed", "form", "input", "video", "audio"],
    FORBID_ATTR: ["style", "class", "id", "onerror", "onload", "onclick"],
  })

  // 二次断言：DOMPurify 不应残留任何 <img> 或 <script>。
  const verify = document.createElement("div")
  verify.innerHTML = cleaned
  if (verify.querySelector("img, script, style, iframe")) {
    // 极端情况：丢弃所有 HTML，返回纯文本。
    return {
      html: DOMPurify.sanitize(input.html, { USE_PROFILES: { html: false } }),
      images,
    }
  }

  return { html: cleaned, images }
}

function makeCandidate(
  kind: PasteImageCandidate["kind"],
  alt: string,
  extra: { file?: File; sourceURL?: string }
): PasteImageCandidate {
  candidateCounter += 1
  const key = `paste-image-${Date.now()}-${candidateCounter}`
  return { key, kind, alt, ...extra }
}

function replaceWithMarker(img: Element, key: string, alt: string) {
  const anchor = document.createElement("a")
  anchor.setAttribute("data-xuanchu-paste-image", key)
  // 文本 marker 在 Tiptap 解析 HTML 时仍会保留；属性仅用于 sanitizer 的调用方定位。
  anchor.textContent = `[[xuanchu-paste:${key}:${alt}]]`
  img.replaceWith(anchor)
}

// pickRemoteImageURL 按 src → data-src/data-original → srcset 最大 descriptor 顺序挑选。
function pickRemoteImageURL(img: Element): string | null {
  const src = img.getAttribute("src")
  if (src && /^https?:\/\//i.test(src)) return normalizeRemoteURL(src)
  const dataSrc = img.getAttribute("data-src") ?? img.getAttribute("data-original")
  if (dataSrc && /^https?:\/\//i.test(dataSrc)) return normalizeRemoteURL(dataSrc)
  const srcset = img.getAttribute("srcset") ?? img.getAttribute("data-srcset")
  if (srcset) {
    const best = pickBestSrcsetEntry(srcset)
    if (best && /^https?:\/\//i.test(best)) return normalizeRemoteURL(best)
  }
  return null
}

// normalizeRemoteURL 规范化为：scheme/host 小写、移除 fragment、保留 path/query。
function normalizeRemoteURL(raw: string): string {
  try {
    const u = new URL(raw)
    u.hash = ""
    u.host = u.host.toLowerCase()
    u.protocol = u.protocol.toLowerCase()
    return u.toString()
  } catch {
    return raw
  }
}

function pickBestSrcsetEntry(srcset: string): string | null {
  let bestURL: string | null = null
  let bestDescriptor = -1
  for (const entry of srcset.split(",")) {
    const trimmed = entry.trim()
    if (!trimmed) continue
    const parts = trimmed.split(/\s+/)
    const url = parts[0]
    if (!url) continue
    let descriptor = 0
    if (parts[1] && parts[1].endsWith("w")) {
      const n = Number.parseInt(parts[1].slice(0, -1), 10)
      if (!Number.isNaN(n)) descriptor = n
    }
    if (descriptor > bestDescriptor) {
      bestDescriptor = descriptor
      bestURL = url
    }
  }
  return bestURL
}
