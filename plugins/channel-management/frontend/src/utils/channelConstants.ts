/**
 * Channel-domain shared constants for the plugin frontend.
 *
 * V5 W9 内联自 host frontend `src/constants/channel.ts`. 这些值是后端 service
 * 层 BillingMode/Status 常量在前端的镜像, plugin 自己持有副本以避免对 host
 * 内部目录的耦合.
 */

export const BILLING_MODE_TOKEN = 'token' as const
export const BILLING_MODE_PER_REQUEST = 'per_request' as const
export const BILLING_MODE_IMAGE = 'image' as const
export type BillingMode =
  | typeof BILLING_MODE_TOKEN
  | typeof BILLING_MODE_PER_REQUEST
  | typeof BILLING_MODE_IMAGE
