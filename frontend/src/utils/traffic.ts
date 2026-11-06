const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB'] as const
const BANDWIDTH_UNITS = ['bps', 'kbps', 'Mbps', 'Gbps', 'Tbps', 'Pbps', 'Ebps'] as const

const isKnownByteCount = (bytes: number | null | undefined): bytes is number =>
  typeof bytes === 'number' && Number.isFinite(bytes) && bytes >= 0

/** Format measured HTTP body bytes; missing observations must not become zero. */
export function formatTrafficBytes(bytes: number | null | undefined): string {
  if (!isKnownByteCount(bytes)) return '—'

  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit += 1
  }

  // Carry rounding at a unit boundary (rather than displaying "1024 KiB").
  if (Number(value.toFixed(2)) >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${Number(value.toFixed(2))} ${BYTE_UNITS[unit]}`
}

/** Average equivalent body bandwidth over the full statistics window, in SI bits/s. */
export function formatTrafficBandwidth(
  bytes: number | null | undefined,
  durationSeconds: number | null | undefined,
): string {
  if (!isKnownByteCount(bytes) || typeof durationSeconds !== 'number' || !Number.isFinite(durationSeconds) || durationSeconds <= 0) {
    return '—'
  }

  let value = (bytes / durationSeconds) * 8
  if (!Number.isFinite(value)) return '—'
  if (value > 0 && value < 0.01) return '< 0.01 bps'

  let unit = 0
  while (value >= 1000 && unit < BANDWIDTH_UNITS.length - 1) {
    value /= 1000
    unit += 1
  }
  if (Number(value.toFixed(2)) >= 1000 && unit < BANDWIDTH_UNITS.length - 1) {
    value /= 1000
    unit += 1
  }
  return `${Number(value.toFixed(2))} ${BANDWIDTH_UNITS[unit]}`
}

/**
 * Body bytes followed by their equivalent bandwidth over the same window,
 * comma-separated, for example `959.97 MiB, 1.5 Mbps`.
 * Missing measurements keep the byte placeholder instead of inventing a rate;
 * a known total with an unknown window reports the bandwidth as unknown.
 */
export function formatTrafficBytesWithBandwidth(
  bytes: number | null | undefined,
  durationSeconds: number | null | undefined,
): string {
  const formattedBytes = formatTrafficBytes(bytes)
  if (formattedBytes === '—') return formattedBytes
  return `${formattedBytes}, ${formatTrafficBandwidth(bytes, durationSeconds)}`
}

/** Savings relative to the original body. Compression can increase its size. */
export function formatTrafficCompressionRate(
  originalBytes: number | null | undefined,
  actualBytes: number | null | undefined,
): string {
  if (!isKnownByteCount(originalBytes) || originalBytes === 0 || !isKnownByteCount(actualBytes)) {
    return '—'
  }
  const rate = (1 - actualBytes / originalBytes) * 100
  if (!Number.isFinite(rate)) return '—'
  // Normalize rounded negative zero without clamping genuine negative rates.
  return `${Number(rate.toFixed(1)).toFixed(1)}%`
}
