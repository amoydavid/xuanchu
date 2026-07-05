export type DateBoundary = "start" | "end"

export function dateToUnix(date: Date, boundary: DateBoundary): number {
  const hours = boundary === "end" ? 23 : 0
  const minutes = boundary === "end" ? 59 : 0
  const seconds = boundary === "end" ? 59 : 0
  return Math.floor(
    new Date(
      date.getFullYear(),
      date.getMonth(),
      date.getDate(),
      hours,
      minutes,
      seconds
    ).getTime() / 1000
  )
}

export function formatLocalDate(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, "0")
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}
