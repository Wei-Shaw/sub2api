import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const apiMocks = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  updateS3Config: vi.fn(),
  testS3Connection: vi.fn(),
  getImageStorageConfig: vi.fn(),
  updateImageStorageConfig: vi.fn(),
  testImageStorageConnection: vi.fn(),
  getSchedule: vi.fn(),
  updateSchedule: vi.fn(),
  listBackups: vi.fn(),
  createBackup: vi.fn(),
  getBackup: vi.fn(),
  getDownloadURL: vi.fn(),
  restoreBackup: vi.fn(),
  deleteBackup: vi.fn(),
}))

const storeMocks = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: apiMocks,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => storeMocks,
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
      params?.index === undefined ? key : `${key}:${params.index}`,
  }),
}))

vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: defineComponent({
    name: 'ConfirmDialog',
    props: {
      show: Boolean,
      title: String,
      message: String,
      confirmText: String,
      cancelText: String,
    },
    emits: ['confirm', 'cancel'],
    setup(props, { emit }) {
      return () =>
        props.show
          ? h('div', { class: 'confirm-dialog-stub' }, [
              h('h3', props.title),
              h('p', props.message),
              h('button', { type: 'button', onClick: () => emit('cancel') }, props.cancelText),
              h('button', { type: 'button', onClick: () => emit('confirm') }, props.confirmText),
            ])
          : null
    },
  }),
}))

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: defineComponent({
    name: 'BaseDialog',
    props: {
      show: Boolean,
      title: String,
      width: String,
    },
    emits: ['close'],
    setup(props, { slots, emit }) {
      return () =>
        props.show
          ? h('section', { class: 'base-dialog-stub', 'data-title': props.title }, [
              h('h3', props.title),
              slots.default?.(),
              slots.footer?.({ close: () => emit('close') }),
            ])
          : null
    },
  }),
}))

vi.mock('@/components/common/Input.vue', () => ({
  default: defineComponent({
    name: 'InputStub',
    props: {
      modelValue: {
        type: [String, Number],
        default: '',
      },
      type: String,
      label: String,
      placeholder: String,
      autocomplete: String,
    },
    emits: ['update:modelValue', 'enter'],
    setup(props, { emit }) {
      return () =>
        h('label', [
          props.label ? h('span', props.label) : null,
          h('input', {
            type: props.type || 'text',
            value: props.modelValue ?? '',
            placeholder: props.placeholder,
            autocomplete: props.autocomplete,
            onInput: (event: Event) =>
              emit('update:modelValue', (event.target as HTMLInputElement).value),
            onKeyup: (event: KeyboardEvent) => {
              if (event.key === 'Enter') emit('enter', event)
            },
          }),
        ])
    },
  }),
}))

import BackupView from '../BackupView.vue'

const backupRecord = {
  id: 'backup-1',
  status: 'completed',
  file_name: 'backup.sql.gz',
  size_bytes: 2048,
  triggered_by: 'manual',
  started_at: '2026-01-01T00:00:00Z',
}

function mountBackupView() {
  return mount(BackupView, {
    global: {
      stubs: {
        TotpStepUpDialog: true,
      },
    },
  })
}

async function mountLoadedView() {
  const wrapper = mountBackupView()
  await flushPromises()
  return wrapper
}

describe('BackupView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getS3Config.mockResolvedValue({
      endpoint: '',
      region: 'auto',
      bucket: '',
      access_key_id: '',
      prefix: 'backups/',
      force_path_style: false,
    })
    apiMocks.getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    apiMocks.getSchedule.mockResolvedValue({
      enabled: false,
      cron_expr: '0 2 * * *',
      retain_days: 14,
      retain_count: 10,
    })
    apiMocks.listBackups.mockResolvedValue({ items: [backupRecord] })
    apiMocks.restoreBackup.mockResolvedValue({
      ...backupRecord,
      restore_status: 'running',
    })
  })

  it('恢复备份使用统一密码输入弹框而不是 window.prompt', async () => {
    const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue('native-password')
    const wrapper = await mountLoadedView()

    const restoreButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'admin.backup.actions.restore')

    expect(restoreButton).toBeTruthy()
    await restoreButton!.trigger('click')
    await flushPromises()

    const confirmButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'common.confirm')

    expect(confirmButton).toBeTruthy()
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(promptSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.backup.actions.restorePasswordPrompt')

    const passwordInput = wrapper.find('input[autocomplete="current-password"]')
    expect(passwordInput.exists()).toBe(true)
    await passwordInput.setValue('restore-password')

    const submitButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'common.confirm')

    expect(submitButton).toBeTruthy()
    await submitButton!.trigger('click')
    await flushPromises()

    expect(apiMocks.restoreBackup).toHaveBeenCalledWith('backup-1', 'restore-password')
    promptSpy.mockRestore()
  })
})

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

describe('admin BackupView 分卷备份', () => {
  beforeEach(() => {
    apiMocks.getS3Config.mockResolvedValue({})
    apiMocks.getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    apiMocks.getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 })
    apiMocks.getDownloadURL.mockReset()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('显示分卷数并在下载时列出每个分卷链接', async () => {
    apiMocks.listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 }, { index: 2 }, { index: 3 }])],
    })
    apiMocks.getDownloadURL.mockResolvedValue({
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
    apiMocks.listBackups.mockResolvedValue({ items: [baseRecord('legacy')] })
    apiMocks.getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' })

    const wrapper = mountBackupView()
    await flushPromises()
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(apiMocks.getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  })

  it('运行中的备份不显示删除入口', async () => {
    apiMocks.listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' }],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })
})
