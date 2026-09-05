import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import type { RelayApi } from './api'

function fakeApi(overrides: Partial<RelayApi> = {}): RelayApi {
  return {
    getRuntime: vi.fn().mockResolvedValue({ base_url: 'http://127.0.0.1:8091' }),
    listPlatforms: vi.fn().mockResolvedValue([
      { key: 'agnes', display_name: 'Agnes', default_base_url: 'https://apihub.agnes-ai.com/v1' },
      { key: 'openai', display_name: 'OpenAI', default_base_url: 'https://api.openai.com/v1' },
      { key: 'opencode', display_name: 'OpenCode', default_base_url: 'https://opencode.ai/zen' },
    ]),
    listRoutes: vi.fn().mockResolvedValue({ items: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 1 } }),
    createRoute: vi.fn().mockResolvedValue({}),
    updateRoute: vi.fn().mockResolvedValue({}),
    deleteRoutes: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

describe('AI Relay plugin application', () => {
  it('renders the OpenAI platform returned by the plugin service', async () => {
    const wrapper = mount(App, { props: { api: fakeApi() } })
    await flushPromises()
    await wrapper.get('[data-testid="route-add"]').trigger('click')

    expect(wrapper.find('select option[value="openai"]').text()).toBe('OpenAI')
  })

  it('uses the OpenCode default Base URL for new routes', async () => {
    const wrapper = mount(App, { props: { api: fakeApi() } })
    await flushPromises()
    await wrapper.get('[data-testid="route-add"]').trigger('click')

    const platformSelect = wrapper.get<HTMLSelectElement>('form select')
    const baseURLInput = wrapper.findAll<HTMLInputElement>('form input[required]')[2]
    await platformSelect.setValue('opencode')

    expect(wrapper.find('form option[value="opencode"]').text()).toBe('OpenCode')
    expect(baseURLInput.element.value).toBe('https://opencode.ai/zen')
  })

  it('renames a route by deleting the old slug before creating the new slug', async () => {
    const route = {
      platform: 'openai',
      slug: 'old-slug',
      name: 'OpenAI upstream',
      base_url: 'https://api.example.com/v1',
      path_mappings: { 'v1/responses': 'v4/responses' },
    }
    const deleteRoutes = vi.fn().mockResolvedValue(undefined)
    const createRoute = vi.fn().mockResolvedValue({})
    const updateRoute = vi.fn().mockResolvedValue({})
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          listRoutes: vi.fn().mockResolvedValue({
            items: [route],
            pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
          }),
          deleteRoutes,
          createRoute,
          updateRoute,
        }),
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="route-edit"]').trigger('click')

    const slugInput = wrapper.findAll<HTMLInputElement>('input[required]')[1]
    const baseURLInput = wrapper.findAll<HTMLInputElement>('input[required]')[2]
    expect(slugInput.attributes('disabled')).toBeUndefined()
    expect(baseURLInput.element.value).toBe('https://api.example.com/v1')
    await slugInput.setValue('new-slug')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(deleteRoutes).toHaveBeenCalledWith([{ platform: 'openai', slug: 'old-slug' }])
    expect(createRoute).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'openai',
      slug: 'new-slug',
      path_mappings: { 'v1/responses': 'v4/responses' },
    }))
    expect(updateRoute).not.toHaveBeenCalled()
    expect(deleteRoutes.mock.invocationCallOrder[0]).toBeLessThan(createRoute.mock.invocationCallOrder[0])
  })

  it('updates an edited route directly when the slug is unchanged', async () => {
    const route = {
      platform: 'openai',
      slug: 'same-slug',
      name: 'OpenAI upstream',
      base_url: 'https://api.example.com/v1',
      path_mappings: {},
    }
    const deleteRoutes = vi.fn().mockResolvedValue(undefined)
    const createRoute = vi.fn().mockResolvedValue({})
    const updateRoute = vi.fn().mockResolvedValue({})
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          listRoutes: vi.fn().mockResolvedValue({
            items: [route],
            pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
          }),
          deleteRoutes,
          createRoute,
          updateRoute,
        }),
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="route-edit"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateRoute).toHaveBeenCalledWith('openai', 'same-slug', expect.objectContaining({ slug: 'same-slug' }))
    expect(deleteRoutes).not.toHaveBeenCalled()
    expect(createRoute).not.toHaveBeenCalled()
  })

  it('adds and removes path mapping rows in the create dialog', async () => {
    const wrapper = mount(App, { props: { api: fakeApi() } })
    await flushPromises()
    await wrapper.get('[data-testid="route-add"]').trigger('click')
    await wrapper.get('[data-testid="path-mapping-add"]').trigger('click')
    expect(wrapper.findAll('[data-testid="path-mapping-source"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="path-mapping-remove"]').attributes('aria-label')).toBe('Remove path mapping')
    await wrapper.get('[data-testid="path-mapping-remove"]').trigger('click')
    expect(wrapper.findAll('[data-testid="path-mapping-source"]')).toHaveLength(0)
  })

  it('offers OpenAI source paths while allowing custom input', async () => {
    const wrapper = mount(App, { props: { api: fakeApi() } })
    await flushPromises()
    await wrapper.get('[data-testid="route-add"]').trigger('click')
    await wrapper.get('[data-testid="path-mapping-add"]').trigger('click')

    const source = wrapper.get('[data-testid="path-mapping-source"]')
    const target = wrapper.get('[data-testid="path-mapping-target"]')
    expect(source.attributes('list')).toBe('openai-source-paths')
    expect(source.attributes('placeholder')).toBe('v1/chat/completions')
    expect(target.attributes('placeholder')).toBe('chat/completions')
    expect(wrapper.findAll('#openai-source-paths option').map(option => option.attributes('value'))).toEqual([
      'v1/models', 'v1/chat/completions', 'v1/responses', 'v1/responses/compact',
      'v1/embeddings', 'v1/images/generations', 'v1/images/edits',
    ])
    await source.setValue('custom/endpoint')
    expect((source.element as HTMLInputElement).value).toBe('custom/endpoint')
  })

  it('submits normalized mappings and omits blank rows', async () => {
    const createRoute = vi.fn().mockResolvedValue({})
    const wrapper = mount(App, { props: { api: fakeApi({ createRoute }) } })
    await flushPromises()
    await wrapper.get('[data-testid="route-add"]').trigger('click')
    await wrapper.get('input[required]').setValue('Zhipu')
    await wrapper.findAll<HTMLInputElement>('input[required]')[1].setValue('zhipu')
    await wrapper.findAll<HTMLInputElement>('input[required]')[2].setValue('https://open.bigmodel.cn/v1')
    await wrapper.get('[data-testid="path-mapping-add"]').trigger('click')
    await wrapper.get('[data-testid="path-mapping-add"]').trigger('click')
    await wrapper.findAll<HTMLInputElement>('[data-testid="path-mapping-source"]')[0].setValue('/v1/responses/compact/')
    await wrapper.findAll<HTMLInputElement>('[data-testid="path-mapping-target"]')[0].setValue('/chat/completions/')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createRoute).toHaveBeenCalledWith(expect.objectContaining({
      path_mappings: { 'v1/responses/compact': 'chat/completions' },
    }))
  })

  it('shows an API error without hiding the route management surface', async () => {
    const wrapper = mount(App, { props: { api: fakeApi({ listRoutes: vi.fn().mockRejectedValue(new Error('offline')) }) } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('offline')
    expect(wrapper.find('[data-testid="route-add"]').exists()).toBe(true)
  })

  it('shows the complete runtime plugin URL without a secondary slug', async () => {
    const route = {
      platform: 'agnes',
      slug: 'zhipu',
      name: 'Zhipu',
      base_url: 'https://open.bigmodel.cn/v4',
      path_mappings: {},
    }
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          getRuntime: vi.fn().mockResolvedValue({ base_url: 'http://plugin-server:8091' }),
          listRoutes: vi.fn().mockResolvedValue({
            items: [route],
            pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
          }),
        }),
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="route-name"]').text()).toBe('Zhipu')
    expect(wrapper.find('[data-testid="route-name"] small').exists()).toBe(false)
    expect(wrapper.get('[data-testid="route-plugin-url"]').text()).toBe(
      'http://plugin-server:8091/plugins/ai-relay/agnes/zhipu',
    )

    await wrapper.get('[aria-label="Copy route URL"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('http://plugin-server:8091/plugins/ai-relay/agnes/zhipu')
    expect(wrapper.get('[role="status"]').text()).toContain('Plugin URL copied')
  })

  it('shows an error toast when copying the route URL fails', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('clipboard denied'))
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          listRoutes: vi.fn().mockResolvedValue({
            items: [{ platform: 'agnes', slug: 'zhipu', name: 'Zhipu', base_url: 'https://open.bigmodel.cn/v4', path_mappings: {} }],
            pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
          }),
        }),
      },
    })
    await flushPromises()

    await wrapper.get('[aria-label="Copy route URL"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role="status"]').text()).toContain('Failed to copy Plugin URL')
  })

  it('copies the route URL with the legacy API when Clipboard API is unavailable', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    const execCommand = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          listRoutes: vi.fn().mockResolvedValue({
            items: [{ platform: 'agnes', slug: 'zhipu', name: 'Zhipu', base_url: 'https://open.bigmodel.cn/v4', path_mappings: {} }],
            pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
          }),
        }),
      },
    })
    await flushPromises()

    await wrapper.get('[aria-label="Copy route URL"]').trigger('click')
    await flushPromises()

    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(wrapper.get('[role="status"]').text()).toContain('Plugin URL copied')
  })

  it('deletes one route from its action button after confirmation', async () => {
    const route = {
      platform: 'agnes',
      slug: 'zhipu',
      name: 'Zhipu',
      base_url: 'https://open.bigmodel.cn/v4',
      path_mappings: {},
    }
    const deleteRoutes = vi.fn().mockResolvedValue(undefined)
    const listRoutes = vi.fn().mockResolvedValue({
      items: [route],
      pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
    })
    const wrapper = mount(App, { props: { api: fakeApi({ listRoutes, deleteRoutes }) } })
    await flushPromises()

    await wrapper.get('[data-testid="route-delete"]').trigger('click')
    expect(wrapper.get('[role="alertdialog"]').text()).toContain('Delete selected routes?')
    await wrapper.get('[data-testid="route-delete-confirm"]').trigger('click')
    await flushPromises()

    expect(deleteRoutes).toHaveBeenCalledWith([{ platform: 'agnes', slug: 'zhipu' }])
  })

  it('deletes all checked routes from the bulk action after confirmation', async () => {
    const routes = [
      { platform: 'agnes', slug: 'one', name: 'One', base_url: 'https://example.test/v1', path_mappings: {} },
      { platform: 'openai', slug: 'two', name: 'Two', base_url: 'https://example.test/v1', path_mappings: {} },
    ]
    const deleteRoutes = vi.fn().mockResolvedValue(undefined)
    const wrapper = mount(App, {
      props: {
        api: fakeApi({
          listRoutes: vi.fn().mockResolvedValue({
            items: routes,
            pagination: { page: 1, page_size: 20, total: 2, total_pages: 1 },
          }),
          deleteRoutes,
        }),
      },
    })
    await flushPromises()

    for (const checkbox of wrapper.findAll('tbody input[type="checkbox"]')) await checkbox.setValue(true)
    expect(wrapper.get('[data-testid="route-delete-selected"]').text()).toBe('Delete selected')
    await wrapper.get('[data-testid="route-delete-selected"]').trigger('click')
    await wrapper.get('[data-testid="route-delete-confirm"]').trigger('click')
    await flushPromises()

    expect(deleteRoutes).toHaveBeenCalledWith([
      { platform: 'agnes', slug: 'one' },
      { platform: 'openai', slug: 'two' },
    ])
  })
})
