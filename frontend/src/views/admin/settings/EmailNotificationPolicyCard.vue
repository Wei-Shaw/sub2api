<template>
  <div class="card">
    <div
      class="flex items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
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
          <Icon
            name="refresh"
            size="sm"
            :class="loading ? 'animate-spin' : ''"
          />
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loading || saving || !policy"
          @click="savePolicy"
        >
          {{ saving ? t("common.saving") : t("common.save") }}
        </button>
      </div>
    </div>

    <div
      v-if="loading && !policy"
      class="px-6 py-8 text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t("common.loading") }}
    </div>

    <template v-else-if="policy">
      <section class="divide-y divide-gray-100 dark:divide-dark-700">
        <div
          v-for="channel in editableChannels"
          :key="channel.id"
          :data-channel-id="channel.id"
          class="px-6 py-4"
        >
          <div
            class="grid gap-4 lg:grid-cols-[minmax(15rem,1fr)_minmax(18rem,1.4fr)_auto] lg:items-center"
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
                <option
                  v-for="group in selectableRecipientGroups"
                  :key="group.id"
                  :value="group.id"
                >
                  {{ groupLabel(group.id) }}
                </option>
              </select>

              <div v-else-if="channel.id === 'balance'" class="space-y-2">
                <label
                  class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  <input
                    v-model="channel.include_user_primary"
                    type="checkbox"
                    class="mt-0.5 h-4 w-4 rounded border-gray-300"
                    :disabled="!channel.enabled"
                  />
                  <span>
                    <span class="block">{{
                      t("admin.settings.emailPolicy.userPrimary")
                    }}</span>
                    <span
                      class="block text-xs text-gray-500 dark:text-gray-400"
                    >
                      {{ t("admin.settings.emailPolicy.userPrimaryHint") }}
                    </span>
                  </span>
                </label>
                <label
                  class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  <input
                    v-model="channel.include_verified_additional"
                    type="checkbox"
                    class="mt-0.5 h-4 w-4 rounded border-gray-300"
                    :disabled="!channel.enabled"
                  />
                  <span>
                    <span class="block">{{
                      t("admin.settings.emailPolicy.verifiedAdditional")
                    }}</span>
                    <span
                      class="block text-xs text-gray-500 dark:text-gray-400"
                    >
                      {{
                        t("admin.settings.emailPolicy.verifiedAdditionalHint")
                      }}
                    </span>
                  </span>
                </label>
              </div>

              <div v-else class="text-sm text-gray-600 dark:text-gray-300">
                {{ t("admin.settings.emailPolicy.userAccountEmailRecipient") }}
              </div>
            </div>

            <Toggle v-model="channel.enabled" />
          </div>

          <div
            v-if="channel.id === 'subscription'"
            class="mt-4 rounded-lg border border-gray-100 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-700/40"
          >
            <div class="flex items-center justify-between gap-4">
              <div>
                <div
                  class="text-sm font-medium text-gray-800 dark:text-gray-200"
                >
                  {{ t("admin.settings.emailPolicy.subscriptionExpiry.title") }}
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.emailPolicy.subscriptionExpiry.hint") }}
                </p>
              </div>
              <Toggle
                v-model="policy.feature_settings.subscription_expiry_enabled"
                :disabled="!channel.enabled"
              />
            </div>
          </div>

          <div
            v-if="channel.id === 'balance'"
            class="mt-4 rounded-lg border border-gray-100 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-700/40"
          >
            <div class="flex items-center justify-between gap-4">
              <div>
                <div
                  class="text-sm font-medium text-gray-800 dark:text-gray-200"
                >
                  {{ t("admin.settings.emailPolicy.balanceLow.title") }}
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.emailPolicy.balanceLow.hint") }}
                </p>
              </div>
              <Toggle
                v-model="policy.feature_settings.balance_low_enabled"
                :disabled="!channel.enabled"
              />
            </div>
            <div
              v-if="policy.feature_settings.balance_low_enabled"
              class="mt-4 grid gap-4 md:grid-cols-2"
            >
              <div>
                <label class="input-label">{{
                  t("admin.settings.emailPolicy.balanceLow.threshold")
                }}</label>
                <div class="relative">
                  <span
                    class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    >$</span
                  >
                  <input
                    v-model.number="
                      policy.feature_settings.balance_low_threshold
                    "
                    type="number"
                    min="0.01"
                    step="0.01"
                    class="input pl-7"
                    :disabled="!channel.enabled"
                  />
                </div>
              </div>
              <div>
                <label class="input-label">{{
                  t("admin.settings.emailPolicy.balanceLow.rechargeURL")
                }}</label>
                <input
                  v-model.trim="
                    policy.feature_settings.balance_low_recharge_url
                  "
                  type="url"
                  class="input"
                  placeholder="https://example.com/recharge"
                  :disabled="!channel.enabled"
                />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="border-t border-gray-200 px-6 py-5 dark:border-dark-600">
        <h3 class="text-base font-medium text-gray-900 dark:text-white">
          {{ t("admin.settings.emailPolicy.recipientGroups") }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.emailPolicy.recipientGroupsHint") }}
        </p>
        <div class="mt-4 flex flex-col gap-2 sm:flex-row">
          <input
            v-model.trim="bulkAdminEmail"
            data-testid="bulk-admin-email"
            type="email"
            class="input min-w-0 flex-1"
            :placeholder="t('admin.settings.emailPolicy.bulkEmailPlaceholder')"
            @keydown.enter.prevent="addAdminEmailToAllGroups"
          />
          <button
            type="button"
            class="btn btn-secondary whitespace-nowrap"
            :disabled="!bulkAdminEmail.trim()"
            @click="addAdminEmailToAllGroups"
          >
            {{ t("admin.settings.emailPolicy.addToAllGroups") }}
          </button>
        </div>
        <div class="mt-4 grid gap-x-8 gap-y-6 xl:grid-cols-2">
          <div
            v-for="group in managedRecipientGroups"
            :key="group.id"
            :data-recipient-group-id="group.id"
            class="min-w-0"
          >
            <div class="mb-2 flex min-h-8 items-center justify-between gap-3">
              <label
                class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >
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
              <div
                v-for="(member, index) in group.members"
                :key="`${group.id}-${index}`"
                class="flex items-center gap-2"
              >
                <input
                  v-model="member.enabled"
                  type="checkbox"
                  class="h-4 w-4 flex-none rounded border-gray-300"
                />
                <input
                  v-model.trim="member.email"
                  type="email"
                  class="input min-w-0 flex-1"
                  :placeholder="
                    t('admin.settings.emailPolicy.emailPlaceholder')
                  "
                />
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

      <section class="border-t border-gray-200 dark:border-dark-600">
        <div
          class="flex flex-wrap items-center justify-between gap-3 px-6 py-4"
        >
          <h3 class="text-base font-medium text-gray-900 dark:text-white">
            {{ t("admin.settings.emailPolicy.deliveries.title") }}
          </h3>
          <div class="flex items-center gap-2">
            <select
              v-model="deliveryStatus"
              class="input h-9 w-36"
              @change="loadDeliveries"
            >
              <option value="">
                {{ t("admin.settings.emailPolicy.deliveries.allStatuses") }}
              </option>
              <option
                v-for="status in deliveryStatuses"
                :key="status"
                :value="status"
              >
                {{ deliveryStatusLabel(status) }}
              </option>
            </select>
            <button
              type="button"
              class="btn btn-secondary btn-sm px-2"
              :disabled="deliveriesLoading"
              :title="t('common.refresh')"
              @click="loadDeliveries"
            >
              <Icon
                name="refresh"
                size="sm"
                :class="deliveriesLoading ? 'animate-spin' : ''"
              />
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[760px] text-left text-sm">
            <thead
              class="border-y border-gray-100 bg-gray-50 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400"
            >
              <tr>
                <th class="px-6 py-2.5 font-medium">
                  {{ t("admin.settings.emailPolicy.deliveries.event") }}
                </th>
                <th class="px-4 py-2.5 font-medium">
                  {{ t("admin.settings.emailPolicy.deliveries.recipient") }}
                </th>
                <th class="px-4 py-2.5 font-medium">
                  {{ t("admin.settings.emailPolicy.deliveries.source") }}
                </th>
                <th class="px-4 py-2.5 font-medium">
                  {{ t("admin.settings.emailPolicy.deliveries.status") }}
                </th>
                <th class="px-4 py-2.5 font-medium">
                  {{ t("admin.settings.emailPolicy.deliveries.updatedAt") }}
                </th>
                <th class="w-12 px-4 py-2.5"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="delivery in deliveries"
                :key="delivery.id"
                class="text-gray-700 dark:text-gray-300"
              >
                <td class="max-w-56 px-6 py-3">
                  <div class="truncate font-medium" :title="delivery.event">
                    {{ delivery.event }}
                  </div>
                  <div
                    v-if="delivery.last_error"
                    class="mt-1 truncate text-xs text-red-600 dark:text-red-400"
                    :title="delivery.last_error"
                  >
                    {{ delivery.last_error }}
                  </div>
                </td>
                <td class="px-4 py-3 font-mono text-xs">
                  {{ delivery.recipient }}
                </td>
                <td class="max-w-48 px-4 py-3">
                  <div
                    class="truncate"
                    :title="`${delivery.source_type}:${delivery.source_id}`"
                  >
                    {{ delivery.source_type }}:{{ delivery.source_id }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span
                    :class="deliveryStatusClass(delivery.status)"
                    class="inline-flex rounded px-2 py-1 text-xs font-medium"
                  >
                    {{ deliveryStatusLabel(delivery.status) }}
                  </span>
                  <div
                    v-if="delivery.attempt_count"
                    class="mt-1 text-xs text-gray-400"
                  >
                    {{ delivery.attempt_count }}/{{ delivery.max_attempts }}
                  </div>
                </td>
                <td
                  class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400"
                >
                  {{ formatDateTime(delivery.updated_at) }}
                </td>
                <td class="px-4 py-3">
                  <button
                    v-if="isRetryable(delivery)"
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :disabled="retryingDeliveryID === delivery.id"
                    :title="t('admin.settings.emailPolicy.deliveries.retry')"
                    @click="retryDelivery(delivery.id)"
                  >
                    <Icon
                      name="refresh"
                      size="xs"
                      :class="
                        retryingDeliveryID === delivery.id ? 'animate-spin' : ''
                      "
                    />
                  </button>
                </td>
              </tr>
              <tr v-if="!deliveriesLoading && deliveries.length === 0">
                <td
                  colspan="6"
                  class="px-6 py-8 text-center text-sm text-gray-400"
                >
                  {{ t("admin.settings.emailPolicy.deliveries.empty") }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  NotificationEmailDelivery,
  NotificationEmailDeliveryStatus,
  NotificationEmailChannelPolicy,
  NotificationEmailFeatureSettings,
  NotificationEmailPolicy,
  NotificationEmailRecipientGroup,
} from "@/api/admin/settings";
import Toggle from "@/components/common/Toggle.vue";
import Icon from "@/components/icons/Icon.vue";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import { formatDateTime } from "@/utils/format";

const { t } = useI18n();
const appStore = useAppStore();
const loading = ref(false);
const saving = ref(false);
const policy = ref<NotificationEmailPolicy | null>(null);
const deliveries = ref<NotificationEmailDelivery[]>([]);
const deliveriesLoading = ref(false);
const deliveryStatus = ref("");
const retryingDeliveryID = ref<number | null>(null);
const bulkAdminEmail = ref("");
const deliveryStatuses: NotificationEmailDeliveryStatus[] = [
  "pending",
  "processing",
  "retry_wait",
  "sent",
  "failed",
  "suppressed",
];
const systemRequiredChannelIDs = new Set([
  "auth_verification",
  "password_reset",
]);

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

const editableChannels = computed(() =>
  (policy.value?.channels || []).filter(
    (channel) => !systemRequiredChannelIDs.has(channel.id),
  ),
);

const managedRecipientGroups = computed(() => {
  if (!policy.value) return [];
  const referenced = new Set(
    policy.value.channels
      .filter((channel) => channel.recipient_kind === "group")
      .map((channel) => channel.recipient_group || "")
      .filter(Boolean),
  );
  return policy.value.recipient_groups.filter((group) =>
    referenced.has(group.id),
  );
});

const selectableRecipientGroups = computed(() => managedRecipientGroups.value);

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
    return channel.id === "balance"
      ? t("admin.settings.emailPolicy.balanceRecipientSummary")
      : t("admin.settings.emailPolicy.userPrimaryRecipientSummary");
  }
  return t("admin.settings.emailPolicy.explicitRecipientSummary");
}

