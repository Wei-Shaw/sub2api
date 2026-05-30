/**
 * Plugin host SDK accessor.
 *
 * install() stores the host-injected SDK into a module singleton; view files
 * call getSdk() to reach notify / http / auth capabilities. Mirrors client.ts
 * but tracks a different concern (full SDK handle vs raw axios client).
 */
import { createSdkAccessor } from '@sub2api/plugin-sdk'

export const { setSdk, getSdk } = createSdkAccessor('plugin-content-moderation')
