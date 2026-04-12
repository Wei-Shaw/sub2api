<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
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
              :class="['settings-tab', activeTab === tab.key && 'settings-tab-active']"
              @click="activeTab = tab.key"
            >
              <span class="settings-tab-icon">
                <Icon :name="tab.icon" size="sm" />
              </span>
              <span>{{ t(`admin.settings.tabs.${tab.key}`) }}</span>
            </button>
          </nav>
        </div>

        <!-- Tab: Security — Admin API Key -->
        <div v-show="activeTab === 'security'" class="space-y-6">
        <!-- Admin API Key Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.adminApiKey.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.adminApiKey.description') }}
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
                  {{ t('admin.settings.adminApiKey.securityWarning') }}
                </p>
              </div>
            </div>

            <!-- Loading State -->
            <div v-if="adminApiKeyLoading" class="flex items-center gap-2 text-gray-500">
              <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
              {{ t('common.loading') }}
            </div>

            <!-- No Key Configured -->
            <div v-else-if="!adminApiKeyExists" class="flex items-center justify-between">
              <span class="text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.adminApiKey.notConfigured') }}
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
                    ? t('admin.settings.adminApiKey.creating')
                    : t('admin.settings.adminApiKey.create')
                }}
              </button>
            </div>

            <!-- Key Exists -->
            <div v-else class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.adminApiKey.currentKey') }}
                  </label>
                  <code
                    class="rounded bg-gray-100 px-2 py-1 font-mono text-sm text-gray-900 dark:bg-dark-700 dark:text-gray-100"
                  >
                    {{ adminApiKeyMasked }}
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
                        ? t('admin.settings.adminApiKey.regenerating')
                        : t('admin.settings.adminApiKey.regenerate')
                    }}
                  </button>
                  <button
                    type="button"
                    @click="deleteAdminApiKey"
                    :disabled="adminApiKeyOperating"
                    class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                  >
                    {{ t('admin.settings.adminApiKey.delete') }}
                  </button>
                </div>
              </div>

              <!-- Newly Generated Key Display -->
              <div
                v-if="newAdminApiKey"
                class="space-y-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20"
              >
                <p class="text-sm font-medium text-green-700 dark:text-green-300">
                  {{ t('admin.settings.adminApiKey.keyWarning') }}
                </p>
                <div class="flex items-center gap-2">
                  <code
                    class="flex-1 select-all break-all rounded border border-green-300 bg-white px-3 py-2 font-mono text-sm dark:border-green-700 dark:bg-dark-800"
                  >
                    {{ newAdminApiKey }}
                  </code>
                  <button
                    type="button"
                    @click="copyNewKey"
                    class="btn btn-primary btn-sm flex-shrink-0"
                  >
                    {{ t('admin.settings.adminApiKey.copyKey') }}
                  </button>
                </div>
                <p class="text-xs text-green-600 dark:text-green-400">
                  {{ t('admin.settings.adminApiKey.usage') }}
                </p>
              </div>
            </div>
          </div>
        </div>
        </div><!-- /Tab: Security — Admin API Key -->

        <!-- Tab: Gateway -->
        <div v-show="activeTab === 'gateway'" class="space-y-6">

        <!-- Overload Cooldown (529) Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.overloadCooldown.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.overloadCooldown.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div v-if="overloadCooldownLoading" class="flex items-center gap-2 text-gray-500">
              <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
              {{ t('common.loading') }}
            </div>

            <template v-else>
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t('admin.settings.overloadCooldown.enabled')
                  }}</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.overloadCooldown.enabledHint') }}
                  </p>
                </div>
                <Toggle v-model="overloadCooldownForm.enabled" />
              </div>

              <div
                v-if="overloadCooldownForm.enabled"
                class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
              >
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.overloadCooldown.cooldownMinutes') }}
                  </label>
                  <input
                    v-model.number="overloadCooldownForm.cooldown_minutes"
                    type="number"
                    min="1"
                    max="120"
                    class="input w-32"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.overloadCooldown.cooldownMinutesHint') }}
                  </p>
                </div>
              </div>

              <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
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
                  {{ overloadCooldownSaving ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </template>
          </div>
        </div>

        <!-- Stream Timeout Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.streamTimeout.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.streamTimeout.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Loading State -->
            <div v-if="streamTimeoutLoading" class="flex items-center gap-2 text-gray-500">
              <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
              {{ t('common.loading') }}
            </div>

            <template v-else>
              <!-- Enable Stream Timeout -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t('admin.settings.streamTimeout.enabled')
                  }}</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.streamTimeout.enabledHint') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.streamTimeout.action') }}
                  </label>
                  <select v-model="streamTimeoutForm.action" class="input w-64">
                    <option value="temp_unsched">{{ t('admin.settings.streamTimeout.actionTempUnsched') }}</option>
                    <option value="error">{{ t('admin.settings.streamTimeout.actionError') }}</option>
                    <option value="none">{{ t('admin.settings.streamTimeout.actionNone') }}</option>
                  </select>
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.streamTimeout.actionHint') }}
                  </p>
                </div>

                <!-- Temp Unsched Minutes (only show when action is temp_unsched) -->
                <div v-if="streamTimeoutForm.action === 'temp_unsched'">
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.streamTimeout.tempUnschedMinutes') }}
                  </label>
                  <input
                    v-model.number="streamTimeoutForm.temp_unsched_minutes"
                    type="number"
                    min="1"
                    max="60"
                    class="input w-32"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.streamTimeout.tempUnschedMinutesHint') }}
                  </p>
                </div>

                <!-- Threshold Count -->
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.streamTimeout.thresholdCount') }}
                  </label>
                  <input
                    v-model.number="streamTimeoutForm.threshold_count"
                    type="number"
                    min="1"
                    max="10"
                    class="input w-32"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.streamTimeout.thresholdCountHint') }}
                  </p>
                </div>

                <!-- Threshold Window Minutes -->
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.streamTimeout.thresholdWindowMinutes') }}
                  </label>
                  <input
                    v-model.number="streamTimeoutForm.threshold_window_minutes"
                    type="number"
                    min="1"
                    max="60"
                    class="input w-32"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.streamTimeout.thresholdWindowMinutesHint') }}
                  </p>
                </div>
              </div>

              <!-- Save Button -->
              <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
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
                  {{ streamTimeoutSaving ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </template>
          </div>
        </div>

        <!-- Request Rectifier Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.rectifier.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.rectifier.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Loading State -->
            <div v-if="rectifierLoading" class="flex items-center gap-2 text-gray-500">
              <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
              {{ t('common.loading') }}
            </div>

            <template v-else>
              <!-- Master Toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t('admin.settings.rectifier.enabled')
                  }}</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.rectifier.enabledHint') }}
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
                    <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                      t('admin.settings.rectifier.thinkingSignature')
                    }}</label>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.settings.rectifier.thinkingSignatureHint') }}
                    </p>
                  </div>
                  <Toggle v-model="rectifierForm.thinking_signature_enabled" />
                </div>

                <!-- Thinking Budget Rectifier -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                      t('admin.settings.rectifier.thinkingBudget')
                    }}</label>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.settings.rectifier.thinkingBudgetHint') }}
                    </p>
                  </div>
                  <Toggle v-model="rectifierForm.thinking_budget_enabled" />
                </div>

                <!-- API Key Signature Rectifier -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                      t('admin.settings.rectifier.apikeySignature')
                    }}</label>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.settings.rectifier.apikeySignatureHint') }}
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
                    <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                      t('admin.settings.rectifier.apikeyPatterns')
                    }}</label>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.settings.rectifier.apikeyPatternsHint') }}
                    </p>
                  </div>
                  <div
                    v-for="(_, index) in rectifierForm.apikey_signature_patterns"
                    :key="index"
                    class="flex items-center gap-2"
                  >
                    <input
                      v-model="rectifierForm.apikey_signature_patterns[index]"
                      type="text"
                      class="input input-sm flex-1"
                      :placeholder="t('admin.settings.rectifier.apikeyPatternPlaceholder')"
                    />
                    <button
                      type="button"
                      @click="rectifierForm.apikey_signature_patterns.splice(index, 1)"
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
                    + {{ t('admin.settings.rectifier.addPattern') }}
                  </button>
                </div>
              </div>

              <!-- Save Button -->
              <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
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
                  {{ rectifierSaving ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </template>
          </div>
        </div>
        <!-- Beta Policy Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.betaPolicy.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.betaPolicy.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Loading State -->
            <div v-if="betaPolicyLoading" class="flex items-center gap-2 text-gray-500">
              <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
              {{ t('common.loading') }}
            </div>

            <template v-else>
              <!-- Rule Cards -->
              <div
                v-for="rule in betaPolicyForm.rules"
                :key="rule.beta_token"
                class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
              >
                <div class="mb-3 flex items-center gap-2">
                  <span class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ getBetaDisplayName(rule.beta_token) }}
                  </span>
                  <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                    {{ rule.beta_token }}
                  </span>
                </div>

                <div class="grid grid-cols-2 gap-4">
                  <!-- Action -->
                  <div>
                    <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                      {{ t('admin.settings.betaPolicy.action') }}
                    </label>
                    <Select
                      :modelValue="rule.action"
                      @update:modelValue="rule.action = $event as any"
                      :options="betaPolicyActionOptions"
                    />
                  </div>

                  <!-- Scope -->
                  <div>
                    <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                      {{ t('admin.settings.betaPolicy.scope') }}
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
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.betaPolicy.errorMessage') }}
                  </label>
                  <input
                    v-model="rule.error_message"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.betaPolicy.errorMessagePlaceholder')"
                  />
                  <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.settings.betaPolicy.errorMessageHint') }}
                  </p>
                </div>

                <!-- Quick Presets (only for tokens with presets) -->
                <div v-if="betaPresets[rule.beta_token]?.length" class="mt-3">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.betaPolicy.quickPresets') }}
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
                      {{ preset.label }}
                    </button>
                  </div>
                </div>

                <!-- Model Whitelist -->
                <div class="mt-3">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.betaPolicy.modelWhitelist') }}
                  </label>
                  <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.settings.betaPolicy.modelWhitelistHint') }}
                  </p>
                  <!-- Existing patterns -->
                  <div
                    v-for="(_, index) in (rule.model_whitelist || [])"
                    :key="index"
                    class="mb-1.5 flex items-center gap-2"
                  >
                    <input
                      v-model="rule.model_whitelist![index]"
                      type="text"
                      class="input input-sm flex-1"
                      :placeholder="t('admin.settings.betaPolicy.modelPatternPlaceholder')"
                    />
                    <button
                      type="button"
                      @click="rule.model_whitelist!.splice(index, 1)"
                      class="shrink-0 rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                    >
                      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  </div>
                  <!-- Add pattern button -->
                  <button
                    type="button"
                    @click="if (!rule.model_whitelist) rule.model_whitelist = []; rule.model_whitelist.push('')"
                    class="mb-2 inline-flex items-center gap-1 text-xs text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  >
                    <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
                    </svg>
                    {{ t('admin.settings.betaPolicy.addModelPattern') }}
                  </button>
                  <!-- Common pattern chips -->
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.settings.betaPolicy.commonPatterns') }}:</span>
                    <button
                      v-for="pattern in commonModelPatterns"
                      :key="pattern"
                      type="button"
                      class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-700 dark:hover:bg-primary-900/30 dark:hover:text-primary-300"
                      @click="addQuickPattern(rule, pattern)"
                    >
                      {{ pattern }}
                    </button>
                  </div>
                </div>

                <!-- Fallback Action (only when model_whitelist is non-empty) -->
                <div v-if="rule.model_whitelist && rule.model_whitelist.length > 0" class="mt-3">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.betaPolicy.fallbackAction') }}
                  </label>
                  <Select
                    :modelValue="rule.fallback_action || 'pass'"
                    @update:modelValue="rule.fallback_action = $event as any"
                    :options="betaPolicyActionOptions"
                  />
                  <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.settings.betaPolicy.fallbackActionHint') }}
                  </p>
                  <!-- Fallback Error Message (only when fallback_action=block) -->
                  <div v-if="rule.fallback_action === 'block'" class="mt-2">
                    <input
                      v-model="rule.fallback_error_message"
                      type="text"
                      class="input"
                      :placeholder="t('admin.settings.betaPolicy.fallbackErrorMessagePlaceholder')"
                    />
                    <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                      {{ t('admin.settings.betaPolicy.errorMessageHint') }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Save Button -->
              <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
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
                  {{ betaPolicySaving ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </template>
          </div>
        </div>

        </div><!-- /Tab: Gateway -->

        <!-- Tab: Security — Registration, Turnstile, LinuxDo -->
        <div v-show="activeTab === 'security'" class="space-y-6">
        <!-- Registration Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.registration.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.registration.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Enable Registration -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.registration.enableRegistration')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.enableRegistrationHint') }}
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
                  t('admin.settings.registration.emailVerification')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.emailVerificationHint') }}
                </p>
              </div>
              <Toggle v-model="form.email_verify_enabled" />
            </div>

            <!-- Email Suffix Whitelist -->
            <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
              <label class="font-medium text-gray-900 dark:text-white">{{
                t('admin.settings.registration.emailSuffixWhitelist')
              }}</label>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.registration.emailSuffixWhitelistHint') }}
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
                    <span>{{ suffix }}</span>
                    <button
                      type="button"
                      class="rounded-full text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-dark-500 dark:hover:text-white"
                      @click="removeRegistrationEmailSuffixWhitelistTag(suffix)"
                    >
                      <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
                    </button>
                  </span>

                  <div
                    class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-primary-300 dark:focus-within:border-primary-700"
                  >
                    <span class="font-mono text-sm text-gray-400 dark:text-gray-500">@</span>
                    <input
                      v-model="registrationEmailSuffixWhitelistDraft"
                      type="text"
                      class="w-full bg-transparent text-sm font-mono text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
                      :placeholder="t('admin.settings.registration.emailSuffixWhitelistPlaceholder')"
                      @input="handleRegistrationEmailSuffixWhitelistDraftInput"
                      @keydown="handleRegistrationEmailSuffixWhitelistDraftKeydown"
                      @blur="commitRegistrationEmailSuffixWhitelistDraft"
                      @paste="handleRegistrationEmailSuffixWhitelistPaste"
                    />
                  </div>
                </div>
              </div>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.registration.emailSuffixWhitelistInputHint') }}
              </p>
            </div>

            <!-- Promo Code -->
            <div
              class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.registration.promoCode')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.promoCodeHint') }}
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
                  t('admin.settings.registration.invitationCode')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.invitationCodeHint') }}
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
                  t('admin.settings.registration.passwordReset')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.passwordResetHint') }}
                </p>
              </div>
              <Toggle v-model="form.password_reset_enabled" />
            </div>
            <!-- Frontend URL - Only show when password reset is enabled -->
            <div
              v-if="form.email_verify_enabled && form.password_reset_enabled"
              class="border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.registration.frontendUrl') }}
              </label>
              <input
                v-model="form.frontend_url"
                type="url"
                class="input"
                :placeholder="t('admin.settings.registration.frontendUrlPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.registration.frontendUrlHint') }}
              </p>
            </div>

            <!-- TOTP 2FA -->
            <div
              class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.registration.totp')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.registration.totpHint') }}
                </p>
                <!-- Warning when encryption key not configured -->
                <p
                  v-if="!form.totp_encryption_key_configured"
                  class="mt-2 text-sm text-amber-600 dark:text-amber-400"
                >
                  {{ t('admin.settings.registration.totpKeyNotConfigured') }}
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
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.turnstile.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.turnstile.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Enable Turnstile -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.turnstile.enableTurnstile')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.turnstile.enableTurnstileHint') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.turnstile.siteKey') }}
                  </label>
                  <input
                    v-model="form.turnstile_site_key"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="0x4AAAAAAA..."
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.turnstile.siteKeyHint') }}
                    <a
                      href="https://dash.cloudflare.com/"
                      target="_blank"
                      class="text-primary-600 hover:text-primary-500"
                      >{{ t('admin.settings.turnstile.cloudflareDashboard') }}</a
                    >
                  </p>
                </div>
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.turnstile.secretKey') }}
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
                        ? t('admin.settings.turnstile.secretKeyConfiguredHint')
                        : t('admin.settings.turnstile.secretKeyHint')
                    }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- LinuxDo Connect OAuth 登录 -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.linuxdo.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.linuxdo.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.linuxdo.enable')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.linuxdo.enableHint') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.linuxdo.clientId') }}
                  </label>
                  <input
                    v-model="form.linuxdo_connect_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.linuxdo.clientIdPlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.linuxdo.clientIdHint') }}
                  </p>
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.linuxdo.clientSecret') }}
                  </label>
                  <input
                    v-model="form.linuxdo_connect_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.linuxdo_connect_client_secret_configured
                        ? t('admin.settings.linuxdo.clientSecretConfiguredPlaceholder')
                        : t('admin.settings.linuxdo.clientSecretPlaceholder')
                    "
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      form.linuxdo_connect_client_secret_configured
                        ? t('admin.settings.linuxdo.clientSecretConfiguredHint')
                        : t('admin.settings.linuxdo.clientSecretHint')
                    }}
                  </p>
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.linuxdo.redirectUrl') }}
                  </label>
                  <input
                    v-model="form.linuxdo_connect_redirect_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.linuxdo.redirectUrlPlaceholder')"
                  />
                  <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm w-fit"
                      @click="setAndCopyLinuxdoRedirectUrl"
                    >
                      {{ t('admin.settings.linuxdo.quickSetCopy') }}
                    </button>
                    <code
                      v-if="linuxdoRedirectUrlSuggestion"
                      class="select-all break-all rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                    >
                      {{ linuxdoRedirectUrlSuggestion }}
                    </code>
                  </div>
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.linuxdo.redirectUrlHint') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Generic OIDC OAuth 登录 -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.oidc.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.oidc.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.oidc.enable')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.oidc.enableHint') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.providerName') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_provider_name"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.oidc.providerNamePlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.clientId') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.clientIdPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.clientSecret') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.oidc_connect_client_secret_configured
                        ? t('admin.settings.oidc.clientSecretConfiguredPlaceholder')
                        : t('admin.settings.oidc.clientSecretPlaceholder')
                    "
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{
                      form.oidc_connect_client_secret_configured
                        ? t('admin.settings.oidc.clientSecretConfiguredHint')
                        : t('admin.settings.oidc.clientSecretHint')
                    }}
                  </p>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.issuerUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_issuer_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.issuerUrlPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.discoveryUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_discovery_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.discoveryUrlPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.authorizeUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_authorize_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.authorizeUrlPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.tokenUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_token_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.tokenUrlPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.userinfoUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_userinfo_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.userinfoUrlPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.jwksUrl') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.scopes') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_scopes"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.scopesPlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.oidc.scopesHint') }}
                  </p>
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.redirectUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_redirect_url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.redirectUrlPlaceholder')"
                  />
                  <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm w-fit"
                      @click="setAndCopyOIDCRedirectUrl"
                    >
                      {{ t('admin.settings.oidc.quickSetCopy') }}
                    </button>
                    <code
                      v-if="oidcRedirectUrlSuggestion"
                      class="select-all break-all rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                    >
                      {{ oidcRedirectUrlSuggestion }}
                    </code>
                  </div>
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.oidc.redirectUrlHint') }}
                  </p>
                </div>

                <div class="lg:col-span-2">
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.frontendRedirectUrl') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_frontend_redirect_url"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.frontendRedirectUrlPlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.oidc.frontendRedirectUrlHint') }}
                  </p>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.tokenAuthMethod') }}
                  </label>
                  <select v-model="form.oidc_connect_token_auth_method" class="input font-mono text-sm">
                    <option value="client_secret_post">client_secret_post</option>
                    <option value="client_secret_basic">client_secret_basic</option>
                    <option value="none">none</option>
                  </select>
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.clockSkewSeconds') }}
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
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.allowedSigningAlgs') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_allowed_signing_algs"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.allowedSigningAlgsPlaceholder')"
                  />
                </div>
              </div>

              <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                <div class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">
                      {{ t('admin.settings.oidc.usePkce') }}
                    </label>
                  </div>
                  <Toggle v-model="form.oidc_connect_use_pkce" />
                </div>

                <div class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">
                      {{ t('admin.settings.oidc.validateIdToken') }}
                    </label>
                  </div>
                  <Toggle v-model="form.oidc_connect_validate_id_token" />
                </div>

                <div class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700">
                  <div>
                    <label class="font-medium text-gray-900 dark:text-white">
                      {{ t('admin.settings.oidc.requireEmailVerified') }}
                    </label>
                  </div>
                  <Toggle v-model="form.oidc_connect_require_email_verified" />
                </div>
              </div>

              <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.userinfoEmailPath') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_userinfo_email_path"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.userinfoEmailPathPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.userinfoIdPath') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_userinfo_id_path"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.userinfoIdPathPlaceholder')"
                  />
                </div>

                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.oidc.userinfoUsernamePath') }}
                  </label>
                  <input
                    v-model="form.oidc_connect_userinfo_username_path"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.oidc.userinfoUsernamePathPlaceholder')"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
        </div><!-- /Tab: Security — Registration, Turnstile, LinuxDo, OIDC -->

        <!-- Tab: Users -->
        <div v-show="activeTab === 'users'" class="space-y-6">
        <!-- Default Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.defaults.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.defaults.description') }}
            </p>
          </div>
          <div class="space-y-6 p-6">
            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.defaults.defaultBalance') }}
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
                  {{ t('admin.settings.defaults.defaultBalanceHint') }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.defaults.defaultConcurrency') }}
                </label>
                <input
                  v-model.number="form.default_concurrency"
                  type="number"
                  min="1"
                  class="input"
                  placeholder="1"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.defaults.defaultConcurrencyHint') }}
                </p>
              </div>
            </div>

            <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
              <div class="mb-3 flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t('admin.settings.defaults.defaultSubscriptions') }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.defaults.defaultSubscriptionsHint') }}
                  </p>
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="addDefaultSubscription"
                  :disabled="subscriptionGroups.length === 0"
                >
                  {{ t('admin.settings.defaults.addDefaultSubscription') }}
                </button>
              </div>

              <div
                v-if="form.default_subscriptions.length === 0"
                class="rounded border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
              >
                {{ t('admin.settings.defaults.defaultSubscriptionsEmpty') }}
              </div>

              <div v-else class="space-y-3">
                <div
                  v-for="(item, index) in form.default_subscriptions"
                  :key="`default-sub-${index}`"
                  class="grid grid-cols-1 gap-3 rounded border border-gray-200 p-3 md:grid-cols-[1fr_160px_auto] dark:border-dark-600"
                >
                  <div>
                    <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                      {{ t('admin.settings.defaults.subscriptionGroup') }}
                    </label>
                    <Select
                      v-model="item.group_id"
                      class="default-sub-group-select"
                      :options="defaultSubscriptionGroupOptions"
                      :placeholder="t('admin.settings.defaults.subscriptionGroup')"
                    >
                      <template #selected="{ option }">
                        <GroupBadge
                          v-if="option"
                          :name="(option as unknown as DefaultSubscriptionGroupOption).label"
                          :platform="(option as unknown as DefaultSubscriptionGroupOption).platform"
                          :subscription-type="(option as unknown as DefaultSubscriptionGroupOption).subscriptionType"
                          :rate-multiplier="(option as unknown as DefaultSubscriptionGroupOption).rate"
                        />
                        <span v-else class="text-gray-400">
                          {{ t('admin.settings.defaults.subscriptionGroup') }}
                        </span>
                      </template>
                      <template #option="{ option, selected }">
                        <GroupOptionItem
                          :name="(option as unknown as DefaultSubscriptionGroupOption).label"
                          :platform="(option as unknown as DefaultSubscriptionGroupOption).platform"
                          :subscription-type="(option as unknown as DefaultSubscriptionGroupOption).subscriptionType"
                          :rate-multiplier="(option as unknown as DefaultSubscriptionGroupOption).rate"
                          :description="(option as unknown as DefaultSubscriptionGroupOption).description"
                          :selected="selected"
                        />
                      </template>
                    </Select>
                  </div>
                  <div>
                    <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                      {{ t('admin.settings.defaults.subscriptionValidityDays') }}
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
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        </div><!-- /Tab: Users -->

        <!-- Tab: Gateway — Claude Code, Scheduling -->
        <div v-show="activeTab === 'gateway'" class="space-y-6">
        <!-- Claude Code Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.claudeCode.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.claudeCode.description') }}
            </p>
          </div>
          <div class="p-6">
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.claudeCode.minVersion') }}
              </label>
              <input
                v-model="form.min_claude_code_version"
                type="text"
                class="input max-w-xs font-mono text-sm"
                :placeholder="t('admin.settings.claudeCode.minVersionPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.claudeCode.minVersionHint') }}
              </p>
            </div>
            <div class="mt-4">
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.claudeCode.maxVersion') }}
              </label>
              <input
                v-model="form.max_claude_code_version"
                type="text"
                class="input max-w-xs font-mono text-sm"
                :placeholder="t('admin.settings.claudeCode.maxVersionPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.claudeCode.maxVersionHint') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Gateway Scheduling Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.scheduling.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.scheduling.description') }}
            </p>
          </div>
          <div class="p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.scheduling.allowUngroupedKey') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.scheduling.allowUngroupedKeyHint') }}
                </p>
              </div>
              <label class="toggle">
                <input v-model="form.allow_ungrouped_key_scheduling" type="checkbox" />
                <span class="toggle-slider"></span>
              </label>
            </div>
          </div>
        </div>

        <!-- Gateway Forwarding Behavior -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.gatewayForwarding.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.gatewayForwarding.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Fingerprint Unification -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.gatewayForwarding.fingerprintUnification') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.gatewayForwarding.fingerprintUnificationHint') }}
                </p>
              </div>
              <Toggle v-model="form.enable_fingerprint_unification" />
            </div>

            <!-- Metadata Passthrough -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.gatewayForwarding.metadataPassthrough') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.gatewayForwarding.metadataPassthroughHint') }}
                </p>
              </div>
              <Toggle v-model="form.enable_metadata_passthrough" />
            </div>

            <!-- CCH Signing -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.gatewayForwarding.cchSigning') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.gatewayForwarding.cchSigningHint') }}
                </p>
              </div>
              <Toggle v-model="form.enable_cch_signing" />
            </div>
          </div>
        </div>
        <!-- Web Search Emulation -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.webSearchEmulation.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.webSearchEmulation.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <!-- Global Toggle -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.webSearchEmulation.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.webSearchEmulation.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="webSearchConfig.enabled" />
            </div>

            <!-- Providers -->
            <div v-if="webSearchConfig.enabled" class="space-y-4">
              <div class="flex items-center justify-between">
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.webSearchEmulation.providers') }}
                </label>
                <button type="button" class="btn btn-secondary btn-sm" @click="addWebSearchProvider">
                  {{ t('admin.settings.webSearchEmulation.addProvider') }}
                </button>
              </div>

              <div v-if="webSearchConfig.providers.length === 0" class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-400 dark:border-dark-600">
                {{ t('admin.settings.webSearchEmulation.noProviders') }}
              </div>

              <div v-for="(provider, pIdx) in webSearchConfig.providers" :key="pIdx"
                class="rounded-lg border border-gray-200 dark:border-dark-600">
                <!-- Collapsible header -->
                <div
                  class="flex cursor-pointer items-center justify-between px-4 py-3"
                  @click="toggleProviderExpand(pIdx)"
                >
                  <div class="flex items-center gap-3">
                    <svg
                      class="h-4 w-4 text-gray-400 transition-transform"
                      :class="{ 'rotate-90': expandedProviders[pIdx] }"
                      fill="none" viewBox="0 0 24 24" stroke="currentColor"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
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
                    <!-- Quota summary in collapsed state -->
                    <span v-if="!expandedProviders[pIdx] && provider.quota_limit > 0" class="text-xs text-gray-400">
                      {{ provider.quota_used ?? 0 }} / {{ provider.quota_limit }}
                    </span>
                    <span v-if="!expandedProviders[pIdx] && provider.api_key_configured" class="text-xs text-green-500">
                      {{ t('admin.settings.webSearchEmulation.apiKeyConfigured') }}
                    </span>
                  </div>
                  <button type="button" class="text-red-500 hover:text-red-700 text-xs" @click.stop="removeWebSearchProvider(pIdx)">
                    {{ t('admin.settings.webSearchEmulation.removeProvider') }}
                  </button>
                </div>

                <!-- Expanded content -->
                <div v-if="expandedProviders[pIdx]" class="space-y-3 border-t border-gray-100 px-4 pb-4 pt-3 dark:border-dark-700">
                  <!-- API Key with show/copy -->
                  <div>
                    <label class="text-xs text-gray-500">{{ t('admin.settings.webSearchEmulation.apiKey') }}</label>
                    <div class="flex items-center gap-1">
                      <input
                        v-model="provider.api_key"
                        :type="apiKeyVisible[pIdx] ? 'text' : 'password'"
                        class="input flex-1 text-sm"
                        :placeholder="provider.api_key_configured ? '••••••••' : t('admin.settings.webSearchEmulation.apiKeyPlaceholder')"
                      />
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm px-2"
                        :title="apiKeyVisible[pIdx] ? t('admin.settings.webSearchEmulation.hideApiKey') : t('admin.settings.webSearchEmulation.showApiKey')"
                        @click="apiKeyVisible[pIdx] = !apiKeyVisible[pIdx]"
                      >
                        <svg v-if="!apiKeyVisible[pIdx]" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        </svg>
                        <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                        </svg>
                      </button>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm px-2"
                        :title="t('admin.settings.webSearchEmulation.copyApiKey')"
                        @click="copyApiKey(pIdx)"
                      >
                        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                      </button>
                    </div>
                  </div>

                  <!-- Quota + Subscription in compact row -->
                  <div class="grid grid-cols-2 gap-3">
                    <div>
                      <label class="text-xs text-gray-500">{{ t('admin.settings.webSearchEmulation.quotaLimit') }}</label>
                      <input v-model.number="provider.quota_limit" type="number" min="0" class="input text-sm" />
                      <p class="mt-0.5 text-xs text-gray-400">{{ t('admin.settings.webSearchEmulation.quotaLimitHint') }}</p>
                    </div>
                    <div>
                      <label class="text-xs text-gray-500">{{ t('admin.settings.webSearchEmulation.subscribedAt') }}</label>
                      <input
                        :value="formatSubscribedAt(provider.subscribed_at)"
                        type="date"
                        class="input text-sm"
                        @input="provider.subscribed_at = parseSubscribedAt(($event.target as HTMLInputElement).value)"
                      />
                      <p class="mt-0.5 text-xs text-gray-400">{{ t('admin.settings.webSearchEmulation.subscribedAtHint') }}</p>
                    </div>
                  </div>

                  <!-- Usage display -->
                  <div v-if="provider.quota_limit > 0" class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">{{ t('admin.settings.webSearchEmulation.quotaUsage') }}:</span>
                    <div class="flex-1 rounded-full bg-gray-200 dark:bg-dark-600" style="height: 6px">
                      <div
                        class="h-full rounded-full transition-all"
                        :class="quotaPercentage(provider) > 90 ? 'bg-red-500' : quotaPercentage(provider) > 70 ? 'bg-yellow-500' : 'bg-green-500'"
                        :style="{ width: Math.min(quotaPercentage(provider), 100) + '%' }"
                      />
                    </div>
                    <span class="text-xs text-gray-500">{{ provider.quota_used ?? 0 }} / {{ provider.quota_limit }}</span>
                  </div>

                  <!-- Proxy -->
                  <div>
                    <label class="text-xs text-gray-500">{{ t('admin.settings.webSearchEmulation.proxy') }}</label>
                    <ProxySelector v-model="provider.proxy_id" :proxies="webSearchProxies" />
                  </div>

                  <!-- Test button -->
                  <div class="border-t border-gray-100 pt-3 dark:border-dark-700">
                    <div class="flex items-center gap-2">
                      <input
                        v-model="wsTestQuery"
                        type="text"
                        class="input flex-1 text-sm"
                        :placeholder="t('admin.settings.webSearchEmulation.testDefaultQuery')"
                        @keyup.enter="testWebSearchProvider()"
                      />
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="wsTestLoading"
                        @click="testWebSearchProvider()"
                      >
                        {{ wsTestLoading ? t('admin.settings.webSearchEmulation.testing') : t('admin.settings.webSearchEmulation.test') }}
                      </button>
                    </div>
                    <!-- Test results -->
                    <div v-if="wsTestResult" class="mt-2 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-700">
                      <p class="mb-1 font-medium text-gray-700 dark:text-gray-300">
                        {{ t('admin.settings.webSearchEmulation.testResultProvider') }}: {{ wsTestResult.provider }}
                      </p>
                      <div v-if="wsTestResult.results.length === 0" class="text-gray-400">
                        {{ t('admin.settings.webSearchEmulation.testNoResults') }}
                      </div>
                      <div v-for="(r, rIdx) in wsTestResult.results.slice(0, 3)" :key="rIdx" class="mt-1">
                        <a :href="r.url" target="_blank" class="font-medium text-blue-600 hover:underline dark:text-blue-400">{{ r.title }}</a>
                        <p class="text-gray-500 dark:text-gray-400">{{ r.snippet && r.snippet.length > 120 ? r.snippet.slice(0, 120) + '...' : r.snippet }}</p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        </div><!-- /Tab: Gateway — Claude Code, Scheduling -->

        <!-- Tab: General -->
        <div v-show="activeTab === 'general'" class="space-y-6">
        <!-- Site Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.site.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.site.description') }}
            </p>
          </div>
          <div class="space-y-6 p-6">
            <!-- Backend Mode -->
            <div
              class="flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20"
            >
              <div>
                <h3 class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.settings.site.backendMode') }}
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.site.backendModeDescription') }}
                </p>
              </div>
              <Toggle v-model="form.backend_mode_enabled" />
            </div>

            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.site.siteName') }}
                </label>
                <input
                  v-model="form.site_name"
                  type="text"
                  class="input"
                  :placeholder="t('admin.settings.site.siteNamePlaceholder')"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.site.siteNameHint') }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.site.siteSubtitle') }}
                </label>
                <input
                  v-model="form.site_subtitle"
                  type="text"
                  class="input"
                  :placeholder="t('admin.settings.site.siteSubtitlePlaceholder')"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.site.siteSubtitleHint') }}
                </p>
              </div>
            </div>

            <!-- API Base URL -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.apiBaseUrl') }}
              </label>
              <input
                v-model="form.api_base_url"
                type="text"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.site.apiBaseUrlPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.apiBaseUrlHint') }}
              </p>
            </div>

            <!-- Global Table Preferences -->
            <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
              <h3 class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.settings.site.tablePreferencesTitle') }}
              </h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.tablePreferencesDescription') }}
              </p>
              <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.site.tableDefaultPageSize') }}
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
                    {{ t('admin.settings.site.tableDefaultPageSizeHint') }}
                  </p>
                </div>
                <div>
                  <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.site.tablePageSizeOptions') }}
                  </label>
                  <input
                    v-model="tablePageSizeOptionsInput"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.site.tablePageSizeOptionsPlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.site.tablePageSizeOptionsHint') }}
                  </p>
                </div>
              </div>
            </div>

            <!-- Custom Endpoints -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.customEndpoints.title') }}
              </label>
              <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.customEndpoints.description') }}
              </p>

              <div class="space-y-3">
                <div
                  v-for="(ep, index) in form.custom_endpoints"
                  :key="index"
                  class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
                >
                  <div class="mb-3 flex items-center justify-between">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {{ t('admin.settings.site.customEndpoints.itemLabel', { n: index + 1 }) }}
                    </span>
                    <button
                      type="button"
                      class="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                      @click="removeEndpoint(index)"
                    >
                      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                    </button>
                  </div>
                  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div>
                      <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                        {{ t('admin.settings.site.customEndpoints.name') }}
                      </label>
                      <input
                        v-model="ep.name"
                        type="text"
                        class="input text-sm"
                        :placeholder="t('admin.settings.site.customEndpoints.namePlaceholder')"
                      />
                    </div>
                    <div>
                      <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                        {{ t('admin.settings.site.customEndpoints.endpointUrl') }}
                      </label>
                      <input
                        v-model="ep.endpoint"
                        type="url"
                        class="input font-mono text-sm"
                        :placeholder="t('admin.settings.site.customEndpoints.endpointUrlPlaceholder')"
                      />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                        {{ t('admin.settings.site.customEndpoints.descriptionLabel') }}
                      </label>
                      <input
                        v-model="ep.description"
                        type="text"
                        class="input text-sm"
                        :placeholder="t('admin.settings.site.customEndpoints.descriptionPlaceholder')"
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
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" /></svg>
                {{ t('admin.settings.site.customEndpoints.add') }}
              </button>
            </div>

            <!-- Contact Info -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.contactInfo') }}
              </label>
              <input
                v-model="form.contact_info"
                type="text"
                class="input"
                :placeholder="t('admin.settings.site.contactInfoPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.contactInfoHint') }}
              </p>
            </div>

            <!-- Doc URL -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.docUrl') }}
              </label>
              <input
                v-model="form.doc_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.site.docUrlPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.docUrlHint') }}
              </p>
            </div>

            <!-- Site Logo Upload -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.siteLogo') }}
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
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.site.homeContent') }}
              </label>
              <textarea
                v-model="form.home_content"
                rows="6"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.site.homeContentPlaceholder')"
              ></textarea>
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.site.homeContentHint') }}
              </p>
              <!-- iframe CSP Warning -->
              <p class="mt-2 text-xs text-amber-600 dark:text-amber-400">
                {{ t('admin.settings.site.homeContentIframeWarning') }}
              </p>
            </div>

            <!-- Hide CCS Import Button -->
            <div
              class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
            >
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{
                  t('admin.settings.site.hideCcsImportButton')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.site.hideCcsImportButtonHint') }}
                </p>
              </div>
              <Toggle v-model="form.hide_ccs_import_button" />
            </div>
          </div>
        </div>


        <!-- Custom Menu Items -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.customMenu.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.customMenu.description') }}
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
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.customMenu.itemLabel', { n: index + 1 }) }}
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
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 15l7-7 7 7" /></svg>
                  </button>
                  <!-- Move down -->
                  <button
                    v-if="index < form.custom_menu_items.length - 1"
                    type="button"
                    class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700"
                    :title="t('admin.settings.customMenu.moveDown')"
                    @click="moveMenuItem(index, 1)"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
                  </button>
                  <!-- Delete -->
                  <button
                    type="button"
                    class="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                    :title="t('admin.settings.customMenu.remove')"
                    @click="removeMenuItem(index)"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <!-- Label -->
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.customMenu.name') }}
                  </label>
                  <input
                    v-model="item.label"
                    type="text"
                    class="input text-sm"
                    :placeholder="t('admin.settings.customMenu.namePlaceholder')"
                  />
                </div>

                <!-- Visibility -->
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.customMenu.visibility') }}
                  </label>
                  <select v-model="item.visibility" class="input text-sm">
                    <option value="user">{{ t('admin.settings.customMenu.visibilityUser') }}</option>
                    <option value="admin">{{ t('admin.settings.customMenu.visibilityAdmin') }}</option>
                  </select>
                </div>

                <!-- URL (full width) -->
                <div class="sm:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.customMenu.url') }}
                  </label>
                  <input
                    v-model="item.url"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.customMenu.urlPlaceholder')"
                  />
                </div>

                <!-- SVG Icon (full width) -->
                <div class="sm:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.settings.customMenu.iconSvg') }}
                  </label>
                  <ImageUpload
                    :model-value="item.icon_svg"
                    mode="svg"
                    size="sm"
                    :upload-label="t('admin.settings.customMenu.uploadSvg')"
                    :remove-label="t('admin.settings.customMenu.removeSvg')"
                    @update:model-value="(v: string) => item.icon_svg = v"
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
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" /></svg>
              {{ t('admin.settings.customMenu.add') }}
            </button>
          </div>
        </div>

        </div><!-- /Tab: General -->

        <!-- Tab: Email -->
<!-- Tab: Payment -->
        <div v-show="activeTab === 'payment'" class="space-y-6">

        <!-- Payment System Settings -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.payment.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.payment.description') }}
              <a :href="locale === 'zh' ? 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md' : 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT.md'" target="_blank" rel="noopener noreferrer" class="ml-2 inline-flex items-center text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300">
                <svg class="mr-0.5 h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" /></svg>
                {{ t('admin.settings.payment.configGuide') }}
              </a>
            </p>
          </div>
          <div class="space-y-4 p-6">
            <!-- Enable toggle -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.payment.enabled') }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.enabledHint') }}</p>
              </div>
              <Toggle v-model="form.payment_enabled" />
            </div>
            <template v-if="form.payment_enabled">
              <!-- Row 1: Product name -->
              <div class="grid grid-cols-3 gap-3">
                <div><label class="input-label">{{ t('admin.settings.payment.productNamePrefix') }}</label><input v-model="form.payment_product_name_prefix" type="text" class="input" placeholder="Sub2API" /></div>
                <div><label class="input-label">{{ t('admin.settings.payment.productNameSuffix') }}</label><input v-model="form.payment_product_name_suffix" type="text" class="input" placeholder="CNY" /></div>
                <div><label class="input-label">{{ t('admin.settings.payment.preview') }}</label><div class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">{{ (form.payment_product_name_prefix || 'Sub2API') + ' 100 ' + (form.payment_product_name_suffix || 'CNY') }}</div></div>
              </div>
              <!-- Row 2: Balance toggle + amounts -->
              <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <div><label class="input-label">{{ t('admin.settings.payment.minAmount') }}</label><input :value="form.payment_min_amount || ''" @input="form.payment_min_amount = parseFloat(($event.target as HTMLInputElement).value) || 0" type="number" step="0.01" min="0" class="input" :placeholder="t('admin.settings.payment.noLimit')" /></div>
                <div><label class="input-label">{{ t('admin.settings.payment.maxAmount') }}</label><input :value="form.payment_max_amount || ''" @input="form.payment_max_amount = parseFloat(($event.target as HTMLInputElement).value) || 0" type="number" step="0.01" min="0" class="input" :placeholder="t('admin.settings.payment.noLimit')" /></div>
                <div><label class="input-label">{{ t('admin.settings.payment.dailyLimit') }}</label><input :value="form.payment_daily_limit || ''" @input="form.payment_daily_limit = parseFloat(($event.target as HTMLInputElement).value) || 0" type="number" step="0.01" min="0" class="input" :placeholder="t('admin.settings.payment.noLimit')" /></div>
                <div><label class="input-label">{{ t('admin.settings.payment.orderTimeout') }} <span class="text-red-500">*</span></label><input v-model.number="form.payment_order_timeout_minutes" type="number" min="1" class="input" required /><p class="mt-0.5 text-xs text-gray-400">{{ t('admin.settings.payment.orderTimeoutHint') }}</p></div>
              </div>
              <!-- Row 3: Pending orders + load balance + cancel rate limit (all in one row) -->
              <div class="flex flex-wrap items-end gap-4">
                <div class="w-28"><label class="input-label">{{ t('admin.settings.payment.maxPendingOrders') }}</label><input v-model.number="form.payment_max_pending_orders" type="number" min="1" class="input" /></div>
                <div>
                  <label class="input-label">{{ t('admin.settings.payment.loadBalanceStrategy') }}</label>
                  <Select v-model="form.payment_load_balance_strategy" :options="loadBalanceOptions" class="w-40" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.settings.payment.cancelRateLimit') }}</label>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      :class="[
                        'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                        form.payment_cancel_rate_limit_enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
                      ]"
                      @click="form.payment_cancel_rate_limit_enabled = !form.payment_cancel_rate_limit_enabled"
                    >
                      <span :class="[
                        'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                        form.payment_cancel_rate_limit_enabled ? 'translate-x-5' : 'translate-x-0'
                      ]" />
                    </button>
                    <Select v-model="form.payment_cancel_rate_limit_window_mode" :options="cancelRateLimitModeOptions" class="w-24" :disabled="!form.payment_cancel_rate_limit_enabled" />
                    <span :class="['text-sm whitespace-nowrap', form.payment_cancel_rate_limit_enabled ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400 dark:text-gray-600']">{{ t('admin.settings.payment.cancelRateLimitEvery') }}</span>
                    <input v-model.number="form.payment_cancel_rate_limit_window" type="number" min="1" required class="input w-14 text-center" :disabled="!form.payment_cancel_rate_limit_enabled" />
                    <Select v-model="form.payment_cancel_rate_limit_unit" :options="cancelRateLimitUnitOptions" class="w-28" :disabled="!form.payment_cancel_rate_limit_enabled" />
                    <span :class="['text-sm whitespace-nowrap', form.payment_cancel_rate_limit_enabled ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400 dark:text-gray-600']">{{ t('admin.settings.payment.cancelRateLimitAllowMax') }}</span>
                    <input v-model.number="form.payment_cancel_rate_limit_max" type="number" min="1" required class="input w-14 text-center" :disabled="!form.payment_cancel_rate_limit_enabled" />
                    <span :class="['text-sm whitespace-nowrap', form.payment_cancel_rate_limit_enabled ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400 dark:text-gray-600']">{{ t('admin.settings.payment.cancelRateLimitTimes') }}</span>
                  </div>
                </div>
              </div>
              <!-- Row 4: Enabled payment types (provider badges like sub2apipay) -->
              <div>
                <label class="input-label">{{ t('admin.settings.payment.enabledPaymentTypes') }}</label>
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
                  >{{ pt.label }}</button>
                </div>
                <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.settings.payment.enabledPaymentTypesHint') }}
                  <a :href="locale === 'zh' ? 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md#%E6%94%AF%E6%8C%81%E7%9A%84%E6%94%AF%E4%BB%98%E6%96%B9%E5%BC%8F' : 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT.md#supported-payment-methods'" target="_blank" rel="noopener noreferrer" class="ml-1 text-primary-500 hover:text-primary-600 dark:text-primary-400 dark:hover:text-primary-300">
                    {{ t('admin.settings.payment.findProvider') }}
                    <svg class="mb-0.5 ml-0.5 inline h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" /></svg>
                  </a>
                </p>
              </div>
              <!-- Row 5: Help image + text -->
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="input-label">{{ t('admin.settings.payment.helpImage') }}</label>
                  <ImageUpload v-model="form.payment_help_image_url" :placeholder="t('admin.settings.payment.helpImagePlaceholder')" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.settings.payment.helpText') }}</label>
                  <textarea v-model="form.payment_help_text" rows="3" class="input" :placeholder="t('admin.settings.payment.helpTextPlaceholder')"></textarea>
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
              <Icon name="mail" size="md" class="mt-0.5 flex-shrink-0 text-gray-400 dark:text-gray-500" />
              <div>
                <h3 class="font-medium text-gray-900 dark:text-white">
                  {{ t('admin.settings.emailTabDisabledTitle') }}
                </h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.emailTabDisabledHint') }}
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
                {{ t('admin.settings.smtp.title') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.smtp.description') }}
              </p>
            </div>
            <button
              type="button"
              @click="testSmtpConnection"
              :disabled="testingSmtp || loadFailed"
              class="btn btn-secondary btn-sm"
            >
              <svg v-if="testingSmtp" class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
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
                  ? t('admin.settings.smtp.testing')
                  : t('admin.settings.smtp.testConnection')
              }}
            </button>
          </div>
          <div class="space-y-6 p-6">
            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.host') }}
                </label>
                <input
                  v-model="form.smtp_host"
                  type="text"
                  class="input"
                  :placeholder="t('admin.settings.smtp.hostPlaceholder')"
                />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.port') }}
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
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.username') }}
                </label>
                <input
                  v-model="form.smtp_username"
                  type="text"
                  class="input"
                  :placeholder="t('admin.settings.smtp.usernamePlaceholder')"
                />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.password') }}
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
                      ? t('admin.settings.smtp.passwordConfiguredHint')
                      : t('admin.settings.smtp.passwordHint')
                  }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.fromEmail') }}
                </label>
                <input
                  v-model="form.smtp_from_email"
                  type="email"
                  class="input"
                  :placeholder="t('admin.settings.smtp.fromEmailPlaceholder')"
                />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.smtp.fromName') }}
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
                  t('admin.settings.smtp.useTls')
                }}</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.smtp.useTlsHint') }}
                </p>
              </div>
              <Toggle v-model="form.smtp_use_tls" />
            </div>

          </div>
        </div>

        <!-- Send Test Email - Only show when email verification is enabled -->
        <div v-if="form.email_verify_enabled" class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.testEmail.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.testEmail.description') }}
            </p>
          </div>
          <div class="p-6">
            <div class="flex items-end gap-4">
              <div class="flex-1">
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.testEmail.recipientEmail') }}
                </label>
                <input
                  v-model="testEmailAddress"
                  type="email"
                  class="input"
                  :placeholder="t('admin.settings.testEmail.recipientEmailPlaceholder')"
                />
              </div>
              <button
                type="button"
                @click="sendTestEmail"
                :disabled="sendingTestEmail || !testEmailAddress || loadFailed"
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
                    ? t('admin.settings.testEmail.sending')
                    : t('admin.settings.testEmail.sendTestEmail')
                }}
              </button>
            </div>
          </div>
        </div>
        <!-- Balance Low Notification -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h3 class="text-base font-medium text-gray-900 dark:text-white">
              {{ t('admin.settings.balanceNotify.title') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.balanceNotify.description') }}
            </p>
          </div>
          <div class="px-6 py-6 space-y-4">
            <div class="flex items-center justify-between">
              <label class="mb-0 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.balanceNotify.enabled') }}</label>
              <Toggle v-model="form.balance_low_notify_enabled" />
            </div>
            <div v-if="form.balance_low_notify_enabled" class="space-y-3">
              <!-- Threshold type selector -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.balanceNotify.thresholdType') }}</label>
                <div class="flex gap-4">
                  <label class="flex items-center gap-1.5 text-sm cursor-pointer">
                    <input type="radio" v-model="form.balance_low_notify_threshold_type" value="fixed" class="accent-primary-500" />
                    {{ t('admin.settings.balanceNotify.typeFixed') }}
                  </label>
                  <label class="flex items-center gap-1.5 text-sm cursor-pointer">
                    <input type="radio" v-model="form.balance_low_notify_threshold_type" value="percentage" class="accent-primary-500" />
                    {{ t('admin.settings.balanceNotify.typePercentage') }}
                  </label>
                </div>
              </div>
              <!-- Threshold value -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.balanceNotify.threshold') }}</label>
                <div class="relative">
                  <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400">
                    {{ form.balance_low_notify_threshold_type === 'percentage' ? '%' : '$' }}
                  </span>
                  <input
                    v-model.number="form.balance_low_notify_threshold"
                    type="number"
                    :min="0"
                    :max="form.balance_low_notify_threshold_type === 'percentage' ? 100 : undefined"
                    :step="form.balance_low_notify_threshold_type === 'percentage' ? 1 : 0.01"
                    class="input pl-7"
                  />
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ form.balance_low_notify_threshold_type === 'percentage'
                    ? t('admin.settings.balanceNotify.percentageHint')
                    : t('admin.settings.balanceNotify.thresholdHint') }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Account Quota Notification -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h3 class="text-base font-medium text-gray-900 dark:text-white">
              {{ t('admin.settings.quotaNotify.title') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.quotaNotify.description') }}
            </p>
          </div>
          <div class="px-6 py-6 space-y-4">
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.quotaNotify.emails') }}</label>
              <div class="space-y-2">
                <div v-for="(_, index) in (form.account_quota_notify_emails || [])" :key="index" class="flex items-center gap-2">
                  <input v-model="form.account_quota_notify_emails[index]" type="email" class="input flex-1" />
                  <button @click="form.account_quota_notify_emails.splice(index, 1)" class="btn btn-secondary px-2" type="button">
                    <Icon name="x" size="xs" class="h-4 w-4" />
                  </button>
                </div>
                <button @click="addQuotaNotifyEmail" class="btn btn-secondary btn-sm" type="button">
                  + {{ t('admin.settings.quotaNotify.addEmail') }}
                </button>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.quotaNotify.emailsHint') }}</p>
            </div>
          </div>
        </div>
        </div><!-- /Tab: Email -->

        <!-- Tab: Backup -->
        <div v-show="activeTab === 'backup'">
          <BackupSettings />
        </div>

        <!-- Save Button -->
        <div v-show="activeTab !== 'backup'" class="flex justify-end">
          <button type="submit" :disabled="saving || loadFailed" class="btn btn-primary">
            <svg v-if="saving" class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
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
            {{ saving ? t('admin.settings.saving') : t('admin.settings.saveSettings') }}
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
      <ConfirmDialog :show="showDeleteProviderDialog" :title="t('admin.settings.payment.deleteProvider')" :message="t('admin.settings.payment.deleteProviderConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeleteProvider" @cancel="showDeleteProviderDialog = false" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type {
  SystemSettings,
  UpdateSettingsRequest,
  DefaultSubscriptionSetting,
  WebSearchEmulationConfig,
  WebSearchProviderConfig,
  WebSearchTestResult,
} from '@/api/admin/settings'
import type { AdminGroup, Proxy } from '@/types'
import type { ProviderInstance } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import PaymentProviderList from '@/components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Toggle from '@/components/common/Toggle.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import ImageUpload from '@/components/common/ImageUpload.vue'
import BackupSettings from '@/views/admin/BackupView.vue'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import {
  isRegistrationEmailSuffixDomainValid,
  normalizeRegistrationEmailSuffixDomain,
  normalizeRegistrationEmailSuffixDomains,
  parseRegistrationEmailSuffixWhitelistInput
} from '@/utils/registrationEmailPolicy'

const { t, locale } = useI18n()
const appStore = useAppStore()
const adminSettingsStore = useAdminSettingsStore()

type SettingsTab = 'general' | 'security' | 'users' | 'gateway' | 'payment' | 'email' | 'backup'
const activeTab = ref<SettingsTab>('general')
const settingsTabs = [
  { key: 'general'  as SettingsTab, icon: 'home'   as const },
  { key: 'security' as SettingsTab, icon: 'shield' as const },
  { key: 'users'    as SettingsTab, icon: 'user'   as const },
  { key: 'gateway'  as SettingsTab, icon: 'server' as const },
  { key: 'payment'  as SettingsTab, icon: 'creditCard' as const },
  { key: 'email'    as SettingsTab, icon: 'mail'   as const },
  { key: 'backup'   as SettingsTab, icon: 'database' as const },
]
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)
const testingSmtp = ref(false)
const sendingTestEmail = ref(false)
const smtpPasswordManuallyEdited = ref(false)
const testEmailAddress = ref('')
const registrationEmailSuffixWhitelistTags = ref<string[]>([])
const registrationEmailSuffixWhitelistDraft = ref('')
const tablePageSizeOptionsInput = ref('10, 20, 50, 100')

// Admin API Key 状态
const adminApiKeyLoading = ref(true)
const adminApiKeyExists = ref(false)
const adminApiKeyMasked = ref('')
const adminApiKeyOperating = ref(false)
const newAdminApiKey = ref('')
const subscriptionGroups = ref<AdminGroup[]>([])

// Overload Cooldown (529) 状态
const overloadCooldownLoading = ref(true)
const overloadCooldownSaving = ref(false)
const overloadCooldownForm = reactive({
  enabled: true,
  cooldown_minutes: 10
})

// Stream Timeout 状态
const streamTimeoutLoading = ref(true)
const streamTimeoutSaving = ref(false)
const streamTimeoutForm = reactive({
  enabled: true,
  action: 'temp_unsched' as 'temp_unsched' | 'error' | 'none',
  temp_unsched_minutes: 5,
  threshold_count: 3,
  threshold_window_minutes: 10
})

// Rectifier 状态
const rectifierLoading = ref(true)
const rectifierSaving = ref(false)
const rectifierForm = reactive({
  enabled: true,
  thinking_signature_enabled: true,
  thinking_budget_enabled: true,
  apikey_signature_enabled: false,
  apikey_signature_patterns: [] as string[]
})

// Beta Policy 状态
const betaPolicyLoading = ref(true)
const betaPolicySaving = ref(false)
const betaPolicyForm = reactive({
  rules: [] as Array<{
    beta_token: string
    action: 'pass' | 'filter' | 'block'
    scope: 'all' | 'oauth' | 'apikey' | 'bedrock'
    error_message?: string
    model_whitelist?: string[]
    fallback_action?: 'pass' | 'filter' | 'block'
    fallback_error_message?: string
  }>
})

const tablePageSizeMin = 5
const tablePageSizeMax = 1000
const tablePageSizeDefault = 20

interface DefaultSubscriptionGroupOption {
  value: number
  label: string
  description: string | null
  platform: AdminGroup['platform']
  subscriptionType: AdminGroup['subscription_type']
  rate: number
  [key: string]: unknown
}

type SettingsForm = SystemSettings & {
  smtp_password: string
  turnstile_secret_key: string
  linuxdo_connect_client_secret: string
  oidc_connect_client_secret: string
}

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
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: 'Subscription to API Conversion Platform',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  backend_mode_enabled: false,
  hide_ccs_import_button: false,
  payment_enabled: false,  payment_min_amount: 1,  payment_max_amount: 10000,  payment_daily_limit: 50000,  payment_max_pending_orders: 3,  payment_order_timeout_minutes: 30,  payment_balance_disabled: false,  payment_enabled_types: [],  payment_help_image_url: '',  payment_help_text: '',  payment_product_name_prefix: '',  payment_product_name_suffix: '',  payment_load_balance_strategy: 'round-robin',  payment_cancel_rate_limit_enabled: false,  payment_cancel_rate_limit_max: 10,  payment_cancel_rate_limit_window: 1,  payment_cancel_rate_limit_unit: 'day',  payment_cancel_rate_limit_window_mode: 'rolling',
  table_default_page_size: tablePageSizeDefault,
  table_page_size_options: [10, 20, 50, 100],
  custom_menu_items: [] as Array<{id: string; label: string; icon_svg: string; url: string; visibility: 'user' | 'admin'; sort_order: number}>,
  custom_endpoints: [] as Array<{name: string; endpoint: string; description: string}>,
  frontend_url: '',
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password: '',
  smtp_password_configured: false,
  smtp_from_email: '',
  smtp_from_name: '',
  smtp_use_tls: true,
  // Cloudflare Turnstile
  turnstile_enabled: false,
  turnstile_site_key: '',
  turnstile_secret_key: '',
  turnstile_secret_key_configured: false,
  // LinuxDo Connect OAuth 登录
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: '',
  linuxdo_connect_client_secret: '',
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: '',
  // Generic OIDC OAuth 登录
  oidc_connect_enabled: false,
  oidc_connect_provider_name: 'OIDC',
  oidc_connect_client_id: '',
  oidc_connect_client_secret: '',
  oidc_connect_client_secret_configured: false,
  oidc_connect_issuer_url: '',
  oidc_connect_discovery_url: '',
  oidc_connect_authorize_url: '',
  oidc_connect_token_url: '',
  oidc_connect_userinfo_url: '',
  oidc_connect_jwks_url: '',
  oidc_connect_scopes: 'openid email profile',
  oidc_connect_redirect_url: '',
  oidc_connect_frontend_redirect_url: '/auth/oidc/callback',
  oidc_connect_token_auth_method: 'client_secret_post',
  oidc_connect_use_pkce: false,
  oidc_connect_validate_id_token: true,
  oidc_connect_allowed_signing_algs: 'RS256,ES256,PS256',
  oidc_connect_clock_skew_seconds: 120,
  oidc_connect_require_email_verified: false,
  oidc_connect_userinfo_email_path: '',
  oidc_connect_userinfo_id_path: '',
  oidc_connect_userinfo_username_path: '',
  // Model fallback
  enable_model_fallback: false,
  fallback_model_anthropic: 'claude-3-5-sonnet-20241022',
  fallback_model_openai: 'gpt-4o',
  fallback_model_gemini: 'gemini-2.5-pro',
  fallback_model_antigravity: 'gemini-2.5-pro',
  // Identity patch (Claude -> Gemini)
  enable_identity_patch: true,
  identity_patch_prompt: '',
  // Ops monitoring (vNext)
  ops_monitoring_enabled: true,
  ops_realtime_monitoring_enabled: true,
  ops_query_mode_default: 'auto',
  ops_metrics_interval_seconds: 60,
  // Claude Code version check
  min_claude_code_version: '',
  max_claude_code_version: '',
  // 分组隔离
  allow_ungrouped_key_scheduling: false,
  // Gateway forwarding behavior
  enable_fingerprint_unification: true,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  // Balance & quota notification
  balance_low_notify_enabled: false,
  balance_low_notify_threshold_type: 'fixed' as 'fixed' | 'percentage',
  balance_low_notify_threshold: 0,
  account_quota_notify_emails: [] as string[]
})

