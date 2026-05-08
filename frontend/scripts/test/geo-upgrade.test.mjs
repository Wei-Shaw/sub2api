import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import * as cheerio from 'cheerio';
import { processHtml } from '../lib/transforms.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const fx = (name) => readFile(join(__dirname, 'fixtures', name), 'utf8');

function readScripts(html) {
  const $ = cheerio.load(html);
  return $('script[type="application/ld+json"]')
    .map((_, el) => JSON.parse($(el).text()))
    .get();
}

test('processHtml on home-page.html: Org sameAs, FAQPage speakable, Service inserted', async () => {
  const html = await fx('home-page.html');
  const result = processHtml(html);
  const scripts = readScripts(result);

  const graphScript = scripts.find(s => Array.isArray(s['@graph']));
  assert.ok(graphScript, 'expected a @graph script');
  const graph = graphScript['@graph'];

  const org = graph.find(n => n['@type'] === 'Organization');
  assert.ok(org.sameAs.includes('https://t.me/tokenprovider_official'));

  const faq = graph.find(n => n['@type'] === 'FAQPage');
  assert.ok(faq.speakable, 'FAQPage missing speakable');

  const service = graph.find(n => n['@type'] === 'Service');
  assert.ok(service, 'Service not injected on home');
  assert.equal(service.hasOfferCatalog.itemListElement.length, 4);
});

test('processHtml on article-page.html: Person author, FAQPage speakable, dateModified refreshed, BreadcrumbList unchanged', async () => {
  const html = await fx('article-page.html');
  const result = processHtml(html);
  const scripts = readScripts(result);

  const article = scripts.find(s => s['@type'] === 'Article');
  assert.equal(article.author['@type'], 'Person');
  assert.equal(article.author.name, 'TokenProvider Engineering Team');
  assert.equal(article.dateModified, new Date().toISOString().slice(0, 10));

  const faq = scripts.find(s => s['@type'] === 'FAQPage');
  assert.ok(faq.speakable);

  const breadcrumbs = scripts.filter(s => s['@type'] === 'BreadcrumbList');
  assert.equal(breadcrumbs.length, 1);
});

test('processHtml on techarticle-page.html: BreadcrumbList added (was missing)', async () => {
  const html = await fx('techarticle-page.html');
  const result = processHtml(html);
  const scripts = readScripts(result);

  const allTypes = scripts.flatMap(s =>
    Array.isArray(s['@graph']) ? s['@graph'].map(n => n['@type']) : [s['@type']]
  );
  assert.ok(allTypes.includes('BreadcrumbList'));
});

test('processHtml is idempotent: running twice produces identical output', async () => {
  const html = await fx('article-page.html');
  const once = processHtml(html);
  const twice = processHtml(once);
  assert.equal(once, twice);
});

test('processHtml preserves doctype and other inline scripts', () => {
  const html = `<!doctype html>
<html lang="en">
<head>
<link rel="canonical" href="https://tokenprovider.store/en/glossary/x.html" />
<title>X | TokenProvider</title>
<script type="application/javascript">var x=1;</script>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"Article","author":{"@type":"Organization","name":"X"},"datePublished":"2026-01-01","dateModified":"2026-01-01"}</script>
</head>
<body></body>
</html>`;
  const out = processHtml(html);
  assert.match(out, /^<!doctype html>/i);
  assert.match(out, /var x=1;/);
});

test('processHtml on zh-page.html uses ZH author name', async () => {
  const html = await fx('zh-page.html');
  const result = processHtml(html);
  const article = readScripts(result).find(s => s['@type'] === 'Article');
  assert.equal(article.author.name, 'TokenProvider 工程团队');
});
