import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CodexIdentityTemplatesSettings from '../CodexIdentityTemplatesSettings.vue'
import type { CodexIdentityTemplate } from '@/types/codexIdentity'

const {
  listTemplates,
  getTemplate,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  listProxies,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  getTemplate: vi.fn(),
  createTemplate: vi.fn(),
  updateTemplate: vi.fn(),
  deleteTemplate: vi.fn(),
  listProxies: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    codexIdentityTemplates: {
      list: listTemplates,
      getByID: getTemplate,
      create: createTemplate,
      update: updateTemplate,
      delete: deleteTemplate,
    },
    proxies: { getAll: listProxies },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/utils/format', () => ({
  formatDateTimeToMinute: (value: string) => value,
}))

const messages: Record<string, string> = {
  'admin.settings.codexProfiles.copyName': '{name} - Copy',
  'admin.settings.codexProfiles.errors.CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT': 'Revision conflict: {current_revision}',
  'admin.settings.codexProfiles.validation.nameRequired': 'Name required',
  'admin.settings.codexProfiles.validation.profilesInvalid': 'Profiles invalid',
  'admin.settings.codexProfiles.created': 'Created',
  'admin.settings.codexProfiles.updated': 'Updated',
  'admin.settings.codexProfiles.deleted': 'Deleted',
}

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    locale: { value: 'en-US' },
    te: (key: string) => key in messages,
    t: (key: string, params?: Record<string, unknown>) => {
      let value = messages[key] ?? key
      for (const [name, replacement] of Object.entries(params ?? {})) {
        value = value.replace(`{${name}}`, String(replacement))
      }
      return value
    },
  }),
}))

const BaseDialogStub = defineComponent({
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show" class="dialog-stub"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
})
const ConfirmDialogStub = defineComponent({
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" class="confirm-stub"><span>{{ message }}</span><button type="button" :data-testid="title.includes(\'updateImpactTitle\') ? \'confirm-update-template\' : \'confirm-delete-template\'" @click="$emit(\'confirm\')">Confirm</button></div>',
})

const template: CodexIdentityTemplate = {
  id: 7,
  name: 'Primary egress',
  description: 'Desktop and CLI',
  revision: 3,
  session_policy: { mode: 'conversation_isolated' },
  affinity_ttl_seconds: 3600,
  unsupported_policy: 'reject',
  profiles: [
    {
      id: 11,
      os_class: 'windows',
      canonical_surface: 'desktop',
      architecture: 'x86_64',
      proxy_mode: 'inherit',
      slot_count: 1,
      catalog_version: 1,
      slots: [],
    },
    {
      id: 12,
      os_class: 'windows',
      canonical_surface: 'cli',
      architecture: 'arm64',
      proxy_mode: 'direct',
      slot_count: 2,
      catalog_version: 1,
      slots: [],
    },
  ],
  assigned_account_count: 2,
  created_at: '2026-08-31T10:00:00Z',
  updated_at: '2026-08-31T11:00:00Z',
}

const mountView = () => mount(CodexIdentityTemplatesSettings, {
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      ConfirmDialog: ConfirmDialogStub,
      Icon: true,
    },
  },
})

describe('CodexIdentityTemplatesSettings', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue([template])
    getTemplate.mockReset().mockResolvedValue(template)
    createTemplate.mockReset().mockResolvedValue({ ...template, id: 8 })
    updateTemplate.mockReset().mockResolvedValue({ ...template, revision: 4 })
    deleteTemplate.mockReset().mockResolvedValue({ message: 'deleted' })
    listProxies.mockReset().mockResolvedValue([{ id: 9, name: 'Tokyo', status: 'active' }])
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('lists composite Profiles and assigned account counts without nesting a form', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Primary egress')
    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.desktop')
    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.cli')
    expect(wrapper.text()).toContain('2')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('creates a template with Desktop and CLI as separate profiles', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="create-codex-template"]').trigger('click')
    expect(wrapper.find('[data-testid="codex-identity-policy-toggle"]').exists()).toBe(false)
    await wrapper.get('[data-testid="codex-template-name"]').setValue('Dual surface')
    await wrapper.get('[data-testid="codex-profile-windows-desktop"] button[role="switch"]').trigger('click')
    await wrapper.get('[data-testid="codex-profile-windows-cli"] button[role="switch"]').trigger('click')
    await wrapper.get('[data-testid="save-codex-template"]').trigger('click')
    await flushPromises()

    expect(createTemplate).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Dual surface',
      profiles: expect.arrayContaining([
        expect.objectContaining({ os_class: 'windows', canonical_surface: 'desktop', catalog_version: 1 }),
        expect.objectContaining({ os_class: 'windows', canonical_surface: 'cli', catalog_version: 1 }),
      ]),
    }))
    expect(updateTemplate).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('Created')
  })

  it('loads the latest detail and sends its expected revision on update', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[aria-label="admin.settings.codexProfiles.edit: Primary egress"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="codex-template-name"]').setValue('Updated egress')
    await wrapper.get('[data-testid="save-codex-template"]').trigger('click')
    expect(updateTemplate).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-update-template"]').trigger('click')
    await flushPromises()

    expect(getTemplate).toHaveBeenCalledWith(7)
    expect(updateTemplate).toHaveBeenCalledWith(7, expect.objectContaining({
      name: 'Updated egress',
      expected_revision: 3,
      confirm_assigned_accounts: true,
    }))
    expect(showSuccess).toHaveBeenCalledWith('Updated')
  })

  it('copies into a new editable template and deletes through confirmation', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[aria-label="admin.settings.codexProfiles.copy: Primary egress"]').trigger('click')
    expect((wrapper.get('[data-testid="codex-template-name"]').element as HTMLInputElement).value)
      .toBe('Primary egress - Copy')
    await wrapper.get('[data-testid="save-codex-template"]').trigger('click')
    await flushPromises()
    expect(createTemplate).toHaveBeenCalled()

    await wrapper.get('[aria-label="admin.settings.codexProfiles.delete: Primary egress"]').trigger('click')
    await wrapper.get('[data-testid="confirm-delete-template"]').trigger('click')
    await flushPromises()
    expect(deleteTemplate).toHaveBeenCalledWith(7)
    expect(showSuccess).toHaveBeenCalledWith('Deleted')
  })

  it('localizes optimistic-concurrency errors', async () => {
    updateTemplate.mockRejectedValueOnce({
      reason: 'CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT',
      metadata: { current_revision: 4 },
      message: 'conflict',
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[aria-label="admin.settings.codexProfiles.edit: Primary egress"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="save-codex-template"]').trigger('click')
    await wrapper.get('[data-testid="confirm-update-template"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('Revision conflict: 4')
  })
})