// Proxies for web search emulation ProxySelector
const webSearchProxies = ref<Proxy[]>([])

// Web Search Emulation config (loaded/saved separately)
const DEFAULT_WEB_SEARCH_QUOTA_LIMIT = 1000

const webSearchConfig = reactive<WebSearchEmulationConfig>({
  enabled: false,
  providers: [],
})

const expandedProviders = reactive<Record<number, boolean>>({})
const apiKeyVisible = reactive<Record<number, boolean>>({})
const wsTestQuery = ref('')
const wsTestLoading = ref(false)
const wsTestResult = ref<WebSearchTestResult | null>(null)

function toggleProviderExpand(idx: number) {
  expandedProviders[idx] = !expandedProviders[idx]
}

function removeWebSearchProvider(idx: number) {
  webSearchConfig.providers.splice(idx, 1)
  // Re-index expandedProviders and apiKeyVisible after removal
  const newExpanded: Record<number, boolean> = {}
  const newVisible: Record<number, boolean> = {}
  for (let i = 0; i < webSearchConfig.providers.length; i++) {
    const oldIdx = i >= idx ? i + 1 : i
    newExpanded[i] = expandedProviders[oldIdx] ?? false
    newVisible[i] = apiKeyVisible[oldIdx] ?? false
  }
  Object.keys(expandedProviders).forEach((k) => delete expandedProviders[Number(k)])
  Object.keys(apiKeyVisible).forEach((k) => delete apiKeyVisible[Number(k)])
  Object.assign(expandedProviders, newExpanded)
  Object.assign(apiKeyVisible, newVisible)
}

