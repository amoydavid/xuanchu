type SeriesReferenceFields = {
  id: string
  series_slug?: string | null
}

/** API 与 Router 使用的 series 引用。优先 series_slug（如 ops-s-1），fallback id（UUID）。 */
export function seriesRouteRef(series: SeriesReferenceFields): string {
  return series.series_slug || series.id
}

/** 用户看到的 series 引用。优先 series_slug，fallback id 前 8 位。 */
export function seriesDisplayRef(series: SeriesReferenceFields): string {
  return series.series_slug || series.id.slice(0, 8)
}
