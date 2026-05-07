import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CONFIG,
  ORG_SCHEMA,
  WEBSITE_SCHEMA,
  SERVICE_SCHEMA,
  PERSON_AUTHOR,
  SPEAKABLE
} from '../lib/schema-rules.mjs';

test('CONFIG.telegramUrl is the expected public URL', () => {
  assert.equal(CONFIG.telegramUrl, 'https://t.me/tokenprovider_official');
});

test('ORG_SCHEMA has stable @id and required entity fields', () => {
  assert.equal(ORG_SCHEMA['@id'], 'https://tokenprovider.store/#organization');
  assert.equal(ORG_SCHEMA['@type'], 'Organization');
  assert.deepEqual(ORG_SCHEMA.sameAs, ['https://t.me/tokenprovider_official']);
  assert.equal(ORG_SCHEMA.contactPoint['@type'], 'ContactPoint');
  assert.equal(ORG_SCHEMA.contactPoint.url, 'https://t.me/tokenprovider_official');
  assert.ok(Array.isArray(ORG_SCHEMA.knowsAbout));
  assert.ok(ORG_SCHEMA.knowsAbout.includes('Claude API'));
});

test('WEBSITE_SCHEMA references Organization @id (no SearchAction unless enabled)', () => {
  assert.equal(WEBSITE_SCHEMA.publisher['@id'], 'https://tokenprovider.store/#organization');
  assert.equal(WEBSITE_SCHEMA.potentialAction, undefined);
});

test('SERVICE_SCHEMA has 4 OfferCatalog items', () => {
  assert.equal(SERVICE_SCHEMA.hasOfferCatalog.itemListElement.length, 4);
  const names = SERVICE_SCHEMA.hasOfferCatalog.itemListElement.map(o => o.itemOffered.name);
  assert.ok(names.some(n => /Claude/.test(n)));
  assert.ok(names.some(n => /ChatGPT/.test(n)));
  assert.ok(names.some(n => /Gemini/.test(n)));
});

test('PERSON_AUTHOR returns en team name for lang=en', () => {
  const a = PERSON_AUTHOR('en');
  assert.equal(a['@type'], 'Person');
  assert.equal(a.name, 'TokenProvider Engineering Team');
  assert.equal(a.worksFor['@id'], 'https://tokenprovider.store/#organization');
});

test('PERSON_AUTHOR returns zh team name for lang=zh-CN', () => {
  const a = PERSON_AUTHOR('zh-CN');
  assert.equal(a.name, 'TokenProvider 工程团队');
});

test('SPEAKABLE includes seo-tldr selector', () => {
  assert.ok(SPEAKABLE.cssSelector.includes('.seo-tldr'));
});
