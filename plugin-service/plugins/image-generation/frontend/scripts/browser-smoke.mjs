import { mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { chromium } from 'playwright-core'

const baseURL = process.env.PLUGIN_URL || 'http://127.0.0.1:8091/plugins/image-generation'
const screenshotDir = resolve(process.env.SCREENSHOT_DIR || tmpdir(), 'sub2api-image-generation-smoke')
const executablePath = process.env.EDGE_PATH || 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
await mkdir(screenshotDir, { recursive: true })

const browser = await chromium.launch({ executablePath, headless: true })
const errors = []
const referenceImage = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z5ZkAAAAASUVORK5CYII=', 'base64')

async function preparePage(viewport) {
  let uploadIndex = 0
  const page = await browser.newPage({ viewport })
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/logo.png', route => route.fulfill({ status: 204 }))
  await page.route('**/api/v1/keys?**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ code: 0, data: { items: [{
      id: 1, key: 'sk-test', name: 'Browser test key', status: 'active',
      group: { allow_image_generation: true, models_list_config: { enabled: true, models: ['gpt-image-2', 'gemini-2.5-flash-image'] } },
    }] } }),
  }))
  await page.route('**/api/v1/user/preferences/image-generation', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ code: 0, data: { last_api_key_id: 1 } }),
  }))
  await page.route('**/plugins/image-generation/api/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/config')) return route.fulfill({ json: {
      image_model_capabilities: {
        'gpt-image-2': {
          max_reference_images: 16, max_output_images: 10,
          sizes: { values: ['1024x1024', '1536x1024', '1024x1536'], default: '1024x1024' },
          quality: { values: ['auto', 'low', 'medium', 'high'], default: 'auto' },
          output_formats: { values: ['png', 'jpeg', 'webp'], default: 'png' },
          output_compression: { min: 0, max: 100, default: 100 },
          background: { values: ['auto', 'transparent', 'opaque'], default: 'auto' },
          input_fidelity: { values: ['low', 'high'], default: 'high' },
        },
        'gemini-2.5-flash-image': {
          max_reference_images: 10, max_output_images: 4,
          aspect_ratios: { values: ['1:1', '2:3', '3:2', '3:4', '4:3', '4:5', '5:4', '9:16', '16:9', '21:9'], default: '1:1' },
          resolutions: { values: ['1K', '2K', '4K'], default: '1K' },
        },
      },
    } })
    if (url.pathname.endsWith('/prompt-models')) return route.fulfill({ json: { models: ['gpt-5.1'] } })
    if (url.pathname.endsWith('/references') && route.request().method() === 'POST') {
      uploadIndex += 1
      return route.fulfill({ status: 201, json: {
        name: `reference-${uploadIndex}.png`, mime_type: 'image/png',
        storage_key: `uploads/${uploadIndex}/original`, preview_storage_key: `uploads/${uploadIndex}/preview`,
        original_url: `/plugins/image-generation/api/references/${uploadIndex}/original`,
        preview_url: `/plugins/image-generation/api/references/${uploadIndex}/preview`,
      } })
    }
    if (url.pathname.includes('/references/')) return route.fulfill({ contentType: 'image/png', body: referenceImage })
    if (url.pathname.includes('/assets/')) return route.fulfill({
      contentType: 'image/png',
      body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z5ZkAAAAASUVORK5CYII=', 'base64'),
    })
    if (url.pathname.endsWith('/conversations')) return route.fulfill({ json: { items: [] } })
    if (url.pathname.includes('/conversations/') && url.pathname.endsWith('/messages')) return route.fulfill({ json: { items: [] } })
    if (url.pathname.endsWith('/generate')) {
      const generationPayload = route.request().postDataJSON()
      if (generationPayload?.output_count !== 3) throw new Error('Generation request is missing output_count=3')
      return route.fulfill({ status: 201, json: {
      job_id: 'browser-job', status: 'succeeded',
      result: { images: [{
        url: '/plugins/image-generation/api/assets/browser-job/result/0',
        preview_url: '/plugins/image-generation/api/assets/browser-job/result/0/preview',
        revised_prompt: 'Browser smoke result',
      }] },
      } })
    }
    return route.fulfill({ status: 404, json: { error: 'not found' } })
  })
  return page
}

