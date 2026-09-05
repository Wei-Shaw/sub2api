import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import ProfileProxyOverrides from '../ProfileProxyOverrides.vue'
import type { CodexOSProfilePolicy } from '@/types/codexIdentity'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    te: () => true,
    t: (key: string) => key,
  }),
}))

const profile: CodexOSProfilePolicy = {
  os_class: 'windows',
  canonical_surface: 'desktop',
  architecture: 'x86_64',
  slot_count: 1,
  proxy_mode: 'inherit',
}

const SelectStub = defineComponent({
  inheritAttrs: false,
  props: ['modelValue', 'options', 'id', 'disabled', 'ariaLabel', 'ariaDescribedby'],
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :id="id"
      :value="modelValue ?? ''"
      :disabled="disabled"
      :aria-label="ariaLabel"
      :aria-describedby="ariaDescribedby"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option>
    </select>
  `,
})

describe('ProfileProxyOverrides', () => {
  it('describes account inheritance generically while editing a reusable template', () => {
    const wrapper = mount(ProfileProxyOverrides, {
      props: {
        modelValue: profile,
        templateContext: true,
        idPrefix: 'template-proxy',
      },
      global: {
        stubs: { Select: true, Icon: true },
      },
    })

    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.templateProxyInherit')
    expect(wrapper.text()).not.toContain('admin.accounts.codexIdentity.accountProxyDirect')
  })

  it('allows each slot to pin a Codex client version without exposing UA fields', async () => {
    const wrapper = mount(ProfileProxyOverrides, {
      props: {
        modelValue: { ...profile, slot_count: 1 },
        templateContext: true,
        idPrefix: 'template-proxy',
      },
      global: {
        stubs: { Select: SelectStub, Icon: true },
      },
    })

    await wrapper.get('[data-testid="slot-proxy-overrides"] summary').trigger('click')
    await wrapper.get('[data-testid="slot-0-client-version-mode"]').setValue('pinned')
    const modeUpdate = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as CodexOSProfilePolicy
    await wrapper.setProps({ modelValue: modeUpdate })
    await wrapper.get('[data-testid="slot-0-client-version"]').setValue('0.146.0')

    const emitted = wrapper.emitted('update:modelValue') ?? []
    const latest = emitted[emitted.length - 1]?.[0] as CodexOSProfilePolicy
    expect(latest.slots).toEqual([{
      index: 0,
      proxy_mode: 'inherit',
      client_version_mode: 'pinned',
      client_version: '0.146.0',
    }])
    expect(wrapper.text()).not.toContain('User-Agent')
    expect(wrapper.text()).not.toContain('App build')
  })
})
