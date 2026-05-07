// Plumbing: parse, locate, serialize JSON-LD inside HTML via cheerio.

export function parseJsonLd(text) {
  try {
    return JSON.parse(text);
  } catch (e) {
    throw new Error(`Invalid JSON in JSON-LD block: ${e.message}`);
  }
}

export function serializeJsonLd(obj) {
  return JSON.stringify(obj, null, 2);
}

export function findJsonLdScripts($) {
  const out = [];
  $('script[type="application/ld+json"]').each((i, el) => {
    const $el = $(el);
    const text = $el.contents().text();
    out.push({ index: i, $el, data: parseJsonLd(text) });
  });
  return out;
}

export function replaceJsonLdInDom($, entry) {
  const next = serializeJsonLd(entry.data);
  // Reset the element's content to a freshly serialized JSON block. Keep the
  // surrounding whitespace pattern that the static pages already use:
  // <script type="application/ld+json">\n    {...}\n    </script>
  entry.$el.empty();
  entry.$el.append(`\n    ${next}\n    `);
}

export function inferLangFromHtml($) {
  return $('html').attr('lang') || 'en';
}

export function inferBreadcrumbFromPage($, homeUrl) {
  const canonical = $('link[rel="canonical"]').attr('href');
  const title = $('title').text();
  const pageName = title.split('|')[0].trim();
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      { '@type': 'ListItem', position: 1, name: 'TokenProvider', item: homeUrl },
      { '@type': 'ListItem', position: 2, name: pageName, item: canonical }
    ]
  };
}
