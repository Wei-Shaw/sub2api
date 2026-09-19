import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import BackupView from '../BackupView.vue'

const {
  getS3Config,
  getImageStorageConfig,
  getSchedule,
  updateSchedule,
  deleteBackup,
  listBackups,
  getDownloadURL,
} = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  getImageStorageConfig: vi.fn(),
  getSchedule: vi.fn(),
  updateSchedule: vi.fn(),
  deleteBackup: vi.fn(),
  listBackups: vi.fn(),
  getDownloadURL: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config,
      updateS3Config: vi.fn(),
      testS3Connection: vi.fn(),
      getImageStorageConfig,
      updateImageStorageConfig: vi.fn(),
      testImageStorageConnection: vi.fn(),
      getSchedule,
      updateSchedule,
      createBackup: vi.fn(),
      listBackups,
      getBackup: vi.fn(),
      deleteBackup,
      getDownloadURL,
      restoreBackup: vi.fn(),
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (fn: () => unknown) => fn() }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.index !== undefined ? `${key}:${params.index}` : params?.day !== undefined ? `${key}:${params.day}` : key,
  }),
}))

const baseRecord = (id: string, parts?: unknown[]) => ({
  id,
  status: 'completed',
  backup_type: 'postgres',
  file_name: `${id}.sql.gz`,
  s3_key: `backups/${id}.sql.gz`,
  parts,
  size_bytes: 10,
  triggered_by: 'manual',
  started_at: '2026-08-09T00:00:00Z',
})

const wrappers: ReturnType<typeof mount>[] = []

function mountBackupView() {
  const wrapper = mount(BackupView, {
    global: {
      stubs: {
        TotpStepUpDialog: true,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('admin BackupView', () => {
  beforeEach(() => {
    getS3Config.mockResolvedValue({})
    getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 })
    updateSchedule.mockReset().mockResolvedValue({})
    deleteBackup.mockReset().mockResolvedValue(undefined)
    listBackups.mockResolvedValue({ items: [] })
    getDownloadURL.mockReset()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('显示分卷数并在下载时列出每个分卷链接', async () => {
    listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 }, { index: 2 }, { index: 3 }])],
    })
    getDownloadURL.mockResolvedValue({
      parts: [
        { index: 1, size_bytes: 5, url: 'https://example.test/part-1' },
        { index: 2, size_bytes: 6, url: 'https://example.test/part-2' },
        { index: 3, size_bytes: 7, url: 'https://example.test/part-3' },
      ],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.text()).toContain('3')
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    expect(downloadButton).toBeDefined()
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:1')
    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:3')
    expect(document.body.querySelector('a[href="https://example.test/part-2"]')).not.toBeNull()
  })

  it('旧单文件记录仍使用单个下载地址', async () => {
    listBackups.mockResolvedValue({ items: [baseRecord('legacy')] })
    getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' })

    const wrapper = mountBackupView()
    await flushPromises()
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  })

  it('运行中的备份不显示删除入口', async () => {
    listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' }],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })

  it('兼容旧配置并保留 0 值，不自动启用月度归档', async () => {
    getSchedule.mockResolvedValue({ enabled: true, cron_expr: '0 4 * * *', retain_days: 0, retain_count: 0 })
    const wrapper = mountBackupView()
    await flushPromises()
    expect((wrapper.get('[data-testid="backup-retain-days"]').element as HTMLInputElement).value).toBe('0')
    expect((wrapper.get('[data-testid="backup-retain-count"]').element as HTMLInputElement).value).toBe('0')
    expect((wrapper.get('[data-testid="archive-enabled"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenCalledWith(expect.objectContaining({ retain_days: 0, retain_count: 0, monthly_archive: expect.objectContaining({ enabled: false }) }))
  })

  it('多选日期，手填份数；永久保留隐藏数量并在取消后恢复', async () => {
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    await wrapper.get('#backup-archive-dates').trigger('click')
    await wrapper.get('#backup-archive-options input[value="15"]').setValue(true)
    await wrapper.get('#backup-archive-options input[value="last"]').setValue(true)
    expect(wrapper.get('#backup-archive-dates').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="archive-count"]').exists()).toBe(false)
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    await wrapper.get('[data-testid="archive-count"]').setValue(17)
    await wrapper.get('[data-testid="archive-forever"]').setValue(true)
    expect(wrapper.find('[data-testid="archive-count"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'BackupArchiveSettings' }).text()).not.toContain('admin.backup.archive.copies')
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    expect((wrapper.get('[data-testid="archive-count"]').element as HTMLInputElement).value).toBe('17')
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: { enabled: true, days: [1, 15], include_month_end: true, retain_count: 17 } }))
    await wrapper.get('[data-testid="archive-forever"]').setValue(true)
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: expect.objectContaining({ retain_count: 0 }) }))
  })

  it('空日期和非法份数不可保存，输入 0 不会误选永久保留', async () => {
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    await wrapper.get('#backup-archive-dates').trigger('click')
    await wrapper.get('#backup-archive-options input[value="1"]').setValue(false)
    const save = wrapper.get('[data-testid="backup-schedule"] .btn-primary')
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('#backup-archive-options input[value="15"]').setValue(true)
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    for (const value of ['0', '-1', '1.5', '']) {
      await wrapper.get('[data-testid="archive-count"]').setValue(value)
      expect(save.attributes('disabled')).toBeDefined()
      expect((wrapper.get('[data-testid="archive-forever"]').element as HTMLInputElement).checked).toBe(false)
    }
    await save.trigger('click')
    expect(updateSchedule).not.toHaveBeenCalled()
  })

  it('加载归档设置并支持键盘关闭日期选择器', async () => {
    getSchedule.mockResolvedValue({ enabled: true, cron_expr: '0 4 * * *', retain_days: 14, retain_count: 10,
      monthly_archive: { enabled: true, days: [1, 15], include_month_end: true, retain_count: 23 } })
    const wrapper = mountBackupView()
    await flushPromises()
    expect((wrapper.get('[data-testid="archive-count"]').element as HTMLInputElement).value).toBe('23')
    await wrapper.get('#backup-archive-dates').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(wrapper.find('#backup-archive-options').exists()).toBe(true)
    expect((wrapper.get('#backup-archive-options input[value="15"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('#backup-archive-options').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('#backup-archive-options').exists()).toBe(false)
  })

  it('归档删除需要单独确认，有限归档不会显示永不过期', async () => {
    listBackups.mockResolvedValue({ items: [{ ...baseRecord('archived'), monthly_archive: { dates: ['2026-09-01', '2026-09-15'], retain_count: 12 } }] })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountBackupView()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.backup.archive.badge')
    expect(wrapper.get('tbody tr td:nth-child(6)').text()).toBe('admin.backup.archive.retainLatest')
    const button = wrapper.findAll('button').find(button => button.text() === 'common.delete')!
    await button.trigger('click')
    expect(confirm).toHaveBeenCalledWith('admin.backup.archive.deleteConfirm')
    expect(deleteBackup).not.toHaveBeenCalled()
    confirm.mockReturnValue(true)
    await button.trigger('click')
    await flushPromises()
    expect(deleteBackup).toHaveBeenCalledWith('archived', true)
  })
})
