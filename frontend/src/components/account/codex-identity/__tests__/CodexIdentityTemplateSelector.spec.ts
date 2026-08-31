import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CodexIdentityTemplateSelector from '../CodexIdentityTemplateSelector.vue'
import type { CodexIdentityAssignment, CodexIdentityTemplate } from '@/types/codexIdentity'

const { listTemplates } = vi.hoisted(() => ({ listTemplates: vi.fn() }))
vi.mock('@/api/admin', () => ({
  adminAPI: { codexIdentityTemplates: { list: listTemplates } },
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params
      ? Object.entries(params).reduce(
          (value, [name, replacement]) => value.replace(`{${name}}`, String(replacement)),
          key,
        )
      : key,
  }),
}))

const template: CodexIdentityTemplate = {
  id: 7,
  name: 'Primary egress',
  description: 'Windows Desktop and CLI',
  revision: 4,
  session_policy: { mode: 'conversation_isolated' },
  affinity_ttl_seconds: 3600,
  unsupported_policy: 'reject',
  profiles: [{
    os_class: 'windows',
    canonical_surface: 'desktop',
    architecture: 'x86_64',
    slot_count: 2,
    proxy_mode: 'inherit',
    catalog_version: 1,
  }],
  assigned_account_count: 1,
  created_at: '2026-08-31T00:00:00Z',
  updated_at: '2026-08-31T00:00:00Z',
}

const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'id', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <select :id="id" :value="modelValue ?? ''" :disabled="disabled" @change="$emit('update:modelValue', Number($event.target.value))">
      <option value="">Select</option>
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `,
})

const mountSelector = (initial: CodexIdentityAssignment = { enabled: false }) => {
  const Harness = defineComponent({
    components: { CodexIdentityTemplateSelector },
    setup() {
      const assignment = ref(initial)
      return { assignment }
    },
    template: '<CodexIdentityTemplateSelector v-model="assignment" id-prefix="test-assignment" />',
  })
  return mount(Harness, { global: { stubs: { Select: SelectStub, Icon: true } } })
}

describe('CodexIdentityTemplateSelector', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue([template])
  })

  it('loads templates through the admin API and shows the selected summary', async () => {
    const wrapper = mountSelector({ enabled: true, template_id: 7 })
    await flushPromises()

    expect(listTemplates).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Primary egress')
    expect(wrapper.text()).toContain('Windows Desktop and CLI')
    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.windows')
    expect(wrapper.get('a[href="/admin/settings?tab=codexProfiles"]').exists()).toBe(true)
  })

  it('requires a template after enabling and emits a selected assignment', async () => {
    const wrapper = mountSelector()
    await flushPromises()
    await wrapper.get('[data-testid="codex-template-assignment-toggle"]').trigger('click')

    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.templateRequired')
    await wrapper.get('#test-assignment-template').setValue('7')

    const harness = wrapper.vm as unknown as { assignment: CodexIdentityAssignment }
    expect(harness.assignment).toEqual({ enabled: true, template_id: 7, expected_revision: 4 })
  })

  it('emits a clean disabled assignment and remembers the selection when re-enabled', async () => {
    const wrapper = mountSelector({ enabled: true, template_id: 7, expected_revision: 4 })
    await flushPromises()
    const toggle = wrapper.get('[data-testid="codex-template-assignment-toggle"]')
    await toggle.trigger('click')

    let harness = wrapper.vm as unknown as { assignment: CodexIdentityAssignment }
    expect(harness.assignment).toEqual({ enabled: false })
    await toggle.trigger('click')
    harness = wrapper.vm as unknown as { assignment: CodexIdentityAssignment }
    expect(harness.assignment).toEqual({ enabled: true, template_id: 7, expected_revision: 4 })
  })
})
