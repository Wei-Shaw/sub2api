/** Channel status values (must match service.Status* constants in Go). */
export const CHANNEL_STATUS_ACTIVE = 'active' as const
export const CHANNEL_STATUS_DISABLED = 'disabled' as const
export type ChannelStatus = typeof CHANNEL_STATUS_ACTIVE | typeof CHANNEL_STATUS_DISABLED

/** Billing mode values (must match service.BillingMode* constants in Go). */
export const BILLING_MODE_TOKEN = 'token' as const
export const BILLING_MODE_PER_REQUEST = 'per_request' as const
export const BILLING_MODE_IMAGE = 'image' as const
export type BillingMode =
  | typeof BILLING_MODE_TOKEN
  | typeof BILLING_MODE_PER_REQUEST
  | typeof BILLING_MODE_IMAGE

/** Billing-model-source values (must match service.BillingModelSource* constants in Go). */
export const BILLING_MODEL_SOURCE_REQUESTED = 'requested' as const
export const BILLING_MODEL_SOURCE_UPSTREAM = 'upstream' as const
export const BILLING_MODEL_SOURCE_CHANNEL_MAPPED = 'channel_mapped' as const
export type BillingModelSource =
  | typeof BILLING_MODEL_SOURCE_REQUESTED
  | typeof BILLING_MODEL_SOURCE_UPSTREAM
  | typeof BILLING_MODEL_SOURCE_CHANNEL_MAPPED
