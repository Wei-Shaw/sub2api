package tutorialhtml

import (
	"html"
	"net/url"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedTags = map[string]bool{
	"p": true, "br": true, "strong": true, "b": true, "em": true, "i": true, "u": true, "s": true,
	"span": true, "a": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "blockquote": true, "pre": true, "code": true, "hr": true, "img": true, "div": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
}

var stripNodeTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
}

var (
	languageClassPattern = regexp.MustCompile(`^language-[a-z0-9_-]+$`)
	rgbColorPattern      = regexp.MustCompile(`^rgba?\(\s*(\d{1,3}\s*,\s*){2}\d{1,3}(\s*,\s*(0|0?\.\d+|1(\.0+)?))?\s*\)$`)
	hslColorPattern      = regexp.MustCompile(`^hsla?\(\s*\d{1,3}(\.\d+)?\s*,\s*\d{1,3}%\s*,\s*\d{1,3}%(\s*,\s*(0|0?\.\d+|1(\.0+)?))?\s*\)$`)
	htmlTagPattern       = regexp.MustCompile(`(?is)<[^>]+>`)
)

func RewriteRelativePageImageSources(rawHTML string, pageSlug string) string {
	pageSlug = strings.TrimSpace(pageSlug)
	if pageSlug == "" || strings.TrimSpace(rawHTML) == "" {
		return rawHTML
	}

	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rawHTML), context)
	if err != nil {
		return rawHTML
	}

	changed := false
	var walk func(node *xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && strings.EqualFold(strings.TrimSpace(node.Data), "img") {
			for i := range node.Attr {
				if !strings.EqualFold(strings.TrimSpace(node.Attr[i].Key), "src") {
					continue
				}
				src := strings.TrimSpace(node.Attr[i].Val)
				if isRelativePageImageSource(src) {
					node.Attr[i].Val = BuildPageImageURL(pageSlug, src)
					changed = true
				}
				break
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	for _, node := range nodes {
		walk(node)
	}
	if !changed {
		return rawHTML
	}

	var b strings.Builder
	for _, node := range nodes {
		if err := xhtml.Render(&b, node); err != nil {
			return rawHTML
		}
	}
	return b.String()
}

func BuildPageImageURL(pageSlug string, src string) string {
	trimmed := strings.TrimSpace(src)
	if decoded, err := url.PathUnescape(trimmed); err == nil {
		trimmed = decoded
	}
	pathPart := trimmed
	suffix := ""
	if idx := strings.IndexAny(trimmed, "?#"); idx >= 0 {
		pathPart = trimmed[:idx]
		suffix = trimmed[idx:]
	}

	encodedParts := make([]string, 0, len(strings.Split(pathPart, "/")))
	for _, part := range strings.Split(pathPart, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		encodedParts = append(encodedParts, url.PathEscape(part))
	}

	return "/api/v1/pages/" + url.PathEscape(strings.TrimSpace(pageSlug)) + "/images/" + strings.Join(encodedParts, "/") + suffix
}

func isRelativePageImageSource(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return false
	}
	if strings.Contains(trimmed, `\`) {
		return false
	}
	if parsed, err := url.Parse(trimmed); err == nil && strings.TrimSpace(parsed.Scheme) != "" {
		return false
	}
	for _, part := range strings.Split(trimmed, "/") {
		part = strings.TrimSpace(part)
		if part == ".." {
			return false
		}
	}
	return true
}

func SanitizeTutorialHTML(raw string) string {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), context)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, node := range nodes {
		b.WriteString(sanitizeNodeString(node))
	}
	return b.String()
}

func sanitizeNodeString(node *xhtml.Node) string {
	switch node.Type {
	case xhtml.DocumentNode:
		return sanitizeChildren(node)
	case xhtml.TextNode:
		return html.EscapeString(node.Data)
	case xhtml.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		if tag == "" {
			return ""
		}
		if stripNodeTags[tag] {
			return ""
		}
		if !allowedTags[tag] {
			return sanitizeChildren(node)
		}

		attrs := sanitizeAttrs(tag, node.Attr)
		if tag == "img" {
			if !hasAttr(attrs, "src") {
				return ""
			}
			return renderStartTag(tag, attrs)
		}
		if tag == "br" || tag == "hr" {
			return renderStartTag(tag, attrs)
		}

		childHTML := sanitizeChildren(node)
		if tag == "a" && !hasAttr(attrs, "href") {
			return childHTML
		}
		if shouldDropEmptyElement(tag, childHTML) {
			return ""
		}
		return renderElement(tag, attrs, childHTML)
	}
	return ""
}

func sanitizeChildren(node *xhtml.Node) string {
	var b strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(sanitizeNodeString(child))
	}
	return b.String()
}

func renderStartTag(tag string, attrs []xhtml.Attribute) string {
	var b strings.Builder
	b.WriteString("<")
	b.WriteString(tag)
	for _, attr := range attrs {
		b.WriteString(" ")
		b.WriteString(attr.Key)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(attr.Val))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	return b.String()
}

func renderElement(tag string, attrs []xhtml.Attribute, childHTML string) string {
	var b strings.Builder
	b.WriteString(renderStartTag(tag, attrs))
	b.WriteString(childHTML)
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">")
	return b.String()
}

func hasAttr(attrs []xhtml.Attribute, key string) bool {
	for _, attr := range attrs {
		if strings.EqualFold(strings.TrimSpace(attr.Key), key) && strings.TrimSpace(attr.Val) != "" {
			return true
		}
	}
	return false
}

func shouldDropEmptyElement(tag, childHTML string) bool {
	switch tag {
	case "a", "p", "span", "div", "blockquote", "li", "h1", "h2", "h3", "h4", "h5", "h6", "pre", "code", "ul", "ol", "table", "thead", "tbody", "tr", "th", "td":
		return !hasMeaningfulContent(childHTML)
	default:
		return false
	}
}

func hasMeaningfulContent(content string) bool {
	plain := strings.TrimSpace(html.UnescapeString(htmlTagPattern.ReplaceAllString(content, " ")))
	if plain != "" {
		return true
	}
	return strings.Contains(content, "<img") || strings.Contains(content, "<br") || strings.Contains(content, "<hr")
}

func sanitizeAttrs(tag string, attrs []xhtml.Attribute) []xhtml.Attribute {
	out := make([]xhtml.Attribute, 0, len(attrs))
	linkHref := ""
	linkTarget := ""
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		val := strings.TrimSpace(attr.Val)
		if key == "" || val == "" {
			continue
		}
		switch tag {
		case "a":
			if key == "href" && isAllowedHref(val) {
				linkHref = val
			}
			if key == "target" && (val == "_blank" || val == "_self") {
				linkTarget = val
			}
		case "img":
			if key == "src" && isAllowedImageSrc(val) {
				out = append(out, xhtml.Attribute{Key: "src", Val: val})
			}
			if key == "alt" || key == "title" {
				out = append(out, xhtml.Attribute{Key: key, Val: val})
			}
		case "span", "p", "div", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
			if key == "style" {
				if safe := sanitizeStyle(val); safe != "" {
					out = append(out, xhtml.Attribute{Key: "style", Val: safe})
				}
			}
		case "code":
			if key == "class" && languageClassPattern.MatchString(strings.ToLower(val)) {
				out = append(out, xhtml.Attribute{Key: "class", Val: val})
			}
		}
	}
	if tag == "a" {
		if linkHref != "" {
			out = append(out, xhtml.Attribute{Key: "href", Val: linkHref})
		}
		if linkTarget != "" {
			out = append(out, xhtml.Attribute{Key: "target", Val: linkTarget})
		}
		if linkHref != "" || linkTarget != "" {
			out = append(out, xhtml.Attribute{Key: "rel", Val: "noopener noreferrer nofollow"})
		}
	}
	return out
}

func sanitizeStyle(style string) string {
	parts := strings.Split(style, ";")
	allowed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		switch key {
		case "color", "background-color":
			if isSafeCSSColor(val) {
				allowed = append(allowed, key+": "+val)
			}
		case "text-align":
			switch strings.ToLower(val) {
			case "left", "center", "right", "justify":
				allowed = append(allowed, key+": "+strings.ToLower(val))
			}
		}
	}
	return strings.Join(allowed, "; ")
}

func isSafeCSSColor(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		if len(hex) != 3 && len(hex) != 6 {
			return false
		}
		for _, ch := range hex {
			if !strings.ContainsRune("0123456789abcdef", ch) {
				return false
			}
		}
		return true
	}
	if rgbColorPattern.MatchString(value) || hslColorPattern.MatchString(value) {
		return true
	}
	switch value {
	case "black", "white", "red", "blue", "green", "yellow", "orange", "purple", "gray", "grey", "teal", "pink", "brown":
		return true
	default:
		return false
	}
}

func isAllowedHref(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "/") {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "tel":
		return true
	default:
		return false
	}
}

func isAllowedImageSrc(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "/api/v1/pages/tutorial/images/") || strings.HasPrefix(raw, "/") {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}
