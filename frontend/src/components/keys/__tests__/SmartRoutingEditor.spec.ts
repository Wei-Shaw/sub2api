import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import type { Group } from '@/types'
import SmartRoutingEditor from '../SmartRoutingEditor.vue'

const messages: Record<string, string> = {
  'keys.smartRouting.title': 'Smart routing',
  'keys.smartRouting.fixed': 'Fixed group',
  'keys.smartRouting.order': 'Routing order',
  'keys.smartRouting.description': 'Try groups in order.',
  'keys.smartRouting.first': 'Preferred group',
  'keys.smartRouting.empty': 'No routing groups',
  'keys.smartRouting.emptyHint': 'Add a group to begin.',
  'keys.smartRouting.add': 'Add group',
  'keys.smartRouting.noGroups': 'No available groups',
  'keys.smartRouting.limit': 'Maximum reached',
  'keys.smartRouting.billing': 'Billing',
  'keys.smartRouting.scope': 'Usage is billed by the selected group.',
  'keys.smartRouting.unavailable': 'Group unavailable: {id}',
  'keys.smartRouting.moveUp': 'Move {name} up',
  'keys.smartRouting.moveDown': 'Move {name} down',
  'keys.smartRouting.removeGroup': 'Remove {name}',
  'keys.smartRouting.up': 'Move up',
  'keys.smartRouting.down': 'Move down',
  'keys.smartRouting.remove': 'Remove',
  'keys.smartRouting.mode': 'Routing mode',
  'keys.smartRouting.added': 'Group added',
  'keys.smartRouting.moved': 'Moved to position {position}',
  'keys.smartRouting.removed': 'Group removed',
  'keys.searchGroup': 'Search groups',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        let value = messages[key] ?? key
        for (const [name, replacement] of Object.entries(params ?? {})) {
          value = value.replace(`{${name}}`, String(replacement))
        }
        return value
      },
    }),
  }
})

const SelectStub = defineComponent({
  name: 'SmartRoutingSelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean], default: null },
    options: { type: Array, default: () => [] },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  template: `
    <div data-testid="route-picker">
      <button data-testid="route-picker-trigger" type="button" :disabled="disabled">Add</button>
      <button
        v-for="option in options"
        :key="option.value"
        data-testid="route-option"
        type="button"
        @click="$emit('update:modelValue', option.value)"
      >{{ option.label }}</button>
    </div>
  `,
})

const GroupBadgeStub = defineComponent({
  name: 'GroupBadge',
  props: { name: { type: String, required: true } },
  template: '<span data-testid="group-badge">{{ name }}</span>',
})

const makeGroup = (
  id: number,
  name = `Group ${id}`,
  platform: Group['platform'] = 'openai',
  status: Group['status'] = 'active',
): Group => ({
  id,
  name,
  description: null,
  platform,
  status,
  subscription_type: 'free',
  rate_multiplier: 1,
  is_exclusive: false,
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  long_context_pricing_enabled: false,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  search_price_per_1k: null,
  audio_realtime_price_per_min: null,
  audio_tts_price_per_million_chars: null,
  audio_stt_price_per_hour: null,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_live: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
})

const mountEditor = (props: Record<string, unknown> = {}) => mount(SmartRoutingEditor, {
  props: {
    enabled: true,
    modelValue: [],
    groups: [],
    rates: {},
    fixedGroupId: null,
    ...props,
  },
  global: {
    stubs: {
      Select: SelectStub,
      GroupBadge: GroupBadgeStub,
    },
  },
})

describe('SmartRoutingEditor', () => {
  it('renders an empty state and disables the picker when no groups are available', () => {
    const wrapper = mountEditor()

    expect(wrapper.text()).toContain('No routing groups')
    expect(wrapper.get('[data-testid="route-picker-trigger"]').attributes('disabled')).toBeDefined()
  })

  it('seeds the fixed group when smart routing is enabled', async () => {
    const wrapper = mountEditor({
      enabled: false,
      fixedGroupId: 2,
      groups: [makeGroup(2), makeGroup(3)],
    })

    await wrapper.findAll('button[aria-pressed="false"]')[0]!.trigger('click')
    await nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([[[2]]])
    expect(wrapper.emitted('update:enabled')).toEqual([[true]])
    await flushPromises()
  })

  it('adds groups, preserves order while moving, and removes a route', async () => {
    const wrapper = mountEditor({
      modelValue: [1, 2],
      groups: [makeGroup(1), makeGroup(2), makeGroup(3)],
    })

    await wrapper.get('[data-testid="route-option"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[1, 2, 3]])
    await wrapper.setProps({ modelValue: [1, 2, 3] })

    const actions = () => wrapper.findAll('button.route-action')
    await actions()[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[2, 1, 3]])
    await wrapper.setProps({ modelValue: [2, 1, 3] })

    await actions()[5]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[2, 3]])
  })

  it('keeps stale routes visible for repair and caps the editor at ten groups', async () => {
    const wrapper = mountEditor({
      modelValue: [99],
      groups: [makeGroup(1)],
    })

    expect(wrapper.text()).toContain('Group unavailable: 99')
    await wrapper.find('button.route-action[title="Remove"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[]])

    await wrapper.setProps({
      modelValue: Array.from({ length: 10 }, (_, index) => index + 1),
      groups: Array.from({ length: 10 }, (_, index) => makeGroup(index + 1)),
    })
    expect(wrapper.get('[data-testid="route-picker-trigger"]').attributes('disabled')).toBeDefined()
  })

  it('hides route controls after switching back to fixed mode without clearing the model', async () => {
    const wrapper = mountEditor({ modelValue: [1], groups: [makeGroup(1)] })

    await wrapper.find('button[aria-pressed="false"]').trigger('click')
    await wrapper.setProps({ enabled: false })

    expect(wrapper.emitted('update:enabled')).toEqual([[false]])
    expect(wrapper.find('[data-testid="route-picker"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
