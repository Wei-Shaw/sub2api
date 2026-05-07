// Composable transforms: each takes (data, ctx?) → data. All are pure and idempotent.

import { ORG_SCHEMA, WEBSITE_SCHEMA, PERSON_AUTHOR } from './schema-rules.mjs';

const AUTHOR_TYPES = new Set(['Article', 'TechArticle', 'NewsArticle', 'BlogPosting', 'HowTo']);

function ensureGraph(data) {
  if (Array.isArray(data['@graph'])) return data;
  const { '@context': ctx, ...rest } = data;
  return { '@context': ctx || 'https://schema.org', '@graph': [rest] };
}

export function upsertById(graph, node) {
  const idx = graph.findIndex(n => n['@id'] === node['@id']);
  if (idx >= 0) {
    graph[idx] = node;
  } else {
    graph.push(node);
  }
  return graph;
}

export function upgradeOrganization(data) {
  const out = ensureGraph(structuredClone(data));
  upsertById(out['@graph'], structuredClone(ORG_SCHEMA));
  return out;
}

export function upgradeWebSite(data) {
  const out = ensureGraph(structuredClone(data));
  upsertById(out['@graph'], structuredClone(WEBSITE_SCHEMA));
  return out;
}

function applyAuthorOnNode(node, lang) {
  if (node && node['@type'] && AUTHOR_TYPES.has(node['@type']) && node.author) {
    return { ...node, author: PERSON_AUTHOR(lang) };
  }
  return node;
}

export function upgradeArticleAuthor(data, lang) {
  const out = structuredClone(data);
  if (Array.isArray(out['@graph'])) {
    out['@graph'] = out['@graph'].map(n => applyAuthorOnNode(n, lang));
    return out;
  }
  return applyAuthorOnNode(out, lang);
}
