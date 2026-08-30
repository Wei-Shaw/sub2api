import type { PublicSettings } from '@/types'

/** Render the supported public site settings in custom HTML home content. */
export function renderHomeTemplate(
  content: string,
  settings: PublicSettings | null | undefined,
): string {
  if (!content || !settings) return content

  const values: Record<string, string> = {
    site_name: settings.site_name || '',
    site_logo: settings.site_logo || '',
    site_subtitle: settings.site_subtitle || '',
    api_base_url: settings.api_base_url || '',
    contact_info: settings.contact_info || '',
    doc_url: settings.doc_url || '',
    version: settings.version || '',
  }

  return content.replace(
    /{{\s*(site_name|site_logo|site_subtitle|api_base_url|contact_info|doc_url|version)\s*}}/g,
    (_, key: string) => escapeHtml(values[key] ?? ''),
  )
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[character] || character)
}
