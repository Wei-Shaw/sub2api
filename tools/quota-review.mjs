import http from 'node:http'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import { performance } from 'node:perf_hooks'
import { pathToFileURL } from 'node:url'

// Run against static production builds only. No backend or provider is contacted.
const args = Object.fromEntries(process.argv.slice(2).map(x => x.replace(/^--/, '').split('=')))
const { chromium } = await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE).href)
const out = path.resolve(args.out)
await fs.mkdir(out, { recursive: true })
const clock = Date.now()
const resets = [2, 72, 5, 120, 96].map(hours => new Date(clock + hours * 3600000).toISOString())
const user = { id: 1, email: 'review@example.test', username: 'Review', role: 'admin', status: 'active', balance: 0, concurrency: 10 }
function account(id) {
  return { id, name: `Demo ${String(id).padStart(4, '0')}`, platform: 'antigravity', type: 'oauth',
    status: 'active', schedulable: true, concurrency: 1, priority: 1, rate_multiplier: 1,
    credentials: { email: `demo-${id}@example.test` }, extra: { email: `demo-${id}@example.test` },
    groups: [], group_ids: [], created_at: new Date(clock).toISOString(), updated_at: new Date(clock).toISOString(),
    proxy_id: null, error_message: null, expires_at: null, last_used_at: null,
    rate_limit_reset_at: null, overload_until: null, temp_unschedulable_until: null,
    session_window_start: null, session_window_end: null }
}
function usage(id, revision = 0) {
  const used = (id * 7 + revision * 10) % 90
  return { updated_at: new Date(clock).toISOString(), source: 'active', subscription_tier: 'PRO',
    antigravity_quota_groups: [
      { key: 'gemini', name: 'Gemini Models', windows: [
        { window: '5h', utilization: used, resets_at: resets[0] },
        { window: 'weekly', utilization: 35, resets_at: resets[1] }] },
      { key: '3p', name: 'Claude and GPT models', windows: [
        { window: '5h', utilization: 48, resets_at: resets[2] },
        { window: 'weekly', utilization: 65, resets_at: resets[3] }] },
      { key: 'gemini-argon-wtu', name: 'Gemini 4 Argon', windows: [
        { window: 'weekly', utilization: 20, resets_at: resets[4] }] }
    ], antigravity_quota: { 'gemini-3-flash': { utilization: used, reset_time: resets[0] } },
    ai_credits: [{ credit_type: 'GOOGLE_ONE_AI', amount: 25, minimum_balance: 5 }] }
}
async function server(root) {
  root = path.resolve(root)
  const types = { '.html':'text/html', '.js':'text/javascript', '.css':'text/css', '.svg':'image/svg+xml', '.png':'image/png', '.woff2':'font/woff2' }
  const s = http.createServer(async (req, res) => {
    try {
      const pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname)
      if (pathname.startsWith('/api/') || pathname.startsWith('/v1/')) { res.writeHead(501).end('Mock interception required'); return }
      let file = path.resolve(root, '.' + pathname)
      if (!file.startsWith(root + path.sep)) file = path.join(root, 'index.html')
      try { if (!(await fs.stat(file)).isFile()) file = path.join(root, 'index.html') } catch { file = path.join(root, 'index.html') }
      res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream')
      res.end(await fs.readFile(file))
    } catch { res.writeHead(500).end() }
  })
  await new Promise(resolve => s.listen(0, '127.0.0.1', resolve))
  return { origin: `http://127.0.0.1:${s.address().port}`, close: () => new Promise(resolve => s.close(resolve)) }
}
const browser = await chromium.launch({ executablePath: process.env.BROWSER_EXECUTABLE, headless: true })
const results = { browser: browser.version(), simulatedLatencyMs: { list: 25, usage: 50 }, cases: [] }
async function scenario(origin, count, label, mobile = false, fault = false, overview = true, screenshots = false) {
  const context = await browser.newContext({ viewport: mobile ? { width: 390, height: 844 } : { width: 1440, height: 1000 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai', serviceWorkers: 'block' })
  const requests = [], unexpected = [], blocked = [], errors = []
  let revision = 0, failures = fault
  await context.addInitScript(({ user }) => {
    localStorage.setItem('auth_token', 'mock-review-token')
    localStorage.setItem('auth_user', JSON.stringify(user))
    localStorage.setItem('sub2api_locale', 'zh')
    localStorage.setItem('theme', 'light')
    localStorage.setItem('admin_guide_1_admin_v4_interactive', 'true')
    window.__reviewLongTasks = []
    new PerformanceObserver(list => window.__reviewLongTasks.push(...list.getEntries().map(e => ({ start: e.startTime, duration: e.duration })))).observe({ type: 'longtask', buffered: true })
  }, { user })
  await context.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url())
    if (url.origin !== origin) { blocked.push(req.url()); await route.abort(); return }
    if (!url.pathname.startsWith('/api/')) { await route.continue(); return }
    const endpoint = url.pathname.replace('/api/v1', '')
    const body = req.postDataJSON() || {}
    requests.push({ endpoint, method: req.method(), ids: body.account_ids, force: body.force, page: url.searchParams.get('page'), pageSize: url.searchParams.get('page_size') })
    let data
    if (endpoint === '/admin/accounts') {
      const page = Number(url.searchParams.get('page') || 1), size = Number(url.searchParams.get('page_size') || 20)
      data = { items: Array.from({ length: Math.max(0, Math.min(size, count - (page - 1) * size)) }, (_, i) => account((page - 1) * size + i + 1)), total: count, page, page_size: size, pages: Math.ceil(count / size) }
      await new Promise(r => setTimeout(r, 25))
    } else if (endpoint === '/admin/accounts/usage/batch') {
      data = { usage: {}, errors: {} }
      for (const id of body.account_ids) {
        if (failures && id % 10 === 0) data.errors[id] = 'mock quota unavailable'
        else data.usage[id] = usage(id, revision)
      }
      await new Promise(r => setTimeout(r, args.mode === 'slow' && body.account_ids.length > 100 ? 31000 : 50))
    } else if (/^\/admin\/accounts\/\d+\/usage$/.test(endpoint)) data = usage(Number(endpoint.split('/')[3]), revision)
    else if (endpoint === '/admin/accounts/today-stats/batch') data = { stats: Object.fromEntries((body.account_ids || []).map(id => [id, { requests: 0, tokens: 0, cost: 0, standard_cost: 0, user_cost: 0 }])) }
    else if (endpoint === '/auth/me' || endpoint === '/user/me') data = user
    else if (endpoint === '/settings/public' || endpoint === '/admin/settings') data = { site_name: 'Sub2API · 模拟数据', registration_enabled: false, custom_menu_items: [], ops_monitoring_enabled: false }
    else if (endpoint === '/admin/proxies/all' || endpoint === '/admin/groups/all') data = []
    else if (endpoint === '/admin/accounts/upstream-billing-probe/settings') data = { enabled: false }
    else if (endpoint === '/admin/accounts/upstream-billing-rates') data = { items: [], total: 0 }
    else if (endpoint === '/announcements' || endpoint === '/announcements/unread') data = []
    else if (['/admin/compliance', '/admin/payment/config', '/admin/settings/web-search-emulation', '/admin/system/check-updates'].includes(endpoint)) data = {}
    else if (endpoint === '/subscriptions/active') data = []
    else if (endpoint === '/keys') data = { items: [], total: 0 }
    else if (endpoint.includes('/version')) data = { version: 'review', latest_version: 'review', has_update: false }
    else { unexpected.push(endpoint); data = {} }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) })
  })
  const page = await context.newPage()
  page.on('pageerror', e => { errors.push(e.message); console.error('PAGE_ERROR', e.message) })
  const start = performance.now()
  await page.goto(origin + '/admin/accounts')
  await page.getByText('Demo 0001', { exact: true }).first().waitFor({ timeout: 30000 })
  await page.waitForTimeout(350)
  if (screenshots) {
    await page.evaluate(() => {
      for (let el = document.querySelector('table')?.parentElement; el; el = el.parentElement) {
        if (el.scrollWidth > el.clientWidth + 50) { const header = [...document.querySelectorAll('th')].find(h => h.textContent.includes('用量窗口')); if (header) el.scrollLeft += header.getBoundingClientRect().left - el.getBoundingClientRect().left - 240; break }
      }
    })
    if (mobile) await page.evaluate(() => window.scrollTo(0, 320))
    await page.waitForTimeout(100)
    await page.screenshot({ path: path.join(out, `${label}-list.png`) })
  }
  const result = { label, count, mobile, listReadyMs: Math.round(performance.now() - start), unexpected, blocked, errors }
  if (overview) {
    const open = page.getByRole('button', { name: '额度总览', exact: true })
    const openAt = await page.evaluate(() => { window.__reviewLongTasks = []; return performance.now() })
    const reqStart = requests.length
    await open.click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('button', { name: '刷新', exact: true }).waitFor({ timeout: 120000 })
    await page.waitForTimeout(100)
    const metrics = await page.evaluate(openAt => ({ openMs: Math.round(performance.now() - openAt), domNodes: document.querySelectorAll('[role="dialog"] *').length, longTasks: window.__reviewLongTasks, rows: [...document.querySelectorAll('[role="dialog"] [title]')].filter(e => /^Demo \d+$/.test(e.getAttribute('title'))).length }), openAt)
    Object.assign(result, metrics)
    result.initialErrors = await dialog.getByText('额度暂时读取失败', { exact: true }).count()
    result.openRequests = requests.slice(reqStart)
    result.initialQuotaRequests = requests.filter(r => r.endpoint.endsWith('/usage/batch')).map(r => ({ ids: r.ids, force: r.force }))
    result.summaryText = await dialog.locator('.card').allTextContents()
    if (args.verify === 'true') {
      assert.equal(errors.length, 0, 'browser runtime errors')
      assert.equal(unexpected.length, 0, 'unmocked API endpoint')
      assert.equal(result.initialErrors > 0, fault, 'partial failures must remain visible')
      assert.ok(result.rows <= 25, 'detail rendering must remain bounded')
      const ids = result.initialQuotaRequests.flatMap(r => r.ids)
      assert.equal(ids.length, count, 'each account must be fetched exactly once')
      assert.equal(new Set(ids).size, count, 'overlapping list/overview queries must coalesce')
      assert.ok(result.initialQuotaRequests.every(r => r.ids.length <= 50), 'batch limit')
      if (count > 25) {
        const beforePage = requests.length
        await dialog.getByRole('button', { name: '下一页', exact: true }).click()
        await dialog.locator('[data-quota-account="26"]').waitFor()
        assert.deepEqual(await dialog.locator('.card').allTextContents(), result.summaryText, 'paging must preserve totals')
        await dialog.locator('select').first().selectOption(String(count))
        assert.equal(await dialog.locator('[data-quota-account]').count(), 1)
        assert.equal(await dialog.locator(`[data-quota-account="${count}"]`).count(), 1)
        await dialog.locator('select').first().selectOption('all')
        await dialog.locator('[data-quota-account="1"]').waitFor()
        assert.equal(requests.length, beforePage, 'paging/filtering must not fetch quotas')
      }
    }
    if (screenshots) await page.screenshot({ path: path.join(out, `${label}-overview.png`) })
    const probe = await page.evaluate(async () => {
      const stamps = [performance.now()]
      await new Promise(resolve => { const frame = () => { stamps.push(performance.now()); if (stamps.at(-1) - stamps[0] >= 2200) resolve(); else requestAnimationFrame(frame) }; requestAnimationFrame(frame) })
      return { maxFrameGapMs: Math.round(Math.max(...stamps.slice(1).map((n, i) => n - stamps[i]))), longTasks: window.__reviewLongTasks }
    })
    result.idle = probe
    const beforeReopen = requests.length
    await dialog.getByRole('button', { name: 'Close modal' }).click()
    await open.click()
    await dialog.getByRole('button', { name: '刷新', exact: true }).waitFor({ timeout: 120000 })
    result.reopenRequests = requests.slice(beforeReopen)
    if (args.verify === 'true') assert.equal(result.reopenRequests.filter(r => r.endpoint.endsWith('/usage/batch')).length, 0, 'reopen uses shared cache')
    failures = false; revision++
    const beforeRefresh = requests.length, refreshStart = performance.now()
    await dialog.getByRole('button', { name: '刷新', exact: true }).click()
    await page.waitForTimeout(150)
    await dialog.getByRole('button', { name: '刷新', exact: true }).waitFor({ timeout: 120000 })
    result.refreshMs = Math.round(performance.now() - refreshStart)
    result.refreshRequests = requests.slice(beforeRefresh)
    result.remainingErrors = await dialog.getByText('额度暂时读取失败', { exact: true }).count()
    if (args.verify === 'true') {
      assert.equal(result.remainingErrors, 0, 'manual refresh recovers failed accounts')
      const refreshed = result.refreshRequests.filter(r => r.endpoint.endsWith('/usage/batch'))
      assert.equal(refreshed.flatMap(r => r.ids).length, count)
      assert.ok(refreshed.every(r => r.force === true && r.ids.length <= 50))
      const beforeReload = requests.length
      await page.reload()
      await page.getByText('Demo 0001', { exact: true }).first().waitFor()
      await page.getByRole('button', { name: '额度总览', exact: true }).click()
      await page.getByRole('dialog').getByRole('button', { name: '刷新', exact: true }).waitFor()
      result.reloadQuotaRequests = requests.slice(beforeReload).filter(r => r.endpoint.endsWith('/usage/batch')).length
      assert.equal(result.reloadQuotaRequests, 0, 'reload uses persisted cache')
    }
  }
  results.cases.push(result)
  await fs.writeFile(path.join(out, 'results.json'), JSON.stringify(results, null, 2))
  console.log(JSON.stringify({ label, count, openMs: result.openMs, domNodes: result.domNodes, rows: result.rows, maxFrameGapMs: result.idle?.maxFrameGapMs, unexpected, blocked, errors }))
  await context.close()
}
const servers = []
try {
  if (args.before) {
    const s = await server(args.before); servers.push(s)
    await scenario(s.origin, 6, 'main-desktop', false, false, false, true)
    await scenario(s.origin, 6, 'main-mobile', true, false, false, true)
  }
  const s = await server(args.after); servers.push(s)
  if (args.mode === 'perf') {
    for (const count of [100, 500, 1000]) await scenario(s.origin, count, `desktop-${count}`)
    await scenario(s.origin, 1000, 'mobile-1000', true)
  }
  else if (args.mode === 'slow') await scenario(s.origin, 1000, 'slow-1000')
  else if (args.mode === 'shots') {
    await scenario(s.origin, 6, 'pr-desktop', false, false, true, true)
    await scenario(s.origin, 6, 'pr-mobile', true, false, true, true)
    await scenario(s.origin, 1000, 'pr-large', false, false, true, true)
  }
  else if (args.mode === 'smoke') await scenario(s.origin, 6, 'pr-desktop', false, false, true, true)
  else {
    await scenario(s.origin, 6, 'pr-desktop', false, false, true, true)
    await scenario(s.origin, 6, 'pr-mobile', true, false, true, true)
    for (const count of [100, 500, 1000]) await scenario(s.origin, count, `desktop-${count}`)
    await scenario(s.origin, 1000, 'mobile-1000', true)
    await scenario(s.origin, 100, 'partial-failure-100', false, true)
  }
} finally {
  await browser.close()
  for (const s of servers) await s.close()
}
