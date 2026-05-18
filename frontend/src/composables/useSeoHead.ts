import { watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSeoStore } from '@/stores/seo'

/**
 * Synchronize SEO <head> tags with the current route + locale.
 * Backend already injects full SEO on first byte; this composable keeps SPA
 * navigation consistent without reflowing the JSON-LD block (left alone).
 */
export function useSeoHead() {
  const route = useRoute()
  const i18n = useI18n()
  const seo = useSeoStore()

  watchEffect(() => {
    const rec = seo.resolve(route.path, i18n.locale.value)
    document.title = rec.title

    setMeta('description', rec.description)
    setMeta('keywords', (rec.keywords ?? []).join(','))
    setMeta('robots', rec.indexable ? 'index,follow' : 'noindex,follow')
    setMetaProperty('og:title', rec.title)
    setMetaProperty('og:description', rec.description)
    setMetaProperty('og:url', `${location.origin}${route.path}`)
    setLink('canonical', `${location.origin}${route.path}`)
  })
}

function setMeta(name: string, content: string) {
  if (!content) return
  let el = document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute('name', name)
    document.head.appendChild(el)
  }
  el.setAttribute('content', content)
}

function setMetaProperty(prop: string, content: string) {
  if (!content) return
  let el = document.querySelector<HTMLMetaElement>(`meta[property="${prop}"]`)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute('property', prop)
    document.head.appendChild(el)
  }
  el.setAttribute('content', content)
}

function setLink(rel: string, href: string) {
  if (!href) return
  let el = document.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`)
  if (!el) {
    el = document.createElement('link')
    el.setAttribute('rel', rel)
    document.head.appendChild(el)
  }
  el.setAttribute('href', href)
}
