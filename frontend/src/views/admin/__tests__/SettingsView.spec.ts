import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { defineComponent, h, ref REDACTED from 'vue'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import SettingsView from '../SettingsView.vue'

const {
  getSettings,
  updateSettings,
  getWebSearchEmulationConfig,
  updateWebSearchEmulationConfig,
  getAdminApiKey,
  getOverloadCooldownSettings,
  getStreamTimeoutSettings,
  getRectifierSettings,
  getBetaPolicySettings,
  getGroups,
  listProxies,
  getProviders,
  fetchPublicSettings,
  adminSettingsFetch,
  showError,
  showSuccess,
REDACTED = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getWebSearchEmulationConfig: vi.fn(),
  updateWebSearchEmulationConfig: vi.fn(),
  getAdminApiKey: vi.fn(),
  getOverloadCooldownSettings: vi.fn(),
  getStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  getGroups: vi.fn(),
  listProxies: vi.fn(),
  getProviders: vi.fn(),
  fetchPublicSettings: vi.fn(),
  adminSettingsFetch: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
REDACTED))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings,
      updateSettings,
      getWebSearchEmulationConfig,
      updateWebSearchEmulationConfig,
      getAdminApiKey,
      getOverloadCooldownSettings,
      getStreamTimeoutSettings,
      getRectifierSettings,
      getBetaPolicySettings,
    REDACTED,
    groups: {
      getAll: getGroups,
    REDACTED,
    proxies: {
      list: listProxies,
    REDACTED,
    payment: {
      getProviders,
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning: vi.fn(),
    showInfo: vi.fn(),
    fetchPublicSettings,
  REDACTED),
REDACTED))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    fetch: adminSettingsFetch,
  REDACTED),
REDACTED))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn(),
  REDACTED),
REDACTED))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'error',
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: ref('zh-CN'),
    REDACTED),
  REDACTED
REDACTED)

