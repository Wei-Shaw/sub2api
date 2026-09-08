import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const css = readFileSync(resolve(process.cwd(), 'src/styles/app.css'), 'utf8')

describe('implicit scrollbars', () => {
  it('renders the centered reference add control without a filled background', () => {
    expect(css).toMatch(/\.reference-add-core\s*\{[^}]*border:\s*1px dashed #94a3b8;[^}]*background:\s*transparent;[^}]*box-shadow:\s*none;/s)
  })

  it('keeps conversation scroll regions scrollable while revealing their thumbs on hover', () => {
    expect(css).toMatch(/\.history-list,\s*\.chat-thread\s*\{[^}]*scrollbar-width:\s*thin;[^}]*scrollbar-color:\s*transparent transparent;/s)
    expect(css).toMatch(/\.history-list:hover,\s*\.chat-thread:hover\s*\{[^}]*scrollbar-color:\s*rgba\(100,\s*116,\s*139,\s*\.45\) transparent;/s)
    expect(css).toMatch(/:is\(\.history-list,\s*\.chat-thread\)::\-webkit-scrollbar\s*\{[^}]*width:\s*6px;/s)
    expect(css).toMatch(/:is\(\.history-list,\s*\.chat-thread\):hover::\-webkit-scrollbar-thumb\s*\{[^}]*background:\s*rgba\(100,\s*116,\s*139,\s*\.45\);/s)
  })

  it('keeps the image preview centered and contained on mobile viewports', () => {
    expect(css).toMatch(/\.image-preview-backdrop\s*\{[^}]*display:\s*grid;[^}]*place-items:\s*center;/s)
    expect(css).toMatch(/\.image-preview-backdrop\s*\{[^}]*padding:\s*max\(16px,\s*env\(safe-area-inset-top\)\)\s+max\(16px,\s*env\(safe-area-inset-right\)\)\s+max\(16px,\s*env\(safe-area-inset-bottom\)\)\s+max\(16px,\s*env\(safe-area-inset-left\)\);/s)
    expect(css).toMatch(/\.image-preview-dialog\s*\{[^}]*width:\s*min\(100%,\s*1200px\);[^}]*max-height:\s*calc\(100dvh\s*-\s*32px\);/s)
    expect(css).toMatch(/\.image-preview-toolbar\s*\{[^}]*flex:\s*0\s+0\s+auto;/s)
    expect(css).toMatch(/\.image-preview-dialog\s*>\s*img\s*\{[^}]*margin:\s*auto;[^}]*max-width:\s*100%;[^}]*max-height:\s*100%;[^}]*object-fit:\s*contain;/s)
  })
  it('places the historical reference action beside a mobile-safe thumbnail', () => {
    expect(css).toMatch(/\.reference-item\s*\{[^}]*display:\s*flex;[^}]*align-items:\s*center;[^}]*gap:\s*8px;/s)
    expect(css).toMatch(/\.reference-item\s+\.reference-open\s*\{[^}]*flex:\s*0\s+1\s+80px;[^}]*min-width:\s*56px;/s)
    expect(css).toMatch(/\.reference-actions\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;[^}]*gap:\s*6px;/s)
    expect(css).toMatch(/\.reference-use-button\s*\{[^}]*height:\s*32px;[^}]*min-height:\s*32px;[^}]*padding:\s*0\s+10px;[^}]*border-radius:\s*8px;[^}]*font-size:\s*11px;/s)
  })

  it('keeps native generation selects in normal document flow', () => {
    expect(css).toMatch(/\.composer-select\s*\{[^}]*display:\s*inline-flex;[^}]*flex:\s*0\s+0\s+auto;/s)
    expect(css).not.toMatch(/\.composer-select select\s*\{[^}]*position:\s*absolute;/s)
  })

  it('wraps toolbar controls before their labels can be compressed', () => {
    expect(css).toMatch(/\.compression-control\s*\{[^}]*flex:\s*0\s+0\s+auto;/s)
    expect(css).toMatch(/\.preset-button\s*\{[^}]*flex:\s*0\s+0\s+auto;/s)
  })

  it('wraps long generation parameter summaries inside narrow message bubbles', () => {
    expect(css).toMatch(/\.request-settings span\s*\{[^}]*max-width:\s*100%;[^}]*white-space:\s*normal;[^}]*overflow-wrap:\s*anywhere;/s)
    expect(css).not.toMatch(/\.request-settings span\s*\{[^}]*white-space:\s*nowrap;/s)
  })
})
