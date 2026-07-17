import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import BackupView from '../BackupView.vue'

const { backupAPI, showError } = vi.hoisted(() => ({
  backupAPI: {
    getS3Config: vi.fn(),
    updateS3Config: vi.fn(),
    testS3Connection: vi.fn(),
    getSchedule: vi.fn(),
    updateSchedule: vi.fn(),
    createBackup: vi.fn(),
    listBackups: vi.fn(),
    getBackup: vi.fn(),
    deleteBackup: vi.fn(),
    getDownloadURL: vi.fn(),
    restoreBackup: vi.fn(),
  },
  showError: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/api', () => ({
  adminAPI: { backup: backupAPI },
  totpAPI: { stepUp: vi.fn() },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

describe('BackupView S3 configuration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    backupAPI.getS3Config.mockResolvedValue({
      endpoint: '',
      region: 'auto',
      bucket: '',
      access_key_id: '',
      secret_access_key: '',
      prefix: 'backups/',
      force_path_style: false,
    })
    backupAPI.getSchedule.mockResolvedValue({
      enabled: false,
      cron_expr: '0 2 * * *',
      retain_days: 14,
      retain_count: 10,
    })
    backupAPI.listBackups.mockResolvedValue({ items: [] })
  })

  it('opens the TOTP step-up dialog when saving S3 configuration requires recent verification', async () => {
    backupAPI.updateS3Config.mockRejectedValue({
      status: 403,
      code: 'STEP_UP_REQUIRED',
      message: 'This operation requires recent two-factor verification',
    })

    const wrapper = mount(BackupView)
    await flushPromises()

    const saveButton = wrapper.findAll('button').find(button => button.text() === 'common.save')
    expect(saveButton).toBeDefined()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('stepUp.title')
    expect(showError).not.toHaveBeenCalledWith('This operation requires recent two-factor verification')
  })
})
