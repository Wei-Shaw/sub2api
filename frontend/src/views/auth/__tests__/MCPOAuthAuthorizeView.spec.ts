import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MCPOAuthAuthorizeView from '../MCPOAuthAuthorizeView.vue'
import type { MCPAuthorization, MCPConsent } from '@/api/mcp'

const { auth, previewAuthorization, authorize, routeState, assign } = vi.hoisted(() => ({
  auth: { isAdmin: false, user: { email: 'user@example.com' } },
  previewAuthorization: vi.fn(), authorize: vi.fn(), assign: vi.fn(),
  routeState: { current: {} as { fullPath: string; query: Record<string, unknown> } }
}))
vi.mock('vue-router', () => ({ useRoute: () => routeState.current }))
vi.mock('@/api/mcp', () => ({ mcpAPI: { previewAuthorization, authorize } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/auth/TotpStepUpDialog.vue', () => ({
  default: {
    props: ['controller'],
    template: '<div v-if="controller.visible.value" data-testid="step-up"><button data-testid="step-up-verify" @click="controller.onVerified()">Verify</button><button data-testid="step-up-cancel" @click="controller.onCancel()">Cancel</button></div>'
  }
}))
const request: MCPAuthorization = {
  response_type: 'code', client_id: 'client-1', redirect_uri: 'https://client.example.com/callback?workspace=team',
  scope: 'mcp', state: 'opaque+value&with=reserved?characters', code_challenge: 'a'.repeat(43),
  code_challenge_method: 'S256', resource: 'https://api.example.com/api/v1/mcp'
}
const consent: MCPConsent = {
  client_id: request.client_id, client_name: 'Desktop assistant', redirect_uri: request.redirect_uri,
  resource: request.resource, scope: 'mcp'
}
const redirectURL = `${request.redirect_uri}&code=opaque-code&state=encoded-state`
const originalLocation = window.location
const mounted: ReturnType<typeof mount>[] = []
function render() {
  const wrapper = mount(MCPOAuthAuthorizeView, { global: { stubs: { Icon: true, RouterLink: true } } })
  mounted.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  auth.isAdmin = false
  routeState.current = reactive({ fullPath: '/oauth/mcp/authorize?request=1', query: { ...request } })
  previewAuthorization.mockResolvedValue({ ...consent })
  authorize.mockResolvedValue({ redirect_url: redirectURL })
  Object.defineProperty(window, 'location', { configurable: true, value: { ...originalLocation, assign } })
})
afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount())
  Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
})

