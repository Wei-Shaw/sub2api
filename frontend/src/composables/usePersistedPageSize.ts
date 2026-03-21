const STORAGE_KEY = 'table-page-size'
const DEFAULT_PAGE_SIZE = 20

/**
 * 从 localStorage 读取/写入 pageSize
 * 全局共享一个 key，所有表格统一偏好
 */
export function getPersistedPageSize(fallback = DEFAULT_PAGE_SIZE): number {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = Number(stored)
      if (Number.isFinite(parsed) && parsed > 0) return parsed
    }
  } catch {
    // localStorage 不可用（隐私模式等）
  }
  return fallback
}

export function setPersistedPageSize(size: number): void {
  try {
    localStorage.setItem(STORAGE_KEY, String(size))
  } catch {
    // 静默失败
  }
}
