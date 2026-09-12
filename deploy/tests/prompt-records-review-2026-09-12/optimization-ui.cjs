const { chromium } = require('/tmp/browser/node_modules/playwright-core');
const fs = require('node:fs');
const assert = require('node:assert/strict');
let browser;
let page;
(async () => {
  browser = await chromium.launch({executablePath:'/usr/bin/chromium', headless:true, args:['--no-sandbox','--disable-dev-shm-usage']});
  page = await browser.newPage({reducedMotion:'reduce'});
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const results = [];
  for (const width of [320,375,768,1440]) {
    await page.setViewportSize({width,height:1000});
    await page.goto('http://127.0.0.1:5173/record-optimization-preview.html');
    await page.locator('[data-test="prompt-record-detail-7"]').waitFor();
    await page.locator('[data-test="record-queue-stats"] summary').click();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
    assert.equal(overflow,false,`page overflow at ${width}px`);
    await page.screenshot({path:`/audit/optimization-ui-${width}.png`,fullPage:true});
    await page.locator('[data-test="record-next"]').click();
    await page.locator('[data-test="record-next"][disabled]').waitFor();
    assert.equal(await page.locator('[data-test="record-previous"]').isDisabled(),false);
    await page.locator('[data-test="record-previous"]').click();
    await page.locator('[data-test="record-previous"][disabled]').waitFor();
    await page.locator('#prompt-record-retention').fill('7');
    await page.locator('form').first().locator('button[type="submit"]').click();
    await page.waitForFunction(() => window.previewRetentionSaved === 7);
    await page.locator('[data-test="prompt-record-detail-7"]').click();
    await page.getByText('此记录来自 WebSocket；当前版本尚未采集该协议的响应。').waitFor();
    await page.getByText('此记录来自 WebSocket；当前版本尚未采集该协议的响应。').scrollIntoViewIfNeeded();
    await page.screenshot({path:`/audit/optimization-detail-${width}.png`});
    await page.keyboard.press('Escape');
    await page.getByText('此记录来自 WebSocket；当前版本尚未采集该协议的响应。').waitFor({state:'hidden'});
    results.push({width, pageOverflow:overflow, pagination:true, retentionSave:true, websocketStatus:true});
  }
  assert.deepEqual(errors,[]);
  fs.writeFileSync('/audit/optimization-ui.json',JSON.stringify({results,errors},null,2));
  console.log(JSON.stringify(results));
  await browser.close();
})().catch(async error => {
  console.error(error);
  if (page) {
    console.log(await page.evaluate(() => ({lang:document.documentElement.lang, saved:window.previewRetentionSaved, text:document.body.innerText.slice(0,2500)})));
    await page.screenshot({path:'/audit/optimization-ui-failure.png',fullPage:true});
  }
  await browser?.close(); process.exitCode=1;
});
