import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'test-account',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

const position = { top: 100, left: 100 }

// AccountActionMenu uses <Teleport to="body">; content is rendered in document.body, not in wrapper.
const getBodyText = () => document.body.textContent ?? ''
const getBodyButtons = () => Array.from(document.body.querySelectorAll('button'))

describe('AccountActionMenu — spark shadow 按钮可见性', () => {
  it('普通账号显示「复制账号」按钮', () => {
    const account = makeAccount({ platform: 'anthropic', type: 'apikey', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).toContain('admin.accounts.duplicateAccount')
    wrapper.unmount()
  })

  it('影子账号隐藏「复制账号」按钮', () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: 42 })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.accounts.duplicateAccount')
    wrapper.unmount()
  })

  it.each(['oauth', 'setup-token'] as const)('%s 账号隐藏「复制账号」按钮，避免共享可轮换令牌', (type) => {
    const account = makeAccount({ platform: 'openai', type, parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.accounts.duplicateAccount')
    wrapper.unmount()
  })

  it('点击「复制账号」触发 duplicate 事件并携带 account', async () => {
    const account = makeAccount({ platform: 'anthropic', type: 'apikey', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })

    const duplicateBtn = getBodyButtons().find(b => b.textContent?.includes('admin.accounts.duplicateAccount'))
    expect(duplicateBtn).toBeDefined()

    duplicateBtn!.click()
    await wrapper.vm.$nextTick()

    const emitted = wrapper.emitted('duplicate')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ id: account.id, name: account.name })
    wrapper.unmount()
  })

  it('OpenAI OAuth 母账号（无 parent_account_id）显示「创建 spark 影子」按钮', () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).toContain('admin.accounts.createSparkShadow')
    wrapper.unmount()
  })

  it('影子账号（parent_account_id 非 null）隐藏「创建 spark 影子」按钮', () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: 42 })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.accounts.createSparkShadow')
    wrapper.unmount()
  })

  it('非 OpenAI 账号隐藏「创建 spark 影子」按钮', () => {
    const account = makeAccount({ platform: 'antigravity', type: 'oauth', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.accounts.createSparkShadow')
    wrapper.unmount()
  })

  it('影子账号隐藏凭据/隐私类操作(重授权/刷新token/隐私)— 外审 G4', () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: 42 })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    const body = getBodyText()
    expect(body).not.toContain('admin.accounts.reAuthorize')
    expect(body).not.toContain('admin.accounts.refreshToken')
    expect(body).not.toContain('admin.accounts.setPrivacy')
    wrapper.unmount()
  })

  it('普通 OpenAI OAuth 母账号仍显示凭据/隐私类操作', () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })
    const body = getBodyText()
    expect(body).toContain('admin.accounts.reAuthorize')
    expect(body).toContain('admin.accounts.setPrivacy')
    wrapper.unmount()
  })

  it('点击按钮触发 create-spark-shadow 事件并携带 account', async () => {
    const account = makeAccount({ platform: 'openai', type: 'oauth', parent_account_id: null })
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, position },
      attachTo: document.body,
    })

    // Content is teleported to body — find button by text there
    const sparkBtn = getBodyButtons().find(b => b.textContent?.includes('admin.accounts.createSparkShadow'))
    expect(sparkBtn).toBeDefined()

    sparkBtn!.click()
    await wrapper.vm.$nextTick()

    const emitted = wrapper.emitted('create-spark-shadow')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ id: account.id, platform: 'openai' })

    wrapper.unmount()
  })
})
