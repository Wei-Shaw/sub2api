// Composable transforms: each takes (data, ctx?) → data. All are pure and idempotent.

import {
  ORG_SCHEMA,
  WEBSITE_SCHEMA,
  SERVICE_SCHEMA,
  SPEAKABLE,
  PERSON_AUTHOR,
  CONFIG
} from './schema-rules.mjs';

const AUTHOR_TYPES = new Set(['Article', 'TechArticle', 'NewsArticle', 'BlogPosting', 'HowTo']);
const DATED_TYPES = new Set(['Article', 'TechArticle', 'NewsArticle', 'BlogPosting']);

const HOMEPAGE_CANONICALS = new Set([
  'https://tokenprovider.store/',
  'https://tokenprovider.store/en.html'
]);

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

function applySpeakableOnNode(node) {
  if (node && node['@type'] === 'FAQPage' && !node.speakable) {
    return { ...node, speakable: structuredClone(SPEAKABLE) };
  }
  return node;
}

export function addSpeakableToFAQPage(data) {
  const out = structuredClone(data);
  if (Array.isArray(out['@graph'])) {
    out['@graph'] = out['@graph'].map(applySpeakableOnNode);
    return out;
  }
  return applySpeakableOnNode(out);
}

export function injectServiceOnHomepages(data, canonical) {
  if (!HOMEPAGE_CANONICALS.has(canonical)) return data;
  const out = structuredClone(data);
  if (!Array.isArray(out['@graph'])) {
    const { '@context': ctx, ...rest } = out;
    return {
      '@context': ctx || 'https://schema.org',
      '@graph': [rest, structuredClone(SERVICE_SCHEMA)]
    };
  }
  upsertById(out['@graph'], structuredClone(SERVICE_SCHEMA));
  return out;
}

export function addBreadcrumbIfMissing(data, breadcrumb) {
  const hasBreadcrumb =
    data['@type'] === 'BreadcrumbList' ||
    (Array.isArray(data['@graph']) && data['@graph'].some(n => n['@type'] === 'BreadcrumbList'));
  if (hasBreadcrumb) return data;
  const out = structuredClone(data);
  if (Array.isArray(out['@graph'])) {
    out['@graph'].push(structuredClone(breadcrumb));
    return out;
  }
  const { '@context': ctx, ...rest } = out;
  return {
    '@context': ctx || 'https://schema.org',
    '@graph': [rest, structuredClone(breadcrumb)]
  };
}

function applyDateOnNode(node) {
  if (node && node['@type'] && DATED_TYPES.has(node['@type']) && node.dateModified) {
    return { ...node, dateModified: CONFIG.todayIso };
  }
  return node;
}

export function refreshDateModified(data) {
  const out = structuredClone(data);
  if (Array.isArray(out['@graph'])) {
    out['@graph'] = out['@graph'].map(applyDateOnNode);
    return out;
  }
  return applyDateOnNode(out);
}