function addWebSearchProvider() {
  const idx = webSearchConfig.providers.length
  webSearchConfig.providers.push({
    type: 'brave',
    api_key: '',
    api_key_configured: false,
    quota_limit: DEFAULT_WEB_SEARCH_QUOTA_LIMIT,
    subscribed_at: null,
    proxy_id: null,
    expires_at: null,
  } as WebSearchProviderConfig)
  expandedProviders[idx] = true
}

function formatSubscribedAt(ts: number | null): string {
  if (!ts) return ''
  // Use UTC to avoid timezone drift on repeated edits
  const d = new Date(ts * 1000)
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function parseSubscribedAt(dateStr: string): number | null {
  if (!dateStr) return null
  // Parse as UTC to match formatSubscribedAt
  return Math.floor(new Date(dateStr + 'T00:00:00Z').getTime() / 1000)
}

function quotaPercentage(provider: WebSearchProviderConfig): number {
  if (!provider.quota_limit || provider.quota_limit <= 0) return 0
  return ((provider.quota_used ?? 0) / provider.quota_limit) * 100
}

async function copyApiKey(idx: number) {
  const key = webSearchConfig.providers[idx]?.api_key
  if (!key) {
    appStore.showError(t('admin.settings.webSearchEmulation.apiKeyPlaceholder'))
    return
  }
  try {
    await navigator.clipboard.writeText(key)
    appStore.showSuccess(t('admin.settings.webSearchEmulation.copied'))
  } catch {
    appStore.showError(t('common.error'))
  }
}

async function testWebSearchProvider() {
  wsTestLoading.value = true
  wsTestResult.value = null
  try {
    const query = wsTestQuery.value.trim() || t('admin.settings.webSearchEmulation.testDefaultQuery')
    wsTestResult.value = await adminAPI.settings.testWebSearchEmulation(query)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    wsTestLoading.value = false
  }
}

async function loadWebSearchConfig() {
  try {
    const [resp, proxiesResp] = await Promise.all([
      adminAPI.settings.getWebSearchEmulationConfig(),
      adminAPI.proxies.list().catch(() => ({ items: [] as Proxy[] })),
    ])
    if (resp) {
      webSearchConfig.enabled = resp.enabled || false
      webSearchConfig.providers = resp.providers || []
    }
    webSearchProxies.value = proxiesResp.items || []
  } catch (err: unknown) {
    // 404 is expected when config hasn't been created yet; show error for other failures
    const status = (err as { status?: number })?.status
    if (status !== 404 && status !== undefined) {
      appStore.showError(extractApiErrorMessage(err, t('common.error')))
    }
  }
}

async function saveWebSearchConfig(): Promise<boolean> {
  try {
    await adminAPI.settings.updateWebSearchEmulationConfig({
      enabled: webSearchConfig.enabled,
      providers: webSearchConfig.providers as WebSearchProviderConfig[],
    })
    return true
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    return false
  }
}

const defaultSubscriptionGroupOptions = computed<DefaultSubscriptionGroupOption[]>(() =>
  subscriptionGroups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    platform: group.platform,
    subscriptionType: group.subscription_type,
    rate: group.rate_multiplier
  }))
)

