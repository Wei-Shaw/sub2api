import { ref REDACTED from 'vue'
import { useAppStore REDACTED from '@/stores/app'
import { i18n REDACTED from '@/i18n'

const { t REDACTED = i18n.global

/**
 * 检测是否支持 Clipboard API（需要安全上下文：HTTPS/localhost）
 */
function isClipboardSupported(): boolean {
  return !!(navigator.clipboard && window.isSecureContext)
REDACTED

/**
 * 降级方案：使用 textarea + execCommand
 * 使用 textarea 而非 input，以正确处理多行文本
 */
function fallbackCopy(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.cssText = 'position:fixed;left:-9999px;top:-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  REDACTED finally {
    document.body.removeChild(textarea)
  REDACTED
REDACTED

export function useClipboard() {
  const appStore = useAppStore()
  const copied = ref(false)

  const copyToClipboard = async (
    text: string,
    successMessage?: string
  ): Promise<boolean> => {
    if (!text) return false

    let success = false

    if (isClipboardSupported()) {
      try {
        await navigator.clipboard.writeText(text)
        success = true
      REDACTED catch {
        success = fallbackCopy(text)
      REDACTED
    REDACTED else {
      success = fallbackCopy(text)
    REDACTED

    if (success) {
      copied.value = true
      appStore.showSuccess(successMessage || t('common.copiedToClipboard'))
      setTimeout(() => {
        copied.value = false
      REDACTED, 2000)
    REDACTED else {
      appStore.showError(t('common.copyFailed'))
    REDACTED

    return success
  REDACTED

  return { copied, copyToClipboard REDACTED
REDACTED
