import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { defineComponent REDACTED from 'vue'
import { mount REDACTED from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock, authIsSimpleMode REDACTED = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  authIsSimpleMode: { value: true REDACTED
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  REDACTED)
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isSimpleMode() {
      return authIsSimpleMode.value
    REDACTED
  REDACTED)
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    REDACTED,
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] REDACTED),
      getSettings: vi.fn().mockResolvedValue({REDACTED)
    REDACTED,
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    REDACTED
  REDACTED
REDACTED))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    REDACTED
  REDACTED,
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
REDACTED)

function buildGrokOAuthAccount(
  credentials: Record<string, unknown> = {REDACTED,
  extra: Record<string, unknown> = {REDACTED
) {
  return {
    id: 5,
    name: 'Grok OAuth',
    notes: '',
    platform: 'grok',
    type: 'oauth',
    credentials: {
      expires_at: '2027-01-01T00:00:00Z',
      token_type: 'Bearer',
      ...credentials
    REDACTED,
    credentials_status: { has_access_token: true, has_refresh_token: true REDACTED,
    extra,
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  REDACTED as any
REDACTED

function mountModal(account: any) {
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    REDACTED,
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true
      REDACTED
    REDACTED
  REDACTED)
REDACTED

describe('EditAccountModal Grok OAuth upstream config', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false REDACTED)
  REDACTED)

  it('enabling the custom base URL toggle and saving persists base_url', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    // 官方地址 → 开关初始为关（视同未定制）
    const toggle = wrapper.get('[data-testid="grok-custom-base-url-toggle"]')
    await toggle.trigger('click')

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    await input.setValue('https://my-relay.example.com')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://my-relay.example.com')
  REDACTED)

  it('accepts the official API host as a manual endpoint switch and persists it', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="grok-custom-base-url-toggle"]').trigger('click')
    await wrapper.get('[data-testid="grok-custom-base-url-input"]').setValue('https://api.x.ai/v1')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://api.x.ai/v1')
  REDACTED)

  it('echoes a stored official API endpoint with the toggle on', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://us-west-2.api.x.ai/v1' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://us-west-2.api.x.ai/v1')
  REDACTED)

  it('fills the input from an endpoint preset chip', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="grok-custom-base-url-toggle"]').trigger('click')
    const presets = wrapper.findAll('[data-testid="grok-base-url-preset"]')
    expect(presets.length).toBe(5)

    // 第二个预设为官方 API (api.x.ai/v1)
    await presets[1].trigger('click')
    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://api.x.ai/v1')
  REDACTED)

  it('loads an existing custom base_url with the toggle on and keeps it on save', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://my-relay.example.com' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://my-relay.example.com')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://my-relay.example.com')
  REDACTED)

  it('keeps stored header overrides intact on an untouched save', async () => {
    const account = buildGrokOAuthAccount({
      header_override_enabled: true,
      header_overrides: {
        'user-agent': 'grok-pager/0.2.93',
        'x-grok-client-identifier': 'grok-pager',
        'x-grok-client-version': '0.2.93',
        'x-xai-token-auth': 'xai-grok-cli'
      REDACTED
    REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.header_override_enabled).toBe(true)
    expect(payload?.credentials?.header_overrides).toEqual({
      'user-agent': 'grok-pager/0.2.93',
      'x-grok-client-identifier': 'grok-pager',
      'x-grok-client-version': '0.2.93',
      'x-xai-token-auth': 'xai-grok-cli'
    REDACTED)
  REDACTED)

  it('shows the client-tool cache switch only for Grok OAuth accounts', () => {
    const grokOAuthWrapper = mountModal(buildGrokOAuthAccount())
    expect(grokOAuthWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(true)

    const grokAPIKeyWrapper = mountModal({
      ...buildGrokOAuthAccount(),
      type: 'apikey',
      credentials: { api_key: 'xai-test', base_url: 'https://api.x.ai/v1' REDACTED
    REDACTED)
    expect(grokAPIKeyWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(false)

    const openAIOAuthWrapper = mountModal({
      ...buildGrokOAuthAccount(),
      platform: 'openai'
    REDACTED)
    expect(openAIOAuthWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(false)
  REDACTED)

  it('loads and disables client-tool caching while preserving unrelated extra fields', async () => {
    const account = buildGrokOAuthAccount({REDACTED, {
      grok_client_tool_cache_enabled: true,
      custom_setting: 'keep-me'
    REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await toggle.trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra?.custom_setting).toBe('keep-me')
    expect(payload?.extra?.grok_client_tool_cache_enabled).toBe(false)
  REDACTED)

  it('defaults client-tool caching on when the setting is missing and persists explicit true', async () => {
    const account = buildGrokOAuthAccount({REDACTED, { custom_setting: 'keep-me' REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra).toMatchObject({
      grok_client_tool_cache_enabled: true,
      custom_setting: 'keep-me'
    REDACTED)
  REDACTED)

  it('keeps an explicit false opt-out when saving an untouched account', async () => {
    const account = buildGrokOAuthAccount(
      {REDACTED,
      {
        grok_client_tool_cache_enabled: false,
        custom_setting: 'keep-me'
      REDACTED
    )
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra).toMatchObject({
      grok_client_tool_cache_enabled: false,
      custom_setting: 'keep-me'
    REDACTED)
  REDACTED)

  it('shows malformed cache settings as disabled so the UI matches the fail-closed backend', () => {
    const account = buildGrokOAuthAccount(
      {REDACTED,
      {
        grok_client_tool_cache_enabled: 'true',
        custom_setting: 'keep-me'
      REDACTED
    )

    const wrapper = mountModal(account)
    expect(
      wrapper.get('[data-testid="grok-client-tool-cache-toggle"]').attributes('aria-checked')
    ).toBe('false')
  REDACTED)
REDACTED)
