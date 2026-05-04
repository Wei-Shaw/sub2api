const QR_IMAGE_HOSTS = new Set(['qr.sepay.vn'])

const IMAGE_DATA_URL_RE = /^data:image\//i
const IMAGE_PATH_RE = /\.(?:png|jpe?g|webp|gif|svg)(?:[?#].*)?$/i

export function shouldRenderQRCodeAsImage(qrCode: string): boolean {
  const value = qrCode.trim()
  if (!value) return false
  if (IMAGE_DATA_URL_RE.test(value)) return true
  try {
    const parsed = new URL(value)
    if (QR_IMAGE_HOSTS.has(parsed.hostname.toLowerCase())) return true
    return IMAGE_PATH_RE.test(parsed.pathname)
  } catch {
    return false
  }
}
