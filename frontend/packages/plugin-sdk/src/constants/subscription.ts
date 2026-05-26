/**
 * Subscription type constants — single source of truth shared between
 * gateway plugins (Anthropic / Antigravity / OpenAI) and the payment plugin.
 *
 * Backend contract: Group.subscription_type column, see
 * backend/internal/repository (groups table). Keep these literals in sync
 * with the server-side enum.
 */

export const SUBSCRIPTION_TYPE_SUBSCRIPTION = 'subscription' as const
export const SUBSCRIPTION_TYPE_STANDARD = 'standard' as const

export type SubscriptionType =
  | typeof SUBSCRIPTION_TYPE_SUBSCRIPTION
  | typeof SUBSCRIPTION_TYPE_STANDARD
