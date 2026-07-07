import { describe, expect, it } from 'vitest'
import {
  formatRegistrationEmailSuffixWhitelistForMessage,
  isRegistrationEmailSuffixAllowed,
  isRegistrationEmailSuffixDomainValid,
  normalizeRegistrationEmailSuffixDomain,
  normalizeRegistrationEmailSuffixDomains,
  normalizeRegistrationEmailSuffixWhitelist,
  parseRegistrationEmailSuffixWhitelistInput
} from '@/utils/registrationEmailPolicy'

describe('registrationEmailPolicy utils', () => {
  it('normalizeRegistrationEmailSuffixDomain lowercases, strips @, and ignores invalid chars', () => {
    expect(normalizeRegistrationEmailSuffixDomain(' @Exa!mple.COM ')).toBe('example.com')
    expect(normalizeRegistrationEmailSuffixDomain(' *.EDU!.CN ')).toBe('*.edu.cn')
  })

  it('normalizeRegistrationEmailSuffixDomains deduplicates normalized domains', () => {
    expect(
      normalizeRegistrationEmailSuffixDomains([
        '@example.com',
        'Example.com',
        '',
        '-invalid.com',
        'foo..bar.com',
        ' @foo.bar ',
        '@foo.bar',
        '*.EDU.CN',
        '*.edu.cn'
      ])
    ).toEqual(['example.com', 'foo.bar', '*.edu.cn'])
  })

  it('parseRegistrationEmailSuffixWhitelistInput supports separators and deduplicates', () => {
    const input = '\n  @example.com,example.com，@foo.bar\t@FOO.bar *.EDU.CN  '
    expect(parseRegistrationEmailSuffixWhitelistInput(input)).toEqual([
      'example.com',
      'foo.bar',
      '*.edu.cn'
    ])
  })

  it('parseRegistrationEmailSuffixWhitelistInput drops tokens containing invalid chars', () => {
    const input = '@exa!mple.com, @foo.bar, @bad#token.com, @ok-domain.com'
    expect(parseRegistrationEmailSuffixWhitelistInput(input)).toEqual(['foo.bar', 'ok-domain.com'])
  })

  it('parseRegistrationEmailSuffixWhitelistInput drops structurally invalid domains', () => {
    const input = '@-bad.com, @foo..bar.com, @foo.bar, @xn--ok.com, *., *, *.@, *.foo'
    expect(parseRegistrationEmailSuffixWhitelistInput(input)).toEqual(['foo.bar', 'xn--ok.com'])
  })

  it('parseRegistrationEmailSuffixWhitelistInput returns empty list for blank input', () => {
    expect(parseRegistrationEmailSuffixWhitelistInput('   \n \n')).toEqual([])
  })

  it('normalizeRegistrationEmailSuffixWhitelist returns canonical @domain list', () => {
    expect(
      normalizeRegistrationEmailSuffixWhitelist([
        '@Example.com',
        'foo.bar',
        '',
        '-invalid.com',
        ' @foo.bar ',
        '*.EDU.CN'
      ])
    ).toEqual(['@example.com', '@foo.bar', '*.edu.cn'])
  })

  it('isRegistrationEmailSuffixDomainValid matches backend-compatible domain rules', () => {
    expect(isRegistrationEmailSuffixDomainValid('example.com')).toBe(true)
    expect(isRegistrationEmailSuffixDomainValid('foo-bar.example.com')).toBe(true)
    expect(isRegistrationEmailSuffixDomainValid('*.edu.cn')).toBe(true)
    expect(isRegistrationEmailSuffixDomainValid('-bad.com')).toBe(false)
    expect(isRegistrationEmailSuffixDomainValid('foo..bar.com')).toBe(false)
    expect(isRegistrationEmailSuffixDomainValid('localhost')).toBe(false)
    expect(isRegistrationEmailSuffixDomainValid('*.foo')).toBe(false)
    expect(isRegistrationEmailSuffixDomainValid('*')).toBe(false)
    expect(isRegistrationEmailSuffixDomainValid('*.@')).toBe(false)
  })

  it('isRegistrationEmailSuffixAllowed allows any email when whitelist is empty', () => {
    expect(isRegistrationEmailSuffixAllowed('user@example.com', [])).toBe(true)
  })

  it('isRegistrationEmailSuffixAllowed applies exact suffix matching', () => {
    expect(isRegistrationEmailSuffixAllowed('user@example.com', ['@example.com'])).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('user@sub.example.com', ['@example.com'])).toBe(false)
    expect(isRegistrationEmailSuffixAllowed('user@qq.com', ['@qq.com'])).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('user@sub.qq.com', ['@qq.com'])).toBe(false)
  })

  it('isRegistrationEmailSuffixAllowed applies wildcard suffix matching', () => {
    expect(isRegistrationEmailSuffixAllowed('student@cs.edu.cn', ['*.edu.cn'])).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('student@edu.cn', ['*.edu.cn'])).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('student@foo.cn', ['*.edu.cn'])).toBe(false)
  })

  it('isRegistrationEmailSuffixAllowed supports mixed exact and wildcard entries', () => {
    const whitelist = ['@a.com', '*.b.cn']
    expect(isRegistrationEmailSuffixAllowed('user@a.com', whitelist)).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('user@school.b.cn', whitelist)).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('user@b.cn', whitelist)).toBe(true)
    expect(isRegistrationEmailSuffixAllowed('user@c.cn', whitelist)).toBe(false)
  })

  it('formatRegistrationEmailSuffixWhitelistForMessage lists up to five entries', () => {
    expect(
      formatRegistrationEmailSuffixWhitelistForMessage(
        ['@a.com', '@b.com', '@c.com', '@d.com', '@e.com'],
        { separator: ', ', more: (count) => `and ${count} more` }
      )
    ).toBe('@a.com, @b.com, @c.com, @d.com, @e.com')
    expect(
      formatRegistrationEmailSuffixWhitelistForMessage(
        ['@a.com', '@b.com', '@c.com', '@d.com', '@e.com', '*.edu.cn', '@f.com'],
        { separator: ', ', more: (count) => `and ${count} more` }
      )
    ).toBe('@a.com, @b.com, @c.com, @d.com, @e.com, and 2 more')
  })
})
