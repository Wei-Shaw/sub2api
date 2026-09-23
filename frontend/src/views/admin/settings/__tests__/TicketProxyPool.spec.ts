import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import TicketProxyPool from '../TicketProxyPool.vue';
import zhSettings from '@/i18n/locales/zh/admin/settings';

const { getPool, importPool } = vi.hoisted(() => ({ getPool: vi.fn(), importPool: vi.fn() }));
vi.mock('@/api', () => ({ adminAPI: { settings: { getTicketProxyPool: getPool, importTicketProxyPool: importPool } } }));
// The test configuration uses the runtime-only i18n build. Resolve the real
// translated strings here, following the existing SettingsView test pattern.
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, values: Record<string, unknown> = {}) => {
    const messages = zhSettings.settings.gatewayForwarding;
    const message = messages[key.split('.').at(-1) as keyof typeof messages];
    return typeof message === 'string' ? message.replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? name)) : key;
  },
}) }));
const wrappers: ReturnType<typeof mount>[] = [];
function render() {
  const wrapper = mount(TicketProxyPool, { global: { stubs: { OnesProxyImport: true } } });
  wrappers.push(wrapper);
  return wrapper;
}
beforeEach(() => {
  vi.clearAllMocks();
  getPool.mockResolvedValue({ managed: true, count: 200, removed_count: 4, failure_limit: 1 });
});
afterEach(() => { wrappers.splice(0).forEach((w) => w.unmount()); vi.useRealTimers(); });

describe('ticket proxy pool', () => {
  it('imports trimmed nonempty lines and displays counts without a settings save', async () => {
    importPool.mockResolvedValue({ managed: true, count: 201, removed_count: 4, failure_limit: 1, added: 1, duplicates: 1 });
    const w = render(); await flushPromises();
    expect(w.get('[data-testid="pool-status"]').text()).toContain('200');
    await w.get('textarea').setValue(' 192.0.2.1:8080 \r\n\n192.0.2.1:8080');
    await w.get('[data-testid="pool-import"]').trigger('click'); await flushPromises();
    expect(importPool).toHaveBeenCalledWith(['192.0.2.1:8080', '192.0.2.1:8080']);
    expect(w.get('textarea').element.value).toBe('');
    expect(w.get('[role="status"]').text()).toContain('已新增 1 条');
    expect(w.get('[data-testid="pool-status"]').text()).toContain('201');
  });
  it('preserves input when validation fails', async () => {
    importPool.mockRejectedValue({ response: { data: { message: '第 2 行格式不正确' } } });
    const w = render(); await flushPromises();
    await w.get('textarea').setValue('invalid');
    await w.get('[data-testid="pool-import"]').trigger('click'); await flushPromises();
    expect(w.get('textarea').element.value).toBe('invalid');
    expect(w.get('[role="alert"]').text()).toContain('第 2 行');
    expect(w.get('[data-testid="pool-status"]').text()).toContain('200');
  });
  it('blocks an oversized list before calling the API', async () => {
    const w = render(); await flushPromises();
    await w.get('textarea').setValue(Array(2001).fill('192.0.2.1:80').join('\n'));
    await w.get('[data-testid="pool-import"]').trigger('click');
    expect(importPool).not.toHaveBeenCalled();
    expect(w.get('[role="alert"]').text()).toContain('2000');
  });
  it('loads a text file for review and does not import until clicked', async () => {
    const w = render(); await flushPromises();
    const file = { size: 40, text: vi.fn().mockResolvedValue('192.0.2.1:80\n192.0.2.2:80') };
    Object.defineProperty(w.get('input[type="file"]').element, 'files', { value: [file], configurable: true });
    await w.get('input[type="file"]').trigger('change'); await flushPromises();
    expect(w.get('textarea').element.value).toContain('192.0.2.2:80');
    expect(importPool).not.toHaveBeenCalled();
  });
  it('shows an empty pool as paused and clears the polling timer on unmount', async () => {
    vi.useFakeTimers();
    getPool.mockResolvedValue({ managed: true, count: 0, removed_count: 200, failure_limit: 1 });
    const w = render(); await flushPromises();
    expect(w.text()).toContain('暂停打票');
    w.unmount(); wrappers.pop();
    const calls = getPool.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30000);
    expect(getPool).toHaveBeenCalledTimes(calls);
  });
});
