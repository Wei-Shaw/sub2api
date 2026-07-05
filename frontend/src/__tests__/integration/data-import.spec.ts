import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  REDACTED)
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: vi.fn()
    REDACTED
  REDACTED
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  REDACTED)
REDACTED))

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
  REDACTED)

  it('未选择文件时提示错误', async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true REDACTED,
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' REDACTED
        REDACTED
      REDACTED
    REDACTED)

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  REDACTED)

  it('无效 JSON 时提示解析失败', async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true REDACTED,
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' REDACTED
        REDACTED
      REDACTED
    REDACTED)

    const input = wrapper.find('input[type="file"]')
    const file = new File(['invalid json'], 'data.json', { type: 'application/json' REDACTED)
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('invalid json')
    REDACTED)
    Object.defineProperty(input.element, 'files', {
      value: [file]
    REDACTED)

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await Promise.resolve()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  REDACTED)

  it('merges multiple selected JSON files before importing', async () => {
    const { adminAPI REDACTED = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    REDACTED)

    const wrapper = mount(ImportDataModal, {
      props: { show: true REDACTED,
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' REDACTED
        REDACTED
      REDACTED
    REDACTED)

    const input = wrapper.find('input[type="file"]')
    const first = new File([
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' REDACTED] REDACTED)
    ], 'first.json', { type: 'application/json' REDACTED)
    const second = new File([
      JSON.stringify({ exported_at: '2026-07-05T00:00:01Z', proxies: [{ proxy_key: 'p' REDACTED], accounts: [{ name: 'b' REDACTED] REDACTED)
    ], 'second.json', { type: 'application/json' REDACTED)
    Object.defineProperty(first, 'text', {
      value: () => Promise.resolve(JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' REDACTED] REDACTED))
    REDACTED)
    Object.defineProperty(second, 'text', {
      value: () => Promise.resolve(JSON.stringify({ exported_at: '2026-07-05T00:00:01Z', proxies: [{ proxy_key: 'p' REDACTED], accounts: [{ name: 'b' REDACTED] REDACTED))
    REDACTED)

    Object.defineProperty(input.element, 'files', {
      value: [first, second]
    REDACTED)

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        proxies: [{ proxy_key: 'p' REDACTED],
        accounts: [{ name: 'a' REDACTED, { name: 'b' REDACTED]
      REDACTED),
      skip_default_group_bind: true
    REDACTED)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  REDACTED)
REDACTED)
