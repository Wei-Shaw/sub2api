import type { HostSdk } from '@sub2api/plugin-sdk'

let sdk: HostSdk | null = null

export function setSdk(instance: HostSdk): void {
  sdk = instance
}

export function getSdk(): HostSdk {
  if (!sdk) {
    throw new Error(
      '[plugin-gateway-openai] HostSdk not initialized. Call setSdk() during plugin install.',
    )
  }
  return sdk
}
