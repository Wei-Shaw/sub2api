import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import ImportDataModal from '@/components/admin/proxy/ImportDataModal.vue'

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
    proxies: {
      importData: vi.fn()
    REDACTED
  REDACTED
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  REDACTED)
REDACTED))

describe('Proxy ImportDataModal', () => {
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
    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportSelectFile')
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

    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportParseFailed')
  REDACTED)
REDACTED)
