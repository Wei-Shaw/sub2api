#!/usr/bin/env node
// scripts/verify-seo.mjs
// Quick smoke test for SEO/GEO endpoints. Run after starting the server with seo.enabled=true.
// Usage: node scripts/verify-seo.mjs http://localhost:8080

const base = process.argv[2] || 'http://localhost:8080'
let failures = 0

async function check(label, url, asserts) {
  let text = ''
  let res
  try {
    res = await fetch(url, { headers: { 'User-Agent': 'Googlebot' } })
    text = await res.text()
  } catch (e) {
    console.log(`FAIL  ${label}  ${url}  (fetch error: ${e.message})`)
    failures += 1
    return
  }
  let ok = res.ok
  for (const [name, fn] of Object.entries(asserts)) {
    const passed = fn(text, res)
    console.log(`  ${passed ? 'PASS' : 'FAIL'}  ${name}`)
    if (!passed) {
      ok = false
      failures += 1
    }
  }
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${label}  ${url}`)
  console.log('')
}

await check('Home page SEO', `${base}/home`, {
  status_200: (_, r) => r.status === 200,
  has_description_meta: t => /<meta\s+name="description"/.test(t),
  has_robots_index: t => /<meta\s+name="robots"\s+content="index/.test(t),
  has_og_title: t => /<meta\s+property="og:title"/.test(t),
  has_jsonld: t => /application\/ld\+json/.test(t),
  has_hreflang_zh: t => /hreflang="zh-CN"/.test(t),
  has_hreflang_en: t => /hreflang="en"/.test(t)
})

await check('Login page (noindex)', `${base}/login`, {
  has_noindex: t => /<meta\s+name="robots"\s+content="noindex/.test(t)
})

await check('robots.txt', `${base}/robots.txt`, {
  has_user_agent_all: t => /User-agent:\s*\*/i.test(t),
  has_sitemap_link: t => /Sitemap:\s*http/i.test(t),
  has_gpt_bot: t => /GPTBot/.test(t)
})

await check('sitemap.xml', `${base}/sitemap.xml`, {
  is_xml: (_, r) => (r.headers.get('content-type') || '').includes('xml'),
  has_xhtml_ns: t => /xmlns:xhtml/.test(t),
  has_home: t => /\/home/.test(t),
  has_faq: t => /\/faq/.test(t)
})

await check('llms.txt', `${base}/llms.txt`, {
  is_markdown: (_, r) => (r.headers.get('content-type') || '').includes('markdown'),
  has_title: t => /^# /m.test(t),
  has_faq_section: t => /关键问答|FAQ/.test(t)
})

if (failures > 0) {
  console.error(`\n${failures} assertions failed`)
  process.exit(1)
}
console.log('All SEO assertions passed.')
