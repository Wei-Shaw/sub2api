import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MCPView from '../MCPView.vue'
import type { MCPGrant } from '@/api/mcp'

const { getConnectionConfig, listGrants, revokeGrant, copyToClipboard, showSuccess } = vi.hoisted(() => ({
  getConnectionConfig: vi.fn(), listGrants: vi.fn(), revokeGrant: vi.fn(), copyToClipboard: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/mcp', () => ({ mcpAPI: { getConnectionConfig, listGrants, revokeGrant } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))
vi.mock('@/utils/format', () => ({ formatDate: (value: string) => value }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const item: MCPGrant = {
  id: 'grant-1', client_id: 'client-1', client_name: 'Desktop assistant', scope: 'user', read_only: true,
  created_at: '2026-10-28T00:00:00Z', expires_at: '2099-11-27T00:00:00Z', last_used_at: null,
  revoked_at: null, invalidated: false
}
const mounted: ReturnType<typeof mount>[] = []
function render() {
  const wrapper = mount(MCPView, {
    global: {
      stubs: {
        BaseDialog: {
          props: ['show', 'title'], emits: ['close'],
          template: '<section v-if="show" role="dialog" :aria-label="title"><slot /><slot name="footer" /></section>'
        },
        Icon: true
      }
    }
  })
  mounted.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  getConnectionConfig.mockResolvedValue({ resource: 'https://mcp.example.com/api/v1/mcp' })
  listGrants.mockResolvedValue([])
  revokeGrant.mockResolvedValue(undefined)
})
afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('MCP OAuth connections', () => {
  it('shows OAuth instructions and only copies the server URL, without static credential creation', async () => {
    const wrapper = render()
    await flushPromises()
    expect(getConnectionConfig).toHaveBeenCalledTimes(1)
    expect(listGrants).toHaveBeenCalledTimes(1)
    expect((wrapper.get('#mcp-endpoint').element as HTMLInputElement).value).toBe('https://mcp.example.com/api/v1/mcp')
    expect(wrapper.text()).toContain('mcp.connectStep2')
    expect(wrapper.text()).toContain('mcp.empty')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-token"]').exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/Bearer|mcp-new-token|mcp-secret/)
    await wrapper.get('[data-testid="copy-endpoint"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('https://mcp.example.com/api/v1/mcp')
  })

  it('does not guess the resource when server configuration fails and keeps grants usable', async () => {
    getConnectionConfig.mockRejectedValueOnce({ message: 'MCP public URL is not configured correctly' })
    listGrants.mockResolvedValue([item])
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('MCP public URL is not configured correctly')
    expect((wrapper.get('#mcp-endpoint').element as HTMLInputElement).value).toBe('')
    expect((wrapper.get('[data-testid="copy-endpoint"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.find('[data-grant-id="grant-1"] button').exists()).toBe(true)
    await wrapper.get('[data-testid="retry-connection"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect((wrapper.get('#mcp-endpoint').element as HTMLInputElement).value).toBe('https://mcp.example.com/api/v1/mcp')
    expect((wrapper.get('[data-testid="copy-endpoint"]').element as HTMLButtonElement).disabled).toBe(false)
    expect(listGrants).toHaveBeenCalledTimes(1)
  })

  it('disables URL copying while configuration is loading independently from grants', async () => {
    let resolveConfig!: (value: { resource: string }) => void
    getConnectionConfig.mockReturnValueOnce(new Promise<{ resource: string }>(resolve => { resolveConfig = resolve }))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('mcp.empty')
    expect((wrapper.get('[data-testid="copy-endpoint"]').element as HTMLButtonElement).disabled).toBe(true)
    resolveConfig({ resource: 'https://configured.example.com/api/v1/mcp' })
    await flushPromises()
    expect((wrapper.get('#mcp-endpoint').element as HTMLInputElement).value).toBe('https://configured.example.com/api/v1/mcp')
  })

  it('loads authorized applications and shows their permissions', async () => {
    listGrants.mockResolvedValue([item, { ...item, id: 'grant-2', scope: 'admin', read_only: false }])
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-grant-id="grant-1"]').text()).toContain('Desktop assistant')
    expect(wrapper.get('[data-grant-id="grant-1"]').text()).toContain('mcp.readOnly')
    expect(wrapper.get('[data-grant-id="grant-2"]').text()).toContain('mcp.adminScope')
    expect(wrapper.get('[data-grant-id="grant-2"]').text()).toContain('mcp.readWrite')
  })

  it('keeps loading failures visible and can refresh', async () => {
    listGrants.mockRejectedValueOnce({ message: 'Service unavailable' }).mockResolvedValueOnce([item])
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Service unavailable')
    await wrapper.get('[data-testid="refresh-grants"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[data-grant-id="grant-1"]').exists()).toBe(true)
  })

  it('revokes only after confirmation and updates the application state', async () => {
    listGrants.mockResolvedValue([item])
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-grant-id="grant-1"] button').trigger('click')
    expect(revokeGrant).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-revoke"]').trigger('click')
    await flushPromises()
    expect(revokeGrant).toHaveBeenCalledWith(item.id)
    expect(wrapper.get('[data-grant-id="grant-1"]').text()).toContain('mcp.revoked')
    expect(wrapper.find('[data-grant-id="grant-1"] button').exists()).toBe(false)
    expect(showSuccess).toHaveBeenCalledWith('mcp.revokeSuccess')
  })

  it('keeps the revoke dialog and grant active when revocation fails', async () => {
    listGrants.mockResolvedValue([item])
    revokeGrant.mockRejectedValue({ message: 'Try again later' })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-grant-id="grant-1"] button').trigger('click')
    await wrapper.get('[data-testid="confirm-revoke"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Try again later')
    expect(wrapper.get('[data-grant-id="grant-1"]').text()).toContain('mcp.active')
    expect(wrapper.find('[data-testid="confirm-revoke"]').exists()).toBe(true)
  })

  it('shows invalidated and expired grants as inactive while allowing revocation', async () => {
    listGrants.mockResolvedValue([
      { ...item, invalidated: true },
      { ...item, id: 'grant-2', expires_at: '2000-01-01T00:00:00Z' }
    ])
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-grant-id="grant-1"]').text()).toContain('mcp.invalidated')
    expect(wrapper.get('[data-grant-id="grant-2"]').text()).toContain('mcp.expired')
    expect(wrapper.find('[data-grant-id="grant-1"] button').exists()).toBe(true)
  })
})