const registrationEmailSuffixWhitelistSeparatorKeys = new Set([' ', ',', '，', 'Enter', 'Tab'])

function removeRegistrationEmailSuffixWhitelistTag(suffix: string) {
  registrationEmailSuffixWhitelistTags.value = registrationEmailSuffixWhitelistTags.value.filter(
    (item) => item !== suffix
  )
}

function addRegistrationEmailSuffixWhitelistTag(raw: string) {
  const suffix = normalizeRegistrationEmailSuffixDomain(raw)
  if (
    !isRegistrationEmailSuffixDomainValid(suffix) ||
    registrationEmailSuffixWhitelistTags.value.includes(suffix)
  ) {
    return
  }
  registrationEmailSuffixWhitelistTags.value = [
    ...registrationEmailSuffixWhitelistTags.value,
    suffix
  ]
}

function commitRegistrationEmailSuffixWhitelistDraft() {
  if (!registrationEmailSuffixWhitelistDraft.value) {
    return
  }
  addRegistrationEmailSuffixWhitelistTag(registrationEmailSuffixWhitelistDraft.value)
  registrationEmailSuffixWhitelistDraft.value = ''
}

function handleRegistrationEmailSuffixWhitelistDraftInput() {
  registrationEmailSuffixWhitelistDraft.value = normalizeRegistrationEmailSuffixDomain(
    registrationEmailSuffixWhitelistDraft.value
  )
}

