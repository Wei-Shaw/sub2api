<template>
  <AppLayout>
    <div class="space-y-6">
      <header>
        <h1 class="text-xl font-semibold">
          {{ t("admin.modelDowngradeGuard.title") }}
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">
          {{ t("admin.modelDowngradeGuard.description") }}
        </p>
      </header>

      <div class="card">
        <div class="space-y-5 p-6">
          <!-- Loading State -->
          <div
            v-if="settingsLoading"
            class="flex items-center gap-2 text-gray-500"
          >
            <div
              class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
            ></div>
            {{ t("common.loading") }}
          </div>

          <template v-else>
            <!-- Enable Guard -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t("admin.modelDowngradeGuard.enabled")
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.enabledHint") }}
                </p>
              </div>
              <Toggle v-model="form.enabled" />
            </div>

            <!-- Settings - Only show when enabled -->
            <div
              v-if="form.enabled"
              class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <!-- Action -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.action") }}
                </label>
                <select
                  v-model="form.action"
                  class="input w-72"
                  data-testid="model-downgrade-guard-action"
                >
                  <option value="model_block">
                    {{ t("admin.modelDowngradeGuard.actionModelBlock") }}
                  </option>
                  <option value="temp_unsched">
                    {{ t("admin.modelDowngradeGuard.actionTempUnsched") }}
                  </option>
                  <option value="none">
                    {{ t("admin.modelDowngradeGuard.actionNone") }}
                  </option>
                </select>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.actionHint") }}
                </p>
              </div>

              <!-- Downgrade Pairs -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.pairs") }}
                </label>
                <div class="space-y-2">
                  <div
                    v-for="(pair, index) in form.pairs"
                    :key="index"
                    class="flex flex-wrap items-center gap-2"
                  >
                    <input
                      v-model="pair.sent_model"
                      type="text"
                      class="input w-56"
                      :aria-label="t('admin.modelDowngradeGuard.pairSentModel')"
                      :placeholder="
                        t('admin.modelDowngradeGuard.pairSentModelPlaceholder')
                      "
                    />
                    <span class="text-gray-400">&#8594;</span>
                    <input
                      v-model="pair.response_model"
                      type="text"
                      class="input w-56"
                      :aria-label="
                        t('admin.modelDowngradeGuard.pairResponseModel')
                      "
                      :placeholder="
                        t(
                          'admin.modelDowngradeGuard.pairResponseModelPlaceholder',
                        )
                      "
                    />
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm"
                      @click="removePair(index)"
                    >
                      {{ t("admin.modelDowngradeGuard.removePair") }}
                    </button>
                  </div>
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm mt-2"
                  data-testid="model-downgrade-guard-add-pair"
                  @click="addPair"
                >
                  {{ t("admin.modelDowngradeGuard.addPair") }}
                </button>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.pairsHint") }}
                </p>
              </div>

              <!-- Threshold Count -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.thresholdCount") }}
                </label>
                <input
                  v-model.number="form.threshold_count"
                  data-testid="model-downgrade-guard-threshold-count"
                  type="number"
                  min="1"
                  max="100"
                  class="input w-32"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.thresholdCountHint") }}
                </p>
              </div>

              <!-- Threshold Window Minutes -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.thresholdWindowMinutes") }}
                </label>
                <input
                  v-model.number="form.threshold_window_minutes"
                  type="number"
                  min="1"
                  max="1440"
                  class="input w-32"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.thresholdWindowMinutesHint") }}
                </p>
              </div>

              <!-- Block Hours -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.blockHours") }}
                </label>
                <input
                  v-model.number="form.block_hours"
                  type="number"
                  min="1"
                  max="72"
                  class="input w-32"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.blockHoursHint") }}
                </p>
              </div>

              <!-- Max Blocked Ratio -->
              <div v-if="form.action !== 'none'">
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.modelDowngradeGuard.maxBlockedRatio") }}
                </label>
                <input
                  v-model.number="form.max_blocked_ratio"
                  type="number"
                  min="0"
                  max="1"
                  step="0.05"
                  class="input w-32"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.modelDowngradeGuard.maxBlockedRatioHint") }}
                </p>
              </div>
            </div>

            <!-- Save Button -->
            <div
              class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <button
                type="button"
                data-testid="model-downgrade-guard-save"
                :disabled="settingsSaving"
                class="btn btn-primary btn-sm"
                @click="saveSettings"
              >
                <svg
                  v-if="settingsSaving"
                  class="mr-1 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                {{ settingsSaving ? t("common.saving") : t("common.save") }}
              </button>
            </div>
          </template>
        </div>
      </div>

      <!-- Currently Blocked Accounts -->
      <div class="card">
        <div class="space-y-4 p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="font-medium text-gray-900 dark:text-white">
                {{ t("admin.modelDowngradeGuard.blocked.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.modelDowngradeGuard.blocked.subtitle") }}
              </p>
              <p
                class="mt-1 text-sm text-gray-500 dark:text-gray-400"
                data-testid="model-downgrade-guard-blocked-summary"
              >
                {{ blockedSummaryText }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm shrink-0"
              data-testid="model-downgrade-guard-blocked-refresh"
              :disabled="blockedLoading"
              @click="loadBlockedAccounts"
            >
              {{ t("admin.modelDowngradeGuard.blocked.refresh") }}
            </button>
          </div>

          <div
            v-if="blockedLoading && !blockedLoaded"
            class="flex items-center gap-2 text-gray-500"
          >
            <div
              class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
            ></div>
            {{ t("common.loading") }}
          </div>

          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <caption class="sr-only">
                {{
                  t("admin.modelDowngradeGuard.blocked.title")
                }}
              </caption>
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th
                    v-for="key in blockedColumns"
                    :key="key"
                    scope="col"
                    class="whitespace-nowrap p-3 font-medium"
                  >
                    {{ t(`admin.modelDowngradeGuard.blocked.${key}`) }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in blockedAccounts"
                  :key="blockedRowKey(item)"
                  class="border-t border-gray-200 align-top dark:border-dark-700"
                  data-testid="model-downgrade-guard-blocked-row"
                >
                  <td class="min-w-44 p-3">
                    <router-link
                      class="text-primary-600 hover:underline dark:text-primary-400"
                      to="/admin/accounts"
                    >
                      #{{ item.account_id }} {{ item.account_name || "—" }}
                    </router-link>
                  </td>
                  <td class="whitespace-nowrap p-3">
                    <span
                      class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                      :class="statusBadgeClass(item)"
                      :title="statusTitle(item)"
                      data-testid="model-downgrade-guard-blocked-status"
                    >
                      {{ statusText(item) }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap p-3">
                    {{ scopeText(item) }}
                  </td>
                  <td class="whitespace-nowrap p-3">
                    {{ downgradeText(item) }}
                  </td>
                  <td class="whitespace-nowrap p-3 tabular-nums">
                    {{
                      t("admin.modelDowngradeGuard.blocked.hits", {
                        count: item.trigger_count,
                        window: item.trigger_window_minutes,
                      })
                    }}
                  </td>
                  <td class="whitespace-nowrap p-3 tabular-nums">
                    {{ dateTime(item.triggered_at) }}
                  </td>
                  <td class="whitespace-nowrap p-3 tabular-nums">
                    {{ dateTime(item.until) }}
                  </td>
                  <td
                    class="whitespace-nowrap p-3 tabular-nums"
                    :class="
                      isEndingSoon(item)
                        ? 'text-amber-600 dark:text-amber-400'
                        : ''
                    "
                  >
                    {{ remainingText(item) }}
                  </td>
                  <td class="whitespace-nowrap p-3">
                    <div class="flex items-center gap-2">
                      <!-- 观察行没有真的被限制过，能做的是「转正」或「清掉记录」。 -->
                      <template v-if="isObserved(item)">
                        <button
                          type="button"
                          class="btn btn-secondary btn-sm"
                          data-testid="model-downgrade-guard-blocked-apply"
                          :disabled="pendingKey === blockedRowKey(item)"
                          @click="applyBlockNow(item)"
                        >
                          {{ t("admin.modelDowngradeGuard.blocked.apply") }}
                        </button>
                        <button
                          type="button"
                          class="btn btn-secondary btn-sm"
                          data-testid="model-downgrade-guard-blocked-clear"
                          :disabled="pendingKey === blockedRowKey(item)"
                          @click="clearObserved(item)"
                        >
                          {{ t("admin.modelDowngradeGuard.blocked.clear") }}
                        </button>
                      </template>
                      <button
                        v-else
                        type="button"
                        class="btn btn-secondary btn-sm"
                        data-testid="model-downgrade-guard-blocked-restore"
                        :disabled="pendingKey === blockedRowKey(item)"
                        @click="restoreAccount(item)"
                      >
                        {{ t("admin.modelDowngradeGuard.blocked.restore") }}
                      </button>
                    </div>
                  </td>
                </tr>
                <tr v-if="!blockedAccounts.length">
                  <td
                    :colspan="blockedColumns.length"
                    class="p-8 text-center text-gray-500 dark:text-gray-400"
                  >
                    {{ t("admin.modelDowngradeGuard.blocked.empty") }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";

import { adminAPI } from "@/api";
import type {
  ModelDowngradeBlockedAccount,
  ModelDowngradeGuardAction,
  ModelDowngradeGuardSettings,
} from "@/api/admin/settings";
import Toggle from "@/components/common/Toggle.vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import { useAppStore } from "@/stores";
import {
  extractApiErrorCode,
  extractApiErrorMessage,
  extractApiErrorMetadata,
} from "@/utils/apiError";

const { t } = useI18n();
const appStore = useAppStore();

// ==================== Guard Settings ====================

const settingsLoading = ref(true);
const settingsSaving = ref(false);
const form = reactive({
  enabled: false,
  action: "model_block" as ModelDowngradeGuardAction,
  pairs: [] as { sent_model: string; response_model: string }[],
  threshold_count: 5,
  threshold_window_minutes: 30,
  block_hours: 24,
  max_blocked_ratio: 0.3,
});

function addPair() {
  form.pairs.push({ sent_model: "", response_model: "" });
}

function removePair(index: number) {
  form.pairs.splice(index, 1);
}

function applySettings(settings: ModelDowngradeGuardSettings) {
  Object.assign(form, {
    enabled: settings.enabled,
    action: settings.action,
    threshold_count: settings.threshold_count,
    threshold_window_minutes: settings.threshold_window_minutes,
    block_hours: settings.block_hours,
    max_blocked_ratio: settings.max_blocked_ratio,
  });
  form.pairs = (settings.pairs ?? []).map((pair) => ({ ...pair }));
}

async function loadSettings() {
  settingsLoading.value = true;
  try {
    applySettings(await adminAPI.settings.getModelDowngradeGuardSettings());
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.modelDowngradeGuard.loadFailed")),
    );
  } finally {
    settingsLoading.value = false;
  }
}

async function saveSettings() {
  settingsSaving.value = true;
  try {
    const updated = await adminAPI.settings.updateModelDowngradeGuardSettings({
      enabled: form.enabled,
      action: form.action,
      pairs: form.pairs.map((pair) => ({ ...pair })),
      threshold_count: form.threshold_count,
      threshold_window_minutes: form.threshold_window_minutes,
      block_hours: form.block_hours,
      max_blocked_ratio: form.max_blocked_ratio,
    });
    applySettings(updated);
    appStore.showSuccess(t("admin.modelDowngradeGuard.saved"));
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.modelDowngradeGuard.saveFailed")),
    );
  } finally {
    settingsSaving.value = false;
  }
}

// ==================== Currently Blocked Accounts ====================

const BLOCKED_REFRESH_MS = 30_000;
const CLOCK_TICK_MS = 60_000;
const HOUR_MS = 3_600_000;
const MINUTE_MS = 60_000;

const RATIO_CAPPED_REASON = "MODEL_DOWNGRADE_BLOCK_RATIO_CAPPED";
const ALREADY_BLOCKED_REASON = "MODEL_DOWNGRADE_BLOCK_ALREADY_ACTIVE";

const blockedColumns = [
  "columnAccount",
  "columnStatus",
  "columnScope",
  "columnDowngrade",
  "columnHits",
  "columnTriggeredAt",
  "columnUntil",
  "columnRemaining",
  "columnActions",
] as const;

const blockedAccounts = ref<ModelDowngradeBlockedAccount[]>([]);
const blockedSummary = reactive({
  blocked: 0,
  observed: 0,
  total_active: 0,
  max_blocked_ratio: 0,
});
const blockedLoading = ref(false);
const blockedLoaded = ref(false);
// pendingKey 标记「这一行正在执行某个操作」，提前恢复、立即处理、清除记录共用。
const pendingKey = ref<string | null>(null);
// 「剩余」列按分钟重算，独立于 30s 的列表轮询，避免整张表频繁重绘。
const now = ref(Date.now());

let blockedRefreshTimer: ReturnType<typeof setInterval> | null = null;
let clockTimer: ReturnType<typeof setInterval> | null = null;

const blockedSummaryText = computed(() => {
  const total = blockedSummary.total_active;
  // 占比只按真实受限算：观察记录并没有占用号池，混进分子会让安全阀的口径失真。
  const pct = total > 0 ? (blockedSummary.blocked / total) * 100 : 0;
  return t("admin.modelDowngradeGuard.blocked.summary", {
    blocked: blockedSummary.blocked,
    observed: blockedSummary.observed,
    total,
    pct: formatPercent(pct),
    max: formatPercent(blockedSummary.max_blocked_ratio * 100),
  });
});

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return (Math.round(value * 10) / 10).toString();
}

function isObserved(item: ModelDowngradeBlockedAccount): boolean {
  return item.status === "observed" || item.status === "ratio_capped";
}

function statusText(item: ModelDowngradeBlockedAccount): string {
  if (item.status === "observed") {
    return t("admin.modelDowngradeGuard.blocked.statusObserved");
  }
  if (item.status === "ratio_capped") {
    return t("admin.modelDowngradeGuard.blocked.statusRatioCapped");
  }
  return t("admin.modelDowngradeGuard.blocked.statusBlocked");
}

function statusBadgeClass(item: ModelDowngradeBlockedAccount): string {
  if (item.status === "observed") {
    return "bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300";
  }
  if (item.status === "ratio_capped") {
    return "bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300";
  }
  // 整账号比仅屏蔽模型更严重。
  return item.scope === "model"
    ? "bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300"
    : "bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300";
}

// 上限拦下的那一行 hover 时给出当时的分子分母，解释「为什么没处理」。
function statusTitle(item: ModelDowngradeBlockedAccount): string {
  if (item.status !== "ratio_capped") return "";
  return t("admin.modelDowngradeGuard.blocked.statusRatioCappedTitle", {
    blocked: item.blocked ?? 0,
    total: item.total ?? 0,
    max: formatPercent((item.max_blocked_ratio ?? 0) * 100),
  });
}

function rowModel(item: ModelDowngradeBlockedAccount): string {
  return item.model || item.sent_model || "";
}

function scopeText(item: ModelDowngradeBlockedAccount): string {
  if (isObserved(item)) {
    return t("admin.modelDowngradeGuard.blocked.scopeObserved", {
      model: rowModel(item) || "—",
    });
  }
  if (item.scope === "model") {
    return t("admin.modelDowngradeGuard.blocked.scopeModel", {
      model: rowModel(item) || "—",
    });
  }
  return t("admin.modelDowngradeGuard.blocked.scopeAccount");
}

function downgradeText(item: ModelDowngradeBlockedAccount): string {
  if (!item.sent_model && !item.response_model) return "—";
  return `${item.sent_model || "—"} → ${item.response_model || "—"}`;
}

function dateTime(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const pad = (n: number) => n.toString().padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    ` ${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

function remainingMs(item: ModelDowngradeBlockedAccount): number {
  const until = new Date(item.until).getTime();
  if (Number.isNaN(until)) return 0;
  return until - now.value;
}

function isEndingSoon(item: ModelDowngradeBlockedAccount): boolean {
  return remainingMs(item) < HOUR_MS;
}

function remainingText(item: ModelDowngradeBlockedAccount): string {
  const diff = remainingMs(item);
  if (diff <= 0) return t("admin.modelDowngradeGuard.blocked.remainingExpired");
  const hours = Math.floor(diff / HOUR_MS);
  const minutes = Math.floor((diff % HOUR_MS) / MINUTE_MS);
  if (hours > 0) {
    return t("admin.modelDowngradeGuard.blocked.remainingHoursMinutes", {
      hours,
      minutes,
    });
  }
  return t("admin.modelDowngradeGuard.blocked.remainingMinutes", {
    minutes: Math.max(1, minutes),
  });
}

async function loadBlockedAccounts() {
  blockedLoading.value = true;
  try {
    const result = await adminAPI.settings.getModelDowngradeGuardBlocked();
    blockedAccounts.value = result.items ?? [];
    Object.assign(blockedSummary, {
      blocked: result.summary?.blocked ?? 0,
      observed: result.summary?.observed ?? 0,
      total_active: result.summary?.total_active ?? 0,
      max_blocked_ratio: result.summary?.max_blocked_ratio ?? 0,
    });
    now.value = Date.now();
    blockedLoaded.value = true;
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.modelDowngradeGuard.blocked.loadFailed"),
      ),
    );
  } finally {
    blockedLoading.value = false;
  }
}

// blockedRowKey 同时充当表格 key 和「这一行正在处理中」的标记：
// 同一个账号可能同时出现整账号和仅模型两行，只按 account_id 会互相串台。
function blockedRowKey(item: ModelDowngradeBlockedAccount) {
  return `${item.account_id}-${item.scope}-${item.model ?? ""}`;
}

async function restoreAccount(item: ModelDowngradeBlockedAccount) {
  const confirmed = window.confirm(
    item.scope === "model"
      ? t("admin.modelDowngradeGuard.blocked.restoreConfirmModel", {
          id: item.account_id,
          name: item.account_name || "—",
          model: item.model ?? "",
        })
      : t("admin.modelDowngradeGuard.blocked.restoreConfirm", {
          id: item.account_id,
          name: item.account_name || "—",
        }),
  );
  if (!confirmed) return;

  pendingKey.value = blockedRowKey(item);
  try {
    // 守卫专用接口：只解除降级来源的那一条，不会连带清空账号上别的模型限流。
    await adminAPI.settings.releaseModelDowngradeGuardBlocked(
      item.account_id,
      item.scope,
      item.model,
    );
    appStore.showSuccess(t("admin.modelDowngradeGuard.blocked.restored"));
    await loadBlockedAccounts();
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.modelDowngradeGuard.blocked.restoreFailed"),
      ),
    );
  } finally {
    pendingKey.value = null;
  }
}

// 清除观察记录只删 Redis 里的那条记录，账号本身没被限制过，不需要恢复调度。
async function clearObserved(item: ModelDowngradeBlockedAccount) {
  const model = rowModel(item);
  const confirmed = window.confirm(
    t("admin.modelDowngradeGuard.blocked.clearConfirm", {
      id: item.account_id,
      name: item.account_name || "—",
      model,
    }),
  );
  if (!confirmed) return;

  pendingKey.value = blockedRowKey(item);
  try {
    await adminAPI.settings.releaseModelDowngradeGuardBlocked(
      item.account_id,
      "observed",
      model,
    );
    appStore.showSuccess(t("admin.modelDowngradeGuard.blocked.cleared"));
    await loadBlockedAccounts();
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.modelDowngradeGuard.blocked.clearFailed"),
      ),
    );
  } finally {
    pendingKey.value = null;
  }
}

// 「立即处理」把一条观察记录转正：动作按当前配置，观察模式下只屏蔽该模型。
// 比例上限仍然生效，被拦下时后端回 409 并带上分子分母。
async function applyBlockNow(item: ModelDowngradeBlockedAccount) {
  const model = rowModel(item);
  const confirmed = window.confirm(
    t("admin.modelDowngradeGuard.blocked.applyConfirm", {
      id: item.account_id,
      name: item.account_name || "—",
      model,
    }),
  );
  if (!confirmed) return;

  pendingKey.value = blockedRowKey(item);
  try {
    await adminAPI.settings.applyModelDowngradeGuardBlocked(
      item.account_id,
      model,
    );
    appStore.showSuccess(t("admin.modelDowngradeGuard.blocked.applied"));
    await loadBlockedAccounts();
  } catch (error: unknown) {
    appStore.showError(applyErrorMessage(error));
  } finally {
    pendingKey.value = null;
  }
}

// 两类 409 都不是「失败」：比例上限拦下要把当时的分子分母摊开说清楚，
// 已被限制则直接说明，其余情况回落到后端 message。
function applyErrorMessage(error: unknown): string {
  const reason = extractApiErrorCode(error);
  if (reason === RATIO_CAPPED_REASON) {
    const metadata = extractApiErrorMetadata(error);
    if (metadata) {
      const maxRatio = Number(metadata.max_blocked_ratio ?? 0);
      return t("admin.modelDowngradeGuard.blocked.applyRatioCapped", {
        blocked: metadata.blocked ?? "0",
        total: metadata.total ?? "0",
        max: formatPercent((Number.isFinite(maxRatio) ? maxRatio : 0) * 100),
      });
    }
  }
  if (reason === ALREADY_BLOCKED_REASON) {
    return t("admin.modelDowngradeGuard.blocked.applyAlreadyBlocked");
  }
  return extractApiErrorMessage(
    error,
    t("admin.modelDowngradeGuard.blocked.applyFailed"),
  );
}

onMounted(() => {
  loadSettings();
  loadBlockedAccounts();
  blockedRefreshTimer = setInterval(loadBlockedAccounts, BLOCKED_REFRESH_MS);
  clockTimer = setInterval(() => {
    now.value = Date.now();
  }, CLOCK_TICK_MS);
});

onUnmounted(() => {
  if (blockedRefreshTimer !== null) {
    clearInterval(blockedRefreshTimer);
    blockedRefreshTimer = null;
  }
  if (clockTimer !== null) {
    clearInterval(clockTimer);
    clockTimer = null;
  }
});
</script>
