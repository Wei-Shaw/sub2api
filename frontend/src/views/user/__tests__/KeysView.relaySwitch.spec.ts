import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import KeysView from '../KeysView.vue'

const {
  buildRelaySwitchProviderImportDeeplink,
  getAvailable,
  getDashboardApiKeysUsage,
  getPublicSettings,
  getUserGroupRates,
  list,
  showError
} = vi.hoisted(() => ({
  buildRelaySwitchProviderImportDeeplink: vi.fn(),
  getAvailable: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getPublicSettings: vi.fn(),
  getUserGroupRates: vi.fn(),
  list: vi.fn(),
  showError: vi.fn()
}))

const messages: Record<string, string> = {
  'keys.importToCcSwitch': 'Import to CCS',
  'keys.importToRelaySwitch': 'Import to RS',
  'keys.relaySwitchNotInstalled': 'RelaySwitch missing'
}

vi.mock('@/api', () => ({
  authAPI: {
    getPublicSettings
  },
  keysAPI: {
    list
  },
  usageAPI: {
    getDashboardApiKeysUsage
  },
  userGroupsAPI: {
    getAvailable,
    getUserGroupRates
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError
  })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('@/utils/relayswitchImport', () => ({
  buildRelaySwitchProviderImportDeeplink
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

const publicSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  force_email_on_third_party_signup: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: false,
  password_reset_enabled: true,
  invitation_code_enabled: false,
  turnstile_enabled: false,
  turnstile_site_key: '',
  site_name: 'Test Site',
  site_logo: '',
  site_subtitle: '',
  api_base_url: 'https://api.example.com',
  contact_info: '',
  doc_url: '',
  home_content: '',
  hide_ccs_import_button: false,
  payment_enabled: false,
  risk_control_enabled: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50],
  custom_menu_items: [],
  custom_endpoints: [],
  linuxdo_oauth_enabled: false,
  wechat_oauth_enabled: false
}

const apiKeyRow = {
  id: 1,
  name: 'Primary key',
  key: 'sk-row',
  status: 'active',
  group_id: null,
  group: null,
  quota: null,
  quota_used: 0,
  rate_limit_5h: null,
  rate_limit_1d: null,
  rate_limit_7d: null,
  rate_limit_5h_used: 0,
  rate_limit_1d_used: 0,
  rate_limit_7d_used: 0,
  rate_limit_5h_reset_at: null,
  rate_limit_1d_reset_at: null,
  rate_limit_7d_reset_at: null,
  expires_at: null,
  last_used_at: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot /></div>'
}
const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}
const IconStub = {
  props: ['name'],
  template: '<span :data-icon="name" />'
}

function mountView() {
  return mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Icon: IconStub,
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Pagination: true,
        SearchInput: true,
        Select: true,
        UseKeyModal: true
      }
    }
  })
}

describe('KeysView RelaySwitch import', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    buildRelaySwitchProviderImportDeeplink.mockReset()
    buildRelaySwitchProviderImportDeeplink.mockReturnValue('#relay-switch-import')
    getAvailable.mockReset()
    getAvailable.mockResolvedValue([])
    getDashboardApiKeysUsage.mockReset()
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getPublicSettings.mockReset()
    getPublicSettings.mockResolvedValue(publicSettings)
    getUserGroupRates.mockReset()
    getUserGroupRates.mockResolvedValue({})
    list.mockReset()
    list.mockResolvedValue({
      items: [apiKeyRow],
      total: 1,
      pages: 1
    })
    showError.mockReset()

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible'
    })
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('shows the RelaySwitch import button beside the CCS import button', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Import to CCS')
    expect(wrapper.text()).toContain('Import to RS')
  })

  it('hides the RelaySwitch import button when CCS import is hidden', async () => {
    getPublicSettings.mockResolvedValue({
      ...publicSettings,
      hide_ccs_import_button: true
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Import to CCS')
    expect(wrapper.text()).not.toContain('Import to RS')
  })

  it('builds a RelaySwitch provider deeplink and warns when the protocol handler does not open', async () => {
    const wrapper = mountView()
    await flushPromises()

    const importButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Import to RS'))
    expect(importButton).toBeTruthy()

    await importButton!.trigger('click')
    await nextTick()

    expect(buildRelaySwitchProviderImportDeeplink).toHaveBeenCalledWith({
      name: 'Test Site',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-row'
    })

    vi.advanceTimersByTime(1400)
    await nextTick()

    expect(showError).toHaveBeenCalledWith('RelaySwitch missing')
  })
})