function handleRegistrationEmailSuffixWhitelistDraftKeydown(event: KeyboardEvent) {
  if (event.isComposing) {
    return
  }

  if (registrationEmailSuffixWhitelistSeparatorKeys.has(event.key)) {
    event.preventDefault()
    commitRegistrationEmailSuffixWhitelistDraft()
    return
  }

  if (
    event.key === 'Backspace' &&
    !registrationEmailSuffixWhitelistDraft.value &&
    registrationEmailSuffixWhitelistTags.value.length > 0
  ) {
    registrationEmailSuffixWhitelistTags.value.pop()
  }
}

function handleRegistrationEmailSuffixWhitelistPaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text') || ''
  if (!text.trim()) {
    return
  }
  event.preventDefault()
  const tokens = parseRegistrationEmailSuffixWhitelistInput(text)
  for (const token of tokens) {
    addRegistrationEmailSuffixWhitelistTag(token)
  }
}

// Quota notify email helpers
const addQuotaNotifyEmail = () => {
  if (!form.account_quota_notify_emails) {
    form.account_quota_notify_emails = []
  }
  form.account_quota_notify_emails.push('')
}

// LinuxDo OAuth redirect URL suggestion
const linuxdoRedirectUrlSuggestion = computed(() => {
  if (typeof window === 'undefined') return ''
  const origin =
    window.location.origin || `${window.location.protocol}//${window.location.host}`
  return `${origin}/api/v1/auth/oauth/linuxdo/callback`
})

