import createDOMPurify from 'dompurify';
import { JSDOM } from 'jsdom';
export var BASE_PUBLIC_PRERENDER_ROUTES = [
    '/home',
    '/docs/tutorial',
];
var prerenderWindow = new JSDOM('').window;
var prerenderDOMPurify = createDOMPurify(prerenderWindow);
function escapeHTML(value) {
    return value
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}
function sanitizePrerenderHTML(raw) {
    return prerenderDOMPurify.sanitize(raw, {
        ADD_ATTR: ['style', 'target', 'rel', 'class'],
    });
}
export function collectPrerenderRoutes(payload) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _j, _k;
    var routes = new Map();
    for (var _i = 0, BASE_PUBLIC_PRERENDER_ROUTES_1 = BASE_PUBLIC_PRERENDER_ROUTES; _i < BASE_PUBLIC_PRERENDER_ROUTES_1.length; _i++) {
        var route = BASE_PUBLIC_PRERENDER_ROUTES_1[_i];
        routes.set(route, { route: route, title: '', source: 'base' });
    }
    var data = payload === null || payload === void 0 ? void 0 : payload.data;
    routes.set('/docs/tutorial', {
        route: '/docs/tutorial',
        title: '教程文档',
        markdownSlug: 'tutorial',
        source: 'tutorial',
    });
    for (var _l = 0, _m = (_a = data === null || data === void 0 ? void 0 : data.login_agreement_documents) !== null && _a !== void 0 ? _a : []; _l < _m.length; _l++) {
        var item = _m[_l];
        var id = String((_b = item === null || item === void 0 ? void 0 : item.id) !== null && _b !== void 0 ? _b : '').trim();
        if (id) {
            routes.set("/legal/".concat(id), {
                route: "/legal/".concat(id),
                title: String((_c = item === null || item === void 0 ? void 0 : item.title) !== null && _c !== void 0 ? _c : '').trim(),
                markdown: String((_d = item === null || item === void 0 ? void 0 : item.content_md) !== null && _d !== void 0 ? _d : ''),
                source: 'legal',
            });
        }
    }
    for (var _o = 0, _p = (_e = data === null || data === void 0 ? void 0 : data.custom_menu_items) !== null && _e !== void 0 ? _e : []; _o < _p.length; _o++) {
        var item = _p[_o];
        var id = String((_f = item === null || item === void 0 ? void 0 : item.id) !== null && _f !== void 0 ? _f : '').trim();
        var label = String((_g = item === null || item === void 0 ? void 0 : item.label) !== null && _g !== void 0 ? _g : '').trim();
        var visibility = String((_h = item === null || item === void 0 ? void 0 : item.visibility) !== null && _h !== void 0 ? _h : '').trim();
        var pageSlug = String((_j = item === null || item === void 0 ? void 0 : item.page_slug) !== null && _j !== void 0 ? _j : '').trim();
        var url = String((_k = item === null || item === void 0 ? void 0 : item.url) !== null && _k !== void 0 ? _k : '').trim();
        var isMarkdown = Boolean(pageSlug || url.startsWith('md:'));
        if (id && visibility !== 'admin' && isMarkdown) {
            routes.set("/custom/".concat(id), {
                route: "/custom/".concat(id),
                title: label,
                markdownSlug: pageSlug || url.replace(/^md:/, '').trim(),
                source: 'custom-markdown',
            });
        }
    }
    return Array.from(routes.values()).sort(function (a, b) { return a.route.localeCompare(b.route); });
}
export function renderSimpleMarkdownHTML(markdown) {
    var lines = markdown.replace(/\r\n/g, '\n').split('\n');
    var html = [];
    var inList = false;
    var inOrderedList = false;
    var inCodeBlock = false;
    var paragraph = [];
    var inline = function (value) {
        return escapeHTML(value)
            .replace(/`([^`]+)`/g, '<code>$1</code>')
            .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
            .replace(/\*([^*]+)\*/g, '<em>$1</em>')
            .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">')
            .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');
    };
    var flushParagraph = function () {
        if (paragraph.length > 0) {
            html.push("<p>".concat(inline(paragraph.join(' ')), "</p>"));
            paragraph = [];
        }
    };
    var closeLists = function () {
        if (inList) {
            html.push('</ul>');
            inList = false;
        }
        if (inOrderedList) {
            html.push('</ol>');
            inOrderedList = false;
        }
    };
    for (var _i = 0, lines_1 = lines; _i < lines_1.length; _i++) {
        var rawLine = lines_1[_i];
        var line = rawLine.trimEnd();
        var trimmed = line.trim();
        if (trimmed.startsWith('```')) {
            flushParagraph();
            closeLists();
            html.push(inCodeBlock ? '</code></pre>' : '<pre><code>');
            inCodeBlock = !inCodeBlock;
            continue;
        }
        if (inCodeBlock) {
            html.push(line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'));
            continue;
        }
        if (!trimmed) {
            flushParagraph();
            closeLists();
            continue;
        }
        var heading = trimmed.match(/^(#{1,4})\s+(.+)$/);
        if (heading) {
            flushParagraph();
            closeLists();
            var level = heading[1].length;
            html.push("<h".concat(level, ">").concat(inline(heading[2]), "</h").concat(level, ">"));
            continue;
        }
        if (/^[-*]\s+/.test(trimmed)) {
            flushParagraph();
            if (inOrderedList) {
                html.push('</ol>');
                inOrderedList = false;
            }
            if (!inList) {
                html.push('<ul>');
                inList = true;
            }
            html.push("<li>".concat(inline(trimmed.replace(/^[-*]\s+/, '')), "</li>"));
            continue;
        }
        if (/^\d+\.\s+/.test(trimmed)) {
            flushParagraph();
            if (inList) {
                html.push('</ul>');
                inList = false;
            }
            if (!inOrderedList) {
                html.push('<ol>');
                inOrderedList = true;
            }
            html.push("<li>".concat(inline(trimmed.replace(/^\d+\.\s+/, '')), "</li>"));
            continue;
        }
        paragraph.push(trimmed);
    }
    flushParagraph();
    closeLists();
    if (inCodeBlock) {
        html.push('</code></pre>');
    }
    return html.join('\n');
}
export function injectPrerenderContent(indexHTML, entry) {
    if (!entry.markdown && !entry.html) {
        return indexHTML;
    }
    var safeTitle = entry.title || 'Document';
    var contentHTML = sanitizePrerenderHTML(entry.html || renderSimpleMarkdownHTML(entry.markdown || ''));
    var body = "\n  <div class=\"min-h-screen bg-gray-50 text-gray-900\">\n    <main class=\"mx-auto max-w-6xl px-4 py-8 sm:px-6 lg:py-10\">\n      <article class=\"overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm\">\n        <div class=\"border-b border-gray-200 px-6 py-5\">\n          <h1 class=\"mt-2 text-3xl font-bold text-gray-950\">".concat(escapeHTML(safeTitle), "</h1>\n        </div>\n        <div class=\"public-markdown-content p-6 md:p-10\">").concat(contentHTML, "</div>\n      </article>\n    </main>\n  </div>");
    return indexHTML.replace('<div id="app"></div>', "<div id=\"app\">".concat(body, "</div>"));
}
export function buildPrerenderManifest(entries) {
    return {
        generated_at: new Date().toISOString(),
        total_routes: entries.length,
        routes: entries.map(function (entry) { return ({
            route: entry.route,
            title: entry.title,
            source: entry.source || 'base',
            has_markdown: Boolean(entry.markdown),
            has_html: Boolean(entry.html),
            markdown_slug: entry.markdownSlug || '',
            output: "".concat(entry.route.replace(/^\/+/, '') || 'index.html', "/index.html"),
        }); }),
    };
}
