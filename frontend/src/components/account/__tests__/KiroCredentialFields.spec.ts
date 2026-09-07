import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import KiroCredentialFields from '../KiroCredentialFields.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('KiroCredentialFields', () => {
  it('social 只显示 refreshToken，不显示客户端凭据与 API Key', () => {
    const wrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'social' } }
    })
    expect(wrapper.find('[data-test="kiro-refresh-token"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="kiro-client-id"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="kiro-api-key"]').exists()).toBe(false)
  })

  it('idc 显示 issuer_url 与客户端凭据', () => {
    const wrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'idc' } }
    })
    expect(wrapper.find('[data-test="kiro-issuer-url"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="kiro-client-id"]').exists()).toBe(true)
  })

  it('api_key 只显示密钥输入', () => {
    const wrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'api_key' } }
    })
    expect(wrapper.find('[data-test="kiro-api-key"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="kiro-refresh-token"]').exists()).toBe(false)
  })

  it('假思考开关默认关闭', () => {
    const wrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'social' } }
    })
    const toggle = wrapper.find('[data-test="kiro-fake-thinking"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('aria-checked')).not.toBe('true')
  })

  // Profile ARN（可选）：idc/builder_id 的 token 交换不会自动带回 profileArn
  // （真实账号测试 + 第三方参考实现 kirocc/opencode-kiro-auth 均证实这一点，
  // 只有 social 会），需要一个手填入口，否则 ListAvailableModels 等依赖
  // profileArn 的接口永远拿不到值。api_key 方式不使用 profileArn，不应该
  // 显示这个字段。
  it('idc 显示 Profile ARN 输入，api_key 不显示', () => {
    const idcWrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'idc' } }
    })
    expect(idcWrapper.find('[data-test="kiro-profile-arn"]').exists()).toBe(true)

    const apiKeyWrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'api_key' } }
    })
    expect(apiKeyWrapper.find('[data-test="kiro-profile-arn"]').exists()).toBe(false)
  })

  it('builder_id 显示 Profile ARN 输入，填写后触发 update:modelValue', async () => {
    const wrapper = mount(KiroCredentialFields, {
      props: { modelValue: { authMethod: 'builder_id', profileArn: '' } }
    })
    const input = wrapper.find('[data-test="kiro-profile-arn"]')
    expect(input.exists()).toBe(true)

    await input.setValue('arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    const lastEmit = emitted![emitted!.length - 1][0] as Record<string, unknown>
    expect(lastEmit.profileArn).toBe('arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456')
  })

  // client_secret 是独立于 refresh_token/api_key 的另一个敏感字段（I7 修复：
  // 之前遗漏在后端 SensitiveCredentialKeys 之外，会明文回传），需要自己的
  // "留空即保留"提示，不能复用 hasExistingSecret（那个只反映 refresh_token/
  // api_key 是否已配置，两者可能不同时为真）。
  it('hasExistingClientSecret 只控制 client secret 的提示，不受 hasExistingSecret 影响', () => {
    const wrapper = mount(KiroCredentialFields, {
      props: {
        modelValue: { authMethod: 'idc' },
        hasExistingSecret: false,
        hasExistingClientSecret: true
      }
    })
    const hints = wrapper.findAll('.input-hint').map(el => el.text())
    expect(hints.some(text => text.includes('admin.accounts.oauth.kiro.leaveEmptyHint'))).toBe(true)
  })
})
