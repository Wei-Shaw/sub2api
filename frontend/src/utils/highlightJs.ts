/**
 * Lightweight JS syntax highlighter for admin script preview.
 * Escapes HTML then wraps tokens in span classes (no external deps).
 */
const JS_KEYWORDS = new Set([
  'break', 'case', 'catch', 'class', 'const', 'continue', 'debugger', 'default',
  'delete', 'do', 'else', 'export', 'extends', 'false', 'finally', 'for',
  'function', 'if', 'import', 'in', 'instanceof', 'let', 'new', 'null',
  'return', 'super', 'switch', 'this', 'throw', 'true', 'try', 'typeof',
  'undefined', 'var', 'void', 'while', 'with', 'yield', 'async', 'await',
  'of', 'static', 'get', 'set',
])

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function span(cls: string, text: string): string {
  return `<span class="${cls}">${text}</span>`
}

/**
 * Returns HTML with syntax spans. Input is plain JS source (not HTML).
 */
export function highlightJavaScript(source: string): string {
  if (!source) return ''
  const s = source
  let i = 0
  let out = ''
  const n = s.length

  while (i < n) {
    const ch = s[i]

    // line comment
    if (ch === '/' && s[i + 1] === '/') {
      let j = i + 2
      while (j < n && s[j] !== '\n') j++
      out += span('tok-comment', escapeHtml(s.slice(i, j)))
      i = j
      continue
    }

    // block comment
    if (ch === '/' && s[i + 1] === '*') {
      let j = i + 2
      while (j < n && !(s[j] === '*' && s[j + 1] === '/')) j++
      if (j < n) j += 2
      out += span('tok-comment', escapeHtml(s.slice(i, j)))
      i = j
      continue
    }

    // string
    if (ch === '"' || ch === "'" || ch === '`') {
      const quote = ch
      let j = i + 1
      while (j < n) {
        if (s[j] === '\\') {
          j += 2
          continue
        }
        if (s[j] === quote) {
          j++
          break
        }
        // template interpolation: keep simple (stop at unescaped ${ only for nesting skip)
        j++
      }
      out += span('tok-string', escapeHtml(s.slice(i, j)))
      i = j
      continue
    }

    // regex (heuristic: after = ( [ , : ! & | ? { ; or start / newline whitespace)
    if (ch === '/') {
      const prev = out.replace(/<[^>]+>/g, '').replace(/\s+$/, '')
      const last = prev.slice(-1)
      const canRegex =
        prev === '' ||
        /[=([{,;:!&|?+\-~*%^<>]/.test(last) ||
        prev.endsWith('return') ||
        prev.endsWith('typeof') ||
        prev.endsWith('case')
      if (canRegex) {
        let j = i + 1
        let closed = false
        while (j < n) {
          if (s[j] === '\\') {
            j += 2
            continue
          }
          if (s[j] === '\n') break
          if (s[j] === '/') {
            j++
            closed = true
            break
          }
          j++
        }
        if (closed) {
          while (j < n && /[gimsuy]/.test(s[j])) j++
          out += span('tok-regex', escapeHtml(s.slice(i, j)))
          i = j
          continue
        }
      }
    }

    // number
    if (/[0-9]/.test(ch) || (ch === '.' && /[0-9]/.test(s[i + 1] || ''))) {
      let j = i
      if (s[j] === '0' && (s[j + 1] === 'x' || s[j + 1] === 'X')) {
        j += 2
        while (j < n && /[0-9a-fA-F_]/.test(s[j])) j++
      } else {
        while (j < n && /[0-9_]/.test(s[j])) j++
        if (s[j] === '.') {
          j++
          while (j < n && /[0-9_]/.test(s[j])) j++
        }
        if (s[j] === 'e' || s[j] === 'E') {
          j++
          if (s[j] === '+' || s[j] === '-') j++
          while (j < n && /[0-9_]/.test(s[j])) j++
        }
      }
      out += span('tok-number', escapeHtml(s.slice(i, j)))
      i = j
      continue
    }

    // identifier / keyword
    if (/[A-Za-z_$]/.test(ch)) {
      let j = i + 1
      while (j < n && /[A-Za-z0-9_$]/.test(s[j])) j++
      const word = s.slice(i, j)
      if (JS_KEYWORDS.has(word)) {
        out += span('tok-keyword', escapeHtml(word))
      } else {
        // function name: identifier followed by (
        let k = j
        while (k < n && /\s/.test(s[k])) k++
        if (s[k] === '(') {
          out += span('tok-function', escapeHtml(word))
        } else {
          out += span('tok-ident', escapeHtml(word))
        }
      }
      i = j
      continue
    }

    // operators / punctuation
    if (/[{}()[\];,.]/.test(ch)) {
      out += span('tok-punct', escapeHtml(ch))
      i++
      continue
    }
    if (/[+\-*/%<>=!&|^~?:]/.test(ch)) {
      let j = i + 1
      while (j < n && /[+\-*/%<>=!&|^~?:]/.test(s[j]) && j - i < 3) j++
      out += span('tok-operator', escapeHtml(s.slice(i, j)))
      i = j
      continue
    }

    out += escapeHtml(ch)
    i++
  }

  return out
}

export default highlightJavaScript
