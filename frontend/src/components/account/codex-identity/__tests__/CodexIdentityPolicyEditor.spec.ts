import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import CodexIdentityPolicyEditor from '../CodexIdentityPolicyEditor.vue'
import type { CodexIdentityPolicy } from '@/types/codexIdentity'
import { createDefaultCodexIdentityPolicy } from '@/utils/codexIdentityValidation'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    te: () => false,
  }),
}))

const SelectStub = defineComponent({
  name: 'IdentitySelectStub',
  props: ['modelValue', 'options', 'id', 'disabled', 'ariaLabel'],
  emits: ['update:modelValue'],
  template: `
    <select
      :id="id"
      :value="modelValue ?? ''"
      :disabled="disabled"
      :aria-label="ariaLabel"
      @change="$emit('update:modelValue', $event.target.value === '' ? null : ($event.target.value.match(/^\\d+$/) ? Number($event.target.value) : $event.target.value))"
    >
      <option v-for="option in options" :key="String(option.value)" :value="option.value ?? ''" :disabled="option.disabled">
        {{ option.label }}
      </option>
    </select>
  `,
})

const mountEditor = (initial: CodexIdentityPolicy = createDefaultCodexIdentityPolicy()) => {
  const Harness = defineComponent({
    components: { CodexIdentityPolicyEditor },
    setup() {
      const policy = ref(initial)
      return { policy }
    },
    template: `
      <CodexIdentityPolicyEditor
        v-model="policy"
        :proxies="[{ id: 7, name: 'Tokyo', status: 'active' }]"
        :account-proxy-id="7"
        id-prefix="test-policy"
      />
    `,
  })
  return mount(Harness, {
    global: {
      stubs: {
        Select: SelectStub,
        Icon: { template: '<span aria-hidden="true" />' },
      },
    },
  })
}

describe('CodexIdentityPolicyEditor', () => {
  it('starts disabled and exposes an accessible switch', () => {
    const wrapper = mountEditor()
    const toggle = wrapper.get('[data-testid="codex-identity-policy-toggle"]')

    expect(toggle.attributes('role')).toBe('switch')
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(toggle.attributes('aria-labelledby')).toBe('test-policy-title')
    expect(wrapper.get('[data-testid="codex-identity-policy-off"]').text()).toContain('Disabled')
    expect(wrapper.find('[data-testid="codex-profile-list"]').exists()).toBe(false)
  })

  it('requires a profile, then accepts Linux Desktop with stable slots', async () => {
    const wrapper = mountEditor()
    await wrapper.get('[data-testid="codex-identity-policy-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="codex-identity-errors"]').text()).toContain('Enable at least one')

    const linux = wrapper.get('[data-testid="codex-profile-linux"]')
    await linux.get('button[role="switch"]').trigger('click')
    await linux.get('input[type="radio"][value="desktop"]').setValue(true)
    await linux.get('button[aria-label="Increase device slots"]').trigger('click')

    const harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles).toEqual([
      expect.objectContaining({
        os_class: 'linux',
        canonical_surface: 'desktop',
        architecture: 'x86_64',
        slot_count: 2,
      }),
    ])
    expect(wrapper.find('[data-testid="codex-identity-errors"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="codex-identity-valid"]').attributes('role')).toBe('status')
  })

  it('offers Generic SDK and third-party without an architecture field', async () => {
    const wrapper = mountEditor()
    await wrapper.get('[data-testid="codex-identity-policy-toggle"]').trigger('click')
    const generic = wrapper.get('[data-testid="codex-profile-generic"]')
    await generic.get('button[role="switch"]').trigger('click')
    await generic.get('input[type="radio"][value="third_party"]').setValue(true)

    const harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles?.[0]).toMatchObject({
      os_class: 'generic',
      canonical_surface: 'third_party',
      architecture: '',
    })
    expect(generic.text()).toContain('Not applicable')
  })

  it('configures a bounded session pool and emits a high-risk status for device sharing', async () => {
    const wrapper = mountEditor({
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [{
        os_class: 'windows',
        canonical_surface: 'desktop',
        architecture: 'x86_64',
        slot_count: 1,
      }],
    })

    await wrapper.get('input[type="radio"][value="session_pool"]').setValue(true)
    await wrapper.get('button[aria-label="Increase session slots"]').trigger('click')
    let harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.session_policy).toEqual({ mode: 'session_pool', sessions_per_device: 3 })
    expect(wrapper.get('[role="status"]').text()).toContain('Different API keys')

    await wrapper.get('input[type="radio"][value="device_shared"]').setValue(true)
    harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.session_policy).toEqual({
      mode: 'device_shared',
      max_active_conversations_per_slot: 1,
      disable_cross_key_continuation: true,
    })
    expect(wrapper.get('[role="status"]').text()).toContain('High risk')
    expect(wrapper.get('[data-testid="device-shared-restrictions"]').text()).toContain('One active')
  })

  it('uses mobile-first layout classes and labelled controls', async () => {
    const wrapper = mountEditor({
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [{
        os_class: 'windows',
        canonical_surface: 'desktop',
        architecture: 'x86_64',
        slot_count: 1,
      }],
    })

    expect(wrapper.get('[data-testid="codex-profile-windows"] .grid').classes()).toContain('grid-cols-1')
    expect(wrapper.html()).toContain('md:grid-cols-3')
    expect(wrapper.html()).toContain('lg:grid-cols-2')
    expect(wrapper.findAll('fieldset').length).toBeGreaterThan(1)
    expect(wrapper.findAll('[aria-describedby]').length).toBeGreaterThan(0)
  })
})
