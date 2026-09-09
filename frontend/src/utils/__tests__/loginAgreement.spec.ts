import { describe, expect, it } from 'vitest'

import type { LoginAgreementDocument } from '@/types'
import {
  resolveLoginAgreementDocument,
  resolveLoginAgreementDocuments,
} from '../loginAgreement'

describe('login agreement document localization', () => {
  const doc: LoginAgreementDocument = {
    id: 'terms',
    title: '服务条款',
    content_md: '中文正文',
    title_i18n: {
      zh: '服务条款',
      vi: 'Điều khoản dịch vụ',
      en: 'Terms of Service',
      ko: '서비스 이용약관',
    },
    content_md_i18n: {
      zh: '中文正文',
      vi: 'Nội dung tiếng Việt',
      en: 'English body',
      ko: '한국어 본문',
    },
  }

  it('resolves document title and markdown content for the active locale', () => {
    expect(resolveLoginAgreementDocument(doc, 'vi')).toMatchObject({
      title: 'Điều khoản dịch vụ',
      content_md: 'Nội dung tiếng Việt',
    })

    expect(resolveLoginAgreementDocument(doc, 'ko-KR')).toMatchObject({
      title: '서비스 이용약관',
      content_md: '한국어 본문',
    })
  })

  it('falls back to English then Chinese then legacy fields', () => {
    const partial: LoginAgreementDocument = {
      id: 'usage-policy',
      title: '使用政策',
      content_md: '中文政策',
      title_i18n: {
        zh: '使用政策',
        en: 'Usage Policy',
      },
      content_md_i18n: {
        zh: '中文政策',
        en: 'English policy',
      },
    }

    expect(resolveLoginAgreementDocument(partial, 'vi')).toMatchObject({
      title: 'Usage Policy',
      content_md: 'English policy',
    })

    expect(resolveLoginAgreementDocument({ id: 'x', title: 'Legacy', content_md: 'Legacy body' }, 'vi')).toMatchObject({
      title: 'Legacy',
      content_md: 'Legacy body',
    })
  })

  it('filters documents without a localized or legacy title', () => {
    expect(resolveLoginAgreementDocuments([{ id: 'empty', title: ' ', content_md: 'body' }], 'vi')).toEqual([])
  })
})
