// Pure data: schema constants and config used by the GEO codemod.

export const CONFIG = {
  baseUrl: 'https://tokenprovider.store',
  telegramUrl: 'https://t.me/tokenprovider_official',
  authorEn: 'TokenProvider Engineering Team',
  authorZh: 'TokenProvider 工程团队',
  todayIso: new Date().toISOString().slice(0, 10),
  enableSearchAction: false  // turn on only if site adds /?q= search
};

export const ORG_SCHEMA = {
  '@type': 'Organization',
  '@id': `${CONFIG.baseUrl}/#organization`,
  name: 'TokenProvider',
  alternateName: ['Token Provider', 'TokenProvider AI Gateway'],
  url: `${CONFIG.baseUrl}/`,
  logo: {
    '@type': 'ImageObject',
    url: `${CONFIG.baseUrl}/logo.png`,
    width: 512,
    height: 512
  },
  description: 'Third-party API gateway for Claude, Claude Code, ChatGPT, and Gemini. Pay-as-you-go tokens, sticky sessions, OpenAI-compatible.',
  foundingDate: '2024',
  knowsAbout: [
    'Claude API',
    'Claude Code',
    'ChatGPT API',
    'Gemini API',
    'AI API gateway',
    'OpenAI-compatible proxy'
  ],
  sameAs: [CONFIG.telegramUrl],
  contactPoint: {
    '@type': 'ContactPoint',
    contactType: 'customer support',
    url: CONFIG.telegramUrl,
    availableLanguage: ['en', 'zh-CN']
  }
};

export const WEBSITE_SCHEMA = {
  '@type': 'WebSite',
  '@id': `${CONFIG.baseUrl}/#website`,
  url: `${CONFIG.baseUrl}/`,
  name: 'TokenProvider',
  alternateName: 'Token Provider',
  publisher: { '@id': `${CONFIG.baseUrl}/#organization` },
  inLanguage: ['en', 'zh-CN']
  // SearchAction intentionally omitted; flip CONFIG.enableSearchAction when site adds query support.
};

export const SERVICE_SCHEMA = {
  '@type': 'Service',
  '@id': `${CONFIG.baseUrl}/#service`,
  serviceType: 'AI API gateway',
  provider: { '@id': `${CONFIG.baseUrl}/#organization` },
  areaServed: 'Worldwide',
  audience: { '@type': 'Audience', audienceType: 'Developers' },
  hasOfferCatalog: {
    '@type': 'OfferCatalog',
    name: 'TokenProvider model routing',
    itemListElement: [
      { '@type': 'Offer', itemOffered: { '@type': 'Service', name: 'Claude API relay' } },
      { '@type': 'Offer', itemOffered: { '@type': 'Service', name: 'Claude Code relay (ANTHROPIC_BASE_URL)' } },
      { '@type': 'Offer', itemOffered: { '@type': 'Service', name: 'ChatGPT API relay (OpenAI-compatible)' } },
      { '@type': 'Offer', itemOffered: { '@type': 'Service', name: 'Gemini API relay' } }
    ]
  }
};

export const SPEAKABLE = {
  '@type': 'SpeakableSpecification',
  cssSelector: ['.seo-tldr', '.seo-faq h3', '.seo-faq p:first-of-type']
};

export function PERSON_AUTHOR(lang) {
  const name = lang === 'zh-CN' ? CONFIG.authorZh : CONFIG.authorEn;
  return {
    '@type': 'Person',
    name,
    worksFor: { '@id': `${CONFIG.baseUrl}/#organization` },
    knowsAbout: ['Claude API', 'ChatGPT API', 'AI API gateways', 'API integration']
  };
}
