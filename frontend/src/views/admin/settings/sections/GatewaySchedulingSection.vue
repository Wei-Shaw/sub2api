<template>
  <div v-show="activeTab === 'gateway'" class="space-y-6">
    <!-- Claude Code Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.claudeCode.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.claudeCode.description") }}
        </p>
      </div>
      <div class="p-6">
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.claudeCode.minVersion") }}
          </label>
          <input
            v-model="form.min_claude_code_version"
            type="text"
            class="input max-w-xs font-mono text-sm"
            :placeholder="
              t('admin.settings.claudeCode.minVersionPlaceholder')
            "
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.claudeCode.minVersionHint") }}
          </p>
        </div>
        <div class="mt-4">
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.claudeCode.maxVersion") }}
          </label>
          <input
            v-model="form.max_claude_code_version"
            type="text"
            class="input max-w-xs font-mono text-sm"
            :placeholder="
              t('admin.settings.claudeCode.maxVersionPlaceholder')
            "
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.claudeCode.maxVersionHint") }}
          </p>
        </div>
      </div>
    </div>

    <!-- Codex Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.gatewayForwarding.codexHardeningTitle") }}
        </h2>
      </div>
      <div class="p-6 space-y-4">
          <div>
            <h3 class="text-base font-semibold text-ink">
              {{ t("admin.settings.gatewayForwarding.codexClientRestrictionTitle") }}
            </h3>
            <p class="mt-1 text-sm text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.codexHardeningDesc") }}
            </p>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.gatewayForwarding.minCodexVersion") }}
              </label>
              <input
                v-model="form.min_codex_version"
                type="text"
                class="input w-full font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.minCodexVersionPlaceholder',
                  )
                "
              />
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.gatewayForwarding.maxCodexVersion") }}
              </label>
              <input
                v-model="form.max_codex_version"
                type="text"
                class="input w-full font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.maxCodexVersionPlaceholder',
                  )
                "
              />
            </div>
          </div>
          <p class="text-xs text-ink-secondary">
            {{ t("admin.settings.gatewayForwarding.codexVersionHint") }}
          </p>

          <div>
            <label class="block text-sm font-medium text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.codexFingerprintSignals") }}
            </label>
            <p class="mb-2 mt-1 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.codexFingerprintSignalsDesc") }}
            </p>
            <div
              v-for="(row, i) in codexFingerprintRows"
              :key="`codex-fp-${i}`"
              class="mb-2 flex items-center gap-2"
            >
              <select v-model="row.type" class="input w-32 text-sm">
                <option value="header_exact">{{ t("admin.settings.gatewayForwarding.codexFpTypeHeaderExact") }}</option>
                <option value="header_prefix">{{ t("admin.settings.gatewayForwarding.codexFpTypeHeaderPrefix") }}</option>
                <option value="body_path">{{ t("admin.settings.gatewayForwarding.codexFpTypeBodyPath") }}</option>
              </select>
              <input
                v-model="row.match"
                type="text"
                class="input flex-1 font-mono text-sm"
                :placeholder="t('admin.settings.gatewayForwarding.codexFpMatchPlaceholder')"
              />
              <label class="flex shrink-0 items-center gap-1 text-xs text-ink-secondary">
                <input v-model="row.required" type="checkbox" />
                {{ t("admin.settings.gatewayForwarding.codexFpRequired") }}
              </label>
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
                @click="removeCodexFingerprintRow(i)"
              >
                {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
              </button>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="addCodexFingerprintRow">
              {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
            </button>
            <p
              v-if="codexFingerprintNoRequired"
              class="mt-2 text-xs text-amber-600 dark:text-amber-500"
            >
              {{ t("admin.settings.gatewayForwarding.codexFingerprintNoRequiredWarn") }}
            </p>
          </div>

          <div class="flex items-center justify-between">
            <div class="pr-4">
              <label
                class="block text-sm font-medium text-ink-secondary"
              >
                {{
                  t("admin.settings.gatewayForwarding.codexAllowAppServer")
                }}
              </label>
              <p class="mt-1 text-xs text-ink-secondary">
                {{
                  t(
                    "admin.settings.gatewayForwarding.codexAllowAppServerDesc",
                  )
                }}
              </p>
            </div>
            <Toggle
              v-model="form.codex_cli_only_allow_app_server_clients"
            />
          </div>

          <div>
            <label
              class="block text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.gatewayForwarding.codexBlacklist") }}
            </label>
            <p class="mb-2 mt-1 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.codexBlacklistDesc") }}
            </p>
            <div
              v-for="(row, i) in codexBlacklistRows"
              :key="`codex-bl-${i}`"
              class="mb-2 flex gap-2"
            >
              <input
                v-model="row.originator"
                type="text"
                class="input w-1/3 font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                  )
                "
              />
              <input
                v-model="row.uaContains"
                type="text"
                class="input flex-1 font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                  )
                "
              />
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
                @click="removeCodexBlacklistRow(i)"
              >
                {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
              </button>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addCodexBlacklistRow"
            >
              {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
            </button>
          </div>

          <div>
            <label
              class="block text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.gatewayForwarding.codexWhitelist") }}
            </label>
            <p class="mb-2 mt-1 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.codexWhitelistDesc") }}
            </p>
            <div
              v-for="(row, i) in codexWhitelistRows"
              :key="`codex-wl-${i}`"
              class="mb-2 flex gap-2"
            >
              <input
                v-model="row.originator"
                type="text"
                class="input w-1/3 font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                  )
                "
              />
              <input
                v-model="row.uaContains"
                type="text"
                class="input flex-1 font-mono text-sm"
                :placeholder="
                  t(
                    'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                  )
                "
              />
              <label
                class="flex shrink-0 items-center gap-1 text-xs text-ink-secondary"
                :title="
                  t(
                    'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprintTooltip',
                  )
                "
              >
                <input
                  v-model="row.skipEngineFingerprint"
                  type="checkbox"
                />
                {{
                  t(
                    'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprint',
                  )
                }}
              </label>
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
                @click="removeCodexWhitelistRow(i)"
              >
                {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
              </button>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addCodexWhitelistRow"
            >
              {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
            </button>
          </div>
      </div>
    </div>

    <!-- Upstream Billing Probe Settings -->
    <div class="card" data-testid="upstream-billing-probe-settings">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.upstreamBillingProbe.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.upstreamBillingProbe.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div
          v-if="upstreamBillingProbeLoading"
          class="flex items-center gap-2 text-ink-secondary"
        >
          <div
            class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
          ></div>
          {{ t("common.loading") }}
        </div>

        <template v-else>
          <div class="flex items-center justify-between gap-4">
            <div>
              <label class="font-medium text-ink">
                {{ t("admin.settings.upstreamBillingProbe.enabled") }}
              </label>
              <p class="text-sm text-ink-secondary">
                {{ t("admin.settings.upstreamBillingProbe.enabledHint") }}
              </p>
            </div>
            <Toggle
              v-model="upstreamBillingProbeForm.enabled"
              :aria-label="t('admin.settings.upstreamBillingProbe.enabled')"
              data-testid="upstream-billing-probe-enabled"
            />
          </div>

          <div
            v-if="upstreamBillingProbeForm.enabled"
            class="border-t border-line-subtle pt-4"
          >
            <label
              class="mb-2 block text-sm font-medium text-ink-secondary"
              for="upstream-billing-probe-interval"
            >
              {{ t("admin.settings.upstreamBillingProbe.intervalMinutes") }}
            </label>
            <input
              id="upstream-billing-probe-interval"
              v-model.number="upstreamBillingProbeForm.interval_minutes"
              type="number"
              min="5"
              max="1440"
              class="input w-32"
              data-testid="upstream-billing-probe-interval"
              @keydown.enter.prevent="saveUpstreamBillingProbeSettings"
            />
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.upstreamBillingProbe.intervalHint") }}
            </p>
          </div>

          <div
            class="flex justify-end border-t border-line-subtle pt-4"
          >
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="upstreamBillingProbeSaving"
              data-testid="upstream-billing-probe-save"
              @click="saveUpstreamBillingProbeSettings"
            >
              {{
                upstreamBillingProbeSaving
                  ? t("common.saving")
                  : t("common.save")
              }}
            </button>
          </div>
        </template>
      </div>
    </div>

    <!-- Ollama Cloud Usage Settings -->
    <div class="card" data-testid="ollama-cloud-usage-global-settings">
      <div class="border-b border-line-subtle px-6 py-4">
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.ollamaCloudUsage.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.ollamaCloudUsage.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div v-if="ollamaCloudUsageLoading" class="flex items-center gap-2 text-ink-secondary">
          <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
          {{ t("common.loading") }}
        </div>
        <template v-else>
          <div class="flex items-center justify-between gap-4">
            <div>
              <label class="font-medium text-ink">
                {{ t("admin.settings.ollamaCloudUsage.enabled") }}
              </label>
              <p class="text-sm text-ink-secondary">
                {{ t("admin.settings.ollamaCloudUsage.enabledHint") }}
              </p>
            </div>
            <Toggle
              v-model="ollamaCloudUsageForm.enabled"
              :aria-label="t('admin.settings.ollamaCloudUsage.enabled')"
              data-testid="ollama-cloud-usage-global-enabled"
            />
          </div>
          <div v-if="ollamaCloudUsageForm.enabled" class="space-y-4 border-t border-line-subtle pt-4">
            <div>
              <label class="mb-2 block text-sm font-medium text-ink-secondary" for="ollama-cloud-usage-debounce">
                {{ t("admin.settings.ollamaCloudUsage.debounceMinutes") }}
              </label>
              <input
                id="ollama-cloud-usage-debounce"
                v-model.number="ollamaCloudUsageForm.debounce_minutes"
                type="number"
                min="1"
                max="60"
                class="input w-32"
                data-testid="ollama-cloud-usage-global-debounce"
                @keydown.enter.prevent="saveOllamaCloudUsageSettings"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.ollamaCloudUsage.debounceHint") }}
              </p>
            </div>
            <div>
              <label class="mb-2 block text-sm font-medium text-ink-secondary" for="ollama-cloud-usage-interval">
                {{ t("admin.settings.ollamaCloudUsage.intervalMinutes") }}
              </label>
              <input
                id="ollama-cloud-usage-interval"
                v-model.number="ollamaCloudUsageForm.interval_minutes"
                type="number"
                min="15"
                max="1440"
                class="input w-32"
                data-testid="ollama-cloud-usage-global-interval"
                @keydown.enter.prevent="saveOllamaCloudUsageSettings"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.ollamaCloudUsage.intervalHint") }}
              </p>
            </div>
          </div>
          <div class="flex justify-end border-t border-line-subtle pt-4">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="ollamaCloudUsageSaving"
              data-testid="ollama-cloud-usage-global-save"
              @click="saveOllamaCloudUsageSettings"
            >
              {{ ollamaCloudUsageSaving ? t("common.saving") : t("common.save") }}
            </button>
          </div>
        </template>
      </div>
    </div>

    <!-- Gateway Scheduling Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.scheduling.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.scheduling.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.scheduling.allowUngroupedKey") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ t("admin.settings.scheduling.allowUngroupedKeyHint") }}
            </p>
          </div>
          <Toggle v-model="form.allow_ungrouped_key_scheduling" />
        </div>

        <div class="border-t border-line-subtle pt-4">
          <div class="mb-3">
            <label class="font-medium text-ink">
              {{
                t(
                  "admin.settings.scheduling.accountSchedulingThresholdsTitle",
                )
              }}
            </label>
            <p class="mt-1 text-sm text-ink-secondary">
              {{
                t(
                  "admin.settings.scheduling.accountSchedulingThresholdsDescription",
                )
              }}
            </p>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.scheduling.accountSchedulingThresholdsGlobalHint",
                )
              }}
            </p>
            <p class="mt-0.5 text-xs text-warn">
              {{
                t(
                  "admin.settings.scheduling.accountSchedulingThresholdsDisabledHint",
                )
              }}
            </p>
          </div>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <div
              v-for="platform in schedulingThresholdPlatforms"
              :key="platform"
              class="rounded-lg border border-line p-4"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <label
                    class="font-mono text-sm font-medium text-ink"
                  >
                    {{ platform }}
                  </label>
                  <p class="mt-0.5 text-xs text-ink-secondary">
                    {{
                      t(
                        "admin.settings.scheduling.accountSchedulingThresholdsRangeHint",
                      )
                    }}
                  </p>
                </div>
                <span
                  class="rounded bg-surface-sunken px-2 py-0.5 text-[11px] font-medium text-ink-secondary"
                >
                  %
                </span>
              </div>
              <input
                v-model.number="form.account_scheduling_thresholds[platform]"
                type="number"
                min="1"
                max="100"
                step="1"
                class="input mt-3"
                :data-testid="`account-scheduling-threshold-${platform}`"
                placeholder="100"
              />
            </div>
          </div>
        </div>

        <div
          v-if="!form.openai_advanced_scheduler_enabled"
          class="flex items-center justify-between border-t border-line-subtle pt-5"
        >
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.lowRatePriorityTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t("admin.settings.openaiExperimentalScheduler.lowRatePriorityDescription")
              }}
            </p>
          </div>
          <Toggle
            v-model="form.openai_low_upstream_rate_priority_enabled"
            data-testid="openai-low-rate-priority-toggle"
          />
        </div>

        <div
          v-if="!form.openai_advanced_scheduler_enabled && form.openai_low_upstream_rate_priority_enabled"
          class="flex flex-col items-stretch gap-3 border-t border-line-subtle pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6"
        >
          <div class="min-w-0">
            <label
              class="text-sm font-medium text-ink-secondary"
              for="openai-oauth-scheduling-rate-multiplier"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ t("admin.settings.openaiExperimentalScheduler.oauthRatePriorityDescription") }}
            </p>
          </div>
          <div class="relative w-full shrink-0 sm:w-32">
            <input
              id="openai-oauth-scheduling-rate-multiplier"
              v-model.number="form.openai_oauth_scheduling_rate_multiplier"
              class="input pr-8"
              data-testid="openai-oauth-scheduling-rate-multiplier"
              min="0"
              required
              step="0.01"
              type="number"
            />
            <span
              class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-ink-tertiary"
            >x</span>
          </div>
        </div>

        <div class="flex items-center justify-between border-t border-line-subtle pt-5">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.title") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t("admin.settings.openaiExperimentalScheduler.description")
              }}
            </p>
          </div>
          <Toggle
            v-model="form.openai_advanced_scheduler_enabled"
            data-testid="openai-advanced-scheduler-toggle"
          />
        </div>

        <div
          v-if="form.openai_advanced_scheduler_enabled"
          class="flex items-center justify-between border-t border-line-subtle pt-5"
        >
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.stickyWeightedTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t("admin.settings.openaiExperimentalScheduler.stickyWeightedDescription")
              }}
            </p>
          </div>
          <Toggle v-model="form.openai_advanced_scheduler_sticky_weighted_enabled" />
        </div>

        <div
          v-if="form.openai_advanced_scheduler_enabled"
          class="flex items-center justify-between border-t border-line-subtle pt-5"
        >
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityDescription")
              }}
            </p>
          </div>
          <Toggle v-model="form.openai_advanced_scheduler_subscription_priority_enabled" />
        </div>

        <div
          v-if="form.openai_advanced_scheduler_enabled"
          class="flex flex-col items-stretch gap-3 border-t border-line-subtle pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6"
        >
          <div class="min-w-0">
            <label
              class="text-sm font-medium text-ink-secondary"
              for="openai-oauth-scheduling-rate-multiplier"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ t("admin.settings.openaiExperimentalScheduler.oauthRateWeightedDescription") }}
            </p>
          </div>
          <div class="relative w-full shrink-0 sm:w-32">
            <input
              id="openai-oauth-scheduling-rate-multiplier"
              v-model.number="form.openai_oauth_scheduling_rate_multiplier"
              class="input pr-8"
              data-testid="openai-oauth-scheduling-rate-multiplier"
              min="0"
              required
              step="0.01"
              type="number"
            />
            <span
              class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-ink-tertiary"
            >x</span>
          </div>
        </div>

        <div
          v-if="form.openai_advanced_scheduler_enabled"
          class="border-t border-line-subtle pt-5"
        >
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.openaiExperimentalScheduler.weightsTitle") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t("admin.settings.openaiExperimentalScheduler.weightsDescription")
              }}
            </p>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
            <label
              v-for="field in openAIAdvancedSchedulerWeightFields"
              :key="field.key"
              class="block"
            >
              <span class="text-xs font-medium text-ink-secondary">
                {{ field.label }}
              </span>
              <input
                v-model="form[field.key]"
                class="input mt-1"
                inputmode="decimal"
                :placeholder="field.placeholder"
                type="text"
              />
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- Gateway Forwarding Behavior -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.gatewayForwarding.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.gatewayForwarding.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="grid gap-5 border-b border-line-subtle pb-5 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
          <div>
            <label
              for="grok-default-text-model"
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.gatewayForwarding.grokDefaultTextModel") }}
            </label>
            <input
              id="grok-default-text-model"
              v-model.trim="form.grok_default_text_model"
              type="text"
              class="input mt-2 w-full"
              list="grok-default-text-model-options"
              data-testid="grok-default-text-model"
              placeholder="grok-4.5"
            />
            <datalist id="grok-default-text-model-options">
              <option value="grok-4.5" />
              <option value="grok-4.1-fast" />
              <option value="grok-4" />
            </datalist>
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.grokDefaultTextModelHint") }}
            </p>
          </div>
          <div class="flex items-center justify-between gap-5 md:min-w-72">
            <div>
              <label class="text-sm font-medium text-ink-secondary">
                {{ t("admin.settings.gatewayForwarding.grokCrossClientMap") }}
              </label>
              <p class="mt-0.5 max-w-sm text-xs text-ink-secondary">
                {{ t("admin.settings.gatewayForwarding.grokCrossClientMapHint") }}
              </p>
            </div>
            <Toggle
              v-model="form.grok_cross_client_model_map_enabled"
              data-testid="grok-cross-client-model-map-toggle"
            />
          </div>
          </div>
          <div class="md:col-span-2">
            <label
              for="grok-default-base-url-mode"
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.gatewayForwarding.grokDefaultBaseURLMode") }}
            </label>
            <select
              id="grok-default-base-url-mode"
              v-model="form.grok_default_base_url_mode"
              class="input mt-2 w-full"
              data-testid="grok-default-base-url-mode"
            >
              <option value="cli">{{ t("admin.settings.gatewayForwarding.grokBaseURLModeCLI") }}</option>
              <option value="api">{{ t("admin.settings.gatewayForwarding.grokBaseURLModeAPI") }}</option>
              <option value="us-east-1">{{ t("admin.settings.gatewayForwarding.grokBaseURLModeUSEast1") }}</option>
              <option value="us-west-2">{{ t("admin.settings.gatewayForwarding.grokBaseURLModeUSWest2") }}</option>
              <option value="eu-west-1">{{ t("admin.settings.gatewayForwarding.grokBaseURLModeEUWest1") }}</option>
            </select>
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.grokDefaultBaseURLModeHint") }}
            </p>
          </div>

        <!-- Fingerprint Unification -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.fingerprintUnification",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.fingerprintUnificationHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.enable_fingerprint_unification" />
        </div>

        <!-- Metadata Passthrough -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t("admin.settings.gatewayForwarding.metadataPassthrough")
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.metadataPassthroughHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.enable_metadata_passthrough" />
        </div>

        <!-- CCH Signing -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.gatewayForwarding.cchSigning") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ t("admin.settings.gatewayForwarding.cchSigningHint") }}
            </p>
          </div>
          <Toggle v-model="form.enable_cch_signing" />
        </div>

        <!-- Claude OAuth System Prompt Injection -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjection",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjectionHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_claude_oauth_system_prompt_injection"
          />
        </div>

        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocks",
              )
            }}
          </label>
          <div class="space-y-3">
            <div
              v-for="(block, index) in claudeOAuthSystemPromptBlocks"
              :key="block.id"
              class="rounded-lg border border-line bg-gray-50 p-4 dark:bg-dark-800/60"
            >
              <div
                :class="[
                  'flex flex-wrap items-center justify-between gap-3',
                  block.expanded && 'mb-3',
                ]"
              >
                <div class="min-w-0">
                  <div
                    class="text-sm font-medium text-ink"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.systemBlockTitle",
                        { index: index + 1 },
                      )
                    }}
                  </div>
                  <div
                    class="mt-0.5 text-xs text-ink-secondary"
                  >
                    {{ getClaudeOAuthPresetLabel(block.preset) }}
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :title="
                      block.expanded
                        ? t(
                            'admin.settings.gatewayForwarding.systemBlockHide',
                          )
                        : t(
                            'admin.settings.gatewayForwarding.systemBlockShow',
                          )
                    "
                    :aria-label="
                      block.expanded
                        ? t(
                            'admin.settings.gatewayForwarding.systemBlockHide',
                          )
                        : t(
                            'admin.settings.gatewayForwarding.systemBlockShow',
                          )
                    "
                    @click="toggleClaudeOAuthSystemPromptBlock(index)"
                  >
                    <Icon
                      :name="block.expanded ? 'eyeOff' : 'eye'"
                      size="xs"
                    />
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :disabled="index === 0"
                    @click="moveClaudeOAuthSystemPromptBlock(index, -1)"
                  >
                    <Icon name="arrowUp" size="xs" />
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :disabled="
                      index === claudeOAuthSystemPromptBlocks.length - 1
                    "
                    @click="moveClaudeOAuthSystemPromptBlock(index, 1)"
                  >
                    <Icon name="arrowDown" size="xs" />
                  </button>
                  <Toggle v-model="block.enabled" />
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2 text-red-600 hover:text-red-700 dark:text-red-400"
                    @click="removeClaudeOAuthSystemPromptBlock(index)"
                  >
                    <Icon name="trash" size="xs" />
                  </button>
                </div>
              </div>

              <div v-show="block.expanded">
                <div class="grid gap-3 md:grid-cols-2">
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-ink-secondary"
                    >
                      {{
                        t(
                          "admin.settings.gatewayForwarding.systemBlockPreset",
                        )
                      }}
                    </label>
                    <Select
                      v-model="block.preset"
                      :options="claudeOAuthSystemPromptPresetOptions"
                      @change="
                        (value) =>
                          applyClaudeOAuthSystemPromptPreset(index, value)
                      "
                    />
                  </div>
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-ink-secondary"
                    >
                      {{
                        t(
                          "admin.settings.gatewayForwarding.systemBlockType",
                        )
                      }}
                    </label>
                    <Select
                      v-model="block.type"
                      :options="claudeOAuthSystemPromptBlockTypeOptions"
                    />
                  </div>
                </div>

                <div class="mt-3">
                  <label
                    class="mb-1 block text-xs font-medium text-ink-secondary"
                  >
                    {{ t("admin.settings.gatewayForwarding.systemBlockText") }}
                  </label>
                  <textarea
                    v-model="block.text"
                    rows="6"
                    class="input w-full resize-y font-mono text-xs leading-5"
                    @input="markClaudeOAuthSystemPromptBlockCustom(block)"
                  />
                </div>

                <div
                  class="mt-3 grid gap-3 md:grid-cols-[minmax(0,1fr)_160px]"
                >
                  <div class="flex items-center justify-between gap-4">
                    <div>
                      <label
                        class="text-xs font-medium text-ink-secondary"
                      >
                        {{
                          t(
                            "admin.settings.gatewayForwarding.systemBlockCacheControl",
                          )
                        }}
                      </label>
                    </div>
                    <Toggle v-model="block.cacheControlEnabled" />
                  </div>
                  <div v-if="block.cacheControlEnabled">
                    <Select
                      v-model="block.cacheControlTTL"
                      :options="claudeOAuthSystemPromptCacheTTLOptions"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addClaudeOAuthSystemPromptBlock"
            >
              <Icon name="plus" size="xs" />
              {{ t("admin.settings.gatewayForwarding.addSystemBlock") }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="resetClaudeOAuthSystemPromptBlocks"
            >
              <Icon name="refresh" size="xs" />
              {{
                t("admin.settings.gatewayForwarding.resetSystemBlocks")
              }}
            </button>
          </div>
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{
              t(
                "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocksHint",
              )
            }}
          </p>
        </div>

        <!-- Anthropic Cache TTL 1h Injection -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjection",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjectionHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_anthropic_cache_ttl_1h_injection"
          />
        </div>

        <!-- messages cache_control 改写 -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.rewriteMessageCacheControl",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.rewriteMessageCacheControlHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.rewrite_message_cache_control" />
        </div>

        <!-- 客户端 dateline 归一化（仅 Anthropic OAuth/SetupToken） -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.clientDatelineNormalization",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.clientDatelineNormalizationHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_client_dateline_normalization"
          />
        </div>

        <!-- Antigravity UA 版本 -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.antigravityUserAgentVersion",
              )
            }}
          </label>
          <input
            v-model="form.antigravity_user_agent_version"
            type="text"
            class="input max-w-xs font-mono text-sm"
            :placeholder="
              t(
                'admin.settings.gatewayForwarding.antigravityUserAgentVersionPlaceholder',
              )
            "
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{
              t(
                "admin.settings.gatewayForwarding.antigravityUserAgentVersionHint",
              )
            }}
          </p>
        </div>

        <!-- OpenAI Codex UA -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexUserAgent",
              )
            }}
          </label>
          <input
            v-model="form.openai_codex_user_agent"
            type="text"
            class="input w-full font-mono text-sm"
            :placeholder="
              t(
                'admin.settings.gatewayForwarding.openaiCodexUserAgentPlaceholder',
              )
            "
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexUserAgentHint",
              )
            }}
          </p>
        </div>

        <!-- Codex 客户端版本号 -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexClientVersion",
              )
            }}
          </label>
          <input
            v-model="form.openai_codex_client_version"
            type="text"
            class="input w-full font-mono text-sm"
            :placeholder="
              t(
                'admin.settings.gatewayForwarding.openaiCodexClientVersionPlaceholder',
              )
            "
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexClientVersionHint",
              )
            }}
          </p>
        </div>

        <!-- Codex 版本号自动同步 -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexVersionAutoSync",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexVersionAutoSyncHint",
                )
              }}
            </p>
            <p
              v-if="codexSyncedVersionLabel"
              class="mt-0.5 text-xs text-ink-secondary"
            >
              {{ codexSyncedVersionLabel }}
            </p>
          </div>
          <Toggle v-model="form.openai_codex_version_auto_sync_enabled" />
        </div>

      </div>
    </div>

    <!-- Web Search Emulation -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.webSearchEmulation.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.webSearchEmulation.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <!-- Global Toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.webSearchEmulation.enabled") }}
            </label>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ t("admin.settings.webSearchEmulation.enabledHint") }}
            </p>
          </div>
          <Toggle v-model="webSearchConfig.enabled" />
        </div>

        <!-- Providers -->
        <div v-if="webSearchConfig.enabled" class="space-y-4">
          <div class="flex items-center justify-between">
            <label
              class="text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.webSearchEmulation.providers") }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addWebSearchProvider"
            >
              {{ t("admin.settings.webSearchEmulation.addProvider") }}
            </button>
          </div>

          <div
            v-if="webSearchConfig.providers.length === 0"
            class="rounded-lg border border-dashed border-line p-4 text-center text-sm text-ink-tertiary"
          >
            {{ t("admin.settings.webSearchEmulation.noProviders") }}
          </div>

          <div
            v-for="(provider, pIdx) in webSearchConfig.providers"
            :key="pIdx"
            class="rounded-lg border border-line"
          >
            <!-- Collapsible header -->
            <div
              class="flex cursor-pointer items-center justify-between px-4 py-3"
              @click="toggleProviderExpand(pIdx)"
            >
              <div class="flex items-center gap-3">
                <svg
                  class="h-4 w-4 text-ink-tertiary transition-transform"
                  :class="{ 'rotate-90': expandedProviders[pIdx] }"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 5l7 7-7 7"
                  />
                </svg>
                <Select
                  v-model="provider.type"
                  :options="[
                    { value: 'brave', label: 'Brave Search' },
                    { value: 'tavily', label: 'Tavily' },
                  ]"
                  class="w-36"
                  @click.stop
                />
                <!-- Quota summary (always visible) -->
                <span class="text-xs text-ink-tertiary">
                  {{ provider.quota_used ?? 0 }} /
                  {{
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                      ? provider.quota_limit
                      : "∞"
                  }}
                </span>
                <span
                  v-if="
                    !expandedProviders[pIdx] &&
                    provider.api_key_configured
                  "
                  class="text-xs text-green-500"
                >
                  {{
                    t(
                      "admin.settings.webSearchEmulation.apiKeyConfigured",
                    )
                  }}
                </span>
              </div>
              <button
                type="button"
                class="text-red-500 hover:text-red-700 text-xs"
                @click.stop="removeWebSearchProvider(pIdx)"
              >
                {{
                  t("admin.settings.webSearchEmulation.removeProvider")
                }}
              </button>
            </div>

            <!-- Expanded content -->
            <div
              v-if="expandedProviders[pIdx]"
              class="space-y-3 border-t border-line-subtle px-4 pb-4 pt-3"
            >
              <!-- API Key with inline show/copy -->
              <div>
                <label class="text-xs text-ink-secondary">{{
                  t("admin.settings.webSearchEmulation.apiKey")
                }}</label>
                <div class="relative">
                  <input
                    v-model="provider.api_key"
                    :type="apiKeyVisible[pIdx] ? 'text' : 'password'"
                    class="input w-full text-sm"
                    :class="
                      provider.api_key || provider.api_key_configured
                        ? 'pr-16'
                        : ''
                    "
                    :placeholder="
                      provider.api_key_configured
                        ? '••••••••'
                        : t(
                            'admin.settings.webSearchEmulation.apiKeyPlaceholder',
                          )
                    "
                  />
                  <div
                    v-if="provider.api_key || provider.api_key_configured"
                    class="absolute inset-y-0 right-0 flex items-center pr-1.5"
                  >
                    <button
                      type="button"
                      class="rounded p-1 text-ink-tertiary hover:text-ink-secondary"
                      :title="
                        apiKeyVisible[pIdx]
                          ? t(
                              'admin.settings.webSearchEmulation.hideApiKey',
                            )
                          : t(
                              'admin.settings.webSearchEmulation.showApiKey',
                            )
                      "
                      @click="apiKeyVisible[pIdx] = !apiKeyVisible[pIdx]"
                    >
                      <svg
                        v-if="!apiKeyVisible[pIdx]"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                        />
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                        />
                      </svg>
                      <svg
                        v-else
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21"
                        />
                      </svg>
                    </button>
                    <button
                      type="button"
                      class="rounded p-1 text-ink-tertiary hover:text-ink-secondary"
                      :class="{
                        'opacity-30 cursor-not-allowed':
                          !provider.api_key,
                      }"
                      :title="
                        t('admin.settings.webSearchEmulation.copyApiKey')
                      "
                      :disabled="!provider.api_key"
                      @click="copyApiKey(pIdx)"
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>

              <!-- Quota + Subscription in compact row -->
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="text-xs text-ink-secondary">{{
                    t("admin.settings.webSearchEmulation.quotaLimit")
                  }}</label>
                  <input
                    v-model="provider.quota_limit"
                    type="number"
                    min="1"
                    class="input text-sm"
                    :placeholder="'∞'"
                  />
                  <p class="mt-0.5 text-xs text-ink-tertiary">
                    {{
                      t(
                        "admin.settings.webSearchEmulation.quotaLimitHint",
                      )
                    }}
                  </p>
                </div>
                <div>
                  <label class="text-xs text-ink-secondary">{{
                    t("admin.settings.webSearchEmulation.subscribedAt")
                  }}</label>
                  <input
                    :value="formatSubscribedAt(provider.subscribed_at)"
                    type="date"
                    class="input text-sm"
                    @input="
                      provider.subscribed_at = parseSubscribedAt(
                        ($event.target as HTMLInputElement).value,
                      )
                    "
                  />
                  <p class="mt-0.5 text-xs text-ink-tertiary">
                    {{
                      t(
                        "admin.settings.webSearchEmulation.subscribedAtHint",
                      )
                    }}
                  </p>
                </div>
              </div>

              <!-- Usage display -->
              <div class="flex items-center gap-2">
                <span class="text-xs text-ink-secondary"
                  >{{
                    t("admin.settings.webSearchEmulation.quotaUsage")
                  }}:</span
                >
                <div
                  v-if="
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                  "
                  class="flex-1 rounded-full bg-gray-200 dark:bg-dark-600"
                  style="height: 6px"
                >
                  <div
                    class="h-full rounded-full transition-all"
                    :class="
                      quotaPercentage(provider) > 90
                        ? 'bg-danger'
                        : quotaPercentage(provider) > 70
                          ? 'bg-yellow-500'
                          : 'bg-success'
                    "
                    :style="{
                      width:
                        Math.min(quotaPercentage(provider), 100) + '%',
                    }"
                  />
                </div>
                <div v-else class="flex-1" />
                <span class="text-xs text-ink-secondary"
                  >{{ provider.quota_used ?? 0 }} /
                  {{
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                      ? provider.quota_limit
                      : "∞"
                  }}</span
                >
                <button
                  v-if="(provider.quota_used ?? 0) > 0"
                  type="button"
                  class="text-xs text-primary-600 hover:text-primary-700"
                  @click="resetWebSearchUsage(pIdx)"
                >
                  {{ t("admin.settings.webSearchEmulation.resetUsage") }}
                </button>
              </div>

              <!-- Proxy + Test on same row -->
              <div class="flex items-end gap-3">
                <div class="flex-1">
                  <label class="text-xs text-ink-secondary">{{
                    t("admin.settings.webSearchEmulation.proxy")
                  }}</label>
                  <ProxySelector
                    v-model="provider.proxy_id"
                    :proxies="webSearchProxies"
                  />
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm whitespace-nowrap"
                  @click="openTestDialog()"
                >
                  {{ t("admin.settings.webSearchEmulation.test") }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Web Search Test Dialog -->
    <div
      v-if="wsTestDialogOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      @click.self="wsTestDialogOpen = false"
    >
      <div
        class="mx-4 w-full max-w-lg rounded-xl bg-surface p-6 shadow-xl"
      >
        <h3
          class="mb-4 text-lg font-semibold text-ink"
        >
          {{ t("admin.settings.webSearchEmulation.testResultTitle") }}
        </h3>
        <div class="flex items-center gap-2">
          <input
            v-model="wsTestQuery"
            type="text"
            class="input flex-1 text-sm"
            :placeholder="
              t('admin.settings.webSearchEmulation.testDefaultQuery')
            "
            @keyup.enter="testWebSearchProvider()"
          />
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="wsTestLoading"
            @click="testWebSearchProvider()"
          >
            {{
              wsTestLoading
                ? t("admin.settings.webSearchEmulation.testing")
                : t("admin.settings.webSearchEmulation.test")
            }}
          </button>
        </div>
        <!-- Test results -->
        <div
          v-if="wsTestResult"
          class="mt-4 max-h-80 overflow-y-auto rounded-lg bg-surface-sunken p-4"
        >
          <p
            class="mb-2 text-sm font-medium text-ink-secondary"
          >
            {{
              t("admin.settings.webSearchEmulation.testResultProvider")
            }}: {{ wsTestResult.provider }}
          </p>
          <div
            v-if="wsTestResult.results.length === 0"
            class="text-sm text-ink-tertiary"
          >
            {{ t("admin.settings.webSearchEmulation.testNoResults") }}
          </div>
          <div
            v-for="(r, rIdx) in wsTestResult.results"
            :key="rIdx"
            class="mt-2 border-t border-line pt-2 first:mt-0 first:border-0 first:pt-0"
          >
            <a
              :href="r.url"
              target="_blank"
              class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
              >{{ r.title }}</a
            >
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ r.snippet }}
            </p>
          </div>
        </div>
        <div class="mt-4 flex justify-end">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="wsTestDialogOpen = false"
          >
            {{ t("common.close") }}
          </button>
        </div>
      </div>
    </div>

  <!-- Usage Records Settings -->
  <div class="card">
    <div class="border-b border-line-subtle px-6 py-4">
      <h2 class="text-lg font-semibold text-ink">
        {{ t('admin.settings.usageRecords.title') }}
      </h2>
      <p class="mt-1 text-sm text-ink-secondary">
        {{ t('admin.settings.usageRecords.description') }}
      </p>
    </div>
    <div class="space-y-4 p-6">
      <!-- User error requests visibility -->
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-ink-secondary">
            {{ t('admin.settings.user_error_view.label') }}
          </label>
          <p class="text-xs text-ink-secondary">
            {{ t('admin.settings.user_error_view.description') }}
          </p>
        </div>
        <label class="toggle">
          <input v-model="form.allow_user_view_error_requests" type="checkbox" />
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>
  </div>
  </div>
