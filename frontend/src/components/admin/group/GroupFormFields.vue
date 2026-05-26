<template>
  <div class="space-y-5">
    <!-- Name -->
    <div>
      <label class="input-label">{{ t("admin.groups.form.name") }}</label>
      <input
        v-model="formData.name"
        type="text"
        required
        class="input"
        :placeholder="
          mode === 'create' ? t('admin.groups.enterGroupName') : undefined
        "
        :data-tour="
          mode === 'create' ? 'group-form-name' : 'edit-group-form-name'
        "
      />
    </div>

    <!-- Description -->
    <div>
      <label class="input-label">{{
        t("admin.groups.form.description")
      }}</label>
      <textarea
        v-model="formData.description"
        rows="3"
        class="input"
        :placeholder="
          mode === 'create' ? t('admin.groups.optionalDescription') : undefined
        "
      ></textarea>
    </div>

    <!-- Gateway Type -->
    <div>
      <label class="input-label">{{
        t("admin.groups.form.gatewayType")
      }}</label>
      <Select
        v-model="formData.platform"
        :options="platformOptions"
        :disabled="mode === 'edit'"
        data-tour="group-form-platform"
        @change="onPlatformChange"
      />
      <p class="input-hint">
        {{
          mode === 'edit'
            ? t("admin.groups.gatewayTypeNotEditable")
            : t("admin.groups.gatewayTypeHint")
        }}
      </p>
    </div>

    <!-- Copy accounts from groups -->
    <div v-if="filteredCopyGroupOptions.length > 0">
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.copyAccounts.title") }}
        </label>
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="text-xs leading-relaxed text-gray-300">
                {{
                  mode === 'create'
                    ? t("admin.groups.copyAccounts.tooltip")
                    : t("admin.groups.copyAccounts.tooltipEdit")
                }}
              </p>
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <!-- Selected group tags -->
      <div
        v-if="formData.copy_accounts_from_group_ids.length > 0"
        class="mb-2 flex flex-wrap gap-1.5"
      >
        <span
          v-for="groupId in formData.copy_accounts_from_group_ids"
          :key="groupId"
          class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
        >
          {{
            filteredCopyGroupOptions.find((o) => o.value === groupId)?.label ||
            `#${groupId}`
          }}
          <button
            type="button"
            @click="removeCopyGroup(groupId)"
            class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
          >
            <Icon name="x" size="xs" />
          </button>
        </span>
      </div>
      <!-- Group selection dropdown -->
      <select class="input" @change="addCopyGroup">
        <option value="">
          {{ t("admin.groups.copyAccounts.selectPlaceholder") }}
        </option>
        <option
          v-for="opt in filteredCopyGroupOptions"
          :key="opt.value"
          :value="opt.value"
          :disabled="
            formData.copy_accounts_from_group_ids.includes(opt.value)
          "
        >
          {{ opt.label }}
        </option>
      </select>
      <p class="input-hint">
        {{
          mode === 'create'
            ? t("admin.groups.copyAccounts.hint")
            : t("admin.groups.copyAccounts.hintEdit")
        }}
      </p>
    </div>

    <!-- Rate multiplier -->
    <div>
      <label class="input-label">{{
        t("admin.groups.form.rateMultiplier")
      }}</label>
      <input
        v-model.number="formData.rate_multiplier"
        type="number"
        step="0.001"
        min="0.001"
        required
        class="input"
        data-tour="group-form-multiplier"
      />
      <p v-if="mode === 'create'" class="input-hint">
        {{ t("admin.groups.rateMultiplierHint") }}
      </p>
    </div>

    <!-- RPM limit -->
    <div>
      <label class="input-label">{{
        t("admin.groups.form.rpmLimit")
      }}</label>
      <input
        v-model.number="formData.rpm_limit"
        type="number"
        min="0"
        step="1"
        class="input"
        :placeholder="t('admin.groups.form.rpmLimitPlaceholder')"
      />
      <p class="input-hint">{{ t("admin.groups.form.rpmLimitHint") }}</p>
    </div>

    <!-- Exclusive toggle (hidden when subscription type) -->
    <div
      v-if="formData.subscription_type !== 'subscription'"
      :data-tour="mode === 'create' ? 'group-form-exclusive' : undefined"
    >
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.form.exclusive") }}
        </label>
        <!-- Help Tooltip -->
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <!-- Tooltip Popover -->
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="mb-2 text-xs font-medium">
                {{ t("admin.groups.exclusiveTooltip.title") }}
              </p>
              <p class="mb-2 text-xs leading-relaxed text-gray-300">
                {{ t("admin.groups.exclusiveTooltip.description") }}
              </p>
              <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                <p class="text-xs leading-relaxed text-gray-300">
                  <span
                    class="inline-flex items-center gap-1 text-primary-400"
                    ><Icon name="lightbulb" size="xs" />
                    {{ t("admin.groups.exclusiveTooltip.example") }}</span
                  >
                  {{ t("admin.groups.exclusiveTooltip.exampleContent") }}
                </p>
              </div>
              <!-- Arrow -->
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          @click="formData.is_exclusive = !formData.is_exclusive"
          :class="[
            'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
            formData.is_exclusive
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600',
          ]"
        >
          <span
            :class="[
              'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
              formData.is_exclusive ? 'translate-x-6' : 'translate-x-1',
            ]"
          />
        </button>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{
            formData.is_exclusive
              ? t("admin.groups.exclusive")
              : t("admin.groups.public")
          }}
        </span>
      </div>
    </div>

    <!-- Status (edit only) -->
    <div v-if="mode === 'edit'">
      <label class="input-label">{{ t("admin.groups.form.status") }}</label>
      <Select v-model="formData.status" :options="editStatusOptions" />
    </div>

    <!-- Subscription Configuration -->
    <div class="mt-4 border-t pt-4">
      <div>
        <label class="input-label">{{
          t("admin.groups.subscription.type")
        }}</label>
        <Select
          v-model="formData.subscription_type"
          :options="subscriptionTypeOptions"
          :disabled="mode === 'edit'"
        />
        <p class="input-hint">
          {{
            mode === 'edit'
              ? t("admin.groups.subscription.typeNotEditable")
              : t("admin.groups.subscription.typeHint")
          }}
        </p>
      </div>

      <!-- Subscription limits (only when subscription type is selected) -->
      <div
        v-if="formData.subscription_type === 'subscription'"
        class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
      >
        <div>
          <label class="input-label">{{
            t("admin.groups.subscription.dailyLimit")
          }}</label>
          <input
            v-model.number="formData.daily_limit_usd"
            type="number"
            step="0.01"
            min="0"
            class="input"
            :placeholder="t('admin.groups.subscription.noLimit')"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.subscription.weeklyLimit")
          }}</label>
          <input
            v-model.number="formData.weekly_limit_usd"
            type="number"
            step="0.01"
            min="0"
            class="input"
            :placeholder="t('admin.groups.subscription.noLimit')"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.subscription.monthlyLimit")
          }}</label>
          <input
            v-model.number="formData.monthly_limit_usd"
            type="number"
            step="0.01"
            min="0"
            class="input"
            :placeholder="t('admin.groups.subscription.noLimit')"
          />
        </div>
      </div>
    </div>

    <!-- Platform-specific config -->
    <component
      v-if="configComponent"
      :is="configComponent"
      ref="configRef"
      :mode="mode"
      :platform="formData.platform"
      :form-data="formData"
      :groups="groups"
      :editing-group-id="mode === 'edit' ? editingGroupId : undefined"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Select } from "@sub2api/plugin-sdk";