describe('MCP OAuth consent', () => {
  it('previews the exact query, displays the client and full return address, and waits for explicit consent', async () => {
    const wrapper = render()
    await flushPromises()
    expect(previewAuthorization).toHaveBeenCalledWith(request)
    expect(wrapper.get('[data-testid="client-name"]').text()).toBe(consent.client_name)
    expect(wrapper.get('[data-testid="redirect-origin"]').text()).toBe('https://client.example.com')
    expect(wrapper.get('[data-testid="redirect-uri"]').text()).toBe(request.redirect_uri)
    expect(wrapper.text()).toContain('mcp.untrustedApplicationHint')
    expect(authorize).not.toHaveBeenCalled()
    expect(assign).not.toHaveBeenCalled()
    expect(wrapper.find('#mcp-consent-scope option[value="admin"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(authorize).toHaveBeenCalledWith({ authorization: request, approved: true, scope: 'user', read_only: true })
    expect(assign).toHaveBeenCalledWith(redirectURL)
  })

  it('allows an omitted OAuth scope without adding or changing request parameters', async () => {
    delete routeState.current.query.scope
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const { scope: _scope, ...expected } = request
    expect(previewAuthorization).toHaveBeenCalledWith(expected)
    expect(authorize.mock.calls[0][0].authorization).toEqual(expected)
  })

  it('escapes untrusted application names', async () => {
    previewAuthorization.mockResolvedValue({ ...consent, client_name: '<img src=x onerror=alert(1)>' })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-testid="client-name"]').text()).toBe('<img src=x onerror=alert(1)>')
    expect(wrapper.find('[data-testid="client-name"] img').exists()).toBe(false)
  })

  it.each([
    { client_id: ['client-1', 'attacker'] },
    { state: null },
    { resource: undefined },
    { response_type: 'token' },
    { code_challenge_method: 'plain' },
    { code_challenge: 'short' },
    { scope: 'mcp admin' },
    { extra: 'unsupported' }
  ])('blocks malformed requests before preview: %j', async (override) => {
    Object.assign(routeState.current.query, override)
    const wrapper = render()
    await flushPromises()
    expect(previewAuthorization).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toBe('mcp.invalidRequest')
    expect(wrapper.find('[data-testid="approve"]').exists()).toBe(false)
    expect(authorize).not.toHaveBeenCalled()
    expect(assign).not.toHaveBeenCalled()
  })

  it('does not offer approval or follow client redirect when preview is rejected', async () => {
    previewAuthorization.mockRejectedValue({ message: 'Unregistered redirect URI' })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Unregistered redirect URI')
    expect(wrapper.find('[data-testid="approve"]').exists()).toBe(false)
    expect(authorize).not.toHaveBeenCalled()
    expect(assign).not.toHaveBeenCalled()
  })

  it('rejects a preview whose client or redirect differs from the request', async () => {
    previewAuthorization.mockResolvedValue({ ...consent, redirect_uri: 'https://unexpected.example/callback' })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('mcp.invalidRequest')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('denies with the original authorization and follows only the server result', async () => {
    const deniedURL = `${request.redirect_uri}&error=access_denied&state=encoded-state`
    authorize.mockResolvedValue({ redirect_url: deniedURL })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="deny"]').trigger('click')
    await flushPromises()
    expect(authorize).toHaveBeenCalledWith({ authorization: request, approved: false, scope: 'user', read_only: true })
    expect(assign).toHaveBeenCalledWith(deniedURL)
  })

  it('requires admin step-up before retrying approval and keeps selected permissions', async () => {
    auth.isAdmin = true
    authorize.mockRejectedValueOnce({ reason: 'STEP_UP_REQUIRED' }).mockResolvedValueOnce({ redirect_url: redirectURL })
    const wrapper = render()
    await flushPromises()
    expect((wrapper.get('#mcp-consent-scope').element as HTMLSelectElement).value).toBe('user')
    await wrapper.get('#mcp-consent-scope').setValue('admin')
    await wrapper.get('#mcp-consent-permissions').setValue('false')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(authorize).toHaveBeenCalledTimes(1)
    expect(assign).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="step-up-verify"]').trigger('click')
    await flushPromises()
    expect(authorize).toHaveBeenCalledTimes(2)
    expect(authorize.mock.calls[1][0]).toEqual({ authorization: request, approved: true, scope: 'admin', read_only: false })
    expect(assign).toHaveBeenCalledWith(redirectURL)
  })

  it('explains that administrator write access always needs two-factor verification', async () => {
    auth.isAdmin = true
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="admin-write-hint"]').exists()).toBe(false)
    await wrapper.get('#mcp-consent-permissions').setValue('false')
    expect(wrapper.find('[data-testid="admin-write-hint"]').exists()).toBe(false)
    await wrapper.get('#mcp-consent-scope').setValue('admin')
    expect(wrapper.get('[data-testid="admin-write-hint"]').text()).toBe('mcp.adminWriteHint')
    await wrapper.get('#mcp-consent-permissions').setValue('true')
    expect(wrapper.find('[data-testid="admin-write-hint"]').exists()).toBe(false)
  })

  it('cancels admin step-up without granting access', async () => {
    auth.isAdmin = true
    authorize.mockRejectedValue({ reason: 'STEP_UP_REQUIRED' })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#mcp-consent-scope').setValue('admin')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="step-up-cancel"]').trigger('click')
    await flushPromises()
    expect(authorize).toHaveBeenCalledTimes(1)
    expect(assign).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="approve"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('reports submission failures without navigating', async () => {
    authorize.mockRejectedValue({ message: 'Permission denied' })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Permission denied')
    expect(assign).not.toHaveBeenCalled()
  })

  it.each(['https://attacker.example/callback', 'javascript:alert(1)', 'https://client.example.com/other'])('rejects unexpected server redirects: %s', async (destination) => {
    authorize.mockResolvedValue({ redirect_url: destination })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('mcp.invalidRedirect')
    expect(assign).not.toHaveBeenCalled()
  })

  it('submits the query captured at preview even if its source object changes', async () => {
    const wrapper = render()
    await flushPromises()
    routeState.current.query.state = 'changed-after-preview'
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(authorize.mock.calls[0][0].authorization).toEqual(request)
  })

  it('clears old consent on route changes and ignores a late preview response', async () => {
    let resolveFirst!: (value: MCPConsent) => void
    previewAuthorization.mockReturnValueOnce(new Promise(resolve => { resolveFirst = resolve }))
    const wrapper = render()
    routeState.current.query = { ...request, client_id: 'client-2' }
    routeState.current.fullPath = '/oauth/mcp/authorize?request=2'
    previewAuthorization.mockResolvedValueOnce({ ...consent, client_id: 'client-2', client_name: 'Second client' })
    await flushPromises()
    expect(wrapper.get('[data-testid="client-name"]').text()).toBe('Second client')
    resolveFirst(consent)
    await flushPromises()
    expect(wrapper.get('[data-testid="client-name"]').text()).toBe('Second client')
  })
})
