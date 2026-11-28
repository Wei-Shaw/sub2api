import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Channel, CreateChannelRequest, UpdateChannelRequest } from '@/api/admin/channels'
import ChannelsView from '@/views/admin/ChannelsView.vue'

const { listChannels, createChannel, updateChannel, getGroups, showSuccess, showError } = vi.hoisted(() => ({
  listChannels: vi.fn(),
  createChannel: vi.fn(),
  updateChannel: vi.fn(),
  getGroups: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channels: { list: listChannels, create: createChannel, update: updateChannel },
    groups: { getAll: getGroups },
    settings: { getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false }) }
  }
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

function mountView() {
  return mount(ChannelsView, {
    global: {
      stubs: {
        AppLayout: defineComponent({ template: '<main><slot /></main>' }),
        TablePageLayout: defineComponent({
          template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
        }),
        BaseDialog: defineComponent({
          props: ['show'],
          template: '<section v-if="show"><slot /><slot name="footer" /></section>'
        }),
        DataTable: defineComponent({
          props: ['data'],
          template: '<div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div>'
        }),
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        PricingEntryCard: true
      }
    }
  })
}

type View = ReturnType<typeof mountView>

async function clickButton(wrapper: View, text: string) {
  const button = wrapper.findAll('button').find(button => button.text() === text)
  expect(button).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}

function platformCheckbox(wrapper: View, platform: string) {
  return wrapper.findAll('label')
    .find(label => label.text() === `admin.groups.platforms.${platform}`)!
    .get('input[type="checkbox"]')
}


function makeChannel(features: Record<string, unknown>): Channel {
  return {
    id: 1, name: 'Bedrock relay', description: '', status: 'active',
    billing_model_source: 'channel_mapped', restrict_models: false,
    features_config: features, group_ids: [11, 22], model_pricing: [], model_mapping: {},
    apply_pricing_to_account_stats: false, account_stats_pricing_rules: [],
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z'
  }
}

function exposeCheckbox(wrapper: View) {
  return wrapper.get('[data-testid="channel-expose-response-model"]')
}

describe('ChannelsView expose response model', () => {
  let stored: Channel[]

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    stored = []
    listChannels.mockImplementation(async () => ({ items: stored, total: stored.length }))
    getGroups.mockResolvedValue([
      { id: 11, name: 'Anthropic group', platform: 'anthropic', rate_multiplier: 1 },
      { id: 22, name: 'OpenAI group', platform: 'openai', rate_multiplier: 1 }
    ])
    createChannel.mockImplementation(async (request: CreateChannelRequest) => {
      const channel = makeChannel(request.features_config || {})
      channel.group_ids = request.group_ids || []
      stored = [channel]
      return channel
    })
    updateChannel.mockImplementation(async (_id: number, request: UpdateChannelRequest) => {
      stored = [{ ...stored[0], ...request } as Channel]
      return stored[0]
    })
  })

  it.each([true, false])('creates and reopens a channel with expose_response_model %s', async enabled => {
    const wrapper = mountView()
    await flushPromises()
    await clickButton(wrapper, 'admin.channels.createChannel')
    expect((exposeCheckbox(wrapper).element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('input[required]').setValue('Relay')
    await platformCheckbox(wrapper, 'anthropic').setValue(true)
    await clickButton(wrapper, 'admin.groups.platforms.anthropic')
    await wrapper.findAll('label').find(label => label.text().includes('Anthropic group'))!
      .get('input[type="checkbox"]').setValue(true)
    if (enabled) await exposeCheckbox(wrapper).setValue(true)

    await wrapper.get('#channel-form').trigger('submit')
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
    expect(createChannel).toHaveBeenCalledOnce()
    expect(createChannel.mock.calls[0][0].features_config.expose_response_model).toBe(enabled)

    await clickButton(wrapper, 'common.edit')
    expect((exposeCheckbox(wrapper).element as HTMLInputElement).checked).toBe(enabled)
    wrapper.unmount()
  })

  it('turns the toggle off on update while preserving other features', async () => {
    stored = [makeChannel({ expose_response_model: true, future_feature: { enabled: true } })]
    const wrapper = mountView()
    await flushPromises()
    await clickButton(wrapper, 'common.edit')
    expect((exposeCheckbox(wrapper).element as HTMLInputElement).checked).toBe(true)
    await exposeCheckbox(wrapper).setValue(false)

    await wrapper.get('#channel-form').trigger('submit')
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
    expect(updateChannel).toHaveBeenCalledOnce()
    const features = updateChannel.mock.calls[0][1].features_config
    expect(features.expose_response_model).toBe(false)
    expect(features.future_feature).toEqual({ enabled: true })
    wrapper.unmount()
  })
})
