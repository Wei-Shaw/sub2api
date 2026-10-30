/** Legacy five_hour/seven_day keys are slots, not fixed upstream durations. */
export function openAIWindowLabel(minutes: number | undefined, fallback: string): string {
  if (!Number.isSafeInteger(minutes) || !minutes || minutes < 0 || minutes > 366 * 24 * 60) return fallback
  if (minutes % 1440 === 0) return `${minutes / 1440}d`
  if (minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
}