async function setAndCopyLinuxdoRedirectUrl() {
  const url = linuxdoRedirectUrlSuggestion.value
  if (!url) return

  form.linuxdo_connect_redirect_url = url
  await copyToClipboard(url, t('admin.settings.linuxdo.redirectUrlSetAndCopied'))
}

const oidcRedirectUrlSuggestion = computed(() => {
  if (typeof window === 'undefined') return ''
  const origin =
    window.location.origin || `${window.location.protocol}//${window.location.host}`
  return `${origin}/api/v1/auth/oauth/oidc/callback`
})

async function setAndCopyOIDCRedirectUrl() {
  const url = oidcRedirectUrlSuggestion.value
  if (!url) return

  form.oidc_connect_redirect_url = url
  await copyToClipboard(url, t('admin.settings.oidc.redirectUrlSetAndCopied'))
}

// Custom menu item management
function addMenuItem() {
  form.custom_menu_items.push({
    id: '',
    label: '',
    icon_svg: '',
    url: '',
    visibility: 'user',
    sort_order: form.custom_menu_items.length,
  })
}

function removeMenuItem(index: number) {
  form.custom_menu_items.splice(index, 1)
  // Re-index sort_order
  form.custom_menu_items.forEach((item, i) => {
    item.sort_order = i
  })
}

function moveMenuItem(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= form.custom_menu_items.length) return
  const items = form.custom_menu_items
  const temp = items[index]
  items[index] = items[targetIndex]
  items[targetIndex] = temp
  // Re-index sort_order
  items.forEach((item, i) => {
    item.sort_order = i
  })
}

// Custom endpoint management
function addEndpoint() {
  form.custom_endpoints.push({ name: '', endpoint: '', description: '' })
}

function removeEndpoint(index: number) {
  form.custom_endpoints.splice(index, 1)
}

function formatTablePageSizeOptions(options: number[]): string {
  return options.join(', ')
}

function parseTablePageSizeOptionsInput(raw: string): number[] | null {
  const tokens = raw
    .split(',')
    .map((token) => token.trim())
    .filter((token) => token.length > 0)

  if (tokens.length === 0) {
    return null
  }

  const parsed = tokens.map((token) => Number(token))
  if (parsed.some((value) => !Number.isInteger(value))) {
    return null
  }

  const deduped = Array.from(new Set(parsed)).sort((a, b) => a - b)
  if (
    deduped.some((value) => value < tablePageSizeMin || value > tablePageSizeMax)
  ) {
    return null
  }

  return deduped
}

async function loadSettings() {
  loading.value = true
  loadFailed.value = false
  try {
    const settings = await adminAPI.settings.getSettings()
    settings.payment_load_balance_strategy = settings.payment_load_balance_strategy || 'round-robin'
    // Only assign non-null values from backend (null means unconfigured, keep defaults)
    for (const [key, value] of Object.entries(settings)) {
      if (value !== null && value !== undefined) {
        (form as Record<string, unknown>)[key] = value
      }
    }
    form.backend_mode_enabled = settings.backend_mode_enabled
    form.default_subscriptions = Array.isArray(settings.default_subscriptions)
      ? settings.default_subscriptions
          .filter((item) => item.group_id > 0 && item.validity_days > 0)
          .map((item) => ({
            group_id: item.group_id,
            validity_days: item.validity_days
          }))
      : []
    registrationEmailSuffixWhitelistTags.value = normalizeRegistrationEmailSuffixDomains(
      settings.registration_email_suffix_whitelist
    )
    tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
      Array.isArray(settings.table_page_size_options) ? settings.table_page_size_options : [10, 20, 50, 100]
    )
    registrationEmailSuffixWhitelistDraft.value = ''
    form.smtp_password = ''
    smtpPasswordManuallyEdited.value = false
    form.turnstile_secret_key = ''
    form.linuxdo_connect_client_secret = ''
    form.oidc_connect_client_secret = ''

    // Load web search emulation config separately
    await loadWebSearchConfig()
  } catch (error: unknown) {
    loadFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.failedToLoad')))
  } finally {
    loading.value = false
  }
}

async function loadSubscriptionGroups() {
  try {
    const groups = await adminAPI.groups.getAll()
    subscriptionGroups.value = groups.filter(
      (group) => group.subscription_type === 'subscription' && group.status === 'active'
    )
  } catch (_error: unknown) {
    subscriptionGroups.value = []
  }
}

function addDefaultSubscription() {
  if (subscriptionGroups.value.length === 0) return
  const existing = new Set(form.default_subscriptions.map((item) => item.group_id))
  const candidate = subscriptionGroups.value.find((group) => !existing.has(group.id))
  if (!candidate) return
  form.default_subscriptions.push({
    group_id: candidate.id,
    validity_days: 30
  })
}

function removeDefaultSubscription(index: number) {
  form.default_subscriptions.splice(index, 1)
}

