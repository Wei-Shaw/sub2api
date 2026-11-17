import { ref } from 'vue'

const key = 'fork-usage-full-date'
function saved() {
  try { return localStorage.getItem(key) === 'true' } catch { return false }
}
export const fullUsageDate = ref(saved())
export function toggleUsageDate() {
  fullUsageDate.value = !fullUsageDate.value
  try { localStorage.setItem(key, String(fullUsageDate.value)) } catch { /* Storage is optional. */ }
}
export function usageTime(value: string, full = fullUsageDate.value) {
  const date = new Date(value)
  if (!value || !Number.isFinite(date.getTime())) return '—'
  const pad = (v: number) => String(v).padStart(2, '0')
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  return full ? `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${time}` : time
}