</template>

<script setup lang="ts">
import Icon from "@/components/icons/Icon.vue";
import ProxySelector from "@/components/common/ProxySelector.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import { useSettingsFormContext } from "../context";

const {
  activeTab,
  addClaudeOAuthSystemPromptBlock,
  addCodexBlacklistRow,
  addCodexFingerprintRow,
  addCodexWhitelistRow,
  addWebSearchProvider,
  apiKeyVisible,
  applyClaudeOAuthSystemPromptPreset,
  claudeOAuthSystemPromptBlockTypeOptions,
  claudeOAuthSystemPromptBlocks,
  claudeOAuthSystemPromptCacheTTLOptions,
  claudeOAuthSystemPromptPresetOptions,
  codexBlacklistRows,
  codexFingerprintNoRequired,
  codexFingerprintRows,
  codexSyncedVersionLabel,
  codexWhitelistRows,
  copyApiKey,
  expandedProviders,
  form,
  formatSubscribedAt,
  getClaudeOAuthPresetLabel,
  markClaudeOAuthSystemPromptBlockCustom,
  moveClaudeOAuthSystemPromptBlock,
  ollamaCloudUsageForm,
  ollamaCloudUsageLoading,
  ollamaCloudUsageSaving,
  openAIAdvancedSchedulerWeightFields,
  openTestDialog,
  parseSubscribedAt,
  quotaPercentage,
  removeClaudeOAuthSystemPromptBlock,
  removeCodexBlacklistRow,
  removeCodexFingerprintRow,
  removeCodexWhitelistRow,
  removeWebSearchProvider,
  resetClaudeOAuthSystemPromptBlocks,
  resetWebSearchUsage,
  saveOllamaCloudUsageSettings,
  saveUpstreamBillingProbeSettings,
  schedulingThresholdPlatforms,
  t,
  testWebSearchProvider,
  toggleClaudeOAuthSystemPromptBlock,
  toggleProviderExpand,
  upstreamBillingProbeForm,
  upstreamBillingProbeLoading,
  upstreamBillingProbeSaving,
  webSearchConfig,
  webSearchProxies,
  wsTestDialogOpen,
  wsTestLoading,
  wsTestQuery,
  wsTestResult,
} = useSettingsFormContext();
</script>