const AppLayoutStub = { template: '<div><slot /></div>' REDACTED
const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    REDACTED,
  REDACTED,
  emits: ['update:modelValue'],
  setup(props, { emit REDACTED) {
    return () =>
      h('input', {
        class: 'toggle-stub',
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLInputElement).checked)
        REDACTED,
      REDACTED)
  REDACTED,
REDACTED)

const SelectStub = defineComponent({
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: '',
    REDACTED,
    options: {
      type: Array,
      default: () => [],
    REDACTED,
    placeholder: {
      type: String,
      default: '',
    REDACTED,
  REDACTED,
  emits: ['update:modelValue', 'change'],
  setup(props, { emit REDACTED) {
    const onChange = (event: Event) => {
      const target = event.target as HTMLSelectElement
      emit('update:modelValue', target.value)
      const option = (props.options as Array<Record<string, unknown>>).find(
        (item) => String(item.value ?? '') === target.value
      ) ?? null
      emit('change', target.value, option)
    REDACTED

    return () =>
      h(
        'select',
        {
          class: 'select-stub',
          value: props.modelValue ?? '',
          'data-placeholder': props.placeholder,
          onChange,
        REDACTED,
        (props.options as Array<Record<string, unknown>>).map((option) =>
          h(
            'option',
            {
              key: `${String(option.value ?? '')REDACTED:${String(option.label ?? '')REDACTED`,
              value: option.value as string,
            REDACTED,
            String(option.label ?? '')
          )
        )
      )
  REDACTED,
REDACTED)

const baseSettingsResponse = {
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  invitation_code_enabled: false,
  password_reset_enabled: false,
  totp_enabled: false,
  totp_encryption_key_configured: false,
  default_balance: 0,
  default_concurrency: 1,
  default_subscriptions: [],
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: '',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  hide_ccs_import_button: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50, 100],
  backend_mode_enabled: false,
  custom_menu_items: [],
  custom_endpoints: [],
  frontend_url: '',
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password_configured: false,
  smtp_from_email: '',
  smtp_from_name: '',
  smtp_use_tls: true,
  turnstile_enabled: false,
  turnstile_site_key: '',
  turnstile_secret_key_configured: false,
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: '',
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: '',
  oidc_connect_enabled: false,
  oidc_connect_provider_name: 'OIDC',
  oidc_connect_client_id: '',
  oidc_connect_client_secret_configured: false,
  oidc_connect_issuer_url: '',
  oidc_connect_discovery_url: '',
  oidc_connect_authorize_url: '',
  oidc_connect_token_url: '',
  oidc_connect_userinfo_url: '',
  oidc_connect_jwks_url: '',
  oidc_connect_scopes: 'openid email profile',
  oidc_connect_redirect_url: '',
  oidc_connect_frontend_redirect_url: '/auth/oidc/callback',
  oidc_connect_token_auth_method: 'client_secret_post',
  oidc_connect_use_pkce: true,
  oidc_connect_validate_id_token: true,
  oidc_connect_allowed_signing_algs: 'RS256,ES256,PS256',
  oidc_connect_clock_skew_seconds: 120,
  oidc_connect_require_email_verified: false,
  oidc_connect_userinfo_email_path: '',
  oidc_connect_userinfo_id_path: '',
  oidc_connect_userinfo_username_path: '',
  enable_model_fallback: false,
  fallback_model_anthropic: '',
  fallback_model_openai: '',
  fallback_model_gemini: '',
  fallback_model_antigravity: '',
  enable_identity_patch: false,
  identity_patch_prompt: '',
  ops_monitoring_enabled: false,
  ops_realtime_monitoring_enabled: false,
  ops_query_mode_default: 'auto',
  ops_metrics_interval_seconds: 60,
  min_claude_code_version: '',
  max_claude_code_version: '',
  allow_ungrouped_key_scheduling: false,
  enable_fingerprint_unification: true,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  payment_enabled: true,
  payment_min_amount: 1,
  payment_max_amount: 10000,
  payment_daily_limit: 50000,
  payment_order_timeout_minutes: 30,
  payment_max_pending_orders: 3,
  payment_enabled_types: [],
  payment_balance_disabled: false,
  payment_balance_recharge_multiplier: 1,
  payment_recharge_fee_rate: 0,
  payment_load_balance_strategy: 'round-robin',
  payment_product_name_prefix: '',
  payment_product_name_suffix: '',
  payment_help_image_url: '',
  payment_help_text: '',
  payment_cancel_rate_limit_enabled: false,
  payment_cancel_rate_limit_max: 10,
  payment_cancel_rate_limit_window: 1,
  payment_cancel_rate_limit_unit: 'day',
  payment_cancel_rate_limit_window_mode: 'rolling',
  payment_visible_method_alipay_source: 'alipay_direct',
  payment_visible_method_wxpay_source: 'invalid-source',
  payment_visible_method_alipay_enabled: true,
  payment_visible_method_wxpay_enabled: true,
  openai_advanced_scheduler_enabled: false,
  balance_low_notify_enabled: false,
  balance_low_notify_threshold: 0,
  balance_low_notify_recharge_url: '',
  account_quota_notify_enabled: false,
  account_quota_notify_emails: [],
REDACTED

function mountView() {
  return mount(SettingsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Select: SelectStub,
        Toggle: ToggleStub,
        Icon: true,
        ConfirmDialog: true,
        PaymentProviderList: true,
        PaymentProviderDialog: true,
        GroupBadge: true,
        GroupOptionItem: true,
        ProxySelector: true,
        ImageUpload: true,
        BackupSettings: true,
      REDACTED,
    REDACTED,
  REDACTED)
REDACTED

async function openPaymentTab(wrapper: ReturnType<typeof mountView>) {
  const paymentTabButton = wrapper
    .findAll('button')
    .find((node) => node.text().includes('admin.settings.tabs.payment'))

  expect(paymentTabButton).toBeDefined()
  await paymentTabButton?.trigger('click')
  await flushPromises()
REDACTED

describe('admin SettingsView payment visible method controls', () => {
  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    getWebSearchEmulationConfig.mockReset()
    updateWebSearchEmulationConfig.mockReset()
    getAdminApiKey.mockReset()
    getOverloadCooldownSettings.mockReset()
    getStreamTimeoutSettings.mockReset()
    getRectifierSettings.mockReset()
    getBetaPolicySettings.mockReset()
    getGroups.mockReset()
    listProxies.mockReset()
    getProviders.mockReset()
    fetchPublicSettings.mockReset()
    adminSettingsFetch.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getSettings.mockResolvedValue({ ...baseSettingsResponse REDACTED)
    updateSettings.mockImplementation(async (payload) => ({
      ...baseSettingsResponse,
      ...payload,
    REDACTED))
    getWebSearchEmulationConfig.mockResolvedValue({
      enabled: false,
      providers: [],
    REDACTED)
    updateWebSearchEmulationConfig.mockResolvedValue({
      enabled: false,
      providers: [],
    REDACTED)
    getAdminApiKey.mockResolvedValue({
      exists: false,
      masked_key: '',
    REDACTED)
    getOverloadCooldownSettings.mockResolvedValue({
      enabled: true,
      cooldown_minutes: 10,
    REDACTED)
    getStreamTimeoutSettings.mockResolvedValue({
      enabled: true,
      action: 'temp_unsched',
      temp_unsched_minutes: 5,
      threshold_count: 3,
      threshold_window_minutes: 10,
    REDACTED)
    getRectifierSettings.mockResolvedValue({
      enabled: true,
      thinking_signature_enabled: true,
      thinking_budget_enabled: true,
      apikey_signature_enabled: false,
      apikey_signature_patterns: [],
    REDACTED)
    getBetaPolicySettings.mockResolvedValue({
      rules: [],
    REDACTED)
    getGroups.mockResolvedValue([])
    listProxies.mockResolvedValue({
      items: [],
    REDACTED)
    getProviders.mockResolvedValue({
      data: [],
    REDACTED)
    fetchPublicSettings.mockResolvedValue(undefined)
    adminSettingsFetch.mockResolvedValue(undefined)
  REDACTED)

  it('loads canonical source options and normalizes existing values', async () => {
    const wrapper = mountView()

    await flushPromises()
    await openPaymentTab(wrapper)

    const paymentSourceSelects = wrapper
      .findAll('select.select-stub')
      .filter((node) => ['alipay', 'wxpay'].includes(node.attributes('data-placeholder')))

    expect(paymentSourceSelects).toHaveLength(2)

    const alipaySelect = paymentSourceSelects.find(
      (node) => node.attributes('data-placeholder') === 'alipay'
    )
    const wxpaySelect = paymentSourceSelects.find(
      (node) => node.attributes('data-placeholder') === 'wxpay'
    )

    expect(alipaySelect?.element.value).toBe('official_alipay')
    expect(alipaySelect?.findAll('option').map((option) => option.element.value)).toEqual([
      '',
      'official_alipay',
      'easypay_alipay',
    ])

    expect(wxpaySelect?.element.value).toBe('')
    expect(wxpaySelect?.findAll('option').map((option) => option.element.value)).toEqual([
      '',
      'official_wxpay',
      'easypay_wxpay',
    ])
  REDACTED)

  it('saves canonical source keys selected from the dropdowns', async () => {
    const wrapper = mountView()

    await flushPromises()
    await openPaymentTab(wrapper)

    const paymentSourceSelects = wrapper
      .findAll('select.select-stub')
      .filter((node) => ['alipay', 'wxpay'].includes(node.attributes('data-placeholder')))

    const alipaySelect = paymentSourceSelects.find(
      (node) => node.attributes('data-placeholder') === 'alipay'
    )
    const wxpaySelect = paymentSourceSelects.find(
      (node) => node.attributes('data-placeholder') === 'wxpay'
    )

    await alipaySelect?.setValue('easypay_alipay')
    await wxpaySelect?.setValue('official_wxpay')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledTimes(1)
    expect(updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        payment_visible_method_alipay_source: 'easypay_alipay',
        payment_visible_method_wxpay_source: 'official_wxpay',
        payment_visible_method_alipay_enabled: true,
        payment_visible_method_wxpay_enabled: true,
      REDACTED)
    )
  REDACTED)

  it('blocks saving when a visible payment method is enabled without a source', async () => {
    const wrapper = mountView()

    await flushPromises()
    await openPaymentTab(wrapper)

    const paymentSourceSelects = wrapper
      .findAll('select.select-stub')
      .filter((node) => ['alipay', 'wxpay'].includes(node.attributes('data-placeholder')))

    const alipaySelect = paymentSourceSelects.find(
      (node) => node.attributes('data-placeholder') === 'alipay'
    )

    await alipaySelect?.setValue('')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalled()
    expect(String(showError.mock.calls.at(-1)?.[0] ?? '')).toContain('支付来源')
  REDACTED)
REDACTED)
