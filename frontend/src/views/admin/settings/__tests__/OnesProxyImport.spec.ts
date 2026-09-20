import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import OnesProxyImport from '../OnesProxyImport.vue';
import zhSettings from '@/i18n/locales/zh/admin/settings';

const { getConfig, saveConfig, fetchProxies } = vi.hoisted(() => ({ getConfig: vi.fn(), saveConfig: vi.fn(), fetchProxies: vi.fn() }));
vi.mock('@/api', () => ({ adminAPI: { settings: { getTicketProxyProvider: getConfig, saveTicketProxyProvider: saveConfig, fetchTicketProxyProvider: fetchProxies } } }));
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, values: Record<string, unknown> = {}) => {
    const messages = zhSettings.settings.gatewayForwarding;
    const message = messages[key.split('.').at(-1) as keyof typeof messages];
    return typeof message === 'string' ? message.replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? name)) : key;
  },
}) }));
const config = { type: 'generate', user_id: '123', proxy_id: 'SC-test', country: 'US', mode: 1, session_time: 30, route: 'NA', auto_refill: true, configured: true, token_configured: true, extract_configured: false };
const result = { requested: 2000, received: 1500, added: 1499, duplicates: 1, count: 1699, managed: true, removed_count: 0, failure_limit: 1 };
const wrappers: ReturnType<typeof mount>[] = [];
function render() { const w = mount(OnesProxyImport); wrappers.push(w); return w; }
beforeEach(() => {
  vi.clearAllMocks();
  getConfig.mockResolvedValue({ ...config });
  saveConfig.mockResolvedValue({ ...config });
  fetchProxies.mockResolvedValue({ ...result });
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); });

describe('OnesProxy API import', () => {
  it('loads automatic refill without fetching from the browser', async () => {
    const w = render(); await flushPromises();
    expect(w.get<HTMLInputElement>('#onesproxy-count').element.value).toBe('2000');
    expect(w.get<HTMLInputElement>('#onesproxy-token').element.value).toBe('');
    expect(w.get<HTMLInputElement>('#onesproxy-auto-refill').element.checked).toBe(true);
    expect(fetchProxies).not.toHaveBeenCalled();
  });
  it('saves the automatic refill switch without triggering a manual fetch', async () => {
    const w = render(); await flushPromises();
    await w.get('#onesproxy-auto-refill').setValue(false);
    await w.get('[data-testid="onesproxy-save"]').trigger('click'); await flushPromises();
    expect(saveConfig).toHaveBeenCalledWith(expect.objectContaining({ auto_refill: false }));
    expect(fetchProxies).not.toHaveBeenCalled();
  });
  it('can save credentials without fetching and clears the token field after saving', async () => {
    const w = render(); await flushPromises();
    await w.get('#onesproxy-token').setValue('new-secret');
    await w.get('[data-testid="onesproxy-save"]').trigger('click'); await flushPromises();
    expect(saveConfig).toHaveBeenCalledWith(expect.objectContaining({ token: 'new-secret', user_id: '123', proxy_id: 'SC-test' }));
    expect(fetchProxies).not.toHaveBeenCalled();
    expect(w.get<HTMLInputElement>('#onesproxy-token').element.value).toBe('');
    expect(w.get('[role="status"]').text()).toContain('配置已保存');
  });
  it('saves then requests 2000, reports actual counts and updates the parent pool', async () => {
    const w = render(); await flushPromises();
    await w.get('[data-testid="onesproxy-fetch"]').trigger('click'); await flushPromises();
    expect(saveConfig).toHaveBeenCalledTimes(1); expect(fetchProxies).toHaveBeenCalledWith(2000);
    expect(saveConfig.mock.invocationCallOrder[0]).toBeLessThan(fetchProxies.mock.invocationCallOrder[0]);
    expect(w.get('[role="status"]').text()).toContain('实际返回 1500');
    expect(w.get('[role="status"]').text()).toContain('新增 1499');
    expect(w.emitted('imported')?.[0]).toEqual([result]);
    expect(w.emitted('busyChange')).toEqual([[true], [false]]);
  });
  it('requires credentials before making a request', async () => {
    getConfig.mockResolvedValue({ ...config, user_id: '', proxy_id: '', token_configured: false, configured: false });
    const w = render(); await flushPromises();
    await w.get('[data-testid="onesproxy-fetch"]').trigger('click'); await flushPromises();
    expect(saveConfig).not.toHaveBeenCalled(); expect(fetchProxies).not.toHaveBeenCalled();
    expect(w.get('[role="alert"]').text()).toContain('Userid');
  });
  it('does not fetch if configuration saving fails', async () => {
    saveConfig.mockRejectedValue({ message: '配置无效' });
    const w = render(); await flushPromises();
    await w.get('#onesproxy-token').setValue('new-secret');
    await w.get('[data-testid="onesproxy-fetch"]').trigger('click'); await flushPromises();
    expect(fetchProxies).not.toHaveBeenCalled();
    expect(w.get('[role="alert"]').text()).toContain('配置无效');
    expect(w.get<HTMLInputElement>('#onesproxy-token').element.value).toBe('new-secret');
  });
  it('allows a saved extraction URL to remain blank and preserves errors without importing', async () => {
    getConfig.mockResolvedValue({ ...config, type: 'extract', extract_configured: true });
    fetchProxies.mockRejectedValue({ message: 'OnesProxy 未返回代理' });
    const w = render(); await flushPromises();
    expect(w.find('#onesproxy-token').exists()).toBe(false);
    await w.get('[data-testid="onesproxy-fetch"]').trigger('click'); await flushPromises();
    expect(saveConfig).toHaveBeenCalledWith(expect.objectContaining({ type: 'extract', extract_url: '' }));
    expect(w.emitted('imported')).toBeUndefined();
    expect(w.get('[role="alert"]').text()).toContain('未返回代理');
    expect(w.get('fieldset').attributes('disabled')).toBeUndefined();
  });
});
