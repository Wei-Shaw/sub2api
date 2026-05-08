import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as cheerio from 'cheerio';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import {
  parseJsonLd,
  serializeJsonLd,
  findJsonLdScripts,
  replaceJsonLdInDom,
  inferLangFromHtml,
  inferBreadcrumbFromPage
} from '../lib/html-utils.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));

async function loadFixture(name) {
  return readFile(join(__dirname, 'fixtures', name), 'utf8');
}

test('parseJsonLd returns plain object', () => {
  const obj = parseJsonLd('{"@context":"https://schema.org","@type":"Article"}');
  assert.equal(obj['@type'], 'Article');
});

test('parseJsonLd throws clear error on bad JSON', () => {
  assert.throws(() => parseJsonLd('{not json}'), /JSON/);
});

test('serializeJsonLd produces 2-space indented JSON', () => {
  const out = serializeJsonLd({ '@type': 'X', name: 'y' });
  assert.match(out, /^{\n  "@type": "X",\n  "name": "y"\n}$/);
});

test('round-trip preserves data', () => {
  const original = { '@context': 'https://schema.org', '@graph': [{ '@type': 'Org', name: 'TP' }] };
  const restored = parseJsonLd(serializeJsonLd(original));
  assert.deepEqual(restored, original);
});

test('findJsonLdScripts returns each <script type="application/ld+json"> with parsed body', async () => {
  const html = await loadFixture('article-page.html');
  const $ = cheerio.load(html);
  const scripts = findJsonLdScripts($);
  assert.equal(scripts.length, 3);
  assert.equal(scripts[0].data['@type'], 'Article');
  assert.equal(scripts[1].data['@type'], 'FAQPage');
  assert.equal(scripts[2].data['@type'], 'BreadcrumbList');
});

test('findJsonLdScripts ignores non-JSON-LD scripts', () => {
  const $ = cheerio.load(`
    <script type="application/javascript">var x=1;</script>
    <script type="application/ld+json">{"@type":"Org"}</script>
  `);
  const scripts = findJsonLdScripts($);
  assert.equal(scripts.length, 1);
});

test('replaceJsonLdInDom updates the matching script tag content in place', async () => {
  const html = await loadFixture('article-page.html');
  const $ = cheerio.load(html);
  const scripts = findJsonLdScripts($);
  const articleEntry = scripts.find(s => s.data['@type'] === 'Article');
  articleEntry.data.headline = 'NEW';
  replaceJsonLdInDom($, articleEntry);

  const $$ = cheerio.load($.html());
  const reread = findJsonLdScripts($$);
  const article = reread.find(s => s.data['@type'] === 'Article');
  assert.equal(article.data.headline, 'NEW');
});

test('inferLangFromHtml reads <html lang="...">', () => {
  const $ = cheerio.load('<html lang="zh-CN"></html>');
  assert.equal(inferLangFromHtml($), 'zh-CN');
});

test('inferLangFromHtml defaults to "en" when missing', () => {
  const $ = cheerio.load('<html></html>');
  assert.equal(inferLangFromHtml($), 'en');
});

test('inferBreadcrumbFromPage builds 2-step list from canonical+title', () => {
  const $ = cheerio.load(`
    <link rel="canonical" href="https://tokenprovider.store/en/examples/test.html" />
    <title>Test Example | TokenProvider</title>
  `);
  const bc = inferBreadcrumbFromPage($, 'https://tokenprovider.store/en.html');
  assert.deepEqual(bc, {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      { '@type': 'ListItem', position: 1, name: 'TokenProvider', item: 'https://tokenprovider.store/en.html' },
      { '@type': 'ListItem', position: 2, name: 'Test Example', item: 'https://tokenprovider.store/en/examples/test.html' }
    ]
  });
});

test('inferBreadcrumbFromPage uses ZH home for lang=zh-CN', () => {
  const $ = cheerio.load(`
    <html lang="zh-CN">
    <link rel="canonical" href="https://tokenprovider.store/compare/test.html" />
    <title>测试对比 | TokenProvider</title>
    </html>
  `);
  const bc = inferBreadcrumbFromPage($, 'https://tokenprovider.store/');
  assert.equal(bc.itemListElement[0].item, 'https://tokenprovider.store/');
  assert.equal(bc.itemListElement[1].name, '测试对比');
});

