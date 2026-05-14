import { describe, expect, it, vi REDACTED from 'vitest'
import { defineComponent REDACTED from 'vue'
import { mount REDACTED from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock REDACTED = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn()
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
    isSimpleMode: true
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

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    REDACTED
  REDACTED,
  emits: ['update:modelValue'],
  template: `
    <div>
      <button
        type="button"
        data-testid="rewrite-to-snapshot"
        @click="$emit('update:modelValue', ['gpt-5.2-2025-12-11'])"
      >
        rewrite
      </button>
      <span data-testid="model-whitelist-value">
        {{ Array.isArray(modelValue) ? modelValue.join(',') : '' REDACTEDREDACTED
      </span>
    </div>
  `
REDACTED)

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    REDACTED,
    options: {
      type: Array,
      default: () => []
    REDACTED
  REDACTED,
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label REDACTEDREDACTED
      </option>
    </select>
  `
REDACTED)

function buildAccount() {
  return {
    id: 1,
    name: 'OpenAI Key',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.2': 'gpt-5.2'
      REDACTED
    REDACTED,
    extra: {REDACTED,
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

function mountModal(account = buildAccount()) {
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
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      REDACTED
    REDACTED
  REDACTED)
REDACTED

describe('EditAccountModal', () => {
  it('reopening the same account rehydrates the OpenAI whitelist from props', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2-2025-12-11')

    await wrapper.setProps({ show: false REDACTED)
    await wrapper.setProps({ show: true REDACTED)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2': 'gpt-5.2'
    REDACTED)
  REDACTED)

  it('preserves model mappings when editing the whitelist', async () => {
    const account = buildAccount()
    account.credentials.model_mapping = {
      'gpt-5.2': 'gpt-5.2',
      'gpt-latest': 'gpt-5.2'
    REDACTED
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2-2025-12-11': 'gpt-5.2-2025-12-11',
      'gpt-latest': 'gpt-5.2'
    REDACTED)
  REDACTED)

  it('submits OpenAI compact mode and compact-only model mapping', async () => {
    const account = buildAccount()
    account.extra = {
      openai_compact_mode: 'force_on'
    REDACTED
    account.credentials = {
      ...account.credentials,
      compact_model_mapping: {
        'gpt-5.4': 'gpt-5.4-openai-compact'
      REDACTED
    REDACTED
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_compact_mode).toBe('force_on')
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.compact_model_mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-openai-compact'
    REDACTED)
  REDACTED)

  it('submits account-level Codex image generation bridge override', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_bridge: false,
      codex_image_generation_bridge_enabled: true
    REDACTED
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false REDACTED)
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('button[data-testid="codex-image-bridge-enabled"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.codex_image_generation_bridge).toBe(true)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge_enabled')
  REDACTED)
REDACTED)
