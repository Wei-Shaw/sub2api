import { describe, it, expect, vi, beforeEach, afterEach REDACTED from 'vitest'
import { setActivePinia, createPinia REDACTED from 'pinia'

// Mock i18n
vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    REDACTED,
  REDACTED,
REDACTED))

// Mock app store
const mockShowSuccess = vi.fn()
const mockShowError = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
  REDACTED),
REDACTED))

import { useClipboard REDACTED from '@/composables/useClipboard'

describe('useClipboard', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()

    // 默认模拟安全上下文 + Clipboard API
    Object.defineProperty(window, 'isSecureContext', { value: true, writable: true REDACTED)
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: vi.fn().mockResolvedValue(undefined),
      REDACTED,
      writable: true,
      configurable: true,
    REDACTED)
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
    // 恢复 execCommand
    if ('execCommand' in document) {
      delete (document as any).execCommand
    REDACTED
  REDACTED)

  it('复制成功后 copied 变为 true', async () => {
    const { copied, copyToClipboard REDACTED = useClipboard()

    expect(copied.value).toBe(false)

    await copyToClipboard('hello')

    expect(copied.value).toBe(true)
  REDACTED)

  it('copied 在 2 秒后自动恢复为 false', async () => {
    const { copied, copyToClipboard REDACTED = useClipboard()

    await copyToClipboard('hello')
    expect(copied.value).toBe(true)

    vi.advanceTimersByTime(2000)

    expect(copied.value).toBe(false)
  REDACTED)

  it('复制成功时调用 showSuccess', async () => {
    const { copyToClipboard REDACTED = useClipboard()

    await copyToClipboard('hello', '已复制')

    expect(mockShowSuccess).toHaveBeenCalledWith('已复制')
  REDACTED)

  it('无自定义消息时使用 i18n 默认消息', async () => {
    const { copyToClipboard REDACTED = useClipboard()

    await copyToClipboard('hello')

    expect(mockShowSuccess).toHaveBeenCalledWith('common.copiedToClipboard')
  REDACTED)

  it('空文本返回 false 且不复制', async () => {
    const { copyToClipboard, copied REDACTED = useClipboard()

    const result = await copyToClipboard('')

    expect(result).toBe(false)
    expect(copied.value).toBe(false)
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled()
  REDACTED)

  it('Clipboard API 失败时降级到 fallback', async () => {
    const writeTextMock = navigator.clipboard.writeText as any
    writeTextMock.mockRejectedValue(new Error('API failed'))

    // jsdom 没有 execCommand，手动定义
    const documentAny = document as any
    documentAny.execCommand = vi.fn().mockReturnValue(true)

    const { copyToClipboard, copied REDACTED = useClipboard()
    const result = await copyToClipboard('fallback text')

    expect(result).toBe(true)
    expect(copied.value).toBe(true)
    expect(document.execCommand).toHaveBeenCalledWith('copy')
  REDACTED)

  it('非安全上下文使用 fallback', async () => {
    Object.defineProperty(window, 'isSecureContext', { value: false, writable: true REDACTED)

    const documentAny = document as any
    documentAny.execCommand = vi.fn().mockReturnValue(true)

    const { copyToClipboard, copied REDACTED = useClipboard()
    const result = await copyToClipboard('insecure context text')

    expect(result).toBe(true)
    expect(copied.value).toBe(true)
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled()
    expect(document.execCommand).toHaveBeenCalledWith('copy')
  REDACTED)

  it('所有复制方式均失败时调用 showError', async () => {
    const writeTextMock = navigator.clipboard.writeText as any
    writeTextMock.mockRejectedValue(new Error('fail'))

    const documentAny = document as any
    documentAny.execCommand = vi.fn().mockReturnValue(false)

    const { copyToClipboard, copied REDACTED = useClipboard()
    const result = await copyToClipboard('text')

    expect(result).toBe(false)
    expect(copied.value).toBe(false)
    expect(mockShowError).toHaveBeenCalled()
  REDACTED)
REDACTED)