async function saveSettings() {
  saving.value = true
  try {
    const normalizedTableDefaultPageSize = Math.floor(Number(form.table_default_page_size))
    if (
      !Number.isInteger(normalizedTableDefaultPageSize) ||
      normalizedTableDefaultPageSize < tablePageSizeMin ||
      normalizedTableDefaultPageSize > tablePageSizeMax
    ) {
      appStore.showError(
        t('admin.settings.site.tableDefaultPageSizeRangeError', {
          min: tablePageSizeMin,
          max: tablePageSizeMax
        })
      )
      return
    }

    const normalizedTablePageSizeOptions = parseTablePageSizeOptionsInput(
      tablePageSizeOptionsInput.value
    )
    if (!normalizedTablePageSizeOptions) {
      appStore.showError(
        t('admin.settings.site.tablePageSizeOptionsFormatError', {
          min: tablePageSizeMin,
          max: tablePageSizeMax
        })
      )
      return
    }

    form.table_default_page_size = normalizedTableDefaultPageSize
    form.table_page_size_options = normalizedTablePageSizeOptions

    const normalizedDefaultSubscriptions = form.default_subscriptions
      .filter((item) => item.group_id > 0 && item.validity_days > 0)
      .map((item: DefaultSubscriptionSetting) => ({
        group_id: item.group_id,
        validity_days: Math.min(36500, Math.max(1, Math.floor(item.validity_days)))
      }))

    const seenGroupIDs = new Set<number>()
    const duplicateDefaultSubscription = normalizedDefaultSubscriptions.find((item) => {
      if (seenGroupIDs.has(item.group_id)) {
        return true
      }
      seenGroupIDs.add(item.group_id)
      return false
    })
    if (duplicateDefaultSubscription) {
      appStore.showError(
        t('admin.settings.defaults.defaultSubscriptionsDuplicate', {
          groupId: duplicateDefaultSubscription.group_id
        })
      )
      return
    }

    // Validate URL fields — novalidate disables browser-native checks, so we validate here
    const isValidHttpUrl = (url: string): boolean => {
      if (!url) return true
      try {
        const u = new URL(url)
        return u.protocol === 'http:' || u.protocol === 'https:'
      } catch {
        return false
      }
    }
    // Optional URL fields: auto-clear invalid values so they don't cause backend 400 errors
    if (!isValidHttpUrl(form.frontend_url)) form.frontend_url = ''
    if (!isValidHttpUrl(form.doc_url)) form.doc_url = ''

    const payload: UpdateSettingsRequest = {
      registration_enabled: form.registration_enabled,
      email_verify_enabled: form.email_verify_enabled,
      registration_email_suffix_whitelist: registrationEmailSuffixWhitelistTags.value.map(
        (suffix) => `@${suffix}`
      ),
      promo_code_enabled: form.promo_code_enabled,
      invitation_code_enabled: form.invitation_code_enabled,
      password_reset_enabled: form.password_reset_enabled,
      totp_enabled: form.totp_enabled,
      default_balance: form.default_balance,
      default_concurrency: form.default_concurrency,
      default_subscriptions: normalizedDefaultSubscriptions,
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
      linuxdo_connect_client_secret: form.linuxdo_connect_client_secret || undefined,
      linuxdo_connect_redirect_url: form.linuxdo_connect_redirect_url,
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
      oidc_connect_frontend_redirect_url: form.oidc_connect_frontend_redirect_url,
      oidc_connect_token_auth_method: form.oidc_connect_token_auth_method,
      oidc_connect_use_pkce: form.oidc_connect_use_pkce,
      oidc_connect_validate_id_token: form.oidc_connect_validate_id_token,
      oidc_connect_allowed_signing_algs: form.oidc_connect_allowed_signing_algs,
      oidc_connect_clock_skew_seconds: form.oidc_connect_clock_skew_seconds,
      oidc_connect_require_email_verified: form.oidc_connect_require_email_verified,
      oidc_connect_userinfo_email_path: form.oidc_connect_userinfo_email_path,
      oidc_connect_userinfo_id_path: form.oidc_connect_userinfo_id_path,
      oidc_connect_userinfo_username_path: form.oidc_connect_userinfo_username_path,
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
      payment_order_timeout_minutes: Number(form.payment_order_timeout_minutes) || 0,
      payment_balance_disabled: form.payment_balance_disabled,
      payment_enabled_types: form.payment_enabled_types,
      payment_load_balance_strategy: form.payment_load_balance_strategy,
      payment_product_name_prefix: form.payment_product_name_prefix,
      payment_product_name_suffix: form.payment_product_name_suffix,
      payment_help_image_url: form.payment_help_image_url,
      payment_help_text: form.payment_help_text,
      payment_cancel_rate_limit_enabled: form.payment_cancel_rate_limit_enabled,
      payment_cancel_rate_limit_max: Number(form.payment_cancel_rate_limit_max) || 10,
      payment_cancel_rate_limit_window: Number(form.payment_cancel_rate_limit_window) || 1,
      payment_cancel_rate_limit_unit: form.payment_cancel_rate_limit_unit,
      payment_cancel_rate_limit_window_mode: form.payment_cancel_rate_limit_window_mode,
      // Balance & quota notification
      balance_low_notify_enabled: form.balance_low_notify_enabled,
      balance_low_notify_threshold_type: form.balance_low_notify_threshold_type,
      balance_low_notify_threshold: Number(form.balance_low_notify_threshold) || 0,
      account_quota_notify_emails: (form.account_quota_notify_emails || []).filter((e: string) => e.trim() !== ''),
    }

    const updated = await adminAPI.settings.updateSettings(payload)
    for (const [key, value] of Object.entries(updated)) {
      if (value !== null && value !== undefined) {
        (form as Record<string, unknown>)[key] = value
      }
    }
    registrationEmailSuffixWhitelistTags.value = normalizeRegistrationEmailSuffixDomains(
      updated.registration_email_suffix_whitelist
    )
    tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
      Array.isArray(updated.table_page_size_options) ? updated.table_page_size_options : [10, 20, 50, 100]
    )
    registrationEmailSuffixWhitelistDraft.value = ''
    form.smtp_password = ''
    smtpPasswordManuallyEdited.value = false
    form.turnstile_secret_key = ''
    form.linuxdo_connect_client_secret = ''
    form.oidc_connect_client_secret = ''
    // Save web search emulation config separately (errors handled internally)
    const wsOk = await saveWebSearchConfig()
    // Refresh cached settings so sidebar/header update immediately
    await appStore.fetchPublicSettings(true)
    await adminSettingsStore.fetch(true)
    if (wsOk) {
      appStore.showSuccess(t('admin.settings.settingsSaved'))
    }
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.failedToSave')))
  } finally {
    saving.value = false
  }
}

async function testSmtpConnection() {
  testingSmtp.value = true
  try {
    const smtpPasswordForTest = smtpPasswordManuallyEdited.value ? form.smtp_password : ''
    const result = await adminAPI.settings.testSmtpConnection({
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: smtpPasswordForTest,
      smtp_use_tls: form.smtp_use_tls
    })
    // API returns { message: "..." } on success, errors are thrown as exceptions
    appStore.showSuccess(result.message || t('admin.settings.smtpConnectionSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.failedToTestSmtp')))
  } finally {
    testingSmtp.value = false
  }
}

async function sendTestEmail() {
  if (!testEmailAddress.value) {
    appStore.showError(t('admin.settings.testEmail.enterRecipientHint'))
    return
  }

  sendingTestEmail.value = true
  try {
    const smtpPasswordForSend = smtpPasswordManuallyEdited.value ? form.smtp_password : ''
    const result = await adminAPI.settings.sendTestEmail({
      email: testEmailAddress.value,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: smtpPasswordForSend,
      smtp_from_email: form.smtp_from_email,
      smtp_from_name: form.smtp_from_name,
      smtp_use_tls: form.smtp_use_tls
    })
    // API returns { message: "..." } on success, errors are thrown as exceptions
    appStore.showSuccess(result.message || t('admin.settings.testEmailSent'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.failedToSendTestEmail')))
  } finally {
    sendingTestEmail.value = false
  }
}

// Admin API Key 方法
async function loadAdminApiKey() {
  adminApiKeyLoading.value = true
  try {
    const status = await adminAPI.settings.getAdminApiKey()
    adminApiKeyExists.value = status.exists
    adminApiKeyMasked.value = status.masked_key
  } catch (_error: unknown) {
    // Silent fail - admin API key status is non-critical
  } finally {
    adminApiKeyLoading.value = false
  }
}

async function createAdminApiKey() {
  adminApiKeyOperating.value = true
  try {
    const result = await adminAPI.settings.regenerateAdminApiKey()
    newAdminApiKey.value = result.key
    adminApiKeyExists.value = true
    adminApiKeyMasked.value = result.key.substring(0, 10) + '...' + result.key.slice(-4)
    appStore.showSuccess(t('admin.settings.adminApiKey.keyGenerated'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    adminApiKeyOperating.value = false
  }
}

async function regenerateAdminApiKey() {
  if (!confirm(t('admin.settings.adminApiKey.regenerateConfirm'))) return
  await createAdminApiKey()
}

async function deleteAdminApiKey() {
  if (!confirm(t('admin.settings.adminApiKey.deleteConfirm'))) return
  adminApiKeyOperating.value = true
  try {
    await adminAPI.settings.deleteAdminApiKey()
    adminApiKeyExists.value = false
    adminApiKeyMasked.value = ''
    newAdminApiKey.value = ''
    appStore.showSuccess(t('admin.settings.adminApiKey.keyDeleted'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    adminApiKeyOperating.value = false
  }
}

function copyNewKey() {
  navigator.clipboard
    .writeText(newAdminApiKey.value)
    .then(() => {
      appStore.showSuccess(t('admin.settings.adminApiKey.keyCopied'))
    })
    .catch(() => {
      appStore.showError(t('common.copyFailed'))
    })
}

// Overload Cooldown 方法
async function loadOverloadCooldownSettings() {
  overloadCooldownLoading.value = true
  try {
    const settings = await adminAPI.settings.getOverloadCooldownSettings()
    Object.assign(overloadCooldownForm, settings)
  } catch (_error: unknown) {
    // Silent fail - settings will use defaults
  } finally {
    overloadCooldownLoading.value = false
  }
}

async function saveOverloadCooldownSettings() {
  overloadCooldownSaving.value = true
  try {
    const updated = await adminAPI.settings.updateOverloadCooldownSettings({
      enabled: overloadCooldownForm.enabled,
      cooldown_minutes: overloadCooldownForm.cooldown_minutes
    })
    Object.assign(overloadCooldownForm, updated)
    appStore.showSuccess(t('admin.settings.overloadCooldown.saved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.overloadCooldown.saveFailed')))
  } finally {
    overloadCooldownSaving.value = false
  }
}

// Stream Timeout 方法
async function loadStreamTimeoutSettings() {
  streamTimeoutLoading.value = true
  try {
    const settings = await adminAPI.settings.getStreamTimeoutSettings()
    Object.assign(streamTimeoutForm, settings)
  } catch (_error: unknown) {
    // Silent fail - settings will use defaults
  } finally {
    streamTimeoutLoading.value = false
  }
}

async function saveStreamTimeoutSettings() {
  streamTimeoutSaving.value = true
  try {
    const updated = await adminAPI.settings.updateStreamTimeoutSettings({
      enabled: streamTimeoutForm.enabled,
      action: streamTimeoutForm.action,
      temp_unsched_minutes: streamTimeoutForm.temp_unsched_minutes,
      threshold_count: streamTimeoutForm.threshold_count,
      threshold_window_minutes: streamTimeoutForm.threshold_window_minutes
    })
    Object.assign(streamTimeoutForm, updated)
    appStore.showSuccess(t('admin.settings.streamTimeout.saved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.streamTimeout.saveFailed')))
  } finally {
    streamTimeoutSaving.value = false
  }
}

// Rectifier 方法
async function loadRectifierSettings() {
  rectifierLoading.value = true
  try {
    const settings = await adminAPI.settings.getRectifierSettings()
    Object.assign(rectifierForm, settings)
    // 确保 patterns 是数组（旧数据可能为 null）
    if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
      rectifierForm.apikey_signature_patterns = []
    }
  } catch (_error: unknown) {
    // Silent fail - settings will use defaults
  } finally {
    rectifierLoading.value = false
  }
}

async function saveRectifierSettings() {
  rectifierSaving.value = true
  try {
    const updated = await adminAPI.settings.updateRectifierSettings({
      enabled: rectifierForm.enabled,
      thinking_signature_enabled: rectifierForm.thinking_signature_enabled,
      thinking_budget_enabled: rectifierForm.thinking_budget_enabled,
      apikey_signature_enabled: rectifierForm.apikey_signature_enabled,
      apikey_signature_patterns: rectifierForm.apikey_signature_patterns.filter(
        (p) => p.trim() !== ''
      )
    })
    Object.assign(rectifierForm, updated)
    if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
      rectifierForm.apikey_signature_patterns = []
    }
    appStore.showSuccess(t('admin.settings.rectifier.saved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.rectifier.saveFailed')))
  } finally {
    rectifierSaving.value = false
  }
}

const betaPolicyActionOptions = computed(() => [
  { value: 'pass', label: t('admin.settings.betaPolicy.actionPass') },
  { value: 'filter', label: t('admin.settings.betaPolicy.actionFilter') },
  { value: 'block', label: t('admin.settings.betaPolicy.actionBlock') }
])

const betaPolicyScopeOptions = computed(() => [
  { value: 'all', label: t('admin.settings.betaPolicy.scopeAll') },
  { value: 'oauth', label: t('admin.settings.betaPolicy.scopeOAuth') },
  { value: 'apikey', label: t('admin.settings.betaPolicy.scopeAPIKey') },
  { value: 'bedrock', label: t('admin.settings.betaPolicy.scopeBedrock') }
])

// Beta Policy 方法
const betaDisplayNames: Record<string, string> = {
  'fast-mode-2026-02-01': 'Fast Mode',
  'context-1m-2025-08-07': 'Context 1M'
}

// 快捷预设：按 beta_token 定义预设方案
const betaPresets: Record<string, Array<{
  label: string
  description: string
  action: 'pass' | 'filter' | 'block'
  model_whitelist: string[]
  fallback_action: 'pass' | 'filter' | 'block'
}>> = {
  'context-1m-2025-08-07': [
    {
      label: t('admin.settings.betaPolicy.presetOpusOnly'),
      description: t('admin.settings.betaPolicy.presetOpusOnlyDesc'),
      action: 'pass',
      model_whitelist: ['claude-opus-4-6'],
      fallback_action: 'filter',
    },
  ],
}

// 常用模型模式（具体 ID + 通配符示例）
const commonModelPatterns = ['claude-opus-4-6', 'claude-sonnet-4-6', 'claude-opus-*', 'claude-sonnet-*']

function getBetaDisplayName(token: string): string {
  return betaDisplayNames[token] || token
}

function applyBetaPreset(
  rule: (typeof betaPolicyForm.rules)[number],
  preset: { action: 'pass' | 'filter' | 'block'; model_whitelist: string[]; fallback_action: 'pass' | 'filter' | 'block' }
) {
  rule.action = preset.action
  rule.model_whitelist = [...preset.model_whitelist]
  rule.fallback_action = preset.fallback_action
}

function addQuickPattern(rule: (typeof betaPolicyForm.rules)[number], pattern: string) {
  if (!rule.model_whitelist) rule.model_whitelist = []
  if (!rule.model_whitelist.includes(pattern)) {
    rule.model_whitelist.push(pattern)
  }
}

async function loadBetaPolicySettings() {
  betaPolicyLoading.value = true
  try {
    const settings = await adminAPI.settings.getBetaPolicySettings()
    betaPolicyForm.rules = settings.rules
  } catch (_error: unknown) {
    // Silent fail - settings will use defaults
  } finally {
    betaPolicyLoading.value = false
  }
}

async function saveBetaPolicySettings() {
  betaPolicySaving.value = true
  try {
    // Clean up empty patterns before saving
    const cleanedRules = betaPolicyForm.rules.map(rule => {
      const whitelist = rule.model_whitelist?.filter(p => p.trim() !== '')
      const hasWhitelist = whitelist && whitelist.length > 0
      return {
        beta_token: rule.beta_token,
        action: rule.action,
        scope: rule.scope,
        error_message: rule.error_message,
        model_whitelist: hasWhitelist ? whitelist : undefined,
        fallback_action: hasWhitelist ? (rule.fallback_action || 'pass') : undefined,
        fallback_error_message: hasWhitelist && rule.fallback_action === 'block' ? rule.fallback_error_message : undefined,
      }
    })
    const updated = await adminAPI.settings.updateBetaPolicySettings({
      rules: cleanedRules
    })
    betaPolicyForm.rules = updated.rules
    appStore.showSuccess(t('admin.settings.betaPolicy.saved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.betaPolicy.saveFailed')))
  } finally {
    betaPolicySaving.value = false
  }
}

