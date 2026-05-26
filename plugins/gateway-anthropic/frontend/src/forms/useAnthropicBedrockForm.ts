/**
 * Anthropic Bedrock-specific form state and payload builders.
 */
import { ref } from 'vue'
import type { SdkAccount } from '@sub2api/plugin-sdk'

export function useAnthropicBedrockForm() {
  const bedrockAuthMode = ref<'sigv4' | 'apikey'>('sigv4')
  const bedrockAccessKeyId = ref('')
  const bedrockSecretAccessKey = ref('')
  const bedrockSessionToken = ref('')
  const bedrockRegion = ref('us-east-1')
  const bedrockForceGlobal = ref(false)
  const bedrockApiKeyValue = ref('')

  function resetBedrock(): void {
    bedrockAuthMode.value = 'sigv4'
    bedrockAccessKeyId.value = ''
    bedrockSecretAccessKey.value = ''
    bedrockSessionToken.value = ''
    bedrockRegion.value = 'us-east-1'
    bedrockForceGlobal.value = false
    bedrockApiKeyValue.value = ''
  }

  function buildBedrockCredentials(): Record<string, unknown> {
    const credentials: Record<string, unknown> = {
      auth_mode: bedrockAuthMode.value,
      aws_region: bedrockRegion.value.trim() || 'us-east-1',
    }
    if (bedrockAuthMode.value === 'sigv4') {
      credentials.aws_access_key_id = bedrockAccessKeyId.value.trim()
      credentials.aws_secret_access_key = bedrockSecretAccessKey.value.trim()
      if (bedrockSessionToken.value.trim()) {
        credentials.aws_session_token = bedrockSessionToken.value.trim()
      }
    } else {
      credentials.api_key = bedrockApiKeyValue.value.trim()
    }
    if (bedrockForceGlobal.value) credentials.aws_force_global = 'true'
    return credentials
  }

  function initBedrockFromAccount(account: SdkAccount): void {
    const creds = account.credentials
    bedrockAuthMode.value = (creds?.auth_mode as 'sigv4' | 'apikey') || 'sigv4'
    bedrockRegion.value = (creds?.aws_region as string) || 'us-east-1'
    bedrockForceGlobal.value = creds?.aws_force_global === 'true'
  }

  function applyBedrockEditCredentials(newCreds: Record<string, unknown>): void {
    newCreds.auth_mode = bedrockAuthMode.value
    newCreds.aws_region = bedrockRegion.value.trim() || 'us-east-1'
    if (bedrockForceGlobal.value) newCreds.aws_force_global = 'true'
    else delete newCreds.aws_force_global
  }

  return {
    bedrockAuthMode, bedrockAccessKeyId, bedrockSecretAccessKey,
    bedrockSessionToken, bedrockRegion, bedrockForceGlobal, bedrockApiKeyValue,
    resetBedrock, buildBedrockCredentials,
    initBedrockFromAccount, applyBedrockEditCredentials,
  }
}
