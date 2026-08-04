import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import DingTalkOAuthSection from '@/components/auth/DingTalkOAuthSection.vue'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  REDACTED)
REDACTED))

describe('OAuth login sections', () => {
  beforeEach(() => {
    routeState.query = { redirect: '/billing?plan=pro', aff: 'AFF123' REDACTED
    window.sessionStorage.clear()
  REDACTED)

  it.each([
    ['linuxdo', LinuxDoOAuthSection],
    ['dingtalk', DingTalkOAuthSection],
    ['oidc', OidcOAuthSection]
  ] as const)('emits a %s start request from the original button', async (provider, component) => {
    const originalHref = window.location.href
    const wrapper = mount(component, { props: { affCode: 'AFF456' REDACTED REDACTED)

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('start')?.[0]?.[0]).toEqual({
      provider,
      params: { redirect: '/billing?plan=pro' REDACTED
    REDACTED)
    expect(window.sessionStorage.getItem('oauth_aff_code')).toBe('AFF456')
    expect(window.location.href).toBe(originalHref)
  REDACTED)
REDACTED)
