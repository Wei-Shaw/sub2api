export function buildOpsErrorTimeParams(
  timeRange: string,
  customStartTime?: string | null,
  customEndTime?: string | null
): Record<string, string> {
  if (timeRange === 'custom' && customStartTime && customEndTime) {
    return { start_time: customStartTime, end_time: customEndTime }
  }

  return { time_range: timeRange === 'custom' ? '1h' : timeRange }
}