import Icon from "@/components/icons/Icon.vue";
import type { AdminGroup } from "@/types";
import type { Component } from "vue";
import type { GroupConfigExposed } from "./config/types";

const props = defineProps<{
  mode: "create" | "edit";
  formData: Record<string, any>;
  groups: AdminGroup[];
  platformOptions: { value: string; label: string }[];
  configComponent: Component | null;
  editingGroupId?: number | null;
}>();

const { t } = useI18n();

const configRef = ref<GroupConfigExposed | null>(null);

const editStatusOptions = computed(() => [
  { value: "active", label: t("admin.accounts.status.active") },
  { value: "inactive", label: t("admin.accounts.status.inactive") },
]);

const subscriptionTypeOptions = computed(() => [
  { value: "standard", label: t("admin.groups.subscription.standard") },
  {
    value: "subscription",
    label: t("admin.groups.subscription.subscription"),
  },
]);

/** Groups eligible as copy-accounts source (same platform, has accounts, excludes self in edit) */
const filteredCopyGroupOptions = computed(() => {
  const eligibleGroups = props.groups.filter((g) => {
    if (g.platform !== props.formData.platform) return false;
    if ((g.account_count || 0) <= 0) return false;
    if (props.mode === "edit" && g.id === props.editingGroupId) return false;
    return true;
  });
  return eligibleGroups.map((g) => ({
    value: g.id,
    label: `${g.name} (${g.account_count || 0} ${t("admin.groups.copyAccounts.accountUnit")})`,
  }));
});

const onPlatformChange = () => {
  if (props.mode === "create") {
    props.formData.copy_accounts_from_group_ids = [];
  }
};

const addCopyGroup = (e: Event) => {
  const val = Number((e.target as HTMLSelectElement).value);
  if (val && !props.formData.copy_accounts_from_group_ids.includes(val)) {
    props.formData.copy_accounts_from_group_ids.push(val);
  }
  (e.target as HTMLSelectElement).value = "";
};

const removeCopyGroup = (groupId: number) => {
  props.formData.copy_accounts_from_group_ids =
    props.formData.copy_accounts_from_group_ids.filter(
      (id: number) => id !== groupId,
    );
};

// --- Public API (delegates to platform config component) ---

const getRoutingRulesApiFormat = () =>
  configRef.value?.getRoutingRulesApiFormat?.() ?? null;

const loadRoutingRules = async (
  apiFormat: Record<string, number[]> | null,
) => {
  await configRef.value?.loadRoutingRules?.(apiFormat);
};

const resetRoutingRules = () => {
  configRef.value?.resetRoutingRules?.();
};

defineExpose({
  getRoutingRulesApiFormat,
  loadRoutingRules,
  resetRoutingRules,
});
</script>