function addMember(group: NotificationEmailRecipientGroup): void {
  group.members.push({ email: "", enabled: true, status: "admin_trusted" });
}

function isValidEmailAddress(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

function addAdminEmailToAllGroups(): void {
  const email = bulkAdminEmail.value.trim().toLowerCase();
  if (!isValidEmailAddress(email)) {
    appStore.showError(t("admin.settings.emailPolicy.invalidEmail"));
    return;
  }

  const groups = managedRecipientGroups.value;
  const fullGroup = groups.find((group) => {
    const exists = group.members.some(
      (member) => member.email.trim().toLowerCase() === email,
    );
    return (
      !exists &&
      group.members.filter((member) => member.email.trim() !== "").length >= 20
    );
  });
  if (fullGroup) {
    appStore.showError(
      t("admin.settings.emailPolicy.recipientGroupFull", {
        group: groupLabel(fullGroup.id),
      }),
    );
    return;
  }

  for (const group of groups) {
    const existing = group.members.find(
      (member) => member.email.trim().toLowerCase() === email,
    );
    if (existing) {
      existing.enabled = true;
      if (existing.status === "legacy_unverified") {
        existing.status = "admin_trusted";
      }
      continue;
    }
    group.members.push({ email, enabled: true, status: "admin_trusted" });
  }
  bulkAdminEmail.value = "";
  appStore.showSuccess(t("admin.settings.emailPolicy.addedToAllGroups"));
}

function defaultFeatureSettings(): NotificationEmailFeatureSettings {
  return {
    subscription_expiry_enabled: true,
    balance_low_enabled: false,
    balance_low_threshold: 0,
    balance_low_recharge_url: "",
  };
}

function normalizePolicyResponse(
  value: NotificationEmailPolicy,
): NotificationEmailPolicy {
  return {
    ...value,
    channels: Array.isArray(value.channels)
      ? value.channels.map((channel) => ({
          ...channel,
          events: Array.isArray(channel.events) ? [...channel.events] : [],
        }))
      : [],
    recipient_groups: Array.isArray(value.recipient_groups)
      ? value.recipient_groups.map((group) => ({
          ...group,
          members: Array.isArray(group.members) ? [...group.members] : [],
        }))
      : [],
    feature_settings: value.feature_settings
      ? { ...value.feature_settings }
      : defaultFeatureSettings(),
  };
}

async function loadPolicy(): Promise<void> {
  loading.value = true;
  try {
    policy.value = normalizePolicyResponse(
      await adminAPI.settings.getNotificationEmailPolicy(),
    );
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    loading.value = false;
  }
}

async function savePolicy(): Promise<void> {
  if (!policy.value) return;
  const balanceChannel = policy.value.channels.find(
    (channel) => channel.id === "balance",
  );
  if (
    balanceChannel?.enabled &&
    !balanceChannel.include_user_primary &&
    !balanceChannel.include_verified_additional
  ) {
    appStore.showError(
      t("admin.settings.emailPolicy.balanceRecipientRequired"),
    );
    return;
  }
  if (
    policy.value.feature_settings.balance_low_enabled &&
    (!Number.isFinite(policy.value.feature_settings.balance_low_threshold) ||
      policy.value.feature_settings.balance_low_threshold <= 0)
  ) {
    appStore.showError(
      t("admin.settings.emailPolicy.balanceLow.thresholdRequired"),
    );
    return;
  }
  const rechargeURL = policy.value.feature_settings.balance_low_recharge_url;
  if (rechargeURL) {
    try {
      const parsed = new URL(rechargeURL);
      if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
        throw new Error("unsupported protocol");
      }
    } catch {
      appStore.showError(
        t("admin.settings.emailPolicy.balanceLow.invalidRechargeURL"),
      );
      return;
    }
  }

  for (const channel of policy.value.channels) {
    if (systemRequiredChannelIDs.has(channel.id)) {
      channel.enabled = true;
    }
    if (channel.recipient_kind === "user" && channel.id !== "balance") {
      channel.include_user_primary = true;
      channel.include_verified_additional = false;
    }
  }

  saving.value = true;
  try {
    policy.value = normalizePolicyResponse(
      await adminAPI.settings.updateNotificationEmailPolicy({
        channels: policy.value.channels,
        recipient_groups: policy.value.recipient_groups,
        feature_settings: policy.value.feature_settings,
      }),
    );
    appStore.showSuccess(t("admin.settings.emailPolicy.saveSuccess"));
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    saving.value = false;
  }
}

