// Composable transforms: each takes (data, ctx?) → data. All are pure and idempotent.

import * as cheerio from 'cheerio';
import {
  ORG_SCHEMA,
  WEBSITE_SCHEMA,
  SERVICE_SCHEMA,
  SPEAKABLE,
  PERSON_AUTHOR,
  CONFIG
} from './schema-rules.mjs';
import {
  findJsonLdScripts,
  replaceJsonLdInDom,
  inferLangFromHtml,
  inferBreadcrumbFromPage
} from './html-utils.mjs';

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

const ZH_HOME = 'https://tokenprovider.store/';
const EN_HOME = 'https://tokenprovider.store/en.html';

export function processHtml(html) {
  // Preserve leading doctype: cheerio.load can drop it on .html() output
  // depending on options/version; capture and re-prepend if needed.
  const doctypeMatch = html.match(/^\s*<!doctype[^>]*>/i);
  const doctype = doctypeMatch ? doctypeMatch[0] : '';

  const $ = cheerio.load(html, { xmlMode: false, decodeEntities: false });
  const lang = inferLangFromHtml($);
  const homeUrl = lang === 'zh-CN' ? ZH_HOME : EN_HOME;
  const canonical = $('link[rel="canonical"]').attr('href') || '';

  const scripts = findJsonLdScripts($);
  const isHomepage = canonical === ZH_HOME || canonical === EN_HOME;

  // Whether *any* script in the page already contains a BreadcrumbList,
  // either as the root or nested inside a @graph array.
  let breadcrumbPresent = scripts.some(s =>
    s.data['@type'] === 'BreadcrumbList' ||
    (Array.isArray(s.data['@graph']) && s.data['@graph'].some(n => n['@type'] === 'BreadcrumbList'))
  );

  // Org / WebSite / Service nodes belong on a single @graph script. Track
  // whether they have been planted so we don't duplicate when multiple
  // graph scripts exist.
  let orgPlanted = false;
  let webSitePlanted = false;
  let servicePlanted = false;

  for (const entry of scripts) {
    let next = entry.data;
    const isGraph = Array.isArray(next['@graph']);

    if (isGraph && !orgPlanted) {
      next = upgradeOrganization(next);
      orgPlanted = true;
    }
    if (isGraph && !webSitePlanted) {
      next = upgradeWebSite(next);
      webSitePlanted = true;
    }
    if (isHomepage && isGraph && !servicePlanted) {
      next = injectServiceOnHomepages(next, canonical);
      servicePlanted = true;
    }

    next = upgradeArticleAuthor(next, lang);
    next = addSpeakableToFAQPage(next);
    next = refreshDateModified(next);

    entry.data = next;
    replaceJsonLdInDom($, entry);
  }

  // BreadcrumbList backfill: only when missing AND the page is a content page
  // (not a homepage). Inline if there is a @graph script; otherwise append a
  // fresh JSON-LD block to <head>.
  if (!breadcrumbPresent && canonical && canonical !== ZH_HOME && canonical !== EN_HOME) {
    const inferred = inferBreadcrumbFromPage($, homeUrl);
    const firstGraph = scripts.find(s => Array.isArray(s.data['@graph']));
    if (firstGraph) {
      firstGraph.data = addBreadcrumbIfMissing(firstGraph.data, inferred);
      replaceJsonLdInDom($, firstGraph);
    } else {
      const serialized = JSON.stringify(inferred, null, 2);
      $('head').append(`\n    <script type="application/ld+json">\n    ${serialized}\n    </script>`);
    }
  }

  let out = $.html();
  if (doctype && !/^\s*<!doctype/i.test(out)) {
    out = `${doctype}\n${out}`;
  }
  return out;
}