try {
  const desktop = await preparePage({ width: 1440, height: 900 })
  await desktop.goto(baseURL, { waitUntil: 'networkidle' })
  await desktop.getByTestId('image-preset-button').click()
  await desktop.locator('.preset-option').filter({ hasText: '电影感' }).getByRole('checkbox').check()
  await desktop.locator('.preset-option').filter({ hasText: '正面' }).getByRole('checkbox').check()
  await desktop.locator('.preset-option').filter({ hasText: '背面' }).getByRole('checkbox').check()
  await desktop.getByText('将创建 2 个独立角度任务').waitFor()
  const presetDialogLayout = await desktop.evaluate(() => {
    const dialog = document.querySelector('.preset-dialog')?.getBoundingClientRect()
    return Boolean(dialog && dialog.left >= 0 && dialog.right <= innerWidth && dialog.top >= 0 && dialog.bottom <= innerHeight)
  })
  if (!presetDialogLayout) throw new Error('Desktop preset dialog is outside the viewport')
  await desktop.screenshot({ path: resolve(screenshotDir, 'image-presets-desktop.png'), fullPage: false })
  await desktop.getByRole('button', { name: '清空' }).click()
  await desktop.getByRole('button', { name: '应用到提示词' }).click()
  if (await desktop.getByTestId('image-quality-select').count() !== 1) throw new Error('GPT quality control is missing')
  await desktop.getByTestId('image-output-format-select').selectOption('webp')
  if (await desktop.getByTestId('image-output-compression').count() !== 1) throw new Error('WebP compression control is missing')
  await desktop.getByTestId('image-model-select').selectOption('gemini-2.5-flash-image')
  if (await desktop.getByTestId('image-aspect-ratio-select').locator('option').count() !== 10) throw new Error('Gemini ratio list is incomplete')
  if (await desktop.getByTestId('image-quality-select').count() !== 0) throw new Error('GPT controls remain visible for Gemini')
  await desktop.getByTestId('image-model-select').selectOption('gpt-image-2')
  await desktop.getByTestId('reference-image-input').setInputFiles([1, 2].map(index => ({
    name: `reference-${index}.png`, mimeType: 'image/png', buffer: referenceImage,
  })))
  const fanTrigger = desktop.getByTestId('reference-count-toggle')
  await fanTrigger.waitFor()
  await fanTrigger.click()
  await desktop.getByTestId('reference-image-input').setInputFiles({
    name: 'reference-3.png', mimeType: 'image/png', buffer: referenceImage,
  })
  await desktop.waitForFunction(() => document.querySelector('[data-testid="reference-count-toggle"]')?.textContent?.trim() === '3/16')
  await desktop.waitForFunction(() => {
    const composer = document.querySelector('[data-testid="image-chat-composer"]')?.getBoundingClientRect()
    const firstItem = document.querySelector('[data-testid="reference-fan-item"]')?.getBoundingClientRect()
    return composer && firstItem && firstItem.top < composer.top
  })
  const desktopFanLayout = await desktop.evaluate(() => {
    const composer = document.querySelector('[data-testid="image-chat-composer"]')?.getBoundingClientRect()
    const items = Array.from(document.querySelectorAll('[data-testid="reference-fan-item"]')).map(item => item.getBoundingClientRect())
    const expanded = document.querySelector('[data-testid="reference-count-toggle"]')?.getAttribute('aria-expanded')
    return {
      passed: Boolean(composer && items.length === 3
        && items.every(item => item.top < composer.top && item.left >= 0 && item.right <= innerWidth)
        && expanded === 'true'),
      composer: composer && { top: composer.top, left: composer.left, right: composer.right },
      items: items.map(item => ({ top: item.top, left: item.left, right: item.right })),
      expanded,
    }
  })
  if (!desktopFanLayout.passed) throw new Error(`Desktop reference fan layout failed: ${JSON.stringify(desktopFanLayout)}`)
  await desktop.screenshot({ path: resolve(screenshotDir, 'reference-fan-desktop.png'), fullPage: false })
  await desktop.getByTestId('remove-reference-image').last().click()
  await desktop.waitForFunction(() => {
    const toggle = document.querySelector('[data-testid="reference-count-toggle"]')
    return toggle?.textContent?.trim() === '2/16' && toggle.getAttribute('aria-expanded') === 'true'
  })
  await desktop.getByTestId('clear-reference-images').click()
  await desktop.waitForFunction(() => !document.querySelector('[data-testid="reference-count-toggle"]'))
  await desktop.keyboard.press('Escape')
  await desktop.getByTestId('image-prompt-input').fill('Create a browser smoke image')
  await desktop.getByTestId('image-output-count').selectOption('3')
  await desktop.getByTestId('image-send-button').click()
  try {
    await desktop.getByTestId('message-attachments').getByText('Browser smoke result').waitFor()
  } catch (error) {
    console.error('Browser smoke page text:', await desktop.locator('body').innerText())
    console.error('Browser smoke console errors:', errors)
    throw error
  }
  const visualContract = await desktop.evaluate(() => {
    const style = (selector) => getComputedStyle(document.querySelector(selector))
    return {
      bodyBackground: getComputedStyle(document.body).backgroundColor,
      historyBackground: style('[data-testid="image-history"]').backgroundColor,
      historyWidth: style('[data-testid="image-history"]').width,
      composerRadius: style('[data-testid="image-chat-composer"]').borderRadius,
      assistantRadius: style('.message-assistant .message-body').borderRadius,
      chatBackground: style('[data-testid="image-chat-panel"]').backgroundColor,
    }
  })
  const expectedVisualContract = {
    bodyBackground: 'rgb(248, 246, 241)',
    historyBackground: 'rgb(251, 251, 248)',
    historyWidth: '300px',
    composerRadius: '28px',
    assistantRadius: '24px',
    chatBackground: 'rgb(248, 246, 241)',
  }
  if (JSON.stringify(visualContract) !== JSON.stringify(expectedVisualContract)) {
    throw new Error(`Visual contract mismatch: ${JSON.stringify(visualContract)}`)
  }
  await desktop.getByTestId('history-inline-collapse').click()
  await desktop.getByTestId('history-drawer-toggle').waitFor({ state: 'visible' })
  const collapsedRight = await desktop.getByTestId('image-history').evaluate(element => element.getBoundingClientRect().right)
  if (collapsedRight > 0) throw new Error('Desktop history sidebar did not collapse')
  await desktop.getByTestId('history-drawer-toggle').click()
  await desktop.getByTestId('image-history').waitFor({ state: 'visible' })
  const desktopLayout = await desktop.evaluate(() => {
    const composer = document.querySelector('[data-testid="image-chat-composer"]')?.getBoundingClientRect()
    const singleImageBubble = document.querySelector('.message-assistant .message-body:has(.image-grid.single-image)')?.getBoundingClientRect()
    const actionTops = Array.from(document.querySelectorAll('.image-actions button')).map(button => button.getBoundingClientRect().top)
    const userBubble = document.querySelector('.message-user .message-body')?.getBoundingClientRect()
    const userText = document.querySelector('.message-user')?.textContent || ''
    const parameterPills = document.querySelectorAll('.message-user .request-settings span').length
    const checks = {
      composer: Boolean(composer && composer.left >= 0 && composer.right <= innerWidth && composer.bottom <= innerHeight),
      singleImageBubble: Boolean(singleImageBubble && singleImageBubble.width <= 400),
      actionRow: actionTops.length === 4 && new Set(actionTops.map(top => Math.round(top))).size === 1,
      userBubble: Boolean(userBubble && userBubble.width <= 768 && userBubble.left >= 0 && userBubble.right <= innerWidth),
      userText: userText.includes('Prompt') && userText.includes('创作描述') && userText.includes('生成参数'),
      parameterPills: parameterPills === 1,
    }
    return { passed: Object.values(checks).every(Boolean), checks, userBubbleWidth: userBubble?.width, userText }
  })
  if (!desktopLayout.passed) throw new Error(`Desktop layout contract failed: ${JSON.stringify(desktopLayout)}`)
  await desktop.getByRole('button', { name: '查看原图' }).first().click()
  await desktop.getByRole('dialog', { name: '查看原图' }).waitFor()
  const downloadHref = await desktop.getByRole('link', { name: '下载原图' }).getAttribute('href')
  if (!downloadHref?.includes('download=1')) throw new Error('Original download URL is missing download=1')
  await desktop.getByRole('button', { name: '关闭原图' }).click()
  await desktop.screenshot({ path: resolve(screenshotDir, 'image-generation-desktop.png'), fullPage: true })
  await desktop.close()

  const mobile = await preparePage({ width: 390, height: 844 })
  await mobile.goto(baseURL, { waitUntil: 'networkidle' })
  await mobile.getByTestId('image-preset-button').click()
  await mobile.locator('.preset-option').filter({ hasText: '正面' }).getByRole('checkbox').check()
  await mobile.locator('.preset-option').filter({ hasText: '背面' }).getByRole('checkbox').check()
  const mobilePresetLayout = await mobile.evaluate(() => {
    const dialog = document.querySelector('.preset-dialog')?.getBoundingClientRect()
    return Boolean(dialog && dialog.left >= 0 && dialog.right <= innerWidth && document.documentElement.scrollWidth <= innerWidth)
  })
  if (!mobilePresetLayout) throw new Error('Mobile preset dialog overflows the viewport')
  await mobile.screenshot({ path: resolve(screenshotDir, 'image-presets-mobile.png'), fullPage: false })
  await mobile.getByRole('button', { name: '清空' }).click()
  await mobile.getByRole('button', { name: '应用到提示词' }).click()
  await mobile.getByTestId('reference-image-input').setInputFiles([1, 2].map(index => ({
    name: `mobile-reference-${index}.png`, mimeType: 'image/png', buffer: referenceImage,
  })))
  await mobile.getByTestId('reference-count-toggle').click()
  await mobile.getByTestId('reference-image-input').setInputFiles({
    name: 'mobile-reference-3.png', mimeType: 'image/png', buffer: referenceImage,
  })
  await mobile.waitForFunction(() => document.querySelector('[data-testid="reference-count-toggle"]')?.textContent?.trim() === '3/16')
  const mobileControlSeparation = await mobile.evaluate(() => {
    const count = document.querySelector('[data-testid="reference-count-toggle"]')?.getBoundingClientRect()
    const upload = document.querySelector('[data-testid="reference-upload-label"]')?.getBoundingClientRect()
    if (!count || !upload) return false
    return count.left >= upload.right - 2 || count.top >= upload.bottom - 2
  })
  if (!mobileControlSeparation) throw new Error('Mobile count badge overlaps the centered upload control')
  await mobile.waitForFunction(() => {
    const composer = document.querySelector('[data-testid="image-chat-composer"]')?.getBoundingClientRect()
    const firstItem = document.querySelector('[data-testid="reference-fan-item"]')?.getBoundingClientRect()
    return composer && firstItem && firstItem.top < composer.top
  })
  const mobileFanLayout = await mobile.evaluate(() => {
    const composer = document.querySelector('[data-testid="image-chat-composer"]')?.getBoundingClientRect()
    const items = Array.from(document.querySelectorAll('[data-testid="reference-fan-item"]')).map(item => item.getBoundingClientRect())
    return {
      passed: Boolean(composer && items.length === 3
        && items.every(item => item.top < composer.top && item.left >= 0 && item.right <= innerWidth)
        && document.documentElement.scrollWidth <= innerWidth),
      composer: composer && { top: composer.top, left: composer.left, right: composer.right },
      items: items.map(item => ({ top: item.top, left: item.left, right: item.right })),
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth,
    }
  })
  if (!mobileFanLayout.passed) throw new Error(`Mobile reference fan layout failed: ${JSON.stringify(mobileFanLayout)}`)
  await mobile.screenshot({ path: resolve(screenshotDir, 'reference-fan-mobile.png'), fullPage: false })
  const mobileClearButton = await mobile.getByTestId('clear-reference-images').boundingBox()
  if (!mobileClearButton || mobileClearButton.x < 0 || mobileClearButton.x + mobileClearButton.width > 390) {
    throw new Error(`Mobile clear button is outside the viewport: ${JSON.stringify(mobileClearButton)}`)
  }
  await mobile.keyboard.press('Escape')
  await mobile.getByTestId('image-quality-select').selectOption('high')
  await mobile.getByTestId('image-output-format-select').selectOption('webp')
  await mobile.getByTestId('image-output-compression').fill('82')
  await mobile.getByTestId('image-background-select').selectOption('transparent')
  await mobile.getByTestId('image-input-fidelity-select').selectOption('high')
  await mobile.getByTestId('image-output-count').selectOption('3')
  await mobile.getByTestId('image-prompt-input').fill('Create a narrow viewport parameter wrapping test')
  await mobile.getByTestId('image-send-button').click()
  await mobile.getByTestId('message-attachments').getByText('Browser smoke result').waitFor()
  const mobileParameterLayout = await mobile.evaluate(() => {
    const pill = document.querySelector('.message-user .request-settings span')
    if (!pill) return { passed: false, reason: 'parameter pill missing' }
    const rect = pill.getBoundingClientRect()
    const style = getComputedStyle(pill)
    return {
      passed: style.whiteSpace === 'normal'
        && rect.left >= 0
        && rect.right <= innerWidth
        && pill.scrollWidth <= Math.ceil(rect.width)
        && document.documentElement.scrollWidth <= innerWidth,
      whiteSpace: style.whiteSpace,
      rect: { left: rect.left, right: rect.right, width: rect.width, height: rect.height },
      scrollWidth: pill.scrollWidth,
      pageScrollWidth: document.documentElement.scrollWidth,
      innerWidth,
    }
  })
  if (!mobileParameterLayout.passed) throw new Error(`Mobile parameter wrapping failed: ${JSON.stringify(mobileParameterLayout)}`)
  await mobile.locator('.message-user .request-settings span').scrollIntoViewIfNeeded()
  await mobile.screenshot({ path: resolve(screenshotDir, 'parameter-wrap-mobile.png'), fullPage: true })
  await mobile.getByTestId('history-drawer-toggle').click()
  await mobile.getByTestId('image-key-select').waitFor({ state: 'visible' })
  await mobile.waitForTimeout(250)
  const mobileLayout = await mobile.evaluate(() => {
    const sidebar = document.querySelector('.sidebar-wrap')?.getBoundingClientRect()
    return document.documentElement.scrollWidth <= innerWidth && sidebar && sidebar.left >= 0 && sidebar.right > 0
  })
  if (!mobileLayout) throw new Error('Mobile page has horizontal overflow')
  await mobile.screenshot({ path: resolve(screenshotDir, 'image-generation-mobile.png'), fullPage: true })
  await mobile.close()

  if (errors.length) throw new Error(`Browser console errors:\n${errors.join('\n')}`)
  console.log(`Browser smoke test passed. Screenshots: ${screenshotDir}`)
} finally {
  await browser.close()
}
