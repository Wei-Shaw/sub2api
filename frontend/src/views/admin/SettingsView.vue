<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"
        ></div>
      </div>

      <!-- Settings Form -->
      <form v-else @submit.prevent="saveSettings" class="space-y-6" novalidate>
        <!-- Tab Navigation -->
        <div class="sticky top-0 z-10 overflow-x-auto settings-tabs-scroll">
          <nav class="settings-tabs">
            <button
              v-for="tab in settingsTabs"
              :key="tab.key"
              type="button"
              :class="[
                'settings-tab',
                activeTab === tab.key && 'settings-tab-active',
              ]"
              @click="activeTab = tab.key"
            >
              <span class="settings-tab-icon">
                <Icon :name="tab.icon" size="sm" />
              </span>
              <span>{{ t(`admin.settings.tabs.${tab.keyREDACTED`) REDACTEDREDACTED</span>
            </button>
          </nav>
        </div>

        <!-- Tab: Security — Admin API Key -->
        <div v-show="activeTab === 'security'" class="space-y-6">
          <!-- Admin API Key Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.adminApiKey.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.adminApiKey.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Security Warning -->
              <div
                class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20"
              >
                <div class="flex items-start">
                  <Icon
                    name="exclamationTriangle"
                    size="md"
                    class="mt-0.5 flex-shrink-0 text-amber-500"
                  />
                  <p class="ml-3 text-sm text-amber-700 dark:text-amber-300">
                    {{ t("admin.settings.adminApiKey.securityWarning") REDACTEDREDACTED
                  </p>
                </div>
              </div>

              <!-- Loading State -->
              <div
                v-if="adminApiKeyLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") REDACTEDREDACTED
              </div>

              <!-- No Key Configured -->
              <div
                v-else-if="!adminApiKeyExists"
                class="flex items-center justify-between"
              >
                <span class="text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.adminApiKey.notConfigured") REDACTEDREDACTED
                </span>
                <button
                  type="button"
                  @click="createAdminApiKey"
                  :disabled="adminApiKeyOperating"
                  class="btn btn-primary btn-sm"
                >
                  <svg
                    v-if="adminApiKeyOperating"
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
                  {{
                    adminApiKeyOperating
                      ? t("admin.settings.adminApiKey.creating")
                      : t("admin.settings.adminApiKey.create")
                  REDACTEDREDACTED
                </button>
              </div>

              <!-- Key Exists -->
              <div v-else class="space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <label
                      class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.adminApiKey.currentKey") REDACTEDREDACTED
                    </label>
                    <code
                      class="rounded bg-gray-100 px-2 py-1 font-mono text-sm text-gray-900 dark:bg-dark-700 dark:text-gray-100"
                    >
                      {{ adminApiKeyMasked REDACTEDREDACTED
                    </code>
                  </div>
                  <div class="flex gap-2">
                    <button
                      type="button"
                      @click="regenerateAdminApiKey"
                      :disabled="adminApiKeyOperating"
                      class="btn btn-secondary btn-sm"
                    >
                      {{
                        adminApiKeyOperating
                          ? t("admin.settings.adminApiKey.regenerating")
                          : t("admin.settings.adminApiKey.regenerate")
                      REDACTEDREDACTED
                    </button>
                    <button
                      type="button"
                      @click="deleteAdminApiKey"
                      :disabled="adminApiKeyOperating"
                      class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                    >
                      {{ t("admin.settings.adminApiKey.delete") REDACTEDREDACTED
                    </button>
                  </div>
                </div>

                <!-- Newly Generated Key Display -->
                <div
                  v-if="newAdminApiKey"
                  class="space-y-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20"
                >
                  <p
                    class="text-sm font-medium text-green-700 dark:text-green-300"
                  >
                    {{ t("admin.settings.adminApiKey.keyWarning") REDACTEDREDACTED
                  </p>
                  <div class="flex items-center gap-2">
                    <code
                      class="flex-1 select-all break-all rounded border border-green-300 bg-white px-3 py-2 font-mono text-sm dark:border-green-700 dark:bg-dark-800"
                    >
                      {{ newAdminApiKey REDACTEDREDACTED
                    </code>
                    <button
                      type="button"
                      @click="copyNewKey"
                      class="btn btn-primary btn-sm flex-shrink-0"
                    >
                      {{ t("admin.settings.adminApiKey.copyKey") REDACTEDREDACTED
                    </button>
                  </div>
                  <p class="text-xs text-green-600 dark:text-green-400">
                    {{ t("admin.settings.adminApiKey.usage") REDACTEDREDACTED
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Security — Admin API Key -->

        <!-- Tab: Gateway -->
        <div v-show="activeTab === 'gateway'" class="space-y-6">
          <!-- Overload Cooldown (529) Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.overloadCooldown.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.overloadCooldown.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div
                v-if="overloadCooldownLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") REDACTEDREDACTED
              </div>

              <template v-else>
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">{{
                      t("admin.settings.overloadCooldown.enabled")
                    REDACTEDREDACTED</label>
                    <p class="text-sm text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.overloadCooldown.enabledHint") REDACTEDREDACTED
                    </p>
                  </div>
                  <Toggle v-model="overloadCooldownForm.enabled" />
                </div>

                <div
                  v-if="overloadCooldownForm.enabled"
                  class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.overloadCooldown.cooldownMinutes") REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="overloadCooldownForm.cooldown_minutes"
                      type="number"
                      min="1"
                      max="120"
                      class="input w-32"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        t("admin.settings.overloadCooldown.cooldownMinutesHint")
                      REDACTEDREDACTED
                    </p>
                  </div>
                </div>

                <div
                  class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <button
                    type="button"
                    @click="saveOverloadCooldownSettings"
                    :disabled="overloadCooldownSaving"
                    class="btn btn-primary btn-sm"
                  >
                    <svg
                      v-if="overloadCooldownSaving"
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
                    {{
                      overloadCooldownSaving
                        ? t("common.saving")
                        : t("common.save")
                    REDACTEDREDACTED
                  </button>
                </div>
              </template>
            </div>
          </div>

          <!-- Stream Timeout Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.streamTimeout.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.streamTimeout.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="streamTimeoutLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") REDACTEDREDACTED
              </div>

              <template v-else>
                <!-- Enable Stream Timeout -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">{{
                      t("admin.settings.streamTimeout.enabled")
                    REDACTEDREDACTED</label>
                    <p class="text-sm text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.streamTimeout.enabledHint") REDACTEDREDACTED
                    </p>
                  </div>
                  <Toggle v-model="streamTimeoutForm.enabled" />
                </div>

                <!-- Settings - Only show when enabled -->
                <div
                  v-if="streamTimeoutForm.enabled"
                  class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <!-- Action -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.streamTimeout.action") REDACTEDREDACTED
                    </label>
                    <select
                      v-model="streamTimeoutForm.action"
                      class="input w-64"
                    >
                      <option value="temp_unsched">
                        {{
                          t("admin.settings.streamTimeout.actionTempUnsched")
                        REDACTEDREDACTED
                      </option>
                      <option value="error">
                        {{ t("admin.settings.streamTimeout.actionError") REDACTEDREDACTED
                      </option>
                      <option value="none">
                        {{ t("admin.settings.streamTimeout.actionNone") REDACTEDREDACTED
                      </option>
                    </select>
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.streamTimeout.actionHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <!-- Temp Unsched Minutes (only show when action is temp_unsched) -->
                  <div v-if="streamTimeoutForm.action === 'temp_unsched'">
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.streamTimeout.tempUnschedMinutes") REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="streamTimeoutForm.temp_unsched_minutes"
                      type="number"
                      min="1"
                      max="60"
                      class="input w-32"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        t("admin.settings.streamTimeout.tempUnschedMinutesHint")
                      REDACTEDREDACTED
                    </p>
                  </div>

                  <!-- Threshold Count -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.streamTimeout.thresholdCount") REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="streamTimeoutForm.threshold_count"
                      type="number"
                      min="1"
                      max="10"
                      class="input w-32"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.streamTimeout.thresholdCountHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <!-- Threshold Window Minutes -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{
                        t("admin.settings.streamTimeout.thresholdWindowMinutes")
                      REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="
                        streamTimeoutForm.threshold_window_minutes
                      "
                      type="number"
                      min="1"
                      max="60"
                      class="input w-32"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        t(
                          "admin.settings.streamTimeout.thresholdWindowMinutesHint",
                        )
                      REDACTEDREDACTED
                    </p>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <button
                    type="button"
                    @click="saveStreamTimeoutSettings"
                    :disabled="streamTimeoutSaving"
                    class="btn btn-primary btn-sm"
                  >
                    <svg
                      v-if="streamTimeoutSaving"
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
                    {{
                      streamTimeoutSaving
                        ? t("common.saving")
                        : t("common.save")
                    REDACTEDREDACTED
                  </button>
                </div>
              </template>
            </div>
          </div>

          <!-- Request Rectifier Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.rectifier.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.rectifier.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="rectifierLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") REDACTEDREDACTED
              </div>

              <template v-else>
                <!-- Master Toggle -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">{{
                      t("admin.settings.rectifier.enabled")
                    REDACTEDREDACTED</label>
                    <p class="text-sm text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.rectifier.enabledHint") REDACTEDREDACTED
                    </p>
                  </div>
                  <Toggle v-model="rectifierForm.enabled" />
                </div>

                <!-- Sub-toggles (only show when master is enabled) -->
                <div
                  v-if="rectifierForm.enabled"
                  class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <!-- Thinking Signature Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-medium text-gray-700 dark:text-gray-300"
                        >{{
                          t("admin.settings.rectifier.thinkingSignature")
                        REDACTEDREDACTED</label
                      >
                      <p class="text-xs text-gray-500 dark:text-gray-400">
                        {{
                          t("admin.settings.rectifier.thinkingSignatureHint")
                        REDACTEDREDACTED
                      </p>
                    </div>
                    <Toggle
                      v-model="rectifierForm.thinking_signature_enabled"
                    />
                  </div>

                  <!-- Thinking Budget Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-medium text-gray-700 dark:text-gray-300"
                        >{{
                          t("admin.settings.rectifier.thinkingBudget")
                        REDACTEDREDACTED</label
                      >
                      <p class="text-xs text-gray-500 dark:text-gray-400">
                        {{ t("admin.settings.rectifier.thinkingBudgetHint") REDACTEDREDACTED
                      </p>
                    </div>
                    <Toggle v-model="rectifierForm.thinking_budget_enabled" />
                  </div>

                  <!-- API Key Signature Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-medium text-gray-700 dark:text-gray-300"
                        >{{
                          t("admin.settings.rectifier.apikeySignature")
                        REDACTEDREDACTED</label
                      >
                      <p class="text-xs text-gray-500 dark:text-gray-400">
                        {{ t("admin.settings.rectifier.apikeySignatureHint") REDACTEDREDACTED
                      </p>
                    </div>
                    <Toggle v-model="rectifierForm.apikey_signature_enabled" />
                  </div>

                  <!-- Custom Patterns (only when apikey_signature_enabled) -->
                  <div
                    v-if="rectifierForm.apikey_signature_enabled"
                    class="ml-4 space-y-3 border-l-2 border-gray-200 pl-4 dark:border-dark-600"
                  >
                    <div>
                      <label
                        class="text-sm font-medium text-gray-700 dark:text-gray-300"
                        >{{
                          t("admin.settings.rectifier.apikeyPatterns")
                        REDACTEDREDACTED</label
                      >
                      <p class="text-xs text-gray-500 dark:text-gray-400">
                        {{ t("admin.settings.rectifier.apikeyPatternsHint") REDACTEDREDACTED
                      </p>
                    </div>
                    <div
                      v-for="(
                        _, index
                      ) in rectifierForm.apikey_signature_patterns"
                      :key="index"
                      class="flex items-center gap-2"
                    >
                      <input
                        v-model="rectifierForm.apikey_signature_patterns[index]"
                        type="text"
                        class="input input-sm flex-1"
                        :placeholder="
                          t('admin.settings.rectifier.apikeyPatternPlaceholder')
                        "
                      />
                      <button
                        type="button"
                        @click="
                          rectifierForm.apikey_signature_patterns.splice(
                            index,
                            1,
                          )
                        "
                        class="btn btn-ghost btn-xs text-red-500 hover:text-red-700"
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                      </button>
                    </div>
                    <button
                      type="button"
                      @click="rectifierForm.apikey_signature_patterns.push('')"
                      class="btn btn-ghost btn-xs text-primary-600 dark:text-primary-400"
                    >
                      + {{ t("admin.settings.rectifier.addPattern") REDACTEDREDACTED
                    </button>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <button
                    type="button"
                    @click="saveRectifierSettings"
                    :disabled="rectifierSaving"
                    class="btn btn-primary btn-sm"
                  >
                    <svg
                      v-if="rectifierSaving"
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
                    {{
                      rectifierSaving ? t("common.saving") : t("common.save")
                    REDACTEDREDACTED
                  </button>
                </div>
              </template>
            </div>
          </div>
          <!-- Beta Policy Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.betaPolicy.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.betaPolicy.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="betaPolicyLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") REDACTEDREDACTED
              </div>

              <template v-else>
                <!-- Rule Cards -->
                <div
                  v-for="rule in betaPolicyForm.rules"
                  :key="rule.beta_token"
                  class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
                >
                  <div class="mb-3 flex items-center gap-2">
                    <span
                      class="text-sm font-medium text-gray-900 dark:text-white"
                    >
                      {{ getBetaDisplayName(rule.beta_token) REDACTEDREDACTED
                    </span>
                    <span
                      class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                    >
                      {{ rule.beta_token REDACTEDREDACTED
                    </span>
                  </div>

                  <div class="grid grid-cols-2 gap-4">
                    <!-- Action -->
                    <div>
                      <label
                        class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                      >
                        {{ t("admin.settings.betaPolicy.action") REDACTEDREDACTED
                      </label>
                      <Select
                        :modelValue="rule.action"
                        @update:modelValue="rule.action = $event as any"
                        :options="betaPolicyActionOptions"
                      />
                    </div>

                    <!-- Scope -->
                    <div>
                      <label
                        class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                      >
                        {{ t("admin.settings.betaPolicy.scope") REDACTEDREDACTED
                      </label>
                      <Select
                        :modelValue="rule.scope"
                        @update:modelValue="rule.scope = $event as any"
                        :options="betaPolicyScopeOptions"
                      />
                    </div>
                  </div>

                  <!-- Error Message (only when action=block) -->
                  <div v-if="rule.action === 'block'" class="mt-3">
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.betaPolicy.errorMessage") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="rule.error_message"
                      type="text"
                      class="input"
                      :placeholder="
                        t('admin.settings.betaPolicy.errorMessagePlaceholder')
                      "
                    />
                    <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                      {{ t("admin.settings.betaPolicy.errorMessageHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <!-- Quick Presets (only for tokens with presets) -->
                  <div v-if="betaPresets[rule.beta_token]?.length" class="mt-3">
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.betaPolicy.quickPresets") REDACTEDREDACTED
                    </label>
                    <div class="flex flex-wrap gap-2">
                      <button
                        v-for="preset in betaPresets[rule.beta_token]"
                        :key="preset.label"
                        type="button"
                        class="inline-flex items-center gap-1 rounded-md border border-primary-200 bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 transition-colors hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-900/30 dark:text-primary-300 dark:hover:bg-primary-900/50"
                        @click="applyBetaPreset(rule, preset)"
                        :title="preset.description"
                      >
                        {{ preset.label REDACTEDREDACTED
                      </button>
                    </div>
                  </div>

                  <!-- Model Whitelist -->
                  <div class="mt-3">
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.betaPolicy.modelWhitelist") REDACTEDREDACTED
                    </label>
                    <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
                      {{ t("admin.settings.betaPolicy.modelWhitelistHint") REDACTEDREDACTED
                    </p>
                    <!-- Existing patterns -->
                    <div
                      v-for="(_, index) in rule.model_whitelist || []"
                      :key="index"
                      class="mb-1.5 flex items-center gap-2"
                    >
                      <input
                        v-model="rule.model_whitelist![index]"
                        type="text"
                        class="input input-sm flex-1"
                        :placeholder="
                          t('admin.settings.betaPolicy.modelPatternPlaceholder')
                        "
                      />
                      <button
                        type="button"
                        @click="rule.model_whitelist!.splice(index, 1)"
                        class="shrink-0 rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                      </button>
                    </div>
                    <!-- Add pattern button -->
                    <button
                      type="button"
                      @click="
                        if (!rule.model_whitelist) rule.model_whitelist = [];
                        rule.model_whitelist.push('');
                      "
                      class="mb-2 inline-flex items-center gap-1 text-xs text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    >
                      <svg
                        class="h-3.5 w-3.5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M12 4v16m8-8H4"
                        />
                      </svg>
                      {{ t("admin.settings.betaPolicy.addModelPattern") REDACTEDREDACTED
                    </button>
                    <!-- Common pattern chips -->
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="text-xs text-gray-400 dark:text-gray-500"
                        >{{
                          t("admin.settings.betaPolicy.commonPatterns")
                        REDACTEDREDACTED:</span
                      >
                      <button
                        v-for="pattern in commonModelPatterns"
                        :key="pattern"
                        type="button"
                        class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-700 dark:hover:bg-primary-900/30 dark:hover:text-primary-300"
                        @click="addQuickPattern(rule, pattern)"
                      >
                        {{ pattern REDACTEDREDACTED
                      </button>
                    </div>
                  </div>

                  <!-- Fallback Action (only when model_whitelist is non-empty) -->
                  <div
                    v-if="
                      rule.model_whitelist && rule.model_whitelist.length > 0
                    "
                    class="mt-3"
                  >
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.betaPolicy.fallbackAction") REDACTEDREDACTED
                    </label>
                    <Select
                      :modelValue="rule.fallback_action || 'pass'"
                      @update:modelValue="rule.fallback_action = $event as any"
                      :options="betaPolicyActionOptions"
                    />
                    <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                      {{ t("admin.settings.betaPolicy.fallbackActionHint") REDACTEDREDACTED
                    </p>
                    <!-- Fallback Error Message (only when fallback_action=block) -->
                    <div v-if="rule.fallback_action === 'block'" class="mt-2">
                      <input
                        v-model="rule.fallback_error_message"
                        type="text"
                        class="input"
                        :placeholder="
                          t(
                            'admin.settings.betaPolicy.fallbackErrorMessagePlaceholder',
                          )
                        "
                      />
                      <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                        {{ t("admin.settings.betaPolicy.errorMessageHint") REDACTEDREDACTED
                      </p>
                    </div>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
                >
                  <button
                    type="button"
                    @click="saveBetaPolicySettings"
                    :disabled="betaPolicySaving"
                    class="btn btn-primary btn-sm"
                  >
                    <svg
                      v-if="betaPolicySaving"
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
                    {{
                      betaPolicySaving ? t("common.saving") : t("common.save")
                    REDACTEDREDACTED
                  </button>
                </div>
              </template>
            </div>
          </div>
        </div>
        <!-- /Tab: Gateway -->

        <!-- Tab: Security — Registration, Turnstile, LinuxDo -->
        <div v-show="activeTab === 'security'" class="space-y-6">
          <!-- Registration Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.registration.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.registration.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Enable Registration -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.enableRegistration")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{
                      t("admin.settings.registration.enableRegistrationHint")
                    REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.registration_enabled" />
              </div>

              <!-- Email Verification -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.emailVerification")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.registration.emailVerificationHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.email_verify_enabled" />
              </div>

              <!-- Email Suffix Whitelist -->
              <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t("admin.settings.registration.emailSuffixWhitelist")
                REDACTEDREDACTED</label>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{
                    t("admin.settings.registration.emailSuffixWhitelistHint")
                  REDACTEDREDACTED
                </p>
                <div
                  class="mt-3 rounded-lg border border-gray-300 bg-white p-2 dark:border-dark-500 dark:bg-dark-700"
                >
                  <div class="flex flex-wrap items-center gap-2">
                    <span
                      v-for="suffix in registrationEmailSuffixWhitelistTags"
                      :key="suffix"
                      class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-1 text-xs font-mono text-gray-700 dark:bg-dark-600 dark:text-gray-200"
                    >
                      <span class="text-gray-400 dark:text-gray-500">@</span>
                      <span>{{ suffix REDACTEDREDACTED</span>
                      <button
                        type="button"
                        class="rounded-full text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-dark-500 dark:hover:text-white"
                        @click="
                          removeRegistrationEmailSuffixWhitelistTag(suffix)
                        "
                      >
                        <Icon
                          name="x"
                          size="xs"
                          class="h-3.5 w-3.5"
                          :stroke-width="2"
                        />
                      </button>
                    </span>

                    <div
                      class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-primary-300 dark:focus-within:border-primary-700"
                    >
                      <span
                        class="font-mono text-sm text-gray-400 dark:text-gray-500"
                        >@</span
                      >
                      <input
                        v-model="registrationEmailSuffixWhitelistDraft"
                        type="text"
                        class="w-full bg-transparent text-sm font-mono text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
                        :placeholder="
                          t(
                            'admin.settings.registration.emailSuffixWhitelistPlaceholder',
                          )
                        "
                        @input="
                          handleRegistrationEmailSuffixWhitelistDraftInput
                        "
                        @keydown="
                          handleRegistrationEmailSuffixWhitelistDraftKeydown
                        "
                        @blur="commitRegistrationEmailSuffixWhitelistDraft"
                        @paste="handleRegistrationEmailSuffixWhitelistPaste"
                      />
                    </div>
                  </div>
                </div>
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {{
                    t(
                      "admin.settings.registration.emailSuffixWhitelistInputHint",
                    )
                  REDACTEDREDACTED
                </p>
              </div>

              <!-- Promo Code -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.promoCode")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.registration.promoCodeHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.promo_code_enabled" />
              </div>

              <!-- Invitation Code -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.invitationCode")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.registration.invitationCodeHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.invitation_code_enabled" />
              </div>
              <!-- Password Reset - Only show when email verification is enabled -->
              <div
                v-if="form.email_verify_enabled"
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.passwordReset")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.registration.passwordResetHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.password_reset_enabled" />
              </div>
              <!-- Frontend URL - Only show when password reset is enabled -->
              <div
                v-if="form.email_verify_enabled && form.password_reset_enabled"
                class="border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.registration.frontendUrl") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.frontend_url"
                  type="url"
                  class="input"
                  :placeholder="
                    t('admin.settings.registration.frontendUrlPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.registration.frontendUrlHint") REDACTEDREDACTED
                </p>
              </div>

              <!-- TOTP 2FA -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.registration.totp")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.registration.totpHint") REDACTEDREDACTED
                  </p>
                  <!-- Warning when encryption key not configured -->
                  <p
                    v-if="!form.totp_encryption_key_configured"
                    class="mt-2 text-sm text-amber-600 dark:text-amber-400"
                  >
                    {{ t("admin.settings.registration.totpKeyNotConfigured") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle
                  v-model="form.totp_enabled"
                  :disabled="!form.totp_encryption_key_configured"
                />
              </div>
            </div>
          </div>

          <!-- Cloudflare Turnstile Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.turnstile.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.turnstile.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Enable Turnstile -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.turnstile.enableTurnstile")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.turnstile.enableTurnstileHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.turnstile_enabled" />
              </div>

              <!-- Turnstile Keys - Only show when enabled -->
              <div
                v-if="form.turnstile_enabled"
                class="border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div class="grid grid-cols-1 gap-6">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.turnstile.siteKey") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.turnstile_site_key"
                      type="text"
                      class="input font-mono text-sm"
                      placeholder="0x4AAAAAAA..."
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.turnstile.siteKeyHint") REDACTEDREDACTED
                      <a
                        href="https://dash.cloudflare.com/"
                        target="_blank"
                        class="text-primary-600 hover:text-primary-500"
                        >{{
                          t("admin.settings.turnstile.cloudflareDashboard")
                        REDACTEDREDACTED</a
                      >
                    </p>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.turnstile.secretKey") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.turnstile_secret_key"
                      type="password"
                      class="input font-mono text-sm"
                      placeholder="0x4AAAAAAA..."
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        form.turnstile_secret_key_configured
                          ? t(
                              "admin.settings.turnstile.secretKeyConfiguredHint",
                            )
                          : t("admin.settings.turnstile.secretKeyHint")
                      REDACTEDREDACTED
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- LinuxDo Connect OAuth 登录 -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.linuxdo.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.linuxdo.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.linuxdo.enable")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.linuxdo.enableHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.linuxdo_connect_enabled" />
              </div>

              <div
                v-if="form.linuxdo_connect_enabled"
                class="border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div class="grid grid-cols-1 gap-6">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.linuxdo.clientId") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.linuxdo_connect_client_id"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.linuxdo.clientIdPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.linuxdo.clientIdHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.linuxdo.clientSecret") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.linuxdo_connect_client_secret"
                      type="password"
                      class="input font-mono text-sm"
                      :placeholder="
                        form.linuxdo_connect_client_secret_configured
                          ? t(
                              'admin.settings.linuxdo.clientSecretConfiguredPlaceholder',
                            )
                          : t('admin.settings.linuxdo.clientSecretPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        form.linuxdo_connect_client_secret_configured
                          ? t(
                              "admin.settings.linuxdo.clientSecretConfiguredHint",
                            )
                          : t("admin.settings.linuxdo.clientSecretHint")
                      REDACTEDREDACTED
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.linuxdo.redirectUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.linuxdo_connect_redirect_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.linuxdo.redirectUrlPlaceholder')
                      "
                    />
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm w-fit"
                        @click="setAndCopyLinuxdoRedirectUrl"
                      >
                        {{ t("admin.settings.linuxdo.quickSetCopy") REDACTEDREDACTED
                      </button>
                      <code
                        v-if="linuxdoRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                      >
                        {{ linuxdoRedirectUrlSuggestion REDACTEDREDACTED
                      </code>
                    </div>
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.linuxdo.redirectUrlHint") REDACTEDREDACTED
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- WeChat Connect OAuth 登录 -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("微信登录", "WeChat Connect") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{
                  localText(
                    "用于微信开放平台或公众号/小程序的第三方登录配置。",
                    "Third-party login configuration for WeChat Open Platform or Official Account / Mini Program.",
                  )
                REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    localText("启用微信登录", "Enable WeChat Connect")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{
                      localText(
                        "开启后可使用微信第三方登录回调与授权配置。",
                        "Enable this to configure WeChat OAuth callbacks and authorization.",
                      )
                    REDACTEDREDACTED
                  </p>
                </div>
                <Toggle
                  v-model="form.wechat_connect_enabled"
                  data-testid="wechat-connect-enabled"
                />
              </div>

              <div
                v-if="form.wechat_connect_enabled"
                class="space-y-6 border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ localText("AppID", "App ID") REDACTEDREDACTED
                    </label>
                    <input
                      data-testid="wechat-connect-app-id"
                      v-model="form.wechat_connect_app_id"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        localText('微信开放平台 AppID', 'WeChat App ID')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ localText("AppSecret", "App Secret") REDACTEDREDACTED
                    </label>
                    <input
                      data-testid="wechat-connect-app-secret"
                      v-model="form.wechat_connect_app_secret"
                      type="password"
                      class="input font-mono text-sm"
                      :placeholder="
                        form.wechat_connect_app_secret_configured
                          ? localText(
                              '密钥已配置，留空以保留当前值。',
                              'Secret configured. Leave empty to keep the current value.',
                            )
                          : localText(
                              '微信开放平台 AppSecret',
                              'WeChat App Secret',
                            )
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        form.wechat_connect_app_secret_configured
                          ? localText(
                              "密钥已配置，留空以保留当前值。",
                              "Secret configured. Leave empty to keep the current value.",
                            )
                          : localText(
                              "填写后会覆盖当前微信密钥。",
                              "Enter a new secret to replace the current WeChat credential.",
                            )
                      REDACTEDREDACTED
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ localText("模式", "Mode") REDACTEDREDACTED
                    </label>
                    <select
                      data-testid="wechat-connect-mode"
                      v-model="form.wechat_connect_mode"
                      class="input font-mono text-sm"
                      @change="syncWeChatConnectMode"
                    >
                      <option value="open">
                        {{ localText("开放平台", "Open Platform") REDACTEDREDACTED
                      </option>
                      <option value="mp">
                        {{
                          localText(
                            "公众号 / 小程序",
                            "Official Account / Mini Program",
                          )
                        REDACTEDREDACTED
                      </option>
                    </select>
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        localText(
                          "open 对应微信开放平台，mp 对应公众号/小程序授权。",
                          "open maps to WeChat Open Platform, mp maps to Official Account / Mini Program authorization.",
                        )
                      REDACTEDREDACTED
                    </p>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ localText("Scopes", "Scopes") REDACTEDREDACTED
                    </label>
                    <input
                      data-testid="wechat-connect-scopes"
                      v-model="form.wechat_connect_scopes"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        form.wechat_connect_mode === 'mp'
                          ? 'snsapi_userinfo'
                          : 'snsapi_login'
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        localText(
                          "留空时会按模式自动回填默认值。",
                          "Leave empty to use the default scope for the selected mode.",
                        )
                      REDACTEDREDACTED
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ localText("回调地址", "Redirect URL") REDACTEDREDACTED
                    </label>
                    <input
                      data-testid="wechat-connect-redirect-url"
                      v-model="form.wechat_connect_redirect_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        localText(
                          'https://your-site.com/api/v1/auth/oauth/wechat/callback',
                          'https://your-site.com/api/v1/auth/oauth/wechat/callback',
                        )
                      "
                    />
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm w-fit"
                        @click="setAndCopyWeChatRedirectUrl"
                      >
                        {{
                          localText(
                            "使用当前站点生成并复制",
                            "Generate & Copy (current site)",
                          )
                        REDACTEDREDACTED
                      </button>
                      <code
                        v-if="wechatRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                      >
                        {{ wechatRedirectUrlSuggestion REDACTEDREDACTED
                      </code>
                    </div>
                  </div>
                </div>

                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ localText("前端回调地址", "Frontend redirect URL") REDACTEDREDACTED
                  </label>
                  <input
                    data-testid="wechat-connect-frontend-redirect-url"
                    v-model="form.wechat_connect_frontend_redirect_url"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      localText(
                        '/auth/wechat/callback',
                        '/auth/wechat/callback',
                      )
                    "
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      localText(
                        "通常用于前端路由回调地址，需与后端配置保持一致。",
                        "Usually the frontend route callback path; keep it aligned with the backend.",
                      )
                    REDACTEDREDACTED
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- Generic OIDC OAuth 登录 -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.oidc.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.oidc.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.oidc.enable")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.oidc.enableHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.oidc_connect_enabled" />
              </div>

              <div
                v-if="form.oidc_connect_enabled"
                class="space-y-6 border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.providerName") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_provider_name"
                      type="text"
                      class="input"
                      :placeholder="
                        t('admin.settings.oidc.providerNamePlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.clientId") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_client_id"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.clientIdPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.clientSecret") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_client_secret"
                      type="password"
                      class="input font-mono text-sm"
                      :placeholder="
                        form.oidc_connect_client_secret_configured
                          ? t(
                              'admin.settings.oidc.clientSecretConfiguredPlaceholder',
                            )
                          : t('admin.settings.oidc.clientSecretPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        form.oidc_connect_client_secret_configured
                          ? t("admin.settings.oidc.clientSecretConfiguredHint")
                          : t("admin.settings.oidc.clientSecretHint")
                      REDACTEDREDACTED
                    </p>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.issuerUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_issuer_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.issuerUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.discoveryUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_discovery_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.discoveryUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.authorizeUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_authorize_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.authorizeUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.tokenUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_token_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.tokenUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.userinfoUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.jwksUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_jwks_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="t('admin.settings.oidc.jwksUrlPlaceholder')"
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.scopes") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_scopes"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="t('admin.settings.oidc.scopesPlaceholder')"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.oidc.scopesHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.redirectUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_redirect_url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.redirectUrlPlaceholder')
                      "
                    />
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm w-fit"
                        @click="setAndCopyOIDCRedirectUrl"
                      >
                        {{ t("admin.settings.oidc.quickSetCopy") REDACTEDREDACTED
                      </button>
                      <code
                        v-if="oidcRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                      >
                        {{ oidcRedirectUrlSuggestion REDACTEDREDACTED
                      </code>
                    </div>
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.oidc.redirectUrlHint") REDACTEDREDACTED
                    </p>
                  </div>

                  <div class="lg:col-span-2">
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.frontendRedirectUrl") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_frontend_redirect_url"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.frontendRedirectUrlPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.oidc.frontendRedirectUrlHint") REDACTEDREDACTED
                    </p>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.tokenAuthMethod") REDACTEDREDACTED
                    </label>
                    <select
                      v-model="form.oidc_connect_token_auth_method"
                      class="input font-mono text-sm"
                    >
                      <option value="client_secret_post">
                        client_secret_post
                      </option>
                      <option value="client_secret_basic">
                        client_secret_basic
                      </option>
                      <option value="none">none</option>
                    </select>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.clockSkewSeconds") REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="form.oidc_connect_clock_skew_seconds"
                      type="number"
                      min="0"
                      max="600"
                      class="input"
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.allowedSigningAlgs") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_allowed_signing_algs"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.allowedSigningAlgsPlaceholder')
                      "
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div
                    class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                  >
                    <div>
                      <label class="font-medium text-gray-900 dark:text-white">
                        {{ t("admin.settings.oidc.usePkce") REDACTEDREDACTED
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_use_pkce"
                      :disabled="true"
                    />
                  </div>

                  <div
                    class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                  >
                    <div>
                      <label class="font-medium text-gray-900 dark:text-white">
                        {{ t("admin.settings.oidc.validateIdToken") REDACTEDREDACTED
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_validate_id_token"
                      :disabled="true"
                    />
                  </div>

                  <div
                    class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                  >
                    <div>
                      <label class="font-medium text-gray-900 dark:text-white">
                        {{ t("admin.settings.oidc.requireEmailVerified") REDACTEDREDACTED
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_require_email_verified"
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.userinfoEmailPath") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_email_path"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoEmailPathPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.userinfoIdPath") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_id_path"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoIdPathPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.oidc.userinfoUsernamePath") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_username_path"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoUsernamePathPlaceholder')
                      "
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Security — Registration, Turnstile, LinuxDo, OIDC -->

        <!-- Tab: Users -->
        <div v-show="activeTab === 'users'" class="space-y-6">
          <!-- Default Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.defaults.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.defaults.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.defaults.defaultBalance") REDACTEDREDACTED
                  </label>
                  <input
                    v-model.number="form.default_balance"
                    type="number"
                    step="0.01"
                    min="0"
                    class="input"
                    placeholder="0.00"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultBalanceHint") REDACTEDREDACTED
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.defaults.defaultConcurrency") REDACTEDREDACTED
                  </label>
                  <input
                    v-model.number="form.default_concurrency"
                    type="number"
                    min="1"
                    class="input"
                    placeholder="1"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultConcurrencyHint") REDACTEDREDACTED
                  </p>
                </div>
              </div>

              <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
                <div class="mb-3 flex items-center justify-between">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">
                      {{ t("admin.settings.defaults.defaultSubscriptions") REDACTEDREDACTED
                    </label>
                    <p class="text-sm text-gray-500 dark:text-gray-400">
                      {{
                        t("admin.settings.defaults.defaultSubscriptionsHint")
                      REDACTEDREDACTED
                    </p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="addDefaultSubscription"
                    :disabled="subscriptionGroups.length === 0"
                  >
                    {{ t("admin.settings.defaults.addDefaultSubscription") REDACTEDREDACTED
                  </button>
                </div>

                <div
                  v-if="form.default_subscriptions.length === 0"
                  class="rounded border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
                >
                  {{ t("admin.settings.defaults.defaultSubscriptionsEmpty") REDACTEDREDACTED
                </div>

                <div v-else class="space-y-3">
                  <div
                    v-for="(item, index) in form.default_subscriptions"
                    :key="`default-sub-${indexREDACTED`"
                    class="grid grid-cols-1 gap-3 rounded border border-gray-200 p-3 md:grid-cols-[1fr_160px_auto] dark:border-dark-600"
                  >
                    <div>
                      <label
                        class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                      >
                        {{ t("admin.settings.defaults.subscriptionGroup") REDACTEDREDACTED
                      </label>
                      <Select
                        v-model="item.group_id"
                        class="default-sub-group-select"
                        :options="defaultSubscriptionGroupOptions"
                        :placeholder="
                          t('admin.settings.defaults.subscriptionGroup')
                        "
                      >
                        <template #selected="{ option REDACTED">
                          <GroupBadge
                            v-if="option"
                            :name="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).label
                            "
                            :platform="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).platform
                            "
                            :subscription-type="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).subscriptionType
                            "
                            :rate-multiplier="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).rate
                            "
                          />
                          <span v-else class="text-gray-400">
                            {{ t("admin.settings.defaults.subscriptionGroup") REDACTEDREDACTED
                          </span>
                        </template>
                        <template #option="{ option, selected REDACTED">
                          <GroupOptionItem
                            :name="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).label
                            "
                            :platform="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).platform
                            "
                            :subscription-type="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).subscriptionType
                            "
                            :rate-multiplier="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).rate
                            "
                            :description="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).description
                            "
                            :selected="selected"
                          />
                        </template>
                      </Select>
                    </div>
                    <div>
                      <label
                        class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                      >
                        {{
                          t("admin.settings.defaults.subscriptionValidityDays")
                        REDACTEDREDACTED
                      </label>
                      <input
                        v-model.number="item.validity_days"
                        type="number"
                        min="1"
                        max="36500"
                        class="input h-[42px]"
                      />
                    </div>
                    <div class="flex items-end">
                      <button
                        type="button"
                        class="btn btn-secondary default-sub-delete-btn w-full text-red-600 hover:text-red-700 dark:text-red-400"
                        @click="removeDefaultSubscription(index)"
                      >
                        {{ t("common.delete") REDACTEDREDACTED
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("认证来源默认值", "Auth Source Defaults") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{
                  localText(
                    "按注册来源配置新用户默认余额、并发、订阅与授权策略。",
                    "Configure per-source default balance, concurrency, subscriptions, and grant rules.",
                  )
                REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div
                class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{
                      localText(
                        "第三方注册强制补充邮箱",
                        "Require email on third-party signup",
                      )
                    REDACTEDREDACTED
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{
                      localText(
                        "启用后，Linux DO、OIDC、微信注册缺少邮箱时必须先补充邮箱地址。",
                        "When enabled, Linux DO, OIDC, and WeChat signups must provide an email before account creation.",
                      )
                    REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.force_email_on_third_party_signup" />
              </div>

              <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
                <div
                  v-for="authSource in authSourceDefaultsMeta"
                  :key="authSource.source"
                  class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"
                >
                  <div class="mb-4">
                    <div class="font-medium text-gray-900 dark:text-white">
                      {{ authSource.title REDACTEDREDACTED
                    </div>
                    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {{ authSource.description REDACTEDREDACTED
                    </p>
                  </div>

                  <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <label
                        class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                      >
                        {{ t("admin.settings.defaults.defaultBalance") REDACTEDREDACTED
                      </label>
                      <input
                        v-model.number="
                          authSourceDefaults[authSource.source].balance
                        "
                        type="number"
                        step="0.01"
                        min="0"
                        class="input"
                        placeholder="0.00"
                      />
                    </div>
                    <div>
                      <label
                        class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                      >
                        {{ t("admin.settings.defaults.defaultConcurrency") REDACTEDREDACTED
                      </label>
                      <input
                        v-model.number="
                          authSourceDefaults[authSource.source].concurrency
                        "
                        type="number"
                        min="1"
                        class="input"
                        placeholder="5"
                      />
                    </div>
                  </div>

                  <div class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
                    <div
                      class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                    >
                      <div>
                        <label
                          class="font-medium text-gray-900 dark:text-white"
                        >
                          {{ localText("注册即授权", "Grant on signup") REDACTEDREDACTED
                        </label>
                        <p
                          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
                        >
                          {{
                            localText(
                              "来源首次注册成功后立即发放默认权益。",
                              "Grant default entitlements immediately after signup.",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                      <Toggle
                        v-model="
                          authSourceDefaults[authSource.source].grant_on_signup
                        "
                      />
                    </div>

                    <div
                      class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                    >
                      <div>
                        <label
                          class="font-medium text-gray-900 dark:text-white"
                        >
                          {{
                            localText("首次绑定时授权", "Grant on first bind")
                          REDACTEDREDACTED
                        </label>
                        <p
                          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
                        >
                          {{
                            localText(
                              "来源首次绑定到现有账号时发放默认权益。",
                              "Grant default entitlements when the source is first bound to an existing user.",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                      <Toggle
                        v-model="
                          authSourceDefaults[authSource.source]
                            .grant_on_first_bind
                        "
                      />
                    </div>
                  </div>

                  <div
                    class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700"
                  >
                    <div class="mb-3 flex items-center justify-between">
                      <div>
                        <label
                          class="font-medium text-gray-900 dark:text-white"
                        >
                          {{ localText("默认订阅", "Default subscriptions") REDACTEDREDACTED
                        </label>
                        <p class="text-sm text-gray-500 dark:text-gray-400">
                          {{
                            localText(
                              "仅对当前认证来源生效，未配置时不追加来源专属订阅。",
                              "Applies only to this auth source. Leave empty to skip source-specific subscriptions.",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        @click="
                          addAuthSourceDefaultSubscription(authSource.source)
                        "
                        :disabled="subscriptionGroups.length === 0"
                      >
                        {{
                          t("admin.settings.defaults.addDefaultSubscription")
                        REDACTEDREDACTED
                      </button>
                    </div>

                    <div
                      v-if="
                        authSourceDefaults[authSource.source].subscriptions
                          .length === 0
                      "
                      class="rounded border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
                    >
                      {{
                        localText(
                          "当前来源未配置专属默认订阅。",
                          "No source-specific default subscriptions configured.",
                        )
                      REDACTEDREDACTED
                    </div>

                    <div v-else class="space-y-3">
                      <div
                        v-for="(item, index) in authSourceDefaults[
                          authSource.source
                        ].subscriptions"
                        :key="`${authSource.sourceREDACTED-sub-${indexREDACTED`"
                        class="grid grid-cols-1 gap-3 rounded border border-gray-200 p-3 md:grid-cols-[1fr_160px_auto] dark:border-dark-600"
                      >
                        <div>
                          <label
                            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                          >
                            {{ t("admin.settings.defaults.subscriptionGroup") REDACTEDREDACTED
                          </label>
                          <Select
                            v-model="item.group_id"
                            class="default-sub-group-select"
                            :options="defaultSubscriptionGroupOptions"
                            :placeholder="
                              t('admin.settings.defaults.subscriptionGroup')
                            "
                          >
                            <template #selected="{ option REDACTED">
                              <GroupBadge
                                v-if="option"
                                :name="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).label
                                "
                                :platform="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).platform
                                "
                                :subscription-type="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).subscriptionType
                                "
                                :rate-multiplier="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).rate
                                "
                              />
                              <span v-else class="text-gray-400">
                                {{
                                  t("admin.settings.defaults.subscriptionGroup")
                                REDACTEDREDACTED
                              </span>
                            </template>
                            <template #option="{ option, selected REDACTED">
                              <GroupOptionItem
                                :name="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).label
                                "
                                :platform="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).platform
                                "
                                :subscription-type="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).subscriptionType
                                "
                                :rate-multiplier="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).rate
                                "
                                :description="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).description
                                "
                                :selected="selected"
                              />
                            </template>
                          </Select>
                        </div>
                        <div>
                          <label
                            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                          >
                            {{
                              t(
                                "admin.settings.defaults.subscriptionValidityDays",
                              )
                            REDACTEDREDACTED
                          </label>
                          <input
                            v-model.number="item.validity_days"
                            type="number"
                            min="1"
                            max="36500"
                            class="input h-[42px]"
                          />
                        </div>
                        <div class="flex items-end">
                          <button
                            type="button"
                            class="btn btn-secondary w-full text-red-600 hover:text-red-700 dark:text-red-400"
                            @click="
                              removeAuthSourceDefaultSubscription(
                                authSource.source,
                                index,
                              )
                            "
                          >
                            {{ t("common.delete") REDACTEDREDACTED
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Users -->

        <!-- Tab: Gateway — Claude Code, Scheduling -->
        <div v-show="activeTab === 'gateway'" class="space-y-6">
          <!-- Claude Code Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.claudeCode.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.claudeCode.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="p-6">
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.claudeCode.minVersion") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.min_claude_code_version"
                  type="text"
                  class="input max-w-xs font-mono text-sm"
                  :placeholder="
                    t('admin.settings.claudeCode.minVersionPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.claudeCode.minVersionHint") REDACTEDREDACTED
                </p>
              </div>
              <div class="mt-4">
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.claudeCode.maxVersion") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.max_claude_code_version"
                  type="text"
                  class="input max-w-xs font-mono text-sm"
                  :placeholder="
                    t('admin.settings.claudeCode.maxVersionPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.claudeCode.maxVersionHint") REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Gateway Scheduling Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.scheduling.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.scheduling.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="p-6">
              <div class="space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <label
                      class="text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.scheduling.allowUngroupedKey") REDACTEDREDACTED
                    </label>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.scheduling.allowUngroupedKeyHint") REDACTEDREDACTED
                    </p>
                  </div>
                  <label class="toggle">
                    <input
                      v-model="form.allow_ungrouped_key_scheduling"
                      type="checkbox"
                    />
                    <span class="toggle-slider"></span>
                  </label>
                </div>

                <div
                  class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                >
                  <div>
                    <label
                      class="text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{
                        localText(
                          "OpenAI 实验调度策略",
                          "OpenAI experimental scheduler policy",
                        )
                      REDACTEDREDACTED
                    </label>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                      {{
                        localText(
                          "默认关闭。开启后仅影响本网关在 OpenAI 账号间的实验性调度选择逻辑，不代表上游 OpenAI 官方能力。",
                          "Disabled by default. When enabled, this only changes the gateway's experimental account-selection policy for OpenAI traffic; it does not indicate an upstream OpenAI capability.",
                        )
                      REDACTEDREDACTED
                    </p>
                  </div>
                  <Toggle v-model="form.openai_advanced_scheduler_enabled" />
                </div>
              </div>
            </div>
          </div>

          <!-- Gateway Forwarding Behavior -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.gatewayForwarding.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.gatewayForwarding.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Fingerprint Unification -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.fingerprintUnification",
                      )
                    REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.fingerprintUnificationHint",
                      )
                    REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.enable_fingerprint_unification" />
              </div>

              <!-- Metadata Passthrough -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{
                      t("admin.settings.gatewayForwarding.metadataPassthrough")
                    REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.metadataPassthroughHint",
                      )
                    REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.enable_metadata_passthrough" />
              </div>

              <!-- CCH Signing -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.gatewayForwarding.cchSigning") REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.gatewayForwarding.cchSigningHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.enable_cch_signing" />
              </div>
            </div>
          </div>
          <!-- Web Search Emulation -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.webSearchEmulation.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.webSearchEmulation.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Global Toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.webSearchEmulation.enabled") REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.webSearchEmulation.enabledHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="webSearchConfig.enabled" />
              </div>

              <!-- Providers -->
              <div v-if="webSearchConfig.enabled" class="space-y-4">
                <div class="flex items-center justify-between">
                  <label
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.webSearchEmulation.providers") REDACTEDREDACTED
                  </label>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="addWebSearchProvider"
                  >
                    {{ t("admin.settings.webSearchEmulation.addProvider") REDACTEDREDACTED
                  </button>
                </div>

                <div
                  v-if="webSearchConfig.providers.length === 0"
                  class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-400 dark:border-dark-600"
                >
                  {{ t("admin.settings.webSearchEmulation.noProviders") REDACTEDREDACTED
                </div>

                <div
                  v-for="(provider, pIdx) in webSearchConfig.providers"
                  :key="pIdx"
                  class="rounded-lg border border-gray-200 dark:border-dark-600"
                >
                  <!-- Collapsible header -->
                  <div
                    class="flex cursor-pointer items-center justify-between px-4 py-3"
                    @click="toggleProviderExpand(pIdx)"
                  >
                    <div class="flex items-center gap-3">
                      <svg
                        class="h-4 w-4 text-gray-400 transition-transform"
                        :class="{ 'rotate-90': expandedProviders[pIdx] REDACTED"
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
                          { value: 'brave', label: 'Brave Search' REDACTED,
                          { value: 'tavily', label: 'Tavily' REDACTED,
                        ]"
                        class="w-36"
                        @click.stop
                      />
                      <!-- Quota summary (always visible) -->
                      <span class="text-xs text-gray-400">
                        {{ provider.quota_used ?? 0 REDACTEDREDACTED /
                        {{
                          provider.quota_limit != null &&
                          provider.quota_limit > 0
                            ? provider.quota_limit
                            : "∞"
                        REDACTEDREDACTED
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
                        REDACTEDREDACTED
                      </span>
                    </div>
                    <button
                      type="button"
                      class="text-red-500 hover:text-red-700 text-xs"
                      @click.stop="removeWebSearchProvider(pIdx)"
                    >
                      {{
                        t("admin.settings.webSearchEmulation.removeProvider")
                      REDACTEDREDACTED
                    </button>
                  </div>

                  <!-- Expanded content -->
                  <div
                    v-if="expandedProviders[pIdx]"
                    class="space-y-3 border-t border-gray-100 px-4 pb-4 pt-3 dark:border-dark-700"
                  >
                    <!-- API Key with inline show/copy -->
                    <div>
                      <label class="text-xs text-gray-500">{{
                        t("admin.settings.webSearchEmulation.apiKey")
                      REDACTEDREDACTED</label>
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
                            class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
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
                            class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                            :class="{
                              'opacity-30 cursor-not-allowed':
                                !provider.api_key,
                            REDACTED"
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
                        <label class="text-xs text-gray-500">{{
                          t("admin.settings.webSearchEmulation.quotaLimit")
                        REDACTEDREDACTED</label>
                        <input
                          v-model="provider.quota_limit"
                          type="number"
                          min="1"
                          class="input text-sm"
                          :placeholder="'∞'"
                        />
                        <p class="mt-0.5 text-xs text-gray-400">
                          {{
                            t(
                              "admin.settings.webSearchEmulation.quotaLimitHint",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                      <div>
                        <label class="text-xs text-gray-500">{{
                          t("admin.settings.webSearchEmulation.subscribedAt")
                        REDACTEDREDACTED</label>
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
                        <p class="mt-0.5 text-xs text-gray-400">
                          {{
                            t(
                              "admin.settings.webSearchEmulation.subscribedAtHint",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                    </div>

                    <!-- Usage display -->
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-gray-500"
                        >{{
                          t("admin.settings.webSearchEmulation.quotaUsage")
                        REDACTEDREDACTED:</span
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
                              ? 'bg-red-500'
                              : quotaPercentage(provider) > 70
                                ? 'bg-yellow-500'
                                : 'bg-green-500'
                          "
                          :style="{
                            width:
                              Math.min(quotaPercentage(provider), 100) + '%',
                          REDACTED"
                        />
                      </div>
                      <div v-else class="flex-1" />
                      <span class="text-xs text-gray-500"
                        >{{ provider.quota_used ?? 0 REDACTEDREDACTED /
                        {{
                          provider.quota_limit != null &&
                          provider.quota_limit > 0
                            ? provider.quota_limit
                            : "∞"
                        REDACTEDREDACTED</span
                      >
                      <button
                        v-if="(provider.quota_used ?? 0) > 0"
                        type="button"
                        class="text-xs text-primary-600 hover:text-primary-700"
                        @click="resetWebSearchUsage(pIdx)"
                      >
                        {{ t("admin.settings.webSearchEmulation.resetUsage") REDACTEDREDACTED
                      </button>
                    </div>

                    <!-- Proxy + Test on same row -->
                    <div class="flex items-end gap-3">
                      <div class="flex-1">
                        <label class="text-xs text-gray-500">{{
                          t("admin.settings.webSearchEmulation.proxy")
                        REDACTEDREDACTED</label>
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
                        {{ t("admin.settings.webSearchEmulation.test") REDACTEDREDACTED
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
              class="mx-4 w-full max-w-lg rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800"
            >
              <h3
                class="mb-4 text-lg font-semibold text-gray-900 dark:text-white"
              >
                {{ t("admin.settings.webSearchEmulation.testResultTitle") REDACTEDREDACTED
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
                  REDACTEDREDACTED
                </button>
              </div>
              <!-- Test results -->
              <div
                v-if="wsTestResult"
                class="mt-4 max-h-80 overflow-y-auto rounded-lg bg-gray-50 p-4 dark:bg-dark-700"
              >
                <p
                  class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{
                    t("admin.settings.webSearchEmulation.testResultProvider")
                  REDACTEDREDACTED: {{ wsTestResult.provider REDACTEDREDACTED
                </p>
                <div
                  v-if="wsTestResult.results.length === 0"
                  class="text-sm text-gray-400"
                >
                  {{ t("admin.settings.webSearchEmulation.testNoResults") REDACTEDREDACTED
                </div>
                <div
                  v-for="(r, rIdx) in wsTestResult.results"
                  :key="rIdx"
                  class="mt-2 border-t border-gray-200 pt-2 first:mt-0 first:border-0 first:pt-0 dark:border-dark-600"
                >
                  <a
                    :href="r.url"
                    target="_blank"
                    class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
                    >{{ r.title REDACTEDREDACTED</a
                  >
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ r.snippet REDACTEDREDACTED
                  </p>
                </div>
              </div>
              <div class="mt-4 flex justify-end">
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="wsTestDialogOpen = false"
                >
                  {{ t("common.close") REDACTEDREDACTED
                </button>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Gateway — Claude Code, Scheduling -->

        <!-- Tab: General -->
        <div v-show="activeTab === 'general'" class="space-y-6">
          <!-- Site Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.site.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.site.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-6 p-6">
              <!-- Backend Mode -->
              <div
                class="flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20"
              >
                <div>
                  <h3 class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t("admin.settings.site.backendMode") REDACTEDREDACTED
                  </h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.site.backendModeDescription") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.backend_mode_enabled" />
              </div>

              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.site.siteName") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.site_name"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.site.siteNamePlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.site.siteNameHint") REDACTEDREDACTED
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.site.siteSubtitle") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.site_subtitle"
                    type="text"
                    class="input"
                    :placeholder="
                      t('admin.settings.site.siteSubtitlePlaceholder')
                    "
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.site.siteSubtitleHint") REDACTEDREDACTED
                  </p>
                </div>
              </div>

              <!-- API Base URL -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.apiBaseUrl") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.api_base_url"
                  type="text"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.site.apiBaseUrlPlaceholder')"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.apiBaseUrlHint") REDACTEDREDACTED
                </p>
              </div>

              <!-- Global Table Preferences -->
              <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
                <h3 class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ t("admin.settings.site.tablePreferencesTitle") REDACTEDREDACTED
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.tablePreferencesDescription") REDACTEDREDACTED
                </p>
                <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.site.tableDefaultPageSize") REDACTEDREDACTED
                    </label>
                    <input
                      v-model.number="form.table_default_page_size"
                      type="number"
                      min="5"
                      max="1000"
                      step="1"
                      class="input w-40"
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.site.tableDefaultPageSizeHint") REDACTEDREDACTED
                    </p>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.site.tablePageSizeOptions") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="tablePageSizeOptionsInput"
                      type="text"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.site.tablePageSizeOptionsPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.site.tablePageSizeOptionsHint") REDACTEDREDACTED
                    </p>
                  </div>
                </div>
              </div>

              <!-- Custom Endpoints -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.customEndpoints.title") REDACTEDREDACTED
                </label>
                <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.customEndpoints.description") REDACTEDREDACTED
                </p>

                <div class="space-y-3">
                  <div
                    v-for="(ep, index) in form.custom_endpoints"
                    :key="index"
                    class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
                  >
                    <div class="mb-3 flex items-center justify-between">
                      <span
                        class="text-sm font-medium text-gray-700 dark:text-gray-300"
                      >
                        {{
                          t("admin.settings.site.customEndpoints.itemLabel", {
                            n: index + 1,
                          REDACTED)
                        REDACTEDREDACTED
                      </span>
                      <button
                        type="button"
                        class="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                        @click="removeEndpoint(index)"
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                          />
                        </svg>
                      </button>
                    </div>
                    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <div>
                        <label
                          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                        >
                          {{ t("admin.settings.site.customEndpoints.name") REDACTEDREDACTED
                        </label>
                        <input
                          v-model="ep.name"
                          type="text"
                          class="input text-sm"
                          :placeholder="
                            t(
                              'admin.settings.site.customEndpoints.namePlaceholder',
                            )
                          "
                        />
                      </div>
                      <div>
                        <label
                          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                        >
                          {{
                            t("admin.settings.site.customEndpoints.endpointUrl")
                          REDACTEDREDACTED
                        </label>
                        <input
                          v-model="ep.endpoint"
                          type="url"
                          class="input font-mono text-sm"
                          :placeholder="
                            t(
                              'admin.settings.site.customEndpoints.endpointUrlPlaceholder',
                            )
                          "
                        />
                      </div>
                      <div class="sm:col-span-2">
                        <label
                          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                        >
                          {{
                            t(
                              "admin.settings.site.customEndpoints.descriptionLabel",
                            )
                          REDACTEDREDACTED
                        </label>
                        <input
                          v-model="ep.description"
                          type="text"
                          class="input text-sm"
                          :placeholder="
                            t(
                              'admin.settings.site.customEndpoints.descriptionPlaceholder',
                            )
                          "
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <button
                  type="button"
                  class="mt-3 flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-gray-300 px-4 py-2.5 text-sm text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400"
                  @click="addEndpoint"
                >
                  <svg
                    class="h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 4v16m8-8H4"
                    />
                  </svg>
                  {{ t("admin.settings.site.customEndpoints.add") REDACTEDREDACTED
                </button>
              </div>

              <!-- Contact Info -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.contactInfo") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.contact_info"
                  type="text"
                  class="input"
                  :placeholder="t('admin.settings.site.contactInfoPlaceholder')"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.contactInfoHint") REDACTEDREDACTED
                </p>
              </div>

              <!-- Doc URL -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.docUrl") REDACTEDREDACTED
                </label>
                <input
                  v-model="form.doc_url"
                  type="url"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.site.docUrlPlaceholder')"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.docUrlHint") REDACTEDREDACTED
                </p>
              </div>

              <!-- Site Logo Upload -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.siteLogo") REDACTEDREDACTED
                </label>
                <ImageUpload
                  v-model="form.site_logo"
                  mode="image"
                  :upload-label="t('admin.settings.site.uploadImage')"
                  :remove-label="t('admin.settings.site.remove')"
                  :hint="t('admin.settings.site.logoHint')"
                  :max-size="300 * 1024"
                />
              </div>

              <!-- Home Content -->
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t("admin.settings.site.homeContent") REDACTEDREDACTED
                </label>
                <textarea
                  v-model="form.home_content"
                  rows="6"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.site.homeContentPlaceholder')"
                ></textarea>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.site.homeContentHint") REDACTEDREDACTED
                </p>
                <!-- iframe CSP Warning -->
                <p class="mt-2 text-xs text-amber-600 dark:text-amber-400">
                  {{ t("admin.settings.site.homeContentIframeWarning") REDACTEDREDACTED
                </p>
              </div>

              <!-- Hide CCS Import Button -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.site.hideCcsImportButton")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.site.hideCcsImportButtonHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.hide_ccs_import_button" />
              </div>
            </div>
          </div>

          <!-- Custom Menu Items -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.customMenu.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.customMenu.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Existing menu items -->
              <div
                v-for="(item, index) in form.custom_menu_items"
                :key="item.id || index"
                class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
              >
                <div class="mb-3 flex items-center justify-between">
                  <span
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{
                      t("admin.settings.customMenu.itemLabel", { n: index + 1 REDACTED)
                    REDACTEDREDACTED
                  </span>
                  <div class="flex items-center gap-2">
                    <!-- Move up -->
                    <button
                      v-if="index > 0"
                      type="button"
                      class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700"
                      :title="t('admin.settings.customMenu.moveUp')"
                      @click="moveMenuItem(index, -1)"
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M5 15l7-7 7 7"
                        />
                      </svg>
                    </button>
                    <!-- Move down -->
                    <button
                      v-if="index < form.custom_menu_items.length - 1"
                      type="button"
                      class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700"
                      :title="t('admin.settings.customMenu.moveDown')"
                      @click="moveMenuItem(index, 1)"
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M19 9l-7 7-7-7"
                        />
                      </svg>
                    </button>
                    <!-- Delete -->
                    <button
                      type="button"
                      class="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                      :title="t('admin.settings.customMenu.remove')"
                      @click="removeMenuItem(index)"
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </button>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <!-- Label -->
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.customMenu.name") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="item.label"
                      type="text"
                      class="input text-sm"
                      :placeholder="
                        t('admin.settings.customMenu.namePlaceholder')
                      "
                    />
                  </div>

                  <!-- Visibility -->
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.customMenu.visibility") REDACTEDREDACTED
                    </label>
                    <select v-model="item.visibility" class="input text-sm">
                      <option value="user">
                        {{ t("admin.settings.customMenu.visibilityUser") REDACTEDREDACTED
                      </option>
                      <option value="admin">
                        {{ t("admin.settings.customMenu.visibilityAdmin") REDACTEDREDACTED
                      </option>
                    </select>
                  </div>

                  <!-- URL (full width) -->
                  <div class="sm:col-span-2">
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.customMenu.url") REDACTEDREDACTED
                    </label>
                    <input
                      v-model="item.url"
                      type="url"
                      class="input font-mono text-sm"
                      :placeholder="
                        t('admin.settings.customMenu.urlPlaceholder')
                      "
                    />
                  </div>

                  <!-- SVG Icon (full width) -->
                  <div class="sm:col-span-2">
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
                    >
                      {{ t("admin.settings.customMenu.iconSvg") REDACTEDREDACTED
                    </label>
                    <ImageUpload
                      :model-value="item.icon_svg"
                      mode="svg"
                      size="sm"
                      :upload-label="t('admin.settings.customMenu.uploadSvg')"
                      :remove-label="t('admin.settings.customMenu.removeSvg')"
                      @update:model-value="(v: string) => (item.icon_svg = v)"
                    />
                  </div>
                </div>
              </div>

              <!-- Add button -->
              <button
                type="button"
                class="flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-gray-300 py-3 text-sm text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400"
                @click="addMenuItem"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                {{ t("admin.settings.customMenu.add") REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
        <!-- /Tab: General -->

        <!-- Tab: Email -->
        <!-- Tab: Payment -->
        <div v-show="activeTab === 'payment'" class="space-y-6">
          <!-- Payment System Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.payment.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.payment.description") REDACTEDREDACTED
                <a
                  :href="
                    locale === 'zh'
                      ? 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md'
                      : 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT.md'
                  "
                  target="_blank"
                  rel="noopener noreferrer"
                  class="ml-2 inline-flex items-center text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  <svg
                    class="mr-0.5 h-3.5 w-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                    />
                  </svg>
                  {{ t("admin.settings.payment.configGuide") REDACTEDREDACTED
                </a>
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Enable toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.payment.enabled")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.payment.enabledHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.payment_enabled" />
              </div>
              <template v-if="form.payment_enabled">
                <!-- Row 1: Product name -->
                <div class="grid grid-cols-3 gap-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.productNamePrefix")
                    REDACTEDREDACTED</label
                    ><input
                      v-model="form.payment_product_name_prefix"
                      type="text"
                      class="input"
                      placeholder="Sub2API"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.productNameSuffix")
                    REDACTEDREDACTED</label
                    ><input
                      v-model="form.payment_product_name_suffix"
                      type="text"
                      class="input"
                      placeholder="CNY"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.preview")
                    REDACTEDREDACTED</label>
                    <div
                      class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
                    >
                      {{
                        (form.payment_product_name_prefix || "Sub2API") +
                        " 100 " +
                        (form.payment_product_name_suffix || "CNY")
                      REDACTEDREDACTED
                    </div>
                  </div>
                </div>
                <!-- Row 2: Balance toggle + amounts -->
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.minAmount")
                    REDACTEDREDACTED</label
                    ><input
                      :value="form.payment_min_amount || ''"
                      @input="
                        form.payment_min_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.maxAmount")
                    REDACTEDREDACTED</label
                    ><input
                      :value="form.payment_max_amount || ''"
                      @input="
                        form.payment_max_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.dailyLimit")
                    REDACTEDREDACTED</label
                    ><input
                      :value="form.payment_daily_limit || ''"
                      @input="
                        form.payment_daily_limit =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.balanceRechargeMultiplier")
                    REDACTEDREDACTED</label>
                    <input
                      :value="form.payment_balance_recharge_multiplier || ''"
                      @input="
                        form.payment_balance_recharge_multiplier =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 1
                      "
                      type="number"
                      step="0.01"
                      min="0.01"
                      class="input"
                    />
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{
                        t(
                          "admin.settings.payment.balanceRechargeMultiplierHint",
                        )
                      REDACTEDREDACTED
                    </p>
                    <p
                      class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400"
                    >
                      {{
                        t("admin.settings.payment.balanceRechargePreview", {
                          usd: (
                            Number(form.payment_balance_recharge_multiplier) ||
                            1
                          ).toFixed(2),
                        REDACTED)
                      REDACTEDREDACTED
                    </p>
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.rechargeFeeRate")
                    REDACTEDREDACTED</label>
                    <div class="relative">
                      <input
                        :value="form.payment_recharge_fee_rate ?? ''"
                        @input="
                          form.payment_recharge_fee_rate = Math.min(
                            100,
                            Math.max(
                              0,
                              Math.round(
                                parseFloat(
                                  ($event.target as HTMLInputElement).value ||
                                    '0',
                                ) * 100,
                              ) / 100,
                            ),
                          )
                        "
                        type="number"
                        step="0.01"
                        min="0"
                        max="100"
                        class="input pr-8"
                      />
                      <span
                        class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400"
                        >%</span
                      >
                    </div>
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{ t("admin.settings.payment.rechargeFeeRateHint") REDACTEDREDACTED
                    </p>
                    <p
                      v-if="(Number(form.payment_recharge_fee_rate) || 0) > 0"
                      class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400"
                    >
                      {{
                        t("admin.settings.payment.rechargeFeePreview", {
                          fee: (
                            Number(form.payment_recharge_fee_rate) || 0
                          ).toFixed(2),
                        REDACTED)
                      REDACTEDREDACTED
                    </p>
                  </div>
                  <div>
                    <label class="input-label"
                      >{{ t("admin.settings.payment.orderTimeout") REDACTEDREDACTED
                      <span class="text-red-500">*</span></label
                    ><input
                      v-model.number="form.payment_order_timeout_minutes"
                      type="number"
                      min="1"
                      class="input"
                      required
                    />
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{ t("admin.settings.payment.orderTimeoutHint") REDACTEDREDACTED
                    </p>
                  </div>
                </div>
                <!-- Row 3: Pending orders + load balance + cancel rate limit (all in one row) -->
                <div class="flex flex-wrap items-end gap-4">
                  <div class="w-28">
                    <label class="input-label">{{
                      t("admin.settings.payment.maxPendingOrders")
                    REDACTEDREDACTED</label
                    ><input
                      v-model.number="form.payment_max_pending_orders"
                      type="number"
                      min="1"
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.loadBalanceStrategy")
                    REDACTEDREDACTED</label>
                    <Select
                      v-model="form.payment_load_balance_strategy"
                      :options="loadBalanceOptions"
                      class="w-40"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.cancelRateLimit")
                    REDACTEDREDACTED</label>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        :class="[
                          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                          form.payment_cancel_rate_limit_enabled
                            ? 'bg-primary-500'
                            : 'bg-gray-300 dark:bg-dark-600',
                        ]"
                        @click="
                          form.payment_cancel_rate_limit_enabled =
                            !form.payment_cancel_rate_limit_enabled
                        "
                      >
                        <span
                          :class="[
                            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                            form.payment_cancel_rate_limit_enabled
                              ? 'translate-x-5'
                              : 'translate-x-0',
                          ]"
                        />
                      </button>
                      <Select
                        v-model="form.payment_cancel_rate_limit_window_mode"
                        :options="cancelRateLimitModeOptions"
                        class="w-24"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitEvery")
                        REDACTEDREDACTED</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_window"
                        type="number"
                        min="1"
                        required
                        class="input w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <Select
                        v-model="form.payment_cancel_rate_limit_unit"
                        :options="cancelRateLimitUnitOptions"
                        class="w-28"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitAllowMax")
                        REDACTEDREDACTED</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_max"
                        type="number"
                        min="1"
                        required
                        class="input w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitTimes")
                        REDACTEDREDACTED</span
                      >
                    </div>
                  </div>
                </div>
                <!-- Row 4: Enabled payment types (provider badges like sub2apipay) -->
                <div>
                  <label class="input-label">{{
                    t("admin.settings.payment.enabledPaymentTypes")
                  REDACTEDREDACTED</label>
                  <div class="mt-1.5 flex flex-wrap gap-2">
                    <button
                      v-for="pt in allPaymentTypes"
                      :key="pt.value"
                      type="button"
                      @click="togglePaymentType(pt.value)"
                      :class="[
                        'rounded-lg border px-3 py-1.5 text-sm font-medium transition-all',
                        isPaymentTypeEnabled(pt.value)
                          ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                          : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
                      ]"
                    >
                      {{ pt.label REDACTEDREDACTED
                    </button>
                  </div>
                  <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                    {{ t("admin.settings.payment.enabledPaymentTypesHint") REDACTEDREDACTED
                    <a
                      :href="
                        locale === 'zh'
                          ? 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md#%E6%94%AF%E6%8C%81%E7%9A%84%E6%94%AF%E4%BB%98%E6%96%B9%E5%BC%8F'
                          : 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT.md#supported-payment-methods'
                      "
                      target="_blank"
                      rel="noopener noreferrer"
                      class="ml-1 text-primary-500 hover:text-primary-600 dark:text-primary-400 dark:hover:text-primary-300"
                    >
                      {{ t("admin.settings.payment.findProvider") REDACTEDREDACTED
                      <svg
                        class="mb-0.5 ml-0.5 inline h-3 w-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                    </a>
                  </p>
                </div>
                <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
                  <div
                    v-for="visibleMethod in paymentVisibleMethodCards"
                    :key="visibleMethod.key"
                    class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
                  >
                    <div class="flex items-center justify-between">
                      <div>
                        <label
                          class="font-medium text-gray-900 dark:text-white"
                        >
                          {{
                            localText(
                              `${visibleMethod.titleREDACTED 可见方式`,
                              `${visibleMethod.titleREDACTED visible method`,
                            )
                          REDACTEDREDACTED
                        </label>
                        <p
                          class="mt-1 text-sm text-gray-500 dark:text-gray-400"
                        >
                          {{
                            localText(
                              "控制前台结算页是否展示该方式，以及展示时使用的来源键。",
                              "Controls whether checkout shows this method and which source key it exposes.",
                            )
                          REDACTEDREDACTED
                        </p>
                      </div>
                      <Toggle
                        :model-value="
                          getPaymentVisibleMethodEnabled(visibleMethod.key)
                        "
                        @update:model-value="
                          setPaymentVisibleMethodEnabled(
                            visibleMethod.key,
                            $event,
                          )
                        "
                      />
                    </div>

                    <div class="mt-4">
                      <label class="input-label">
                        {{ localText("支付来源", "Payment source") REDACTEDREDACTED
                      </label>
                      <Select
                        :model-value="
                          getPaymentVisibleMethodSource(visibleMethod.key)
                        "
                        :options="
                          getPaymentVisibleMethodSourceSelectOptions(
                            visibleMethod.key,
                          )
                        "
                        @update:model-value="
                          setPaymentVisibleMethodSource(
                            visibleMethod.key,
                            $event,
                          )
                        "
                        :placeholder="visibleMethod.key"
                      />
                      <p class="mt-1.5 text-xs text-gray-400">
                        {{
                          localText(
                            "启用后必须明确选择一个来源；未配置状态不会对外展示该支付方式。",
                            "Choose an explicit source before enabling the method. Not configured methods are not exposed.",
                          )
                        REDACTEDREDACTED
                      </p>
                    </div>
                  </div>
                </div>
                <!-- Row 5: Help image + text -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.helpImage")
                    REDACTEDREDACTED</label>
                    <ImageUpload
                      v-model="form.payment_help_image_url"
                      :placeholder="
                        t('admin.settings.payment.helpImagePlaceholder')
                      "
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.helpText")
                    REDACTEDREDACTED</label>
                    <textarea
                      v-model="form.payment_help_text"
                      rows="3"
                      class="input"
                      :placeholder="
                        t('admin.settings.payment.helpTextPlaceholder')
                      "
                    ></textarea>
                  </div>
                </div>
              </template>
            </div>
          </div>

          <!-- Provider Management -->
          <PaymentProviderList
            v-if="form.payment_enabled"
            :providers="providers"
            :loading="providersLoading"
            :can-create="hasAnyPaymentTypeEnabled"
            :enabled-payment-types="form.payment_enabled_types"
            :all-payment-types="allPaymentTypes"
            :redirect-label="t('admin.settings.payment.easypayRedirect')"
            @refresh="loadProviders"
            @create="openCreateProvider"
            @edit="openEditProvider"
            @delete="confirmDeleteProvider"
            @toggle-field="handleToggleField"
            @toggle-type="handleToggleType"
            @reorder="handleReorderProviders"
          />
        </div>

        <div v-show="activeTab === 'email'" class="space-y-6">
          <!-- Email disabled hint - show when email_verify_enabled is off -->
          <div v-if="!form.email_verify_enabled" class="card">
            <div class="p-6">
              <div class="flex items-start gap-3">
                <Icon
                  name="mail"
                  size="md"
                  class="mt-0.5 flex-shrink-0 text-gray-400 dark:text-gray-500"
                />
                <div>
                  <h3 class="font-medium text-gray-900 dark:text-white">
                    {{ t("admin.settings.emailTabDisabledTitle") REDACTEDREDACTED
                  </h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.emailTabDisabledHint") REDACTEDREDACTED
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- SMTP Settings - Only show when email verification is enabled -->
          <div v-if="form.email_verify_enabled" class="card">
            <div
              class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t("admin.settings.smtp.title") REDACTEDREDACTED
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.smtp.description") REDACTEDREDACTED
                </p>
              </div>
              <button
                type="button"
                @click="testSmtpConnection"
                :disabled="testingSmtp || loadFailed"
                class="btn btn-secondary btn-sm"
              >
                <svg
                  v-if="testingSmtp"
                  class="h-4 w-4 animate-spin"
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
                {{
                  testingSmtp
                    ? t("admin.settings.smtp.testing")
                    : t("admin.settings.smtp.testConnection")
                REDACTEDREDACTED
              </button>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.host") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.smtp_host"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.smtp.hostPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.port") REDACTEDREDACTED
                  </label>
                  <input
                    v-model.number="form.smtp_port"
                    type="number"
                    min="1"
                    max="65535"
                    class="input"
                    :placeholder="t('admin.settings.smtp.portPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.username") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.smtp_username"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.smtp.usernamePlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.password") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.smtp_password"
                    type="password"
                    class="input"
                    autocomplete="new-password"
                    autocapitalize="off"
                    spellcheck="false"
                    @keydown="smtpPasswordManuallyEdited = true"
                    @paste="smtpPasswordManuallyEdited = true"
                    :placeholder="
                      form.smtp_password_configured
                        ? t('admin.settings.smtp.passwordConfiguredPlaceholder')
                        : t('admin.settings.smtp.passwordPlaceholder')
                    "
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      form.smtp_password_configured
                        ? t("admin.settings.smtp.passwordConfiguredHint")
                        : t("admin.settings.smtp.passwordHint")
                    REDACTEDREDACTED
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.fromEmail") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.smtp_from_email"
                    type="email"
                    class="input"
                    :placeholder="t('admin.settings.smtp.fromEmailPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.smtp.fromName") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.smtp_from_name"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.smtp.fromNamePlaceholder')"
                  />
                </div>
              </div>

              <!-- Use TLS Toggle -->
              <div
                class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.smtp.useTls")
                  REDACTEDREDACTED</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.smtp.useTlsHint") REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="form.smtp_use_tls" />
              </div>
            </div>
          </div>

          <!-- Send Test Email - Only show when email verification is enabled -->
          <div v-if="form.email_verify_enabled" class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.testEmail.title") REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.testEmail.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="p-6">
              <div class="flex items-end gap-4">
                <div class="flex-1">
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.testEmail.recipientEmail") REDACTEDREDACTED
                  </label>
                  <input
                    v-model="testEmailAddress"
                    type="email"
                    class="input"
                    :placeholder="
                      t('admin.settings.testEmail.recipientEmailPlaceholder')
                    "
                  />
                </div>
                <button
                  type="button"
                  @click="sendTestEmail"
                  :disabled="
                    sendingTestEmail || !testEmailAddress || loadFailed
                  "
                  class="btn btn-secondary"
                >
                  <svg
                    v-if="sendingTestEmail"
                    class="h-4 w-4 animate-spin"
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
                  {{
                    sendingTestEmail
                      ? t("admin.settings.testEmail.sending")
                      : t("admin.settings.testEmail.sendTestEmail")
                  REDACTEDREDACTED
                </button>
              </div>
            </div>
          </div>
          <!-- Balance Low Notification -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h3 class="text-base font-medium text-gray-900 dark:text-white">
                {{ t("admin.settings.balanceNotify.title") REDACTEDREDACTED
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.balanceNotify.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="px-6 py-6 space-y-4">
              <div class="flex items-center justify-between">
                <label
                  class="mb-0 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t("admin.settings.balanceNotify.enabled") REDACTEDREDACTED</label
                >
                <Toggle v-model="form.balance_low_notify_enabled" />
              </div>
              <div v-if="form.balance_low_notify_enabled">
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t("admin.settings.balanceNotify.threshold") REDACTEDREDACTED</label
                >
                <div class="relative">
                  <span
                    class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    >$</span
                  >
                  <input
                    v-model.number="form.balance_low_notify_threshold"
                    type="number"
                    min="0"
                    step="0.01"
                    class="input pl-7"
                  />
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.balanceNotify.thresholdHint") REDACTEDREDACTED
                </p>
              </div>
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t("admin.settings.balanceNotify.rechargeUrl") REDACTEDREDACTED</label
                >
                <input
                  v-model="form.balance_low_notify_recharge_url"
                  type="url"
                  class="input"
                  :placeholder="currentOrigin"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.balanceNotify.rechargeUrlHint") REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Account Quota Notification -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h3 class="text-base font-medium text-gray-900 dark:text-white">
                {{ t("admin.settings.quotaNotify.title") REDACTEDREDACTED
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.quotaNotify.description") REDACTEDREDACTED
              </p>
            </div>
            <div class="px-6 py-6 space-y-4">
              <div class="flex items-center justify-between">
                <label
                  class="mb-0 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t("admin.settings.quotaNotify.enabled") REDACTEDREDACTED</label
                >
                <Toggle v-model="form.account_quota_notify_enabled" />
              </div>
              <div v-if="form.account_quota_notify_enabled">
                <label
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t("admin.settings.quotaNotify.emails") REDACTEDREDACTED</label
                >
                <div class="space-y-2">
                  <div
                    v-for="(entry, index) in form.account_quota_notify_emails ||
                    []"
                    :key="index"
                    class="flex items-center gap-2"
                  >
                    <label
                      class="relative inline-flex items-center cursor-pointer shrink-0"
                    >
                      <input
                        type="checkbox"
                        :checked="!entry.disabled"
                        @change="entry.disabled = !entry.disabled"
                        class="sr-only peer"
                      />
                      <div
                        class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer dark:bg-gray-600 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all dark:after:border-gray-500 peer-checked:bg-primary-600"
                      ></div>
                    </label>
                    <input
                      v-model="entry.email"
                      type="email"
                      class="input flex-1"
                      :placeholder="
                        t('admin.settings.quotaNotify.emailPlaceholder')
                      "
                    />
                    <button
                      @click="form.account_quota_notify_emails.splice(index, 1)"
                      class="btn btn-secondary px-2"
                      type="button"
                    >
                      <Icon name="x" size="xs" class="h-4 w-4" />
                    </button>
                  </div>
                  <button
                    @click="addQuotaNotifyEmail"
                    class="btn btn-secondary btn-sm"
                    type="button"
                  >
                    + {{ t("admin.settings.quotaNotify.addEmail") REDACTEDREDACTED
                  </button>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.quotaNotify.emailsHint") REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Email -->

        <!-- Tab: Backup -->
        <div v-show="activeTab === 'backup'">
          <BackupSettings />
        </div>

        <!-- Save Button -->
        <div v-show="activeTab !== 'backup'" class="flex justify-end">
          <button
            type="submit"
            :disabled="saving || loadFailed"
            class="btn btn-primary"
          >
            <svg
              v-if="saving"
              class="h-4 w-4 animate-spin"
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
            {{
              saving
                ? t("admin.settings.saving")
                : t("admin.settings.saveSettings")
            REDACTEDREDACTED
          </button>
        </div>
      </form>

      <!-- Provider dialogs placed outside the settings form to prevent form submission bubbling -->
      <PaymentProviderDialog
        ref="providerDialogRef"
        :show="showProviderDialog"
        :saving="providerSaving"
        :editing="editingProvider"
        :all-key-options="providerKeyOptions"
        :enabled-key-options="enabledProviderKeyOptions"
        :all-payment-types="allPaymentTypes"
        :redirect-label="t('admin.settings.payment.easypayRedirect')"
        @close="showProviderDialog = false"
        @save="handleSaveProvider"
      />
      <ConfirmDialog
        :show="showDeleteProviderDialog"
        :title="t('admin.settings.payment.deleteProvider')"
        :message="t('admin.settings.payment.deleteProviderConfirm')"
        :confirm-text="t('common.delete')"
        danger
        @confirm="handleDeleteProvider"
        @cancel="showDeleteProviderDialog = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted REDACTED from "vue";
import { useI18n REDACTED from "vue-i18n";
import { adminAPI REDACTED from "@/api";
import {
  appendAuthSourceDefaultsToUpdateRequest,
  defaultWeChatConnectScopesForMode,
  buildAuthSourceDefaultsState,
  getPaymentVisibleMethodSourceOptions,
  normalizePaymentVisibleMethodSource,
  normalizeDefaultSubscriptionSettings,
  normalizeWeChatConnectMode,
REDACTED from "@/api/admin/settings";
import type {
  AuthSourceDefaultsState,
  AuthSourceType,
  PaymentVisibleMethod,
  SystemSettings,
  UpdateSettingsRequest,
  DefaultSubscriptionSetting,
  WebSearchEmulationConfig,
  WebSearchProviderConfig,
  WebSearchTestResult,
REDACTED from "@/api/admin/settings";
import type { AdminGroup, Proxy, NotifyEmailEntry REDACTED from "@/types";
import type { ProviderInstance REDACTED from "@/types/payment";
import AppLayout from "@/components/layout/AppLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import PaymentProviderList from "@/components/payment/PaymentProviderList.vue";
import PaymentProviderDialog from "@/components/payment/PaymentProviderDialog.vue";
import GroupBadge from "@/components/common/GroupBadge.vue";
import GroupOptionItem from "@/components/common/GroupOptionItem.vue";
import Toggle from "@/components/common/Toggle.vue";
import ProxySelector from "@/components/common/ProxySelector.vue";
import ImageUpload from "@/components/common/ImageUpload.vue";
import BackupSettings from "@/views/admin/BackupView.vue";
import { useClipboard REDACTED from "@/composables/useClipboard";
import { extractApiErrorMessage REDACTED from "@/utils/apiError";
import { useAppStore REDACTED from "@/stores";
import { useAdminSettingsStore REDACTED from "@/stores/adminSettings";
import {
  isRegistrationEmailSuffixDomainValid,
  normalizeRegistrationEmailSuffixDomain,
  normalizeRegistrationEmailSuffixDomains,
  parseRegistrationEmailSuffixWhitelistInput,
REDACTED from "@/utils/registrationEmailPolicy";

const { t, locale REDACTED = useI18n();
const appStore = useAppStore();
const adminSettingsStore = useAdminSettingsStore();

function localText(zh: string, en: string): string {
  return locale.value.startsWith("zh") ? zh : en;
REDACTED

type SettingsTab =
  | "general"
  | "security"
  | "users"
  | "gateway"
  | "payment"
  | "email"
  | "backup";
const activeTab = ref<SettingsTab>("general");
const settingsTabs = [
  { key: "general" as SettingsTab, icon: "home" as const REDACTED,
  { key: "security" as SettingsTab, icon: "shield" as const REDACTED,
  { key: "users" as SettingsTab, icon: "user" as const REDACTED,
  { key: "gateway" as SettingsTab, icon: "server" as const REDACTED,
  { key: "payment" as SettingsTab, icon: "creditCard" as const REDACTED,
  { key: "email" as SettingsTab, icon: "mail" as const REDACTED,
  { key: "backup" as SettingsTab, icon: "database" as const REDACTED,
];
const { copyToClipboard REDACTED = useClipboard();

const loading = ref(true);
const loadFailed = ref(false);
const saving = ref(false);
const testingSmtp = ref(false);
const sendingTestEmail = ref(false);
const smtpPasswordManuallyEdited = ref(false);
const testEmailAddress = ref("");
const registrationEmailSuffixWhitelistTags = ref<string[]>([]);
const registrationEmailSuffixWhitelistDraft = ref("");
const tablePageSizeOptionsInput = ref("10, 20, 50, 100");

// Admin API Key 状态
const adminApiKeyLoading = ref(true);
const adminApiKeyExists = ref(false);
const adminApiKeyMasked = ref("");
const adminApiKeyOperating = ref(false);
const newAdminApiKey = ref("");
const subscriptionGroups = ref<AdminGroup[]>([]);

// Overload Cooldown (529) 状态
const overloadCooldownLoading = ref(true);
const overloadCooldownSaving = ref(false);
const overloadCooldownForm = reactive({
  enabled: true,
  cooldown_minutes: 10,
REDACTED);

// Stream Timeout 状态
const streamTimeoutLoading = ref(true);
const streamTimeoutSaving = ref(false);
const streamTimeoutForm = reactive({
  enabled: true,
  action: "temp_unsched" as "temp_unsched" | "error" | "none",
  temp_unsched_minutes: 5,
  threshold_count: 3,
  threshold_window_minutes: 10,
REDACTED);

// Rectifier 状态
const rectifierLoading = ref(true);
const rectifierSaving = ref(false);
const rectifierForm = reactive({
  enabled: true,
  thinking_signature_enabled: true,
  thinking_budget_enabled: true,
  apikey_signature_enabled: false,
  apikey_signature_patterns: [] as string[],
REDACTED);

// Beta Policy 状态
const betaPolicyLoading = ref(true);
const betaPolicySaving = ref(false);
const betaPolicyForm = reactive({
  rules: [] as Array<{
    beta_token: string;
    action: "pass" | "filter" | "block";
    scope: "all" | "oauth" | "apikey" | "bedrock";
    error_message?: string;
    model_whitelist?: string[];
    fallback_action?: "pass" | "filter" | "block";
    fallback_error_message?: string;
  REDACTED>,
REDACTED);

const tablePageSizeMin = 5;
const tablePageSizeMax = 1000;
const tablePageSizeDefault = 20;

interface DefaultSubscriptionGroupOption {
  value: number;
  label: string;
  description: string | null;
  platform: AdminGroup["platform"];
  subscriptionType: AdminGroup["subscription_type"];
  rate: number;
  [key: string]: unknown;
REDACTED

type SettingsForm = SystemSettings & {
  smtp_password: string;
  turnstile_secret_key: string;
  linuxdo_connect_client_secret: string;
  wechat_connect_app_secret: string;
  oidc_connect_client_secret: string;
  force_email_on_third_party_signup: boolean;
  payment_visible_method_alipay_source: string;
  payment_visible_method_wxpay_source: string;
  payment_visible_method_alipay_enabled: boolean;
  payment_visible_method_wxpay_enabled: boolean;
  openai_advanced_scheduler_enabled: boolean;
REDACTED;

const form = reactive<SettingsForm>({
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  invitation_code_enabled: false,
  password_reset_enabled: false,
  totp_enabled: false,
  totp_encryption_key_configured: false,
  default_balance: 0,
  default_concurrency: 1,
  default_subscriptions: [],
  force_email_on_third_party_signup: false,
  site_name: "Sub2API",
  site_logo: "",
  site_subtitle: "Subscription to API Conversion Platform",
  api_base_url: "",
  contact_info: "",
  doc_url: "",
  home_content: "",
  backend_mode_enabled: false,
  hide_ccs_import_button: false,
  payment_enabled: false,
  payment_min_amount: 1,
  payment_max_amount: 10000,
  payment_daily_limit: 50000,
  payment_max_pending_orders: 3,
  payment_order_timeout_minutes: 30,
  payment_balance_disabled: false,
  payment_balance_recharge_multiplier: 1,
  payment_recharge_fee_rate: 0,
  payment_enabled_types: [],
  payment_help_image_url: "",
  payment_help_text: "",
  payment_product_name_prefix: "",
  payment_product_name_suffix: "",
  payment_load_balance_strategy: "round-robin",
  payment_cancel_rate_limit_enabled: false,
  payment_cancel_rate_limit_max: 10,
  payment_cancel_rate_limit_window: 1,
  payment_cancel_rate_limit_unit: "day",
  payment_cancel_rate_limit_window_mode: "rolling",
  payment_visible_method_alipay_source: "",
  payment_visible_method_wxpay_source: "",
  payment_visible_method_alipay_enabled: false,
  payment_visible_method_wxpay_enabled: false,
  table_default_page_size: tablePageSizeDefault,
  table_page_size_options: [10, 20, 50, 100],
  custom_menu_items: [] as Array<{
    id: string;
    label: string;
    icon_svg: string;
    url: string;
    visibility: "user" | "admin";
    sort_order: number;
  REDACTED>,
  custom_endpoints: [] as Array<{
    name: string;
    endpoint: string;
    description: string;
  REDACTED>,
  frontend_url: "",
  smtp_host: "",
  smtp_port: 587,
  smtp_username: "",
  smtp_password: "",
  smtp_password_configured: false,
  smtp_from_email: "",
  smtp_from_name: "",
  smtp_use_tls: true,
  // Cloudflare Turnstile
  turnstile_enabled: false,
  turnstile_site_key: "",
  turnstile_secret_key: "",
  turnstile_secret_key_configured: false,
  // LinuxDo Connect OAuth 登录
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: "",
  linuxdo_connect_client_secret: "",
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: "",
  wechat_connect_enabled: false,
  wechat_connect_app_id: "",
  wechat_connect_app_secret: "",
  wechat_connect_app_secret_configured: false,
  wechat_connect_mode: "open",
  wechat_connect_scopes: "snsapi_login",
  wechat_connect_redirect_url: "",
  wechat_connect_frontend_redirect_url: "/auth/wechat/callback",
  // Generic OIDC OAuth 登录
  oidc_connect_enabled: false,
  oidc_connect_provider_name: "OIDC",
  oidc_connect_client_id: "",
  oidc_connect_client_secret: "",
  oidc_connect_client_secret_configured: false,
  oidc_connect_issuer_url: "",
  oidc_connect_discovery_url: "",
  oidc_connect_authorize_url: "",
  oidc_connect_token_url: "",
  oidc_connect_userinfo_url: "",
  oidc_connect_jwks_url: "",
  oidc_connect_scopes: "openid email profile",
  oidc_connect_redirect_url: "",
  oidc_connect_frontend_redirect_url: "/auth/oidc/callback",
  oidc_connect_token_auth_method: "client_secret_post",
  oidc_connect_use_pkce: true,
  oidc_connect_validate_id_token: true,
  oidc_connect_allowed_signing_algs: "RS256,ES256,PS256",
  oidc_connect_clock_skew_seconds: 120,
  oidc_connect_require_email_verified: false,
  oidc_connect_userinfo_email_path: "",
  oidc_connect_userinfo_id_path: "",
  oidc_connect_userinfo_username_path: "",
  // Model fallback
  enable_model_fallback: false,
  fallback_model_anthropic: "claude-3-5-sonnet-20241022",
  fallback_model_openai: "gpt-4o",
  fallback_model_gemini: "gemini-2.5-pro",
  fallback_model_antigravity: "gemini-2.5-pro",
  // Identity patch (Claude -> Gemini)
  enable_identity_patch: true,
  identity_patch_prompt: "",
  // Ops monitoring (vNext)
  ops_monitoring_enabled: true,
  ops_realtime_monitoring_enabled: true,
  ops_query_mode_default: "auto",
  ops_metrics_interval_seconds: 60,
  // Claude Code version check
  min_claude_code_version: "",
  max_claude_code_version: "",
  // 分组隔离
  allow_ungrouped_key_scheduling: false,
  openai_advanced_scheduler_enabled: false,
  // Gateway forwarding behavior
  enable_fingerprint_unification: true,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  // Balance & quota notification
  balance_low_notify_enabled: false,
  balance_low_notify_threshold: 0,
  balance_low_notify_recharge_url: "",
  account_quota_notify_enabled: false,
  account_quota_notify_emails: [] as NotifyEmailEntry[],
REDACTED);

const authSourceDefaults = reactive<AuthSourceDefaultsState>(
  buildAuthSourceDefaultsState({REDACTED),
);

const authSourceDefaultsMeta = computed(() => [
  {
    source: "email" as AuthSourceType,
    title: localText("邮箱注册", "Email signup"),
    description: localText(
      "适用于邮箱密码注册的新用户默认配额。",
      "Default quota grants for email-password signups.",
    ),
  REDACTED,
  {
    source: "linuxdo" as AuthSourceType,
    title: localText("Linux DO 登录", "Linux DO signup"),
    description: localText(
      "适用于 Linux DO 第三方注册的新用户默认配额。",
      "Default quota grants for Linux DO signups.",
    ),
  REDACTED,
  {
    source: "oidc" as AuthSourceType,
    title: localText("OIDC 登录", "OIDC signup"),
    description: localText(
      "适用于 OIDC 第三方注册的新用户默认配额。",
      "Default quota grants for OIDC signups.",
    ),
  REDACTED,
  {
    source: "wechat" as AuthSourceType,
    title: localText("微信登录", "WeChat signup"),
    description: localText(
      "适用于微信第三方注册的新用户默认配额。",
      "Default quota grants for WeChat signups.",
    ),
  REDACTED,
]);

const paymentVisibleMethodCards = computed(() => [
  {
    key: "alipay" as const,
    title: t("payment.methods.alipay"),
    enabledField: "payment_visible_method_alipay_enabled" as const,
    sourceField: "payment_visible_method_alipay_source" as const,
  REDACTED,
  {
    key: "wxpay" as const,
    title: t("payment.methods.wxpay"),
    enabledField: "payment_visible_method_wxpay_enabled" as const,
    sourceField: "payment_visible_method_wxpay_source" as const,
  REDACTED,
]);

function getPaymentVisibleMethodEnabled(method: "alipay" | "wxpay"): boolean {
  return method === "alipay"
    ? form.payment_visible_method_alipay_enabled
    : form.payment_visible_method_wxpay_enabled;
REDACTED

function setPaymentVisibleMethodEnabled(
  method: "alipay" | "wxpay",
  enabled: boolean,
) {
  if (method === "alipay") {
    form.payment_visible_method_alipay_enabled = enabled;
    return;
  REDACTED
  form.payment_visible_method_wxpay_enabled = enabled;
REDACTED

function getPaymentVisibleMethodSource(method: "alipay" | "wxpay"): string {
  return method === "alipay"
    ? form.payment_visible_method_alipay_source
    : form.payment_visible_method_wxpay_source;
REDACTED

function getPaymentVisibleMethodSourceSelectOptions(
  method: PaymentVisibleMethod,
) {
  return getPaymentVisibleMethodSourceOptions(method).map((option) => ({
    value: option.value,
    label: localText(option.labelZh, option.labelEn),
  REDACTED));
REDACTED

function setPaymentVisibleMethodSource(
  method: "alipay" | "wxpay",
  source: string | number | boolean | null,
) {
  const normalized = normalizePaymentVisibleMethodSource(method, source);
  if (method === "alipay") {
    form.payment_visible_method_alipay_source = normalized;
    return;
  REDACTED
  form.payment_visible_method_wxpay_source = normalized;
REDACTED

function validatePaymentVisibleMethodSelections(): boolean {
  for (const visibleMethod of paymentVisibleMethodCards.value) {
    if (!getPaymentVisibleMethodEnabled(visibleMethod.key)) {
      continue;
    REDACTED

    if (getPaymentVisibleMethodSource(visibleMethod.key)) {
      continue;
    REDACTED

    appStore.showError(
      localText(
        `${visibleMethod.titleREDACTED 已启用，请先选择支付来源`,
        `Select a payment source before enabling ${visibleMethod.titleREDACTED`,
      ),
    );
    return false;
  REDACTED

  return true;
REDACTED

// Proxies for web search emulation ProxySelector
const webSearchProxies = ref<Proxy[]>([]);

// Web Search Emulation config (loaded/saved separately)
const DEFAULT_WEB_SEARCH_QUOTA_LIMIT = 1000;

const webSearchConfig = reactive<WebSearchEmulationConfig>({
  enabled: false,
  providers: [],
REDACTED);

const expandedProviders = reactive<Record<number, boolean>>({REDACTED);
const apiKeyVisible = reactive<Record<number, boolean>>({REDACTED);
const wsTestQuery = ref("");
const wsTestLoading = ref(false);
const wsTestResult = ref<WebSearchTestResult | null>(null);
const wsTestDialogOpen = ref(false);

function openTestDialog() {
  wsTestResult.value = null;
  wsTestDialogOpen.value = true;
REDACTED

function toggleProviderExpand(idx: number) {
  expandedProviders[idx] = !expandedProviders[idx];
REDACTED

function removeWebSearchProvider(idx: number) {
  webSearchConfig.providers.splice(idx, 1);
  // Re-index expandedProviders and apiKeyVisible after removal
  const newExpanded: Record<number, boolean> = {REDACTED;
  const newVisible: Record<number, boolean> = {REDACTED;
  for (let i = 0; i < webSearchConfig.providers.length; i++) {
    const oldIdx = i >= idx ? i + 1 : i;
    newExpanded[i] = expandedProviders[oldIdx] ?? false;
    newVisible[i] = apiKeyVisible[oldIdx] ?? false;
  REDACTED
  Object.keys(expandedProviders).forEach(
    (k) => delete expandedProviders[Number(k)],
  );
  Object.keys(apiKeyVisible).forEach((k) => delete apiKeyVisible[Number(k)]);
  Object.assign(expandedProviders, newExpanded);
  Object.assign(apiKeyVisible, newVisible);
REDACTED

function addWebSearchProvider() {
  const idx = webSearchConfig.providers.length;
  webSearchConfig.providers.push({
    type: "brave",
    api_key: "",
    api_key_configured: false,
    quota_limit: DEFAULT_WEB_SEARCH_QUOTA_LIMIT,
    subscribed_at: null,
    proxy_id: null,
    expires_at: null,
  REDACTED as WebSearchProviderConfig);
  expandedProviders[idx] = true;
REDACTED

function formatSubscribedAt(ts: number | null): string {
  if (!ts) return "";
  // Use UTC to avoid timezone drift on repeated edits
  const d = new Date(ts * 1000);
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return `${yREDACTED-${mREDACTED-${dayREDACTED`;
REDACTED

function parseSubscribedAt(dateStr: string): number | null {
  if (!dateStr) return null;
  // Parse as UTC to match formatSubscribedAt
  return Math.floor(new Date(dateStr + "T00:00:00Z").getTime() / 1000);
REDACTED

function quotaPercentage(provider: WebSearchProviderConfig): number {
  if (!provider.quota_limit || provider.quota_limit <= 0) return 0;
  return ((provider.quota_used ?? 0) / provider.quota_limit) * 100;
REDACTED

async function resetWebSearchUsage(idx: number) {
  const provider = webSearchConfig.providers[idx];
  if (!provider) return;
  if (!confirm(t("admin.settings.webSearchEmulation.resetUsageConfirm")))
    return;
  try {
    await adminAPI.settings.resetWebSearchUsage({
      provider_type: provider.type,
    REDACTED);
    provider.quota_used = 0;
    appStore.showSuccess(
      t("admin.settings.webSearchEmulation.resetUsageSuccess"),
    );
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED
REDACTED

async function copyApiKey(idx: number) {
  const key = webSearchConfig.providers[idx]?.api_key;
  if (!key) {
    appStore.showError(
      t("admin.settings.webSearchEmulation.apiKeyPlaceholder"),
    );
    return;
  REDACTED
  try {
    await navigator.clipboard.writeText(key);
    appStore.showSuccess(t("admin.settings.webSearchEmulation.copied"));
  REDACTED catch {
    appStore.showError(t("common.error"));
  REDACTED
REDACTED

async function testWebSearchProvider() {
  wsTestLoading.value = true;
  wsTestResult.value = null;
  try {
    const query =
      wsTestQuery.value.trim() ||
      t("admin.settings.webSearchEmulation.testDefaultQuery");
    wsTestResult.value = await adminAPI.settings.testWebSearchEmulation(query);
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    wsTestLoading.value = false;
  REDACTED
REDACTED

async function loadWebSearchConfig() {
  try {
    const [resp, proxiesResp] = await Promise.all([
      adminAPI.settings.getWebSearchEmulationConfig(),
      adminAPI.proxies.list().catch(() => ({ items: [] as Proxy[] REDACTED)),
    ]);
    if (resp) {
      webSearchConfig.enabled = resp.enabled || false;
      webSearchConfig.providers = resp.providers || [];
    REDACTED
    webSearchProxies.value = proxiesResp.items || [];
  REDACTED catch (err: unknown) {
    // 404 is expected when config hasn't been created yet; show error for other failures
    const status = (err as { status?: number REDACTED)?.status;
    if (status !== 404 && status !== undefined) {
      appStore.showError(extractApiErrorMessage(err, t("common.error")));
    REDACTED
  REDACTED
REDACTED

async function saveWebSearchConfig(): Promise<boolean> {
  try {
    for (const p of webSearchConfig.providers) {
      const raw = p.quota_limit;
      if (raw != null && Number(raw) !== 0 && Number(raw) < 1) {
        appStore.showError(
          t("admin.settings.webSearchEmulation.quotaLimitMustBePositive"),
        );
        return false;
      REDACTED
    REDACTED
    const providers = webSearchConfig.providers.map(
      (p: WebSearchProviderConfig) => ({
        ...p,
        quota_limit: Number(p.quota_limit) > 0 ? Number(p.quota_limit) : null,
      REDACTED),
    );
    await adminAPI.settings.updateWebSearchEmulationConfig({
      enabled: webSearchConfig.enabled,
      providers,
    REDACTED);
    return true;
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
    return false;
  REDACTED
REDACTED

const defaultSubscriptionGroupOptions = computed<
  DefaultSubscriptionGroupOption[]
>(() =>
  subscriptionGroups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    platform: group.platform,
    subscriptionType: group.subscription_type,
    rate: group.rate_multiplier,
  REDACTED)),
);

const registrationEmailSuffixWhitelistSeparatorKeys = new Set([
  " ",
  ",",
  "，",
  "Enter",
  "Tab",
]);

function removeRegistrationEmailSuffixWhitelistTag(suffix: string) {
  registrationEmailSuffixWhitelistTags.value =
    registrationEmailSuffixWhitelistTags.value.filter(
      (item) => item !== suffix,
    );
REDACTED

function addRegistrationEmailSuffixWhitelistTag(raw: string) {
  const suffix = normalizeRegistrationEmailSuffixDomain(raw);
  if (
    !isRegistrationEmailSuffixDomainValid(suffix) ||
    registrationEmailSuffixWhitelistTags.value.includes(suffix)
  ) {
    return;
  REDACTED
  registrationEmailSuffixWhitelistTags.value = [
    ...registrationEmailSuffixWhitelistTags.value,
    suffix,
  ];
REDACTED

function commitRegistrationEmailSuffixWhitelistDraft() {
  if (!registrationEmailSuffixWhitelistDraft.value) {
    return;
  REDACTED
  addRegistrationEmailSuffixWhitelistTag(
    registrationEmailSuffixWhitelistDraft.value,
  );
  registrationEmailSuffixWhitelistDraft.value = "";
REDACTED

function handleRegistrationEmailSuffixWhitelistDraftInput() {
  registrationEmailSuffixWhitelistDraft.value =
    normalizeRegistrationEmailSuffixDomain(
      registrationEmailSuffixWhitelistDraft.value,
    );
REDACTED

function handleRegistrationEmailSuffixWhitelistDraftKeydown(
  event: KeyboardEvent,
) {
  if (event.isComposing) {
    return;
  REDACTED

  if (registrationEmailSuffixWhitelistSeparatorKeys.has(event.key)) {
    event.preventDefault();
    commitRegistrationEmailSuffixWhitelistDraft();
    return;
  REDACTED

  if (
    event.key === "Backspace" &&
    !registrationEmailSuffixWhitelistDraft.value &&
    registrationEmailSuffixWhitelistTags.value.length > 0
  ) {
    registrationEmailSuffixWhitelistTags.value.pop();
  REDACTED
REDACTED

function handleRegistrationEmailSuffixWhitelistPaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData("text") || "";
  if (!text.trim()) {
    return;
  REDACTED
  event.preventDefault();
  const tokens = parseRegistrationEmailSuffixWhitelistInput(text);
  for (const token of tokens) {
    addRegistrationEmailSuffixWhitelistTag(token);
  REDACTED
REDACTED

// Quota notify email helpers
const addQuotaNotifyEmail = () => {
  if (!form.account_quota_notify_emails) {
    form.account_quota_notify_emails = [];
  REDACTED
  form.account_quota_notify_emails.push({
    email: "",
    disabled: false,
    verified: true,
  REDACTED);
REDACTED;

const currentOrigin =
  typeof window !== "undefined" ? window.location.origin : "";

// LinuxDo OAuth redirect URL suggestion
const linuxdoRedirectUrlSuggestion = computed(() => {
  if (typeof window === "undefined") return "";
  const origin =
    window.location.origin ||
    `${window.location.protocolREDACTED//${window.location.hostREDACTED`;
  return `${originREDACTED/api/v1/auth/oauth/linuxdo/callback`;
REDACTED);

async function setAndCopyLinuxdoRedirectUrl() {
  const url = linuxdoRedirectUrlSuggestion.value;
  if (!url) return;

  form.linuxdo_connect_redirect_url = url;
  await copyToClipboard(
    url,
    t("admin.settings.linuxdo.redirectUrlSetAndCopied"),
  );
REDACTED

const wechatRedirectUrlSuggestion = computed(() => {
  if (typeof window === "undefined") return "";
  const origin =
    window.location.origin ||
    `${window.location.protocolREDACTED//${window.location.hostREDACTED`;
  return `${originREDACTED/api/v1/auth/oauth/wechat/callback`;
REDACTED);

function syncWeChatConnectMode() {
  form.wechat_connect_mode = normalizeWeChatConnectMode(
    form.wechat_connect_mode,
  );
  if (!form.wechat_connect_scopes.trim()) {
    form.wechat_connect_scopes = defaultWeChatConnectScopesForMode(
      form.wechat_connect_mode,
    );
  REDACTED
REDACTED

async function setAndCopyWeChatRedirectUrl() {
  const url = wechatRedirectUrlSuggestion.value;
  if (!url) return;

  form.wechat_connect_redirect_url = url;
  await copyToClipboard(
    url,
    localText(
      "已使用当前站点生成回调地址并复制到剪贴板",
      "Redirect URL generated and copied to clipboard",
    ),
  );
REDACTED

const oidcRedirectUrlSuggestion = computed(() => {
  if (typeof window === "undefined") return "";
  const origin =
    window.location.origin ||
    `${window.location.protocolREDACTED//${window.location.hostREDACTED`;
  return `${originREDACTED/api/v1/auth/oauth/oidc/callback`;
REDACTED);

async function setAndCopyOIDCRedirectUrl() {
  const url = oidcRedirectUrlSuggestion.value;
  if (!url) return;

  form.oidc_connect_redirect_url = url;
  await copyToClipboard(url, t("admin.settings.oidc.redirectUrlSetAndCopied"));
REDACTED

// Custom menu item management
function addMenuItem() {
  form.custom_menu_items.push({
    id: "",
    label: "",
    icon_svg: "",
    url: "",
    visibility: "user",
    sort_order: form.custom_menu_items.length,
  REDACTED);
REDACTED

function removeMenuItem(index: number) {
  form.custom_menu_items.splice(index, 1);
  // Re-index sort_order
  form.custom_menu_items.forEach((item, i) => {
    item.sort_order = i;
  REDACTED);
REDACTED

function moveMenuItem(index: number, direction: -1 | 1) {
  const targetIndex = index + direction;
  if (targetIndex < 0 || targetIndex >= form.custom_menu_items.length) return;
  const items = form.custom_menu_items;
  const temp = items[index];
  items[index] = items[targetIndex];
  items[targetIndex] = temp;
  // Re-index sort_order
  items.forEach((item, i) => {
    item.sort_order = i;
  REDACTED);
REDACTED

// Custom endpoint management
function addEndpoint() {
  form.custom_endpoints.push({ name: "", endpoint: "", description: "" REDACTED);
REDACTED

function removeEndpoint(index: number) {
  form.custom_endpoints.splice(index, 1);
REDACTED

function formatTablePageSizeOptions(options: number[]): string {
  return options.join(", ");
REDACTED

function parseTablePageSizeOptionsInput(raw: string): number[] | null {
  const tokens = raw
    .split(",")
    .map((token) => token.trim())
    .filter((token) => token.length > 0);

  if (tokens.length === 0) {
    return null;
  REDACTED

  const parsed = tokens.map((token) => Number(token));
  if (parsed.some((value) => !Number.isInteger(value))) {
    return null;
  REDACTED

  const deduped = Array.from(new Set(parsed)).sort((a, b) => a - b);
  if (
    deduped.some(
      (value) => value < tablePageSizeMin || value > tablePageSizeMax,
    )
  ) {
    return null;
  REDACTED

  return deduped;
REDACTED

async function loadSettings() {
  loading.value = true;
  loadFailed.value = false;
  try {
    const settings = await adminAPI.settings.getSettings();
    settings.payment_load_balance_strategy =
      settings.payment_load_balance_strategy || "round-robin";
    // Only assign non-null values from backend (null means unconfigured, keep defaults)
    for (const [key, value] of Object.entries(settings)) {
      if (value !== null && value !== undefined) {
        (form as Record<string, unknown>)[key] = value;
      REDACTED
    REDACTED
    Object.assign(authSourceDefaults, buildAuthSourceDefaultsState(settings));
    form.backend_mode_enabled = settings.backend_mode_enabled;
    form.default_subscriptions = normalizeDefaultSubscriptionSettings(
      settings.default_subscriptions,
    );
    form.payment_visible_method_alipay_source =
      normalizePaymentVisibleMethodSource(
        "alipay",
        settings.payment_visible_method_alipay_source,
      );
    form.payment_visible_method_wxpay_source =
      normalizePaymentVisibleMethodSource(
        "wxpay",
        settings.payment_visible_method_wxpay_source,
      );
    registrationEmailSuffixWhitelistTags.value =
      normalizeRegistrationEmailSuffixDomains(
        settings.registration_email_suffix_whitelist,
      );
    tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
      Array.isArray(settings.table_page_size_options)
        ? settings.table_page_size_options
        : [10, 20, 50, 100],
    );
    registrationEmailSuffixWhitelistDraft.value = "";
    form.smtp_password = "";
    smtpPasswordManuallyEdited.value = false;
    form.turnstile_secret_key = "";
    form.linuxdo_connect_client_secret = "";
    form.wechat_connect_app_secret = "";
    form.wechat_connect_mode = normalizeWeChatConnectMode(
      settings.wechat_connect_mode,
    );
    const wechatConnectScopes =
      typeof settings.wechat_connect_scopes === "string"
        ? settings.wechat_connect_scopes.trim()
        : "";
    form.wechat_connect_scopes =
      wechatConnectScopes ||
      defaultWeChatConnectScopesForMode(form.wechat_connect_mode);
    form.oidc_connect_client_secret = "";

    // Load web search emulation config separately
    await loadWebSearchConfig();
  REDACTED catch (error: unknown) {
    loadFailed.value = true;
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.failedToLoad")),
    );
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

async function loadSubscriptionGroups() {
  try {
    const groups = await adminAPI.groups.getAll();
    subscriptionGroups.value = groups.filter(
      (group) =>
        group.subscription_type === "subscription" && group.status === "active",
    );
  REDACTED catch (_error: unknown) {
    subscriptionGroups.value = [];
  REDACTED
REDACTED

function findNextAvailableSubscriptionGroup(
  existingGroupIDs: number[],
): AdminGroup | undefined {
  const existing = new Set(existingGroupIDs);
  return subscriptionGroups.value.find((group) => !existing.has(group.id));
REDACTED

function addDefaultSubscription() {
  if (subscriptionGroups.value.length === 0) return;
  const candidate = findNextAvailableSubscriptionGroup(
    form.default_subscriptions.map((item) => item.group_id),
  );
  if (!candidate) return;
  form.default_subscriptions.push({
    group_id: candidate.id,
    validity_days: 30,
  REDACTED);
REDACTED

function removeDefaultSubscription(index: number) {
  form.default_subscriptions.splice(index, 1);
REDACTED

function addAuthSourceDefaultSubscription(source: AuthSourceType) {
  if (subscriptionGroups.value.length === 0) return;
  const candidate = findNextAvailableSubscriptionGroup(
    authSourceDefaults[source].subscriptions.map((item) => item.group_id),
  );
  if (!candidate) return;
  authSourceDefaults[source].subscriptions.push({
    group_id: candidate.id,
    validity_days: 30,
  REDACTED);
REDACTED

function removeAuthSourceDefaultSubscription(
  source: AuthSourceType,
  index: number,
) {
  authSourceDefaults[source].subscriptions.splice(index, 1);
REDACTED

function findDuplicateDefaultSubscription(
  subscriptions: DefaultSubscriptionSetting[],
): DefaultSubscriptionSetting | undefined {
  const seenGroupIDs = new Set<number>();

  return subscriptions.find((item) => {
    if (seenGroupIDs.has(item.group_id)) {
      return true;
    REDACTED
    seenGroupIDs.add(item.group_id);
    return false;
  REDACTED);
REDACTED

async function saveSettings() {
  saving.value = true;
  try {
    const normalizedTableDefaultPageSize = Math.floor(
      Number(form.table_default_page_size),
    );
    if (
      !Number.isInteger(normalizedTableDefaultPageSize) ||
      normalizedTableDefaultPageSize < tablePageSizeMin ||
      normalizedTableDefaultPageSize > tablePageSizeMax
    ) {
      appStore.showError(
        t("admin.settings.site.tableDefaultPageSizeRangeError", {
          min: tablePageSizeMin,
          max: tablePageSizeMax,
        REDACTED),
      );
      return;
    REDACTED

    const normalizedTablePageSizeOptions = parseTablePageSizeOptionsInput(
      tablePageSizeOptionsInput.value,
    );
    if (!normalizedTablePageSizeOptions) {
      appStore.showError(
        t("admin.settings.site.tablePageSizeOptionsFormatError", {
          min: tablePageSizeMin,
          max: tablePageSizeMax,
        REDACTED),
      );
      return;
    REDACTED

    form.table_default_page_size = normalizedTableDefaultPageSize;
    form.table_page_size_options = normalizedTablePageSizeOptions;

    const normalizedDefaultSubscriptions = normalizeDefaultSubscriptionSettings(
      form.default_subscriptions,
    );
    const duplicateDefaultSubscription = findDuplicateDefaultSubscription(
      normalizedDefaultSubscriptions,
    );
    if (duplicateDefaultSubscription) {
      appStore.showError(
        t("admin.settings.defaults.defaultSubscriptionsDuplicate", {
          groupId: duplicateDefaultSubscription.group_id,
        REDACTED),
      );
      return;
    REDACTED

    for (const authSource of authSourceDefaultsMeta.value) {
      authSourceDefaults[authSource.source].subscriptions =
        normalizeDefaultSubscriptionSettings(
          authSourceDefaults[authSource.source].subscriptions,
        );
      const duplicate = findDuplicateDefaultSubscription(
        authSourceDefaults[authSource.source].subscriptions,
      );
      if (duplicate) {
        appStore.showError(
          `${authSource.titleREDACTED: ${t(
            "admin.settings.defaults.defaultSubscriptionsDuplicate",
            {
              groupId: duplicate.group_id,
            REDACTED,
          )REDACTED`,
        );
        return;
      REDACTED
    REDACTED

    if (!validatePaymentVisibleMethodSelections()) {
      return;
    REDACTED

    // Validate URL fields — novalidate disables browser-native checks, so we validate here
    const isValidHttpUrl = (url: string): boolean => {
      if (!url) return true;
      try {
        const u = new URL(url);
        return u.protocol === "http:" || u.protocol === "https:";
      REDACTED catch {
        return false;
      REDACTED
    REDACTED;
    // Optional URL fields: auto-clear invalid values so they don't cause backend 400 errors
    if (!isValidHttpUrl(form.frontend_url)) form.frontend_url = "";
    if (!isValidHttpUrl(form.doc_url)) form.doc_url = "";

    const payload: UpdateSettingsRequest = {
      registration_enabled: form.registration_enabled,
      email_verify_enabled: form.email_verify_enabled,
      registration_email_suffix_whitelist:
        registrationEmailSuffixWhitelistTags.value.map(
          (suffix) => `@${suffixREDACTED`,
        ),
      promo_code_enabled: form.promo_code_enabled,
      invitation_code_enabled: form.invitation_code_enabled,
      password_reset_enabled: form.password_reset_enabled,
      totp_enabled: form.totp_enabled,
      default_balance: form.default_balance,
      default_concurrency: form.default_concurrency,
      default_subscriptions: normalizedDefaultSubscriptions,
      force_email_on_third_party_signup: form.force_email_on_third_party_signup,
      site_name: form.site_name,
      site_logo: form.site_logo,
      site_subtitle: form.site_subtitle,
      api_base_url: form.api_base_url,
      contact_info: form.contact_info,
      doc_url: form.doc_url,
      home_content: form.home_content,
      backend_mode_enabled: form.backend_mode_enabled,
      hide_ccs_import_button: form.hide_ccs_import_button,
      table_default_page_size: form.table_default_page_size,
      table_page_size_options: form.table_page_size_options,
      custom_menu_items: form.custom_menu_items,
      custom_endpoints: form.custom_endpoints,
      frontend_url: form.frontend_url,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: form.smtp_password || undefined,
      smtp_from_email: form.smtp_from_email,
      smtp_from_name: form.smtp_from_name,
      smtp_use_tls: form.smtp_use_tls,
      turnstile_enabled: form.turnstile_enabled,
      turnstile_site_key: form.turnstile_site_key,
      turnstile_secret_key: form.turnstile_secret_key || undefined,
      linuxdo_connect_enabled: form.linuxdo_connect_enabled,
      linuxdo_connect_client_id: form.linuxdo_connect_client_id,
      linuxdo_connect_client_secret:
        form.linuxdo_connect_client_secret || undefined,
      linuxdo_connect_redirect_url: form.linuxdo_connect_redirect_url,
      wechat_connect_enabled: form.wechat_connect_enabled,
      wechat_connect_app_id: form.wechat_connect_app_id,
      wechat_connect_app_secret: form.wechat_connect_app_secret || undefined,
      wechat_connect_mode: normalizeWeChatConnectMode(form.wechat_connect_mode),
      wechat_connect_scopes:
        form.wechat_connect_scopes.trim() ||
        defaultWeChatConnectScopesForMode(form.wechat_connect_mode),
      wechat_connect_redirect_url: form.wechat_connect_redirect_url,
      wechat_connect_frontend_redirect_url:
        form.wechat_connect_frontend_redirect_url,
      oidc_connect_enabled: form.oidc_connect_enabled,
      oidc_connect_provider_name: form.oidc_connect_provider_name,
      oidc_connect_client_id: form.oidc_connect_client_id,
      oidc_connect_client_secret: form.oidc_connect_client_secret || undefined,
      oidc_connect_issuer_url: form.oidc_connect_issuer_url,
      oidc_connect_discovery_url: form.oidc_connect_discovery_url,
      oidc_connect_authorize_url: form.oidc_connect_authorize_url,
      oidc_connect_token_url: form.oidc_connect_token_url,
      oidc_connect_userinfo_url: form.oidc_connect_userinfo_url,
      oidc_connect_jwks_url: form.oidc_connect_jwks_url,
      oidc_connect_scopes: form.oidc_connect_scopes,
      oidc_connect_redirect_url: form.oidc_connect_redirect_url,
      oidc_connect_frontend_redirect_url:
        form.oidc_connect_frontend_redirect_url,
      oidc_connect_token_auth_method: form.oidc_connect_token_auth_method,
      oidc_connect_use_pkce: true,
      oidc_connect_validate_id_token: true,
      oidc_connect_allowed_signing_algs: form.oidc_connect_allowed_signing_algs,
      oidc_connect_clock_skew_seconds: form.oidc_connect_clock_skew_seconds,
      oidc_connect_require_email_verified:
        form.oidc_connect_require_email_verified,
      oidc_connect_userinfo_email_path: form.oidc_connect_userinfo_email_path,
      oidc_connect_userinfo_id_path: form.oidc_connect_userinfo_id_path,
      oidc_connect_userinfo_username_path:
        form.oidc_connect_userinfo_username_path,
      enable_model_fallback: form.enable_model_fallback,
      fallback_model_anthropic: form.fallback_model_anthropic,
      fallback_model_openai: form.fallback_model_openai,
      fallback_model_gemini: form.fallback_model_gemini,
      fallback_model_antigravity: form.fallback_model_antigravity,
      enable_identity_patch: form.enable_identity_patch,
      identity_patch_prompt: form.identity_patch_prompt,
      min_claude_code_version: form.min_claude_code_version,
      max_claude_code_version: form.max_claude_code_version,
      allow_ungrouped_key_scheduling: form.allow_ungrouped_key_scheduling,
      enable_fingerprint_unification: form.enable_fingerprint_unification,
      enable_metadata_passthrough: form.enable_metadata_passthrough,
      enable_cch_signing: form.enable_cch_signing,
      // Payment configuration
      payment_enabled: form.payment_enabled,
      payment_min_amount: Number(form.payment_min_amount) || 0,
      payment_max_amount: Number(form.payment_max_amount) || 0,
      payment_daily_limit: Number(form.payment_daily_limit) || 0,
      payment_max_pending_orders: Number(form.payment_max_pending_orders) || 0,
      payment_order_timeout_minutes:
        Number(form.payment_order_timeout_minutes) || 0,
      payment_balance_disabled: form.payment_balance_disabled,
      payment_balance_recharge_multiplier:
        Number(form.payment_balance_recharge_multiplier) || 1,
      payment_recharge_fee_rate: Number(form.payment_recharge_fee_rate) || 0,
      payment_enabled_types: form.payment_enabled_types,
      payment_load_balance_strategy: form.payment_load_balance_strategy,
      payment_product_name_prefix: form.payment_product_name_prefix,
      payment_product_name_suffix: form.payment_product_name_suffix,
      payment_help_image_url: form.payment_help_image_url,
      payment_help_text: form.payment_help_text,
      payment_cancel_rate_limit_enabled: form.payment_cancel_rate_limit_enabled,
      payment_cancel_rate_limit_max:
        Number(form.payment_cancel_rate_limit_max) || 10,
      payment_cancel_rate_limit_window:
        Number(form.payment_cancel_rate_limit_window) || 1,
      payment_cancel_rate_limit_unit: form.payment_cancel_rate_limit_unit,
      payment_cancel_rate_limit_window_mode:
        form.payment_cancel_rate_limit_window_mode,
      payment_visible_method_alipay_source: normalizePaymentVisibleMethodSource(
        "alipay",
        form.payment_visible_method_alipay_source,
      ),
      payment_visible_method_wxpay_source: normalizePaymentVisibleMethodSource(
        "wxpay",
        form.payment_visible_method_wxpay_source,
      ),
      payment_visible_method_alipay_enabled:
        form.payment_visible_method_alipay_enabled,
      payment_visible_method_wxpay_enabled:
        form.payment_visible_method_wxpay_enabled,
      openai_advanced_scheduler_enabled: form.openai_advanced_scheduler_enabled,
      // Balance & quota notification
      balance_low_notify_enabled: form.balance_low_notify_enabled,
      balance_low_notify_threshold:
        Number(form.balance_low_notify_threshold) || 0,
      balance_low_notify_recharge_url: (form.balance_low_notify_recharge_url =
        form.balance_low_notify_recharge_url || currentOrigin),
      account_quota_notify_enabled: form.account_quota_notify_enabled,
      account_quota_notify_emails: (
        form.account_quota_notify_emails || []
      ).filter((e) => e.email.trim() !== ""),
    REDACTED;

    appendAuthSourceDefaultsToUpdateRequest(payload, authSourceDefaults);

    const updated = await adminAPI.settings.updateSettings(payload);
    for (const [key, value] of Object.entries(updated)) {
      if (value !== null && value !== undefined) {
        (form as Record<string, unknown>)[key] = value;
      REDACTED
    REDACTED
    Object.assign(authSourceDefaults, buildAuthSourceDefaultsState(updated));
    form.payment_visible_method_alipay_source =
      normalizePaymentVisibleMethodSource(
        "alipay",
        updated.payment_visible_method_alipay_source,
      );
    form.payment_visible_method_wxpay_source =
      normalizePaymentVisibleMethodSource(
        "wxpay",
        updated.payment_visible_method_wxpay_source,
      );
    registrationEmailSuffixWhitelistTags.value =
      normalizeRegistrationEmailSuffixDomains(
        updated.registration_email_suffix_whitelist,
      );
    tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
      Array.isArray(updated.table_page_size_options)
        ? updated.table_page_size_options
        : [10, 20, 50, 100],
    );
    registrationEmailSuffixWhitelistDraft.value = "";
    form.smtp_password = "";
    smtpPasswordManuallyEdited.value = false;
    form.turnstile_secret_key = "";
    form.linuxdo_connect_client_secret = "";
    form.wechat_connect_app_secret = "";
    form.wechat_connect_mode = normalizeWeChatConnectMode(
      updated.wechat_connect_mode,
    );
    const updatedWechatConnectScopes =
      typeof updated.wechat_connect_scopes === "string"
        ? updated.wechat_connect_scopes.trim()
        : "";
    form.wechat_connect_scopes =
      updatedWechatConnectScopes ||
      defaultWeChatConnectScopesForMode(form.wechat_connect_mode);
    form.oidc_connect_client_secret = "";
    // Save web search emulation config separately (errors handled internally)
    const wsOk = await saveWebSearchConfig();
    // Refresh cached settings so sidebar/header update immediately
    await appStore.fetchPublicSettings(true);
    await adminSettingsStore.fetch(true);
    if (wsOk) {
      appStore.showSuccess(t("admin.settings.settingsSaved"));
    REDACTED
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.failedToSave")),
    );
  REDACTED finally {
    saving.value = false;
  REDACTED
REDACTED

async function testSmtpConnection() {
  testingSmtp.value = true;
  try {
    const smtpPasswordForTest = smtpPasswordManuallyEdited.value
      ? form.smtp_password
      : "";
    const result = await adminAPI.settings.testSmtpConnection({
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: smtpPasswordForTest,
      smtp_use_tls: form.smtp_use_tls,
    REDACTED);
    // API returns { message: "..." REDACTED on success, errors are thrown as exceptions
    appStore.showSuccess(
      result.message || t("admin.settings.smtpConnectionSuccess"),
    );
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.failedToTestSmtp")),
    );
  REDACTED finally {
    testingSmtp.value = false;
  REDACTED
REDACTED

async function sendTestEmail() {
  if (!testEmailAddress.value) {
    appStore.showError(t("admin.settings.testEmail.enterRecipientHint"));
    return;
  REDACTED

  sendingTestEmail.value = true;
  try {
    const smtpPasswordForSend = smtpPasswordManuallyEdited.value
      ? form.smtp_password
      : "";
    const result = await adminAPI.settings.sendTestEmail({
      email: testEmailAddress.value,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: smtpPasswordForSend,
      smtp_from_email: form.smtp_from_email,
      smtp_from_name: form.smtp_from_name,
      smtp_use_tls: form.smtp_use_tls,
    REDACTED);
    // API returns { message: "..." REDACTED on success, errors are thrown as exceptions
    appStore.showSuccess(result.message || t("admin.settings.testEmailSent"));
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.failedToSendTestEmail")),
    );
  REDACTED finally {
    sendingTestEmail.value = false;
  REDACTED
REDACTED

// Admin API Key 方法
async function loadAdminApiKey() {
  adminApiKeyLoading.value = true;
  try {
    const status = await adminAPI.settings.getAdminApiKey();
    adminApiKeyExists.value = status.exists;
    adminApiKeyMasked.value = status.masked_key;
  REDACTED catch (_error: unknown) {
    // Silent fail - admin API key status is non-critical
  REDACTED finally {
    adminApiKeyLoading.value = false;
  REDACTED
REDACTED

async function createAdminApiKey() {
  adminApiKeyOperating.value = true;
  try {
    const result = await adminAPI.settings.regenerateAdminApiKey();
    newAdminApiKey.value = result.key;
    adminApiKeyExists.value = true;
    adminApiKeyMasked.value =
      result.key.substring(0, 10) + "..." + result.key.slice(-4);
    appStore.showSuccess(t("admin.settings.adminApiKey.keyGenerated"));
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  REDACTED finally {
    adminApiKeyOperating.value = false;
  REDACTED
REDACTED

async function regenerateAdminApiKey() {
  if (!confirm(t("admin.settings.adminApiKey.regenerateConfirm"))) return;
  await createAdminApiKey();
REDACTED

async function deleteAdminApiKey() {
  if (!confirm(t("admin.settings.adminApiKey.deleteConfirm"))) return;
  adminApiKeyOperating.value = true;
  try {
    await adminAPI.settings.deleteAdminApiKey();
    adminApiKeyExists.value = false;
    adminApiKeyMasked.value = "";
    newAdminApiKey.value = "";
    appStore.showSuccess(t("admin.settings.adminApiKey.keyDeleted"));
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("common.error")));
  REDACTED finally {
    adminApiKeyOperating.value = false;
  REDACTED
REDACTED

function copyNewKey() {
  navigator.clipboard
    .writeText(newAdminApiKey.value)
    .then(() => {
      appStore.showSuccess(t("admin.settings.adminApiKey.keyCopied"));
    REDACTED)
    .catch(() => {
      appStore.showError(t("common.copyFailed"));
    REDACTED);
REDACTED

// Overload Cooldown 方法
async function loadOverloadCooldownSettings() {
  overloadCooldownLoading.value = true;
  try {
    const settings = await adminAPI.settings.getOverloadCooldownSettings();
    Object.assign(overloadCooldownForm, settings);
  REDACTED catch (_error: unknown) {
    // Silent fail - settings will use defaults
  REDACTED finally {
    overloadCooldownLoading.value = false;
  REDACTED
REDACTED

async function saveOverloadCooldownSettings() {
  overloadCooldownSaving.value = true;
  try {
    const updated = await adminAPI.settings.updateOverloadCooldownSettings({
      enabled: overloadCooldownForm.enabled,
      cooldown_minutes: overloadCooldownForm.cooldown_minutes,
    REDACTED);
    Object.assign(overloadCooldownForm, updated);
    appStore.showSuccess(t("admin.settings.overloadCooldown.saved"));
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.overloadCooldown.saveFailed"),
      ),
    );
  REDACTED finally {
    overloadCooldownSaving.value = false;
  REDACTED
REDACTED

// Stream Timeout 方法
async function loadStreamTimeoutSettings() {
  streamTimeoutLoading.value = true;
  try {
    const settings = await adminAPI.settings.getStreamTimeoutSettings();
    Object.assign(streamTimeoutForm, settings);
  REDACTED catch (_error: unknown) {
    // Silent fail - settings will use defaults
  REDACTED finally {
    streamTimeoutLoading.value = false;
  REDACTED
REDACTED

async function saveStreamTimeoutSettings() {
  streamTimeoutSaving.value = true;
  try {
    const updated = await adminAPI.settings.updateStreamTimeoutSettings({
      enabled: streamTimeoutForm.enabled,
      action: streamTimeoutForm.action,
      temp_unsched_minutes: streamTimeoutForm.temp_unsched_minutes,
      threshold_count: streamTimeoutForm.threshold_count,
      threshold_window_minutes: streamTimeoutForm.threshold_window_minutes,
    REDACTED);
    Object.assign(streamTimeoutForm, updated);
    appStore.showSuccess(t("admin.settings.streamTimeout.saved"));
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.streamTimeout.saveFailed"),
      ),
    );
  REDACTED finally {
    streamTimeoutSaving.value = false;
  REDACTED
REDACTED

// Rectifier 方法
async function loadRectifierSettings() {
  rectifierLoading.value = true;
  try {
    const settings = await adminAPI.settings.getRectifierSettings();
    Object.assign(rectifierForm, settings);
    // 确保 patterns 是数组（旧数据可能为 null）
    if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
      rectifierForm.apikey_signature_patterns = [];
    REDACTED
  REDACTED catch (_error: unknown) {
    // Silent fail - settings will use defaults
  REDACTED finally {
    rectifierLoading.value = false;
  REDACTED
REDACTED

async function saveRectifierSettings() {
  rectifierSaving.value = true;
  try {
    const updated = await adminAPI.settings.updateRectifierSettings({
      enabled: rectifierForm.enabled,
      thinking_signature_enabled: rectifierForm.thinking_signature_enabled,
      thinking_budget_enabled: rectifierForm.thinking_budget_enabled,
      apikey_signature_enabled: rectifierForm.apikey_signature_enabled,
      apikey_signature_patterns: rectifierForm.apikey_signature_patterns.filter(
        (p) => p.trim() !== "",
      ),
    REDACTED);
    Object.assign(rectifierForm, updated);
    if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
      rectifierForm.apikey_signature_patterns = [];
    REDACTED
    appStore.showSuccess(t("admin.settings.rectifier.saved"));
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.rectifier.saveFailed")),
    );
  REDACTED finally {
    rectifierSaving.value = false;
  REDACTED
REDACTED

const betaPolicyActionOptions = computed(() => [
  { value: "pass", label: t("admin.settings.betaPolicy.actionPass") REDACTED,
  { value: "filter", label: t("admin.settings.betaPolicy.actionFilter") REDACTED,
  { value: "block", label: t("admin.settings.betaPolicy.actionBlock") REDACTED,
]);

const betaPolicyScopeOptions = computed(() => [
  { value: "all", label: t("admin.settings.betaPolicy.scopeAll") REDACTED,
  { value: "oauth", label: t("admin.settings.betaPolicy.scopeOAuth") REDACTED,
  { value: "apikey", label: t("admin.settings.betaPolicy.scopeAPIKey") REDACTED,
  { value: "bedrock", label: t("admin.settings.betaPolicy.scopeBedrock") REDACTED,
]);

// Beta Policy 方法
const betaDisplayNames: Record<string, string> = {
  "fast-mode-2026-02-01": "Fast Mode",
  "context-1m-2025-08-07": "Context 1M",
REDACTED;

// 快捷预设：按 beta_token 定义预设方案
const betaPresets: Record<
  string,
  Array<{
    label: string;
    description: string;
    action: "pass" | "filter" | "block";
    model_whitelist: string[];
    fallback_action: "pass" | "filter" | "block";
  REDACTED>
> = {
  "context-1m-2025-08-07": [
    {
      label: t("admin.settings.betaPolicy.presetOpusOnly"),
      description: t("admin.settings.betaPolicy.presetOpusOnlyDesc"),
      action: "pass",
      model_whitelist: ["claude-opus-4-6"],
      fallback_action: "filter",
    REDACTED,
  ],
REDACTED;

// 常用模型模式（具体 ID + 通配符示例）
const commonModelPatterns = [
  "claude-opus-4-6",
  "claude-sonnet-4-6",
  "claude-opus-*",
  "claude-sonnet-*",
];

function getBetaDisplayName(token: string): string {
  return betaDisplayNames[token] || token;
REDACTED

function applyBetaPreset(
  rule: (typeof betaPolicyForm.rules)[number],
  preset: {
    action: "pass" | "filter" | "block";
    model_whitelist: string[];
    fallback_action: "pass" | "filter" | "block";
  REDACTED,
) {
  rule.action = preset.action;
  rule.model_whitelist = [...preset.model_whitelist];
  rule.fallback_action = preset.fallback_action;
REDACTED

function addQuickPattern(
  rule: (typeof betaPolicyForm.rules)[number],
  pattern: string,
) {
  if (!rule.model_whitelist) rule.model_whitelist = [];
  if (!rule.model_whitelist.includes(pattern)) {
    rule.model_whitelist.push(pattern);
  REDACTED
REDACTED

async function loadBetaPolicySettings() {
  betaPolicyLoading.value = true;
  try {
    const settings = await adminAPI.settings.getBetaPolicySettings();
    betaPolicyForm.rules = settings.rules;
  REDACTED catch (_error: unknown) {
    // Silent fail - settings will use defaults
  REDACTED finally {
    betaPolicyLoading.value = false;
  REDACTED
REDACTED

async function saveBetaPolicySettings() {
  betaPolicySaving.value = true;
  try {
    // Clean up empty patterns before saving
    const cleanedRules = betaPolicyForm.rules.map((rule) => {
      const whitelist = rule.model_whitelist?.filter((p) => p.trim() !== "");
      const hasWhitelist = whitelist && whitelist.length > 0;
      return {
        beta_token: rule.beta_token,
        action: rule.action,
        scope: rule.scope,
        error_message: rule.error_message,
        model_whitelist: hasWhitelist ? whitelist : undefined,
        fallback_action: hasWhitelist
          ? rule.fallback_action || "pass"
          : undefined,
        fallback_error_message:
          hasWhitelist && rule.fallback_action === "block"
            ? rule.fallback_error_message
            : undefined,
      REDACTED;
    REDACTED);
    const updated = await adminAPI.settings.updateBetaPolicySettings({
      rules: cleanedRules,
    REDACTED);
    betaPolicyForm.rules = updated.rules;
    appStore.showSuccess(t("admin.settings.betaPolicy.saved"));
  REDACTED catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(error, t("admin.settings.betaPolicy.saveFailed")),
    );
  REDACTED finally {
    betaPolicySaving.value = false;
  REDACTED
REDACTED

// ==================== Provider Management ====================

const allPaymentTypes = computed(() => [
  { value: "easypay", label: t("payment.methods.easypay") REDACTED,
  { value: "alipay", label: t("payment.methods.alipay") REDACTED,
  { value: "wxpay", label: t("payment.methods.wxpay") REDACTED,
  { value: "stripe", label: t("payment.methods.stripe") REDACTED,
]);

function isPaymentTypeEnabled(type: string): boolean {
  return form.payment_enabled_types.includes(type);
REDACTED

const hasAnyPaymentTypeEnabled = computed(
  () => form.payment_enabled_types.length > 0,
);

function togglePaymentType(type: string) {
  if (form.payment_enabled_types.includes(type)) {
    form.payment_enabled_types = form.payment_enabled_types.filter(
      (t) => t !== type,
    );
    // Disable all provider instances matching this type
    disableProvidersByType(type);
  REDACTED else {
    form.payment_enabled_types = [...form.payment_enabled_types, type];
  REDACTED
REDACTED

async function disableProvidersByType(type: string) {
  const matching = providers.value.filter(
    (p) => p.provider_key === type && p.enabled,
  );
  for (const p of matching) {
    try {
      await adminAPI.payment.updateProvider(p.id, { enabled: false REDACTED);
      p.enabled = false;
    REDACTED catch (err: unknown) {
      slog("disable provider failed", p.id, err);
    REDACTED
  REDACTED
REDACTED

function slog(...args: unknown[]) {
  console.warn("[payment]", ...args);
REDACTED

const providersLoading = ref(false);
const providerSaving = ref(false);
const providers = ref<ProviderInstance[]>([]);
const showProviderDialog = ref(false);
const showDeleteProviderDialog = ref(false);
const editingProvider = ref<ProviderInstance | null>(null);
const deletingProviderId = ref<number | null>(null);
const providerDialogRef = ref<InstanceType<
  typeof PaymentProviderDialog
> | null>(null);

const providerKeyOptions = computed(() => [
  { value: "easypay", label: t("admin.settings.payment.providerEasypay") REDACTED,
  { value: "alipay", label: t("admin.settings.payment.providerAlipay") REDACTED,
  { value: "wxpay", label: t("admin.settings.payment.providerWxpay") REDACTED,
  { value: "stripe", label: t("admin.settings.payment.providerStripe") REDACTED,
]);

const enabledProviderKeyOptions = computed(() => {
  const enabled = form.payment_enabled_types;
  return providerKeyOptions.value.filter((opt) => enabled.includes(opt.value));
REDACTED);

const loadBalanceOptions = computed(() => [
  {
    value: "round-robin",
    label: t("admin.settings.payment.strategyRoundRobin"),
  REDACTED,
  {
    value: "least-amount",
    label: t("admin.settings.payment.strategyLeastAmount"),
  REDACTED,
]);

const cancelRateLimitUnitOptions = computed(() => [
  {
    value: "minute",
    label: t("admin.settings.payment.cancelRateLimitUnitMinute"),
  REDACTED,
  { value: "hour", label: t("admin.settings.payment.cancelRateLimitUnitHour") REDACTED,
  { value: "day", label: t("admin.settings.payment.cancelRateLimitUnitDay") REDACTED,
]);

const cancelRateLimitModeOptions = computed(() => [
  {
    value: "rolling",
    label: t("admin.settings.payment.cancelRateLimitWindowModeRolling"),
  REDACTED,
  {
    value: "fixed",
    label: t("admin.settings.payment.cancelRateLimitWindowModeFixed"),
  REDACTED,
]);

const paymentErrorMap = computed(() => ({
  PENDING_ORDERS: t("payment.errors.PENDING_ORDERS"),
REDACTED));

async function loadProviders() {
  providersLoading.value = true;
  try {
    const res = await adminAPI.payment.getProviders();
    providers.value = res.data || [];
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    providersLoading.value = false;
  REDACTED
REDACTED

function openCreateProvider() {
  editingProvider.value = null;
  providerDialogRef.value?.reset(
    enabledProviderKeyOptions.value[0]?.value || "easypay",
  );
  showProviderDialog.value = true;
REDACTED

function openEditProvider(provider: ProviderInstance) {
  editingProvider.value = provider;
  providerDialogRef.value?.loadProvider(provider);
  showProviderDialog.value = true;
REDACTED

async function handleSaveProvider(payload: Partial<ProviderInstance>) {
  providerSaving.value = true;
  try {
    if (editingProvider.value) {
      await adminAPI.payment.updateProvider(editingProvider.value.id, payload);
    REDACTED else {
      await adminAPI.payment.createProvider(payload);
    REDACTED
    showProviderDialog.value = false;
    // Reload full list (API returns decrypted/formatted data with correct sort order)
    await loadProviders();
    // Auto-save settings so provider changes take effect immediately
    await saveSettings();
  REDACTED catch (err: unknown) {
    appStore.showError(
      extractApiErrorMessage(err, t("common.error"), paymentErrorMap.value),
    );
  REDACTED finally {
    providerSaving.value = false;
  REDACTED
REDACTED

async function handleToggleField(
  provider: ProviderInstance,
  field: "enabled" | "refund_enabled" | "allow_user_refund",
) {
  let newValue: boolean;
  if (field === "enabled") newValue = !provider.enabled;
  else if (field === "refund_enabled") newValue = !provider.refund_enabled;
  else newValue = !provider.allow_user_refund;

  const payload: Record<string, boolean> = { [field]: newValue REDACTED;
  // Cascade: turning off refund_enabled also turns off allow_user_refund
  if (field === "refund_enabled" && !newValue) {
    payload.allow_user_refund = false;
  REDACTED
  try {
    await adminAPI.payment.updateProvider(provider.id, payload);
    if (field === "enabled") provider.enabled = newValue;
    else if (field === "refund_enabled") {
      provider.refund_enabled = newValue;
      if (!newValue) provider.allow_user_refund = false;
    REDACTED else {
      provider.allow_user_refund = newValue;
    REDACTED
  REDACTED catch (err: unknown) {
    appStore.showError(
      extractApiErrorMessage(err, t("common.error"), paymentErrorMap.value),
    );
  REDACTED
REDACTED

async function handleToggleType(provider: ProviderInstance, type: string) {
  const updated = provider.supported_types.includes(type)
    ? provider.supported_types.filter((t) => t !== type)
    : [...provider.supported_types, type];
  try {
    await adminAPI.payment.updateProvider(provider.id, {
      supported_types: updated,
    REDACTED as any);
    provider.supported_types = updated;
  REDACTED catch (err: unknown) {
    appStore.showError(
      extractApiErrorMessage(err, t("common.error"), paymentErrorMap.value),
    );
  REDACTED
REDACTED

function confirmDeleteProvider(provider: ProviderInstance) {
  deletingProviderId.value = provider.id;
  showDeleteProviderDialog.value = true;
REDACTED

async function handleReorderProviders(
  updates: { id: number; sort_order: number REDACTED[],
) {
  try {
    await Promise.all(
      updates.map((u) =>
        adminAPI.payment.updateProvider(u.id, {
          sort_order: u.sort_order,
        REDACTED as Partial<ProviderInstance>),
      ),
    );
    // Update local state to match new order
    for (const u of updates) {
      const p = providers.value.find((p) => p.id === u.id);
      if (p) p.sort_order = u.sort_order;
    REDACTED
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
    loadProviders();
  REDACTED
REDACTED

async function handleDeleteProvider() {
  if (!deletingProviderId.value) return;
  try {
    await adminAPI.payment.deleteProvider(deletingProviderId.value);
    appStore.showSuccess(t("common.deleted"));
    showDeleteProviderDialog.value = false;
    loadProviders();
  REDACTED catch (err: unknown) {
    appStore.showError(
      extractApiErrorMessage(err, t("common.error"), paymentErrorMap.value),
    );
  REDACTED
REDACTED

onMounted(() => {
  loadSettings();
  loadSubscriptionGroups();
  loadAdminApiKey();
  loadOverloadCooldownSettings();
  loadStreamTimeoutSettings();
  loadRectifierSettings();
  loadBetaPolicySettings();
  loadProviders();
REDACTED);
</script>

<style scoped>
.default-sub-group-select :deep(.select-trigger) {
  @apply h-[42px];
REDACTED

.default-sub-delete-btn {
  @apply h-[42px];
REDACTED

/* ============ Settings Tab Navigation ============ */

/* Scroll container: thin scrollbar on PC, auto-hide on mobile */
.settings-tabs-scroll {
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
REDACTED
.settings-tabs-scroll:hover {
  scrollbar-color: rgb(0 0 0 / 0.15) transparent;
REDACTED
:root.dark .settings-tabs-scroll:hover {
  scrollbar-color: rgb(255 255 255 / 0.2) transparent;
REDACTED
.settings-tabs-scroll::-webkit-scrollbar {
  height: 3px;
REDACTED
.settings-tabs-scroll::-webkit-scrollbar-track {
  background: transparent;
REDACTED
.settings-tabs-scroll::-webkit-scrollbar-thumb {
  background: transparent;
  border-radius: 3px;
REDACTED
.settings-tabs-scroll:hover::-webkit-scrollbar-thumb {
  background: rgb(0 0 0 / 0.15);
REDACTED
:root.dark .settings-tabs-scroll:hover::-webkit-scrollbar-thumb {
  background: rgb(255 255 255 / 0.2);
REDACTED

.settings-tabs {
  @apply inline-flex min-w-full gap-0.5 rounded-2xl
         border border-gray-100 bg-white/80 p-1 backdrop-blur-sm
         dark:border-dark-700/50 dark:bg-dark-800/80;
  box-shadow:
    0 1px 3px rgb(0 0 0 / 0.04),
    0 1px 2px rgb(0 0 0 / 0.02);
REDACTED

@media (min-width: 640px) {
  .settings-tabs {
    @apply flex;
  REDACTED
REDACTED

.settings-tab {
  @apply relative flex flex-1 items-center justify-center gap-1.5
         whitespace-nowrap rounded-xl px-2.5 py-2
         text-sm font-medium
         text-gray-500 dark:text-dark-400
         transition-all duration-200 ease-out;
REDACTED

.settings-tab:hover:not(.settings-tab-active) {
  @apply text-gray-700 dark:text-gray-300;
  background: rgb(0 0 0 / 0.03);
REDACTED

:root.dark .settings-tab:hover:not(.settings-tab-active) {
  background: rgb(255 255 255 / 0.04);
REDACTED

.settings-tab-active {
  @apply text-primary-600 dark:text-primary-400;
  background: linear-gradient(
    135deg,
    rgba(20, 184, 166, 0.08),
    rgba(20, 184, 166, 0.03)
  );
  box-shadow: 0 1px 2px rgba(20, 184, 166, 0.1);
REDACTED

:root.dark .settings-tab-active {
  background: linear-gradient(
    135deg,
    rgba(45, 212, 191, 0.12),
    rgba(45, 212, 191, 0.05)
  );
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.25);
REDACTED

.settings-tab-icon {
  @apply flex h-6 w-6 items-center justify-center rounded-lg
         transition-all duration-200;
REDACTED

.settings-tab-active .settings-tab-icon {
  @apply bg-primary-500/15 text-primary-600
         dark:bg-primary-400/15 dark:text-primary-400;
REDACTED
</style>
