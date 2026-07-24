<template>
  <div class="card">
    <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.emailPolicy.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.emailPolicy.description") }}
        </p>
      </div>
      <div class="flex gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm px-2"
          :disabled="loading || saving"
          :title="t('common.refresh')"
          @click="loadPolicy"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button type="button" class="btn btn-primary btn-sm" :disabled="loading || saving || !policy" @click="savePolicy">
          {{ saving ? t("common.saving") : t("common.save") }}
        </button>
      </div>
    </div>

    <div v-if="loading && !policy" class="px-6 py-8 text-sm text-gray-500 dark:text-gray-400">
      {{ t("common.loading") }}
    </div>

    <template v-else-if="policy">
      <section class="divide-y divide-gray-100 dark:divide-dark-700">
        <div
          v-for="channel in policy.channels"
          :key="channel.id"
          class="grid gap-4 px-6 py-4 lg:grid-cols-[minmax(15rem,1fr)_minmax(18rem,1.4fr)_auto] lg:items-center"
        >
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              {{ channelLabel(channel.id) }}
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ channelSummary(channel) }}
            </div>
          </div>

          <div class="min-w-0">
            <select
              v-if="channel.recipient_kind === 'group'"
              v-model="channel.recipient_group"
              class="input"
              :disabled="!channel.enabled"
              :aria-label="t('admin.settings.emailPolicy.recipientGroup')"
            >
              <option v-for="group in policy.recipient_groups" :key="group.id" :value="group.id">
                {{ groupLabel(group.id) }}
              </option>
            </select>

            <div v-else-if="channel.recipient_kind === 'user'" class="flex flex-wrap gap-x-5 gap-y-2">
              <label v-if="channel.allow_user_primary" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="channel.include_user_primary" type="checkbox" class="h-4 w-4 rounded border-gray-300" :disabled="!channel.enabled" />
                {{ t("admin.settings.emailPolicy.userPrimary") }}
              </label>
              <label v-if="channel.allow_verified_additional" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="channel.include_verified_additional" type="checkbox" class="h-4 w-4 rounded border-gray-300" :disabled="!channel.enabled" />
                {{ t("admin.settings.emailPolicy.verifiedAdditional") }}
              </label>
            </div>

            <div v-else class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.emailPolicy.eventRecipient") }}
            </div>
          </div>

          <Toggle v-model="channel.enabled" />
        </div>
      </section>

      <section class="border-t border-gray-200 px-6 py-5 dark:border-dark-600">
        <h3 class="text-base font-medium text-gray-900 dark:text-white">
          {{ t("admin.settings.emailPolicy.recipientGroups") }}
        </h3>
        <div class="mt-4 grid gap-x-8 gap-y-6 xl:grid-cols-2">
          <div v-for="group in policy.recipient_groups" :key="group.id" class="min-w-0">
            <div class="mb-2 flex min-h-8 items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ groupLabel(group.id) }}
              </label>
              <button
                type="button"
                class="btn btn-secondary btn-sm px-2"
                :title="t('admin.settings.emailPolicy.addRecipient')"
                @click="addMember(group)"
              >
                <Icon name="plus" size="xs" />
              </button>
            </div>

            <div v-if="group.members.length" class="space-y-2">
              <div v-for="(member, index) in group.members" :key="`${group.id}-${index}`" class="flex items-center gap-2">
                <input v-model="member.enabled" type="checkbox" class="h-4 w-4 flex-none rounded border-gray-300" />
                <input v-model.trim="member.email" type="email" class="input min-w-0 flex-1" :placeholder="t('admin.settings.emailPolicy.emailPlaceholder')" />
                <span
                  v-if="member.status === 'legacy_unverified'"
                  class="whitespace-nowrap text-xs text-amber-600 dark:text-amber-400"
                >
                  {{ t("admin.settings.emailPolicy.legacyUnverified") }}
                </span>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm px-2"
                  :title="t('common.delete')"
                  @click="group.members.splice(index, 1)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </div>
            </div>
            <div v-else class="min-h-10 py-2 text-sm text-gray-400">
              {{ t("admin.settings.emailPolicy.noRecipients") }}
            </div>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  NotificationEmailChannelPolicy,
  NotificationEmailPolicy,
  NotificationEmailRecipientGroup,
} from "@/api/admin/settings";
import Toggle from "@/components/common/Toggle.vue";
import Icon from "@/components/icons/Icon.vue";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";

const { t } = useI18n();
const appStore = useAppStore();
const loading = ref(false);
const saving = ref(false);
const policy = ref<NotificationEmailPolicy | null>(null);

const channelTranslationKeys: Record<string, string> = {
  auth_verification: "authVerification",
  password_reset: "passwordReset",
  subscription: "subscription",
  balance: "balance",
  account_quota: "accountQuota",
  risk_control: "riskControl",
  refund_admin: "refundAdmin",
  refund_user: "refundUser",
  ops_alert: "opsAlert",
  ops_report: "opsReport",
};

const groupTranslationKeys: Record<string, string> = {
  finance: "finance",
  account_quota: "accountQuota",
  security: "security",
  ops_alert: "opsAlert",
  ops_report: "opsReport",
};

function channelLabel(id: string): string {
  const key = channelTranslationKeys[id];
  return key ? t(`admin.settings.emailPolicy.channels.${key}`) : id;
}

function groupLabel(id: string): string {
  const key = groupTranslationKeys[id];
  return key ? t(`admin.settings.emailPolicy.groups.${key}`) : id;
}

function channelSummary(channel: NotificationEmailChannelPolicy): string {
  if (channel.recipient_kind === "group") {
    return t("admin.settings.emailPolicy.groupRecipientSummary");
  }
  if (channel.recipient_kind === "user") {
    return t("admin.settings.emailPolicy.userRecipientSummary");
  }
  return t("admin.settings.emailPolicy.explicitRecipientSummary");
}

function addMember(group: NotificationEmailRecipientGroup): void {
  group.members.push({ email: "", enabled: true, status: "admin_trusted" });
}

async function loadPolicy(): Promise<void> {
  loading.value = true;
  try {
    policy.value = await adminAPI.settings.getNotificationEmailPolicy();
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    loading.value = false;
  }
}

async function savePolicy(): Promise<void> {
  if (!policy.value) return;
  saving.value = true;
  try {
    policy.value = await adminAPI.settings.updateNotificationEmailPolicy({
      channels: policy.value.channels,
      recipient_groups: policy.value.recipient_groups,
    });
    appStore.showSuccess(t("admin.settings.emailPolicy.saveSuccess"));
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    saving.value = false;
  }
}

onMounted(loadPolicy);
</script>
