import test from 'node:test';
import assert from 'node:assert/strict';
import { browserLaunchFailure, browserLaunchOptions } from '../src/browser-launch.mjs';

test('Windows helper uses installed Edge while other platforms keep bundled Chromium', () => {
  assert.deepEqual(browserLaunchOptions('win32', '', ''), { headless: false, chromiumSandbox: true, channel: 'msedge' });
  assert.deepEqual(browserLaunchOptions('linux', '', ''), { headless: false, chromiumSandbox: true });
  assert.deepEqual(browserLaunchOptions('win32', 'chromium', ''), { headless: false, chromiumSandbox: true });
});

test('Edge startup reports a safe error category without raw browser diagnostics', () => {
  assert.equal(browserLaunchFailure(new Error('Executable does not exist at C:\\private\\path'), { channel: 'msedge' }), 'BROWSER_EDGE_NOT_FOUND');
  assert.equal(browserLaunchFailure(new Error('Target page, context or browser has been closed; secret=synthetic'), { channel: 'msedge' }), 'BROWSER_EDGE_CLOSED_ON_START');
  assert.equal(browserLaunchFailure(new Error('unknown secret=synthetic'), { channel: 'msedge' }), 'BROWSER_EDGE_START_FAILED');
});

test('Linux browser uses only an explicit local proxy', () => {
  assert.deepEqual(browserLaunchOptions('linux', '', 'http://127.0.0.1:7890').proxy,
    { server: 'http://127.0.0.1:7890' });
  assert.deepEqual(browserLaunchOptions('linux', '', 'socks5://localhost:7891').proxy,
    { server: 'socks5://localhost:7891' });
  for (const invalid of ['https://127.0.0.1:7890', 'http://example.com:7890',
    'http://user:pass@127.0.0.1:7890', 'http://127.0.0.1:7890/path']) {
    assert.throws(() => browserLaunchOptions('linux', '', invalid), { code: 'BROWSER_PROXY_INVALID' });
  }
});
