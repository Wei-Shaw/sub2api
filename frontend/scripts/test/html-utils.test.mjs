import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseJsonLd, serializeJsonLd } from '../lib/html-utils.mjs';

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
