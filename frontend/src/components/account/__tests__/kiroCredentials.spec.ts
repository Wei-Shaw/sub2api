import { describe, expect, it } from 'vitest'
import {
  buildKiroCredentials,
  kiroRequiredFields,
  validateKiroCredentials,
  KIRO_AUTH_METHOD_OPTIONS,
  type KiroCredentialForm
} from '../kiroCredentials'

const base: KiroCredentialForm = {
  authMethod: 'social',
  refreshToken: '',
  accessToken: '',
  clientId: '',
  clientSecret: '',
  issuerUrl: '',
  region: '',
  profileArn: '',
  apiKey: '',
  fakeThinking: false
}

describe('KIRO_AUTH_METHOD_OPTIONS', () => {
  it('覆盖后端支持的四种接入方式', () => {
    expect(KIRO_AUTH_METHOD_OPTIONS.map((o) => o.value)).toEqual([
      'social',
      'builder_id',
      'idc',
      'api_key'
    ])
  })
})

describe('buildKiroCredentials', () => {
  it('social 只提交 refresh_token 与 region', () => {
    const creds = buildKiroCredentials({ ...base, refreshToken: 'rt', region: 'us-east-1' })
    expect(creds.auth_method).toBe('social')
    expect(creds.refresh_token).toBe('rt')
    expect(creds.region).toBe('us-east-1')
    expect(creds).not.toHaveProperty('client_id')
    expect(creds).not.toHaveProperty('api_key')
  })

  it('idc 提交客户端凭据与 issuer_url', () => {
    const creds = buildKiroCredentials({
      ...base,
      authMethod: 'idc',
      refreshToken: 'rt',
      clientId: 'cid',
      clientSecret: 'csec',
      issuerUrl: 'https://d-90667b4f8e.awsapps.com/start',
      region: 'us-east-1'
    })
    expect(creds.auth_method).toBe('idc')
    expect(creds.client_id).toBe('cid')
    expect(creds.client_secret).toBe('csec')
    expect(creds.issuer_url).toBe('https://d-90667b4f8e.awsapps.com/start')
  })

  it('api_key 只提交 api_key，不带 token 字段', () => {
    const creds = buildKiroCredentials({ ...base, authMethod: 'api_key', apiKey: 'kiro_ak_1' })
    expect(creds.auth_method).toBe('api_key')
    expect(creds.api_key).toBe('kiro_ak_1')
    expect(creds).not.toHaveProperty('refresh_token')
    expect(creds).not.toHaveProperty('access_token')
  })

  // machine_id 由后端在建号时固化，前端插手会破坏设备指纹的稳定性。
  it('从不提交 machine_id', () => {
    for (const m of ['social', 'builder_id', 'idc', 'api_key'] as const) {
      expect(buildKiroCredentials({ ...base, authMethod: m, refreshToken: 'rt', apiKey: 'k' }))
        .not.toHaveProperty('machine_id')
    }
  })

  it('fake_thinking 总是显式提交布尔值——后端默认开启，取消勾选必须能真正关闭', () => {
    expect(buildKiroCredentials({ ...base, refreshToken: 'rt', fakeThinking: false }).fake_thinking).toBe(
      false
    )
    expect(buildKiroCredentials({ ...base, refreshToken: 'rt', fakeThinking: true }).fake_thinking).toBe(
      true
    )
  })

  it('去除字段首尾空白', () => {
    const creds = buildKiroCredentials({ ...base, refreshToken: '  rt  ', region: ' us-east-1 ' })
    expect(creds.refresh_token).toBe('rt')
    expect(creds.region).toBe('us-east-1')
  })
})

describe('validateKiroCredentials', () => {
  it('social 缺 refresh_token 时报错', () => {
    expect(validateKiroCredentials(base)).toBeTruthy()
    expect(validateKiroCredentials({ ...base, refreshToken: 'rt' })).toBeNull()
  })

  it('idc 缺客户端凭据或 issuer_url 时报错', () => {
    const idc: KiroCredentialForm = { ...base, authMethod: 'idc', refreshToken: 'rt' }
    expect(validateKiroCredentials(idc)).toBeTruthy()
    expect(validateKiroCredentials({ ...idc, clientId: 'c', clientSecret: 's' })).toBeTruthy()
    expect(
      validateKiroCredentials({ ...idc, clientId: 'c', clientSecret: 's', issuerUrl: 'https://x/start' })
    ).toBeNull()
  })

  it('api_key 缺密钥时报错', () => {
    expect(validateKiroCredentials({ ...base, authMethod: 'api_key' })).toBeTruthy()
    expect(validateKiroCredentials({ ...base, authMethod: 'api_key', apiKey: 'k' })).toBeNull()
  })
})

describe('kiroRequiredFields', () => {
  it('按接入方式返回必填项，供表单标星', () => {
    expect(kiroRequiredFields('social')).toEqual(['refreshToken'])
    expect(kiroRequiredFields('api_key')).toEqual(['apiKey'])
    expect(kiroRequiredFields('idc')).toEqual(['refreshToken', 'clientId', 'clientSecret', 'issuerUrl'])
    expect(kiroRequiredFields('builder_id')).toEqual(['refreshToken', 'clientId', 'clientSecret'])
  })
})
