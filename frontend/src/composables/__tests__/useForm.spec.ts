import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { setActivePinia, createPinia REDACTED from 'pinia'
import { useForm REDACTED from '@/composables/useForm'
import { useAppStore REDACTED from '@/stores/app'

// Mock API 依赖（app store 内部引用了这些）
vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
REDACTED))
vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn(),
REDACTED))

describe('useForm', () => {
  let appStore: ReturnType<typeof useAppStore>

  beforeEach(() => {
    setActivePinia(createPinia())
    appStore = useAppStore()
    vi.clearAllMocks()
  REDACTED)

  it('submit 期间 loading 为 true，完成后为 false', async () => {
    let resolveSubmit: () => void
    const submitFn = vi.fn(
      () => new Promise<void>((resolve) => { resolveSubmit = resolve REDACTED)
    )

    const { loading, submit REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
    REDACTED)

    expect(loading.value).toBe(false)

    const submitPromise = submit()
    // 提交中
    expect(loading.value).toBe(true)

    resolveSubmit!()
    await submitPromise

    expect(loading.value).toBe(false)
  REDACTED)

  it('submit 成功时显示成功消息', async () => {
    const submitFn = vi.fn().mockResolvedValue(undefined)
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const { submit REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
      successMsg: '保存成功',
    REDACTED)

    await submit()

    expect(showSuccessSpy).toHaveBeenCalledWith('保存成功')
  REDACTED)

  it('submit 成功但无 successMsg 时不调用 showSuccess', async () => {
    const submitFn = vi.fn().mockResolvedValue(undefined)
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const { submit REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
    REDACTED)

    await submit()

    expect(showSuccessSpy).not.toHaveBeenCalled()
  REDACTED)

  it('submit 失败时显示错误消息并抛出错误', async () => {
    const error = Object.assign(new Error('提交失败'), {
      response: { data: { message: '服务器错误' REDACTED REDACTED,
    REDACTED)
    const submitFn = vi.fn().mockRejectedValue(error)
    const showErrorSpy = vi.spyOn(appStore, 'showError')

    const { submit, loading REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
    REDACTED)

    await expect(submit()).rejects.toThrow('提交失败')

    expect(showErrorSpy).toHaveBeenCalled()
    expect(loading.value).toBe(false)
  REDACTED)

  it('submit 失败时使用自定义 errorMsg', async () => {
    const submitFn = vi.fn().mockRejectedValue(new Error('network'))
    const showErrorSpy = vi.spyOn(appStore, 'showError')

    const { submit REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
      errorMsg: '自定义错误提示',
    REDACTED)

    await expect(submit()).rejects.toThrow()

    expect(showErrorSpy).toHaveBeenCalledWith('自定义错误提示')
  REDACTED)

  it('loading 中不会重复提交', async () => {
    let resolveSubmit: () => void
    const submitFn = vi.fn(
      () => new Promise<void>((resolve) => { resolveSubmit = resolve REDACTED)
    )

    const { submit REDACTED = useForm({
      form: { name: 'test' REDACTED,
      submitFn,
    REDACTED)

    // 第一次提交
    const p1 = submit()
    // 第二次提交（应被忽略，因为 loading=true）
    submit()

    expect(submitFn).toHaveBeenCalledTimes(1)

    resolveSubmit!()
    await p1
  REDACTED)

  it('传递 form 数据到 submitFn', async () => {
    const formData = { name: 'test', email: 'test@example.com' REDACTED
    const submitFn = vi.fn().mockResolvedValue(undefined)

    const { submit REDACTED = useForm({
      form: formData,
      submitFn,
    REDACTED)

    await submit()

    expect(submitFn).toHaveBeenCalledWith(formData)
  REDACTED)
REDACTED)
