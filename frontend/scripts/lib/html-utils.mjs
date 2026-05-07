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
