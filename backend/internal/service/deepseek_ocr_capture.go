package service

// The OCR result has at most 1,000 pages. For billing we only need its JSON
// structure, page_count, and the object discriminator. Drop long strings (page
// text) and coalesce whitespace in the private copy. The client gets the
// original bytes. Keep enough bytes for even a fully escaped "page_count" or
// "ocr.result". Final envelope validation and unit calculation stay in
// extractDeepseekToolsUsage.
type deepseekOCRBillingCapture struct {
	body                          []byte
	stringStart                   int
	inString, longString, invalid bool
	escape                        bool
	hexLeft                       int
}

func (c *deepseekOCRBillingCapture) Write(p []byte) (int, error) {
	const maxString = 128
	const maxMetadata = 1 << 20
	if c.invalid {
		return len(p), nil
	}
	for _, b := range p {
		if c.inString {
			// Validate escapes and control characters even in discarded text.
			switch {
			case c.hexLeft > 0:
				if (b < '0' || b > '9') && (b < 'a' || b > 'f') && (b < 'A' || b > 'F') {
					c.invalid = true
				}
				c.hexLeft--
			case c.escape:
				c.escape = false
				switch b {
				case 'u':
					c.hexLeft = 4
				case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				default:
					c.invalid = true
				}
			case b == '\\':
				c.escape = true
			case b == '"':
				c.inString = false
			case b < 0x20:
				c.invalid = true
			}
			if c.inString && !c.longString && len(c.body)-c.stringStart >= maxString {
				c.body = c.body[:c.stringStart]
				c.longString = true
			}
			if !c.inString || !c.longString {
				c.body = append(c.body, b)
			}
		} else {
			switch b {
			case ' ', '\n', '\r', '\t':
				if len(c.body) == 0 || c.body[len(c.body)-1] == ' ' {
					continue
				}
				b = ' '
			case '"':
				c.inString = true
				c.longString = false
				c.stringStart = len(c.body) + 1
			}
			c.body = append(c.body, b)
		}
		if c.invalid || len(c.body) > maxMetadata {
			c.invalid = true
			c.body = nil
			break
		}
	}
	return len(p), nil
}
