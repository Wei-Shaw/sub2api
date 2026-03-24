import { describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

describe('AccountTableFilters', () => {
  it('renders privacy mode options and emits privacy_mode updates', async () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          status: '',
          group: '',
          privacy_mode: ''
        REDACTED,
        groups: []
      REDACTED,
      global: {
        stubs: {
          SearchInput: {
            template: '<div />'
          REDACTED,
          Select: {
            props: ['modelValue', 'options'],
            emits: ['update:modelValue', 'change'],
            template: '<div class="select-stub" :data-options="JSON.stringify(options)" />'
          REDACTED
        REDACTED
      REDACTED
    REDACTED)

    const selects = wrapper.findAll('.select-stub')
    expect(selects).toHaveLength(5)

    const privacyOptions = JSON.parse(selects[3].attributes('data-options'))
    expect(privacyOptions).toEqual([
      { value: '', label: 'admin.accounts.allPrivacyModes' REDACTED,
      { value: '__unset__', label: 'admin.accounts.privacyUnset' REDACTED,
      { value: 'training_off', label: 'Privacy' REDACTED,
      { value: 'training_set_cf_blocked', label: 'CF' REDACTED,
      { value: 'training_set_failed', label: 'Fail' REDACTED
    ])
  REDACTED)
REDACTED)