// ==================== Provider Management ====================

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('payment.methods.easypay') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
])

function isPaymentTypeEnabled(type: string): boolean {
  return form.payment_enabled_types.includes(type)
}

const hasAnyPaymentTypeEnabled = computed(() => form.payment_enabled_types.length > 0)

function togglePaymentType(type: string) {
  if (form.payment_enabled_types.includes(type)) {
    form.payment_enabled_types = form.payment_enabled_types.filter(t => t !== type)
    // Disable all provider instances matching this type
    disableProvidersByType(type)
  } else {
    form.payment_enabled_types = [...form.payment_enabled_types, type]
  }
}

async function disableProvidersByType(type: string) {
  const matching = providers.value.filter(p => p.provider_key === type && p.enabled)
  for (const p of matching) {
    try {
      await adminAPI.payment.updateProvider(p.id, { enabled: false })
      p.enabled = false
    } catch (err: unknown) {
      slog('disable provider failed', p.id, err)
    }
  }
}

function slog(...args: unknown[]) { console.warn('[payment]', ...args) }

const providersLoading = ref(false)
const providerSaving = ref(false)
const providers = ref<ProviderInstance[]>([])
const showProviderDialog = ref(false)
const showDeleteProviderDialog = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)
const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
])

const enabledProviderKeyOptions = computed(() => {
  const enabled = form.payment_enabled_types
  return providerKeyOptions.value.filter(opt => enabled.includes(opt.value))
})

const loadBalanceOptions = computed(() => [
  { value: 'round-robin', label: t('admin.settings.payment.strategyRoundRobin') },
  { value: 'least-amount', label: t('admin.settings.payment.strategyLeastAmount') },
])

const cancelRateLimitUnitOptions = computed(() => [
  { value: 'minute', label: t('admin.settings.payment.cancelRateLimitUnitMinute') },
  { value: 'hour', label: t('admin.settings.payment.cancelRateLimitUnitHour') },
  { value: 'day', label: t('admin.settings.payment.cancelRateLimitUnitDay') },
])

const cancelRateLimitModeOptions = computed(() => [
  { value: 'rolling', label: t('admin.settings.payment.cancelRateLimitWindowModeRolling') },
  { value: 'fixed', label: t('admin.settings.payment.cancelRateLimitWindowModeFixed') },
])

const paymentErrorMap = computed(() => ({
  PENDING_ORDERS: t('payment.errors.PENDING_ORDERS'),
}))

async function loadProviders() {
  providersLoading.value = true
  try { const res = await adminAPI.payment.getProviders(); providers.value = res.data || [] }
  catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { providersLoading.value = false }
}

function openCreateProvider() {
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance) {
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

async function handleSaveProvider(payload: Partial<ProviderInstance>) {
  providerSaving.value = true
  try {
    if (editingProvider.value) {
      await adminAPI.payment.updateProvider(editingProvider.value.id, payload)
    } else {
      await adminAPI.payment.createProvider(payload)
    }
    showProviderDialog.value = false
    // Reload full list (API returns decrypted/formatted data with correct sort order)
    await loadProviders()
    // Auto-save settings so provider changes take effect immediately
    await saveSettings()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), paymentErrorMap.value))
  } finally {
    providerSaving.value = false
  }
}

async function handleToggleField(provider: ProviderInstance, field: 'enabled' | 'refund_enabled') {
  const newValue = field === 'enabled' ? !provider.enabled : !provider.refund_enabled
  try {
    await adminAPI.payment.updateProvider(provider.id, { [field]: newValue })
    if (field === 'enabled') provider.enabled = newValue
    else provider.refund_enabled = newValue
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'), paymentErrorMap.value)) }
}

async function handleToggleType(provider: ProviderInstance, type: string) {
  const updated = provider.supported_types.includes(type)
    ? provider.supported_types.filter(t => t !== type)
    : [...provider.supported_types, type]
  try {
    await adminAPI.payment.updateProvider(provider.id, { supported_types: updated } as any)
    provider.supported_types = updated
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'), paymentErrorMap.value)) }
}

function confirmDeleteProvider(provider: ProviderInstance) {
  deletingProviderId.value = provider.id
  showDeleteProviderDialog.value = true
}

async function handleReorderProviders(updates: { id: number; sort_order: number }[]) {
  try {
    await Promise.all(
      updates.map(u => adminAPI.payment.updateProvider(u.id, { sort_order: u.sort_order } as Partial<ProviderInstance>))
    )
    // Update local state to match new order
    for (const u of updates) {
      const p = providers.value.find(p => p.id === u.id)
      if (p) p.sort_order = u.sort_order
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    loadProviders()
  }
}

async function handleDeleteProvider() {
  if (!deletingProviderId.value) return
  try {
    await adminAPI.payment.deleteProvider(deletingProviderId.value)
    appStore.showSuccess(t('common.deleted'))
    showDeleteProviderDialog.value = false
    loadProviders()
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'), paymentErrorMap.value)) }
}

onMounted(() => {
  loadSettings()
  loadSubscriptionGroups()
  loadAdminApiKey()
  loadOverloadCooldownSettings()
  loadStreamTimeoutSettings()
  loadRectifierSettings()
  loadBetaPolicySettings()
  loadProviders()
})
</script>

<style scoped>
.default-sub-group-select :deep(.select-trigger) {
  @apply h-[42px];
}

.default-sub-delete-btn {
  @apply h-[42px];
}

/* ============ Settings Tab Navigation ============ */

/* Scroll container: thin scrollbar on PC, auto-hide on mobile */
.settings-tabs-scroll {
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}
.settings-tabs-scroll:hover {
  scrollbar-color: rgb(0 0 0 / 0.15) transparent;
}
:root.dark .settings-tabs-scroll:hover {
  scrollbar-color: rgb(255 255 255 / 0.2) transparent;
}
.settings-tabs-scroll::-webkit-scrollbar {
  height: 3px;
}
.settings-tabs-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.settings-tabs-scroll::-webkit-scrollbar-thumb {
  background: transparent;
  border-radius: 3px;
}
.settings-tabs-scroll:hover::-webkit-scrollbar-thumb {
  background: rgb(0 0 0 / 0.15);
}
:root.dark .settings-tabs-scroll:hover::-webkit-scrollbar-thumb {
  background: rgb(255 255 255 / 0.2);
}

.settings-tabs {
  @apply inline-flex min-w-full gap-0.5 rounded-2xl
         border border-gray-100 bg-white/80 p-1 backdrop-blur-sm
         dark:border-dark-700/50 dark:bg-dark-800/80;
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.04), 0 1px 2px rgb(0 0 0 / 0.02);
}

@media (min-width: 640px) {
  .settings-tabs {
    @apply flex;
  }
}

.settings-tab {
  @apply relative flex flex-1 items-center justify-center gap-1.5
         whitespace-nowrap rounded-xl px-2.5 py-2
         text-sm font-medium
         text-gray-500 dark:text-dark-400
         transition-all duration-200 ease-out;
}

.settings-tab:hover:not(.settings-tab-active) {
  @apply text-gray-700 dark:text-gray-300;
  background: rgb(0 0 0 / 0.03);
}

:root.dark .settings-tab:hover:not(.settings-tab-active) {
  background: rgb(255 255 255 / 0.04);
}

.settings-tab-active {
  @apply text-primary-600 dark:text-primary-400;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.08), rgba(20, 184, 166, 0.03));
  box-shadow: 0 1px 2px rgba(20, 184, 166, 0.1);
}

:root.dark .settings-tab-active {
  background: linear-gradient(135deg, rgba(45, 212, 191, 0.12), rgba(45, 212, 191, 0.05));
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.25);
}

.settings-tab-icon {
  @apply flex h-6 w-6 items-center justify-center rounded-lg
         transition-all duration-200;
}

.settings-tab-active .settings-tab-icon {
  @apply bg-primary-500/15 text-primary-600
         dark:bg-primary-400/15 dark:text-primary-400;
}
</style>
