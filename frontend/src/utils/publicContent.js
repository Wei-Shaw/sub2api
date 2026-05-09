var __spreadArray = (this && this.__spreadArray) || function (to, from, pack) {
    if (pack || arguments.length === 2) for (var i = 0, l = from.length, ar; i < l; i++) {
        if (ar || !(i in from)) {
            if (!ar) ar = Array.prototype.slice.call(from, 0, i);
            ar[i] = from[i];
        }
    }
    return to.concat(ar || Array.prototype.slice.call(from));
};
import createDOMPurify from 'dompurify';
import { marked } from 'marked';
marked.setOptions({
    breaks: true,
    gfm: true,
});
var PUBLIC_CONTENT_ALLOWED_TAGS = [
    'p', 'br', 'strong', 'b', 'em', 'i', 'u', 's',
    'span', 'a', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'hr', 'img', 'div',
    'table', 'thead', 'tbody', 'tr', 'th', 'td',
];
var PUBLIC_CONTENT_ALLOWED_ATTR = ['href', 'target', 'rel', 'src', 'alt', 'title', 'style', 'class'];
var PUBLIC_CONTENT_SAFE_TEXT_ALIGN = new Set(['left', 'center', 'right', 'justify']);
var PUBLIC_CONTENT_SAFE_COLOR_NAMES = new Set([
    'black', 'white', 'red', 'blue', 'green', 'yellow', 'orange', 'purple',
    'gray', 'grey', 'teal', 'pink', 'brown',
]);
var rgbColorPattern = /^rgba?\(\s*(\d{1,3}\s*,\s*){2}\d{1,3}(\s*,\s*(0|0?\.\d+|1(\.0+)?)\s*)?\)$/;
var hslColorPattern = /^hsla?\(\s*\d{1,3}(\.\d+)?\s*,\s*\d{1,3}%\s*,\s*\d{1,3}%(\s*,\s*(0|0?\.\d+|1(\.0+)?)\s*)?\)$/;
var languageClassPattern = /^language-[a-z0-9_-]+$/i;
var browserPurifier = null;
function getBrowserPurifier() {
    if (browserPurifier) {
        return browserPurifier;
    }
    if (typeof window === 'undefined') {
        throw new Error('sanitizePublicHTML requires a browser-like window');
    }
    browserPurifier = createDOMPurify(window);
    return browserPurifier;
}
function sanitizeStyle(style) {
    var allowed = [];
    for (var _i = 0, _a = style.split(';'); _i < _a.length; _i++) {
        var part = _a[_i];
        var trimmed = part.trim();
        if (!trimmed) {
            continue;
        }
        var _b = trimmed.split(':', 2), rawKey = _b[0], rawValue = _b[1];
        if (!rawKey || !rawValue) {
            continue;
        }
        var key = rawKey.trim().toLowerCase();
        var value = rawValue.trim();
        switch (key) {
            case 'color':
            case 'background-color':
                if (isSafeCSSColor(value)) {
                    allowed.push("".concat(key, ": ").concat(value));
                }
                break;
            case 'text-align':
                if (PUBLIC_CONTENT_SAFE_TEXT_ALIGN.has(value.toLowerCase())) {
                    allowed.push("".concat(key, ": ").concat(value.toLowerCase()));
                }
                break;
            default:
                break;
        }
    }
    return allowed.join('; ');
}
function isSafeCSSColor(value) {
    var normalized = value.trim().toLowerCase();
    if (!normalized) {
        return false;
    }
    if (normalized.startsWith('#')) {
        var hex = normalized.slice(1);
        return (hex.length === 3 || hex.length === 6) && /^[0-9a-f]+$/i.test(hex);
    }
    return rgbColorPattern.test(normalized)
        || hslColorPattern.test(normalized)
        || PUBLIC_CONTENT_SAFE_COLOR_NAMES.has(normalized);
}
function isAllowedHref(raw) {
    var trimmed = raw.trim();
    if (!trimmed) {
        return false;
    }
    if (trimmed.startsWith('#') || (trimmed.startsWith('/') && !trimmed.startsWith('//'))) {
        return true;
    }
    try {
        var parsed = new URL(trimmed);
        return ['http:', 'https:', 'mailto:', 'tel:'].includes(parsed.protocol.toLowerCase());
    }
    catch (_a) {
        return false;
    }
}
function isAllowedImageSrc(raw) {
    var trimmed = raw.trim();
    if (!trimmed) {
        return false;
    }
    if (trimmed.startsWith('/') && !trimmed.startsWith('//')) {
        return true;
    }
    try {
        var parsed = new URL(trimmed);
        return ['http:', 'https:'].includes(parsed.protocol.toLowerCase());
    }
    catch (_a) {
        return false;
    }
}
function unwrapElement(element) {
    var parent = element.parentNode;
    if (!parent) {
        element.remove();
        return;
    }
    while (element.firstChild) {
        parent.insertBefore(element.firstChild, element);
    }
    parent.removeChild(element);
}
function keepOnlyAllowedAttrs(element, allowed) {
    for (var _i = 0, _a = Array.from(element.attributes); _i < _a.length; _i++) {
        var name_1 = _a[_i].name;
        if (!allowed.includes(name_1.toLowerCase())) {
            element.removeAttribute(name_1);
        }
    }
}
function postProcessSanitizedHTML(root) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _j;
    var elements = Array.from(root.querySelectorAll('*'));
    for (var _i = 0, elements_1 = elements; _i < elements_1.length; _i++) {
        var element = elements_1[_i];
        var tag = element.tagName.toLowerCase();
        switch (tag) {
            case 'a': {
                var href = (_b = (_a = element.getAttribute('href')) === null || _a === void 0 ? void 0 : _a.trim()) !== null && _b !== void 0 ? _b : '';
                if (!isAllowedHref(href)) {
                    unwrapElement(element);
                    break;
                }
                var target = (_c = element.getAttribute('target')) === null || _c === void 0 ? void 0 : _c.trim().toLowerCase();
                if (target !== '_blank' && target !== '_self') {
                    element.removeAttribute('target');
                }
                else {
                    element.setAttribute('target', target);
                }
                element.setAttribute('href', href);
                element.setAttribute('rel', 'noopener noreferrer nofollow');
                keepOnlyAllowedAttrs(element, ['href', 'target', 'rel']);
                break;
            }
            case 'img': {
                var src = (_e = (_d = element.getAttribute('src')) === null || _d === void 0 ? void 0 : _d.trim()) !== null && _e !== void 0 ? _e : '';
                if (!isAllowedImageSrc(src)) {
                    element.remove();
                    break;
                }
                element.setAttribute('src', src);
                keepOnlyAllowedAttrs(element, ['src', 'alt', 'title']);
                break;
            }
            case 'code': {
                var className = (_g = (_f = element.getAttribute('class')) === null || _f === void 0 ? void 0 : _f.trim()) !== null && _g !== void 0 ? _g : '';
                if (!languageClassPattern.test(className)) {
                    element.removeAttribute('class');
                }
                else {
                    element.setAttribute('class', className);
                }
                keepOnlyAllowedAttrs(element, ['class']);
                break;
            }
            case 'span':
            case 'p':
            case 'div':
            case 'blockquote':
            case 'h1':
            case 'h2':
            case 'h3':
            case 'h4':
            case 'h5':
            case 'h6': {
                var style = (_j = (_h = element.getAttribute('style')) === null || _h === void 0 ? void 0 : _h.trim()) !== null && _j !== void 0 ? _j : '';
                var sanitizedStyle = sanitizeStyle(style);
                if (sanitizedStyle) {
                    element.setAttribute('style', sanitizedStyle);
                }
                else {
                    element.removeAttribute('style');
                }
                keepOnlyAllowedAttrs(element, sanitizedStyle ? ['style'] : []);
                break;
            }
            default:
                keepOnlyAllowedAttrs(element, []);
                break;
        }
    }
}
export function sanitizePublicHTMLWithPurifier(raw, purifier, document) {
    var sanitized = purifier.sanitize(raw, {
        ALLOWED_TAGS: __spreadArray([], PUBLIC_CONTENT_ALLOWED_TAGS, true),
        ALLOWED_ATTR: __spreadArray([], PUBLIC_CONTENT_ALLOWED_ATTR, true),
        ALLOW_DATA_ATTR: false,
        FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'svg', 'math', 'form', 'input', 'button', 'video', 'audio', 'source', 'details'],
        FORBID_ATTR: ['srcset'],
    });
    var root = document.createElement('div');
    root.innerHTML = sanitized;
    postProcessSanitizedHTML(root);
    return root.innerHTML;
}
export function sanitizePublicHTML(raw) {
    return sanitizePublicHTMLWithPurifier(raw, getBrowserPurifier(), document);
}
export function renderPublicMarkdownWithPurifier(markdown, purifier, document) {
    var html = marked.parse(markdown);
    return sanitizePublicHTMLWithPurifier(html, purifier, document);
}
export function renderPublicMarkdown(markdown) {
    return renderPublicMarkdownWithPurifier(markdown, getBrowserPurifier(), document);
}