async function loadDeliveries(): Promise<void> {
  deliveriesLoading.value = true;
  try {
    const page = await adminAPI.settings.getNotificationEmailDeliveries({
      page: 1,
      page_size: 20,
      status: deliveryStatus.value || undefined,
    });
    deliveries.value = page.items;
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    deliveriesLoading.value = false;
  }
}

function deliveryStatusLabel(status: string): string {
  return t(`admin.settings.emailPolicy.deliveries.statuses.${status}`);
}

function deliveryStatusClass(status: NotificationEmailDeliveryStatus): string {
  if (status === "sent")
    return "bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400";
  if (status === "failed")
    return "bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-400";
  if (status === "suppressed")
    return "bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300";
  if (status === "retry_wait")
    return "bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400";
  return "bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400";
}

function isRetryable(delivery: NotificationEmailDelivery): boolean {
  return (
    (delivery.status === "failed" || delivery.status === "retry_wait") &&
    (
      ["transport", "internal", "configuration", "template"] as string[]
    ).includes(delivery.last_error_category || "")
  );
}

async function retryDelivery(id: number): Promise<void> {
  retryingDeliveryID.value = id;
  try {
    await adminAPI.settings.retryNotificationEmailDelivery(id);
    await loadDeliveries();
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  } finally {
    retryingDeliveryID.value = null;
  }
}

onMounted(() => {
  void loadPolicy();
  void loadDeliveries();
});
</script>
