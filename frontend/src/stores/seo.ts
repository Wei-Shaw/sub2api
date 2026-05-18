import { defineStore } from 'pinia'
import seoData from '@shared/seo/seo.json'

export interface LangPayload {
  title: string
  description: string
  keywords?: string[]
  ogImage?: string
}

interface RouteRecord {
  indexable: boolean
  priority?: number
  changefreq?: string
  dynamic?: string
  jsonLd?: string[]
  zh: LangPayload
  en: LangPayload
}

interface SeoData {
  site: { name: string; defaultLang: 'zh' | 'en'; supportedLangs: string[] }
  routes: Record<string, RouteRecord>
}

export interface ResolvedSeo extends LangPayload {
  indexable: boolean
}

function matchPattern(pattern: string, path: string): boolean {
  const pp = pattern.split('/')
  const xp = path.split('/')
  if (pp.length !== xp.length) return false
  for (let i = 0; i < pp.length; i++) {
    if (pp[i].startsWith(':')) {
      if (!xp[i]) return false
      continue
    }
    if (pp[i] !== xp[i]) return false
  }
  return true
}

export const useSeoStore = defineStore('seo', () => {
  const data = seoData as unknown as SeoData

  function resolve(path: string, lang: string): ResolvedSeo {
    const normalizedLang = (data.site.supportedLangs.includes(lang)
      ? lang
      : data.site.defaultLang) as 'zh' | 'en'

    let rec = data.routes[path]
    if (!rec) {
      for (const [pat, r] of Object.entries(data.routes)) {
        if (pat.includes(':') && matchPattern(pat, path)) {
          rec = r
          break
        }
      }
    }

    if (!rec) {
      return {
        title: data.site.name,
        description: data.site.name,
        indexable: false
      }
    }
    const lp = rec[normalizedLang]
    return { ...lp, indexable: rec.indexable }
  }

  return { resolve }
})
