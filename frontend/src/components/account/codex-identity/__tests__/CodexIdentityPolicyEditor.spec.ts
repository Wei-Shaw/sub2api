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

    const linuxDesktop = wrapper.get('[data-testid="codex-profile-linux-desktop"]')
    await linuxDesktop.get('button[role="switch"]').trigger('click')
    await linuxDesktop.get('button[aria-label="Increase device slots - Desktop"]').trigger('click')

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

  it('enables Desktop and CLI independently for the same OS', async () => {
    const wrapper = mountEditor()
    await wrapper.get('[data-testid="codex-identity-policy-toggle"]').trigger('click')
    const desktop = wrapper.get('[data-testid="codex-profile-windows-desktop"]')
    const cli = wrapper.get('[data-testid="codex-profile-windows-cli"]')

    await desktop.get('button[role="switch"]').trigger('click')
    await cli.get('button[role="switch"]').trigger('click')
    await desktop.get('button[aria-label="Increase device slots - Desktop"]').trigger('click')
    await cli.get('select[aria-label="Architecture"]').setValue('arm64')
    await cli.get('#test-policy-windows-cli-proxy-profile-proxy').setValue('direct')

    const harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles).toEqual(expect.arrayContaining([
      expect.objectContaining({
        os_class: 'windows',
        canonical_surface: 'desktop',
        architecture: 'x86_64',
        slot_count: 2,
      }),
      expect.objectContaining({
        os_class: 'windows',
        canonical_surface: 'cli',
        architecture: 'arm64',
        slot_count: 1,
        proxy_mode: 'direct',
      }),
    ]))
  })

  it('offers Generic SDK and third-party as independent profiles without architecture fields', async () => {
    const wrapper = mountEditor()
    await wrapper.get('[data-testid="codex-identity-policy-toggle"]').trigger('click')
    const sdk = wrapper.get('[data-testid="codex-profile-generic-sdk"]')
    const thirdParty = wrapper.get('[data-testid="codex-profile-generic-third_party"]')
    await sdk.get('button[role="switch"]').trigger('click')
    await thirdParty.get('button[role="switch"]').trigger('click')

    const harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles).toEqual(expect.arrayContaining([
      expect.objectContaining({ os_class: 'generic', canonical_surface: 'sdk', architecture: '' }),
      expect.objectContaining({ os_class: 'generic', canonical_surface: 'third_party', architecture: '' }),
    ]))
    expect(sdk.text()).toContain('Not applicable')
    expect(thirdParty.text()).toContain('Not applicable')
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

    expect(wrapper.get('[data-testid="codex-profile-windows-desktop"] .grid').classes()).toContain('grid-cols-1')
    expect(wrapper.html()).toContain('sm:grid-cols-2')
    expect(wrapper.html()).toContain('lg:grid-cols-2')
    expect(wrapper.findAll('fieldset').length).toBeGreaterThan(1)
    expect(wrapper.findAll('[aria-describedby]').length).toBeGreaterThan(0)
  })

  it('preserves and edits explicit direct, inherited, and proxy routes', async () => {
    const wrapper = mountEditor({
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [{
        os_class: 'windows',
        canonical_surface: 'desktop',
        architecture: 'x86_64',
        slot_count: 2,
        proxy_mode: 'direct',
        slots: [{ index: 0, proxy_mode: 'direct', client_version_mode: 'inherit' }],
      }],
    })

    const profileProxy = wrapper.get('#test-policy-windows-desktop-proxy-profile-proxy')
    const slotProxy = wrapper.get('#test-policy-windows-desktop-proxy-slot-0-proxy')
    expect((profileProxy.element as HTMLSelectElement).value).toBe('direct')
    expect((slotProxy.element as HTMLSelectElement).value).toBe('direct')

    await profileProxy.setValue('7')
    let harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles?.[0]).toMatchObject({ proxy_mode: 'proxy', proxy_id: 7 })

    await profileProxy.setValue('inherit')
    harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles?.[0]?.proxy_mode).toBe('inherit')
    expect(harness.policy.profiles?.[0]).not.toHaveProperty('proxy_id')

    await slotProxy.setValue('7')
    harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles?.[0]?.slots?.[0]).toEqual({
      index: 0,
      proxy_mode: 'proxy',
      proxy_id: 7,
      client_version_mode: 'inherit',
    })

    await slotProxy.setValue('direct')
    harness = wrapper.vm as unknown as { policy: CodexIdentityPolicy }
    expect(harness.policy.profiles?.[0]?.slots?.[0]).toEqual({
      index: 0,
      proxy_mode: 'direct',
      client_version_mode: 'inherit',
    })
  })
})
