import { describe, expect, it, vi, beforeEach, afterEach REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import { defineComponent REDACTED from 'vue'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModelsMock REDACTED = vi.hoisted(() => ({
  getAvailableModelsMock: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels: getAvailableModelsMock
    REDACTED
  REDACTED
REDACTED))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn()
  REDACTED)
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

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false REDACTED REDACTED,
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
REDACTED)

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' REDACTED,
    options: { type: Array, default: () => [] REDACTED,
    valueKey: { type: String, default: 'value' REDACTED,
    labelKey: { type: String, default: 'label' REDACTED
  REDACTED,
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option
        v-for="option in options"
        :key="option[valueKey]"
        :value="option[valueKey]"
      >
        {{ option[labelKey] REDACTEDREDACTED
      </option>
    </select>
  `
REDACTED)

const TextAreaStub = defineComponent({
  name: 'TextArea',
  props: {
    modelValue: { type: String, default: '' REDACTED
  REDACTED,
  emits: ['update:modelValue'],
  template: `
    <textarea
      v-bind="$attrs"
      :value="modelValue"
      @input="$emit('update:modelValue', $event.target.value)"
    />
  `
REDACTED)

function buildAccount() {
  return {
    id: 1,
    name: 'OpenAI OAuth',
    platform: 'openai',
    type: 'oauth',
    status: 'active',
    credentials: {REDACTED,
    extra: {REDACTED,
    concurrency: 1,
    priority: 1,
    proxy_id: null,
    auto_pause_on_expired: false
  REDACTED as any
REDACTED

describe('AccountTestModal', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    getAvailableModelsMock.mockReset()
    getAvailableModelsMock.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' REDACTED
    ])
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      body: {
        getReader: () => ({
          read: vi.fn().mockResolvedValue({ done: true, value: undefined REDACTED)
        REDACTED)
      REDACTED
    REDACTED as any)
    localStorage.setItem('auth_token', 'test-token')
  REDACTED)

  afterEach(() => {
    global.fetch = originalFetch
    localStorage.clear()
  REDACTED)

  it('posts compact mode for OpenAI compact probe', async () => {
    const wrapper = mount(AccountTestModal, {
      props: {
        show: true,
        account: buildAccount()
      REDACTED,
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          TextArea: TextAreaStub,
          Icon: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()
    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, options] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(options.body)).toMatchObject({
      model_id: 'gpt-5.4',
      mode: 'compact'
    REDACTED)
  REDACTED)

  it('renders Chat Completions path status from test SSE', async () => {
    const encoder = new TextEncoder()
    const chunks = [
      encoder.encode('data: {"type":"status","text":"已通过 /v1/chat/completions 验证"REDACTED\n\n'),
      encoder.encode('data: {"type":"test_complete","success":trueREDACTED\n\n')
    ]
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      body: {
        getReader: () => ({
          read: vi.fn().mockImplementation(() => Promise.resolve(
            chunks.length > 0
              ? { done: false, value: chunks.shift() REDACTED
              : { done: true, value: undefined REDACTED
          ))
        REDACTED)
      REDACTED
    REDACTED as any)

    const wrapper = mount(AccountTestModal, {
      props: {
        show: true,
        account: buildAccount()
      REDACTED,
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          TextArea: TextAreaStub,
          Icon: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()
    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(wrapper.text()).toContain('已通过 /v1/chat/completions 验证')
  REDACTED)
REDACTED)
