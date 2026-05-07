import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  upgradeOrganization,
  upgradeWebSite,
  upgradeArticleAuthor,
  addSpeakableToFAQPage,
  injectServiceOnHomepages
} from '../lib/transforms.mjs';

test('upgradeOrganization replaces existing Org node by @id', () => {
  const data = {
    '@context': 'https://schema.org',
    '@graph': [
      { '@type': 'Organization', '@id': 'https://tokenprovider.store/#organization', name: 'OLD' },
      { '@type': 'WebSite', '@id': 'https://tokenprovider.store/#website' }
    ]
  };
  const out = upgradeOrganization(data);
  const org = out['@graph'].find(n => n['@type'] === 'Organization');
  assert.equal(org.name, 'TokenProvider');
  assert.deepEqual(org.sameAs, ['https://t.me/tokenprovider_official']);
});

test('upgradeOrganization appends when no Org node present', () => {
  const data = { '@context': 'https://schema.org', '@graph': [] };
  const out = upgradeOrganization(data);
  assert.equal(out['@graph'].length, 1);
  assert.equal(out['@graph'][0]['@type'], 'Organization');
});

test('upgradeOrganization wraps single-node schemas into @graph', () => {
  const data = { '@context': 'https://schema.org', '@type': 'Article', headline: 'X' };
  const out = upgradeOrganization(data);
  assert.ok(Array.isArray(out['@graph']));
  assert.ok(out['@graph'].some(n => n['@type'] === 'Article'));
  assert.ok(out['@graph'].some(n => n['@type'] === 'Organization'));
});

test('upgradeWebSite replaces existing WebSite node by @id', () => {
  const data = {
    '@context': 'https://schema.org',
    '@graph': [{ '@type': 'WebSite', '@id': 'https://tokenprovider.store/#website', name: 'OLD' }]
  };
  const out = upgradeWebSite(data);
  const ws = out['@graph'].find(n => n['@type'] === 'WebSite');
  assert.equal(ws.name, 'TokenProvider');
});

test('idempotent: applying twice yields equal result', () => {
  const start = { '@context': 'https://schema.org', '@graph': [] };
  const once = upgradeOrganization(upgradeWebSite(start));
  const twice = upgradeOrganization(upgradeWebSite(once));
  assert.deepEqual(once, twice);
});

test('upgradeArticleAuthor replaces Org author with Person on Article', () => {
  const data = {
    '@context': 'https://schema.org',
    '@type': 'Article',
    headline: 'X',
    author: { '@type': 'Organization', name: 'TokenProvider' }
  };
  const out = upgradeArticleAuthor(data, 'en');
  assert.equal(out.author['@type'], 'Person');
  assert.equal(out.author.name, 'TokenProvider Engineering Team');
});

test('upgradeArticleAuthor handles TechArticle and HowTo too', () => {
  for (const type of ['TechArticle', 'HowTo']) {
    const out = upgradeArticleAuthor({
      '@context': 'https://schema.org',
      '@type': type,
      author: { '@type': 'Organization', name: 'X' }
    }, 'en');
    assert.equal(out.author['@type'], 'Person', `failed for ${type}`);
  }
});

test('upgradeArticleAuthor uses zh team name when lang=zh-CN', () => {
  const out = upgradeArticleAuthor({
    '@context': 'https://schema.org',
    '@type': 'Article',
    author: { '@type': 'Organization', name: 'TokenProvider' }
  }, 'zh-CN');
  assert.equal(out.author.name, 'TokenProvider 工程团队');
});

test('upgradeArticleAuthor descends into @graph', () => {
  const data = {
    '@context': 'https://schema.org',
    '@graph': [
      { '@type': 'Article', author: { '@type': 'Organization', name: 'X' } },
      { '@type': 'FAQPage', mainEntity: [] }
    ]
  };
  const out = upgradeArticleAuthor(data, 'en');
  const art = out['@graph'].find(n => n['@type'] === 'Article');
  assert.equal(art.author['@type'], 'Person');
});

test('upgradeArticleAuthor leaves non-author types alone', () => {
  const data = { '@context': 'https://schema.org', '@type': 'Organization', name: 'TP' };
  const out = upgradeArticleAuthor(data, 'en');
  assert.equal(out.author, undefined);
});

test('upgradeArticleAuthor is idempotent (Person → Person)', () => {
  const once = upgradeArticleAuthor({
    '@context': 'https://schema.org',
    '@type': 'Article',
    author: { '@type': 'Organization', name: 'X' }
  }, 'en');
  const twice = upgradeArticleAuthor(once, 'en');
  assert.deepEqual(once, twice);
});

test('addSpeakableToFAQPage adds speakable to FAQPage in @graph', () => {
  const data = {
    '@context': 'https://schema.org',
    '@graph': [
      { '@type': 'FAQPage', mainEntity: [{ '@type': 'Question', name: 'Q' }] }
    ]
  };
  const out = addSpeakableToFAQPage(data);
  const faq = out['@graph'].find(n => n['@type'] === 'FAQPage');
  assert.equal(faq.speakable['@type'], 'SpeakableSpecification');
  assert.ok(faq.speakable.cssSelector.includes('.seo-tldr'));
});

test('addSpeakableToFAQPage handles single-node FAQPage', () => {
  const out = addSpeakableToFAQPage({
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: []
  });
  assert.ok(out.speakable);
});

test('addSpeakableToFAQPage idempotent', () => {
  const start = { '@context': 'https://schema.org', '@type': 'FAQPage', mainEntity: [] };
  const once = addSpeakableToFAQPage(start);
  const twice = addSpeakableToFAQPage(once);
  assert.deepEqual(once, twice);
});

test('injectServiceOnHomepages adds Service node when canonical is en.html', () => {
  const data = { '@context': 'https://schema.org', '@graph': [] };
  const out = injectServiceOnHomepages(data, 'https://tokenprovider.store/en.html');
  assert.ok(out['@graph'].some(n => n['@type'] === 'Service'));
});

test('injectServiceOnHomepages adds Service when canonical is the bare domain', () => {
  const out = injectServiceOnHomepages(
    { '@context': 'https://schema.org', '@graph': [] },
    'https://tokenprovider.store/'
  );
  assert.ok(out['@graph'].some(n => n['@type'] === 'Service'));
});

test('injectServiceOnHomepages skips non-home pages', () => {
  const out = injectServiceOnHomepages(
    { '@context': 'https://schema.org', '@graph': [] },
    'https://tokenprovider.store/en/compare/x.html'
  );
  assert.equal(out['@graph'].length, 0);
});
