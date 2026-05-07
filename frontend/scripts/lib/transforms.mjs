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

const JSONLD_BLOCK_RE = /<script\s+type="application\/ld\+json"\s*>([\s\S]*?)<\/script>/g;

// Re-serialize a JSON-LD object so that it matches the indent style used by
// the existing static pages: each line of the JSON gets 4 leading spaces so
// the block lines up with the opening <script> tag.
function reserializeJsonLd(data) {
  const json = JSON.stringify(data, null, 2);
  return json.split('\n').map(line => '    ' + line).join('\n');
}

export function processHtml(html) {
  const $ = cheerio.load(html, { xmlMode: false, decodeEntities: false });
  const lang = inferLangFromHtml($);
  const homeUrl = lang === 'zh-CN' ? ZH_HOME : EN_HOME;
  const canonical = $('link[rel="canonical"]').attr('href') || '';
  const isHomepage = canonical === ZH_HOME || canonical === EN_HOME;

  // Locate every JSON-LD block in the original source string. We only ever
  // mutate the inside of these blocks; the surrounding HTML is preserved
  // byte-for-byte (no cheerio re-serialization, no doctype rewrites, no
  // self-closing tag mangling).
  const matches = [...html.matchAll(JSONLD_BLOCK_RE)];
  const entries = matches.map(m => ({
    full: m[0],
    inner: m[1],
    data: JSON.parse(m[1])
  }));

  let breadcrumbPresent = entries.some(e =>
    e.data['@type'] === 'BreadcrumbList' ||
    (Array.isArray(e.data['@graph']) && e.data['@graph'].some(n => n['@type'] === 'BreadcrumbList'))
  );

  let orgPlanted = false;
  let webSitePlanted = false;
  let servicePlanted = false;

  for (const entry of entries) {
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
  }

  // Splice updated JSON-LD blocks back into the original HTML string.
  let out = html;
  for (const entry of entries) {
    const newInner = `\n${reserializeJsonLd(entry.data)}\n    `;
    const newBlock = `<script type="application/ld+json">${newInner}</script>`;
    // Replace the first occurrence of the original block in `out`. Using
    // split/join avoids regex special-character pitfalls in the matched text.
    out = out.replace(entry.full, newBlock);
  }

  // BreadcrumbList backfill: only when missing AND the page is a content page.
  if (!breadcrumbPresent && canonical && !isHomepage) {
    const inferred = inferBreadcrumbFromPage($, homeUrl);
    const newBlock = `    <script type="application/ld+json">\n${reserializeJsonLd(inferred)}\n    </script>\n`;
    // Insert immediately before </head>, preserving the existing indent of </head>.
    out = out.replace(/(\s*)<\/head>/, `\n${newBlock}$1</head>`);
  }

  return out;
}
