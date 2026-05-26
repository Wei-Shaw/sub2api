<template>
  <div class="space-y-5">
    <!-- Name + Notes at top -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="identity" />

    <!-- Credential Fields from plugin JSON Schema -->
    <div v-if="credentialSchema" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.accounts.credentials") }}
      </h3>
      <JsonSchemaForm
        :schema="credentialSchema"
        :model-value="credentials"
        @update:model-value="credentials = $event"
      />
    </div>
    <!-- Extra Fields from plugin JSON Schema -->
    <div v-if="extraSchema" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.accounts.extra") }}
      </h3>
      <JsonSchemaForm
        :schema="extraSchema"
        :model-value="extra"
        @update:model-value="extra = $event"
      />
    </div>
    <p v-if="!credentialSchema && !extraSchema" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.accounts.noFieldsDeclared") }}
    </p>

    <!-- Other settings: proxy, concurrency, expiration, groups, etc. -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="settings" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue"
import { useI18n } from "vue-i18n"
import JsonSchemaForm from "@/components/common/JsonSchemaForm.vue"
import { AccountCommonFields } from "@sub2api/plugin-sdk"
import type { CommonAccountFields } from "@sub2api/plugin-sdk"
import { usePlatforms } from "@/composables/usePlatforms"
import type { PlatformFormContext, PlatformFormPayload, PlatformFormValidation, EditFormPayload } from "./types"
import type { Account } from '@/types'

const props = defineProps<{
  context: PlatformFormContext
  platform: string
}>()
const { t } = useI18n()
const { getAccountTypeDecl } = usePlatforms()

const platform = computed(() => props.platform)
const credentials = ref<Record<string, unknown>>({})
const extra = ref<Record<string, unknown>>({})

const defaultCommonFields: CommonAccountFields = {
  name: '', notes: '',
  proxy_id: null, concurrency: 10, load_factor: null, priority: 1,
  rate_multiplier: 1, expires_at: null, auto_pause_on_expired: true,
  group_ids: [], quota_enabled: false, quota_limit: null,
  quota_daily_limit: null, quota_weekly_limit: null,
}
const commonFields = ref<CommonAccountFields>({ ...defaultCommonFields })

const accountTypeDecl = computed(() =>
  getAccountTypeDecl(platform.value, props.context.accountTypeId)
)
const credentialSchema = computed(() =>
  (accountTypeDecl.value?.credential_schema as Record<string, unknown>) ?? null
)
const extraSchema = computed(() =>
  (accountTypeDecl.value?.extra_schema as Record<string, unknown>) ?? null
)

function validate(): PlatformFormValidation {
  return { valid: true }
}

function getPayload(): PlatformFormPayload {
  return {
    credentials: { ...credentials.value },
    extra: Object.keys(extra.value).length > 0 ? { ...extra.value } : undefined,
    common: commonFields.value,
  }
}

function isOAuthFlow(): boolean { return false }

function reset() {
  credentials.value = {}
  extra.value = {}
  commonFields.value = { ...defaultCommonFields }
}

function initFromAccount(account: Account): void {
  credentials.value = { ...((account.credentials as Record<string, unknown>) || {}) }
  const accountExtra = (account.extra ?? {}) as Record<string, unknown>
  extra.value = { ...accountExtra }
  commonFields.value = {
    name: account.name ?? '',
    notes: (account.notes as string) ?? '',
    proxy_id: account.proxy_id ?? null,
    concurrency: account.concurrency ?? 10,
    load_factor: (account.load_factor as number | null) ?? null,
    priority: account.priority ?? 1,
    rate_multiplier: account.rate_multiplier ?? 1,
    expires_at: account.expires_at ?? null,
    auto_pause_on_expired: account.auto_pause_on_expired ?? true,
    group_ids: account.group_ids ?? [],
    quota_enabled: !!(accountExtra.quota_limit || accountExtra.quota_daily_limit || accountExtra.quota_weekly_limit),
    quota_limit: (accountExtra.quota_limit as number) ?? null,
    quota_daily_limit: (accountExtra.quota_daily_limit as number) ?? null,
    quota_weekly_limit: (accountExtra.quota_weekly_limit as number) ?? null,
  }
}

function getEditPayload(_account: Account): EditFormPayload {
  return {
    credentials: { ...credentials.value },
    extra: Object.keys(extra.value).length > 0 ? { ...extra.value } : undefined,
    common: commonFields.value,
  }
}

defineExpose({ validate, getPayload, isOAuthFlow, reset, initFromAccount, getEditPayload })
</script>