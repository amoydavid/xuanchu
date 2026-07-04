const RAW_HTML_TAG_RE = /<\/?[A-Za-z][A-Za-z0-9:-]*(?:\s[^<>]*)?\s*\/?>/g
const PROTECTED_MARKDOWN_RE = /(```[\s\S]*?```|`[^`\n]*`|\[[^\]\n]+\]\([^)\n]*\))/g
const MARKDOWN_LINK_RE = /^\[[^\]\n]+\]\([^)\n]*\)$/

export function isAllowedMarkdownHref(url: string): boolean {
  return /^(https?:\/\/|mailto:)/i.test(url.trim())
}

export function escapeMarkdownHtml(source: string): string {
  if (!source) {
    return ""
  }

  return source
    .split(PROTECTED_MARKDOWN_RE)
    .map((part) => {
      if (
        part.startsWith("```") ||
        part.startsWith("`") ||
        MARKDOWN_LINK_RE.test(part)
      ) {
        return part
      }

      return part.replace(RAW_HTML_TAG_RE, (tag) =>
        tag.replaceAll("<", "&lt;").replaceAll(">", "&gt;")
      )
    })
    .join("")
}
