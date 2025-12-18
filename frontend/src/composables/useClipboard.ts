import { ref REDACTED from 'vue'
import { useAppStore REDACTED from '@/stores/app'

export function useClipboard() {
  const appStore = useAppStore()
  const copied = ref(false)

  const copyToClipboard = async (text: string, successMessage = 'Copied to clipboard') => {
    if (!text) return false

    try {
      await navigator.clipboard.writeText(text)
      copied.value = true
      appStore.showSuccess(successMessage)
      setTimeout(() => {
        copied.value = false
      REDACTED, 2000)
      return true
    REDACTED catch {
      // Fallback for older browsers
      const input = document.createElement('input')
      input.value = text
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
      copied.value = true
      appStore.showSuccess(successMessage)
      setTimeout(() => {
        copied.value = false
      REDACTED, 2000)
      return true
    REDACTED
  REDACTED

  return {
    copied,
    copyToClipboard
  REDACTED
REDACTED
