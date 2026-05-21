<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkEdit.title')"
    width="wide"
    @close="handleClose"
  >
    <form id="bulk-edit-account-form" class="space-y-5" @submit.prevent="() => handleSubmit()">
      <!-- Info -->
      <div class="rounded-lg bg-blue-50 p-4 dark:bg-blue-900/20">
        <p class="text-sm text-blue-700 dark:text-blue-400">
          <svg class="mr-1.5 inline h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          {{ t('admin.accounts.bulkEdit.selectionInfo', { count: targetMode === 'filtered' ? targetPreviewCount : accountIds.length }) }}
        </p>
      </div>

      <!-- Mixed platform warning -->
      <div v-if="isMixedPlatform" class="rounded-lg bg-amber-50 p-4 dark:bg-amber-900/20">
        <p class="text-sm text-amber-700 dark:text-amber-400">
          <svg class="mr-1.5 inline h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          {{ t('admin.accounts.bulkEdit.mixedPlatformWarning', { platforms: targetSelectedPlatforms.join(', ') }) }}
        </p>
      </div>

      <!-- OpenAI passthrough -->
      <div
        v-if="allOpenAIPassthroughCapable"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-openai-passthrough-label"
              class="input-label mb-0"
              for="bulk-edit-openai-passthrough-enabled"
            >
              {{ t('admin.accounts.openai.oauthPassthrough') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
            </p>
          </div>
          <input
            v-model="enableOpenAIPassthrough"
            id="bulk-edit-openai-passthrough-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-passthrough-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-passthrough-body"
          :class="!enableOpenAIPassthrough && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-openai-passthrough-label"
        >
          <button
            id="bulk-edit-openai-passthrough-toggle"
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              openaiPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="openaiPassthroughEnabled = !openaiPassthroughEnabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                openaiPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Base URL (API Key only) -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-base-url-label"
            class="input-label mb-0"
            for="bulk-edit-base-url-enabled"
          >
            {{ t('admin.accounts.baseUrl') }}
          </label>
          <input
            v-model="enableBaseUrl"
            id="bulk-edit-base-url-enabled"
            type="checkbox"
            aria-controls="bulk-edit-base-url"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <input
          v-model="baseUrl"
          id="bulk-edit-base-url"
          type="text"
          :disabled="!enableBaseUrl"
          class="input"
          :class="!enableBaseUrl && 'cursor-not-allowed opacity-50'"
          :placeholder="t('admin.accounts.bulkEdit.baseUrlPlaceholder')"
          aria-labelledby="bulk-edit-base-url-label"
        />
        <p class="input-hint">
          {{ t('admin.accounts.bulkEdit.baseUrlNotice') }}
        </p>
      </div>

      <!-- Model restriction -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-model-restriction-label"
            class="input-label mb-0"
            for="bulk-edit-model-restriction-enabled"
          >
            {{ t('admin.accounts.modelRestriction') }}
          </label>
          <input
            v-model="enableModelRestriction"
            id="bulk-edit-model-restriction-enabled"
            type="checkbox"
            aria-controls="bulk-edit-model-restriction-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div
          id="bulk-edit-model-restriction-body"
          :class="!enableModelRestriction && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-model-restriction-label"
        >
          <div
            v-if="isOpenAIModelRestrictionDisabled"
            class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
          >
            <p class="text-xs text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
            </p>
          </div>

          <template v-else>
            <!-- Mode Toggle -->
            <div class="mb-4 flex gap-2">
              <button
                type="button"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'whitelist'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
                @click="modelRestrictionMode = 'whitelist'"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                {{ t('admin.accounts.modelWhitelist') }}
              </button>
              <button
                type="button"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'mapping'
                    ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
                @click="modelRestrictionMode = 'mapping'"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                  />
                </svg>
                {{ t('admin.accounts.modelMapping') }}
              </button>
            </div>

            <!-- Whitelist Mode -->
            <div v-if="modelRestrictionMode === 'whitelist'">
              <div class="mb-3 rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
                <p class="text-xs text-blue-700 dark:text-blue-400">
                  <svg
                    class="mr-1 inline h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  {{ t('admin.accounts.selectAllowedModels') }}
                </p>
              </div>

              <ModelWhitelistSelector
                v-model="allowedModels"
                :platforms="targetSelectedPlatforms"
              />

              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
                <span v-if="allowedModels.length === 0">{{
                  t('admin.accounts.supportsAllModels')
                }}</span>
              </p>
            </div>

            <!-- Mapping Mode -->
            <div v-else>
              <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
                <p class="text-xs text-purple-700 dark:text-purple-400">
                  <svg
                    class="mr-1 inline h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  {{ t('admin.accounts.mapRequestModels') }}
                </p>
              </div>

              <!-- Model Mapping List -->
              <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
                <div
                  v-for="(mapping, index) in modelMappings"
                  :key="index"
                  class="flex items-center gap-2"
                >
                  <input
                    v-model="mapping.from"
                    type="text"
                    class="input flex-1"
                    :placeholder="t('admin.accounts.requestModel')"
                  />
                  <svg
                    class="h-4 w-4 flex-shrink-0 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M14 5l7 7m0 0l-7 7m7-7H3"
                    />
                  </svg>
                  <input
                    v-model="mapping.to"
                    type="text"
                    class="input flex-1"
                    :placeholder="t('admin.accounts.actualModel')"
                  />
                  <button
                    type="button"
                    class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                    @click="removeModelMapping(index)"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                      />
                    </svg>
                  </button>
                </div>
              </div>

              <button
                type="button"
                class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
                @click="addModelMapping"
              >
                <svg
                  class="mr-1 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                {{ t('admin.accounts.addMapping') }}
              </button>

              <!-- Quick Add Buttons -->
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="preset in filteredPresets"
                  :key="preset.label"
                  type="button"
                  :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
                  @click="addPresetMapping(preset.from, preset.to)"
                >
                  + {{ preset.label }}
                </button>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Custom error codes -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <label
              id="bulk-edit-custom-error-codes-label"
              class="input-label mb-0"
              for="bulk-edit-custom-error-codes-enabled"
            >
              {{ t('admin.accounts.customErrorCodes') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.customErrorCodesHint') }}
            </p>
          </div>
          <input
            v-model="enableCustomErrorCodes"
            id="bulk-edit-custom-error-codes-enabled"
            type="checkbox"
            aria-controls="bulk-edit-custom-error-codes-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div v-if="enableCustomErrorCodes" id="bulk-edit-custom-error-codes-body" class="space-y-3">
          <div class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
            <p class="text-xs text-amber-700 dark:text-amber-400">
              <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.customErrorCodesWarning') }}
            </p>
          </div>

          <!-- Error Code Buttons -->
          <div class="flex flex-wrap gap-2">
            <button
              v-for="code in commonErrorCodes"
              :key="code.value"
              type="button"
              :class="[
                'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                selectedErrorCodes.includes(code.value)
                  ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
              @click="toggleErrorCode(code.value)"
            >
              {{ code.value }} {{ code.label }}
            </button>
          </div>

          <!-- Manual input -->
          <div class="flex items-center gap-2">
            <input
              v-model="customErrorCodeInput"
              id="bulk-edit-custom-error-code-input"
              type="number"
              min="100"
              max="599"
              class="input flex-1"
              :placeholder="t('admin.accounts.enterErrorCode')"
              aria-labelledby="bulk-edit-custom-error-codes-label"
              @keyup.enter="addCustomErrorCode"
            />
            <button type="button" class="btn btn-secondary px-3" @click="addCustomErrorCode">
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
              </svg>
            </button>
          </div>

          <!-- Selected codes summary -->
          <div class="flex flex-wrap gap-1.5">
            <span
              v-for="code in selectedErrorCodes.sort((a, b) => a - b)"
              :key="code"
              class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
            >
              {{ code }}
              <button
                type="button"
                class="hover:text-red-900 dark:hover:text-red-300"
                @click="removeErrorCode(code)"
              >
                <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
              </button>
            </span>
            <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
              {{ t('admin.accounts.noneSelectedUsesDefault') }}
            </span>
          </div>
        </div>
      </div>

      <!-- Intercept warmup requests (Anthropic only) -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-intercept-warmup-label"
              class="input-label mb-0"
              for="bulk-edit-intercept-warmup-enabled"
            >
              {{ t('admin.accounts.interceptWarmupRequests') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
            </p>
          </div>
          <input
            v-model="enableInterceptWarmup"
            id="bulk-edit-intercept-warmup-enabled"
            type="checkbox"
            aria-controls="bulk-edit-intercept-warmup-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div v-if="enableInterceptWarmup" id="bulk-edit-intercept-warmup-body" class="mt-3">
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Proxy -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-proxy-label"
            class="input-label mb-0"
            for="bulk-edit-proxy-enabled"
          >
            {{ t('admin.accounts.proxy') }}
          </label>
          <input
            v-model="enableProxy"
            id="bulk-edit-proxy-enabled"
            type="checkbox"
            aria-controls="bulk-edit-proxy-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div id="bulk-edit-proxy-body" :class="!enableProxy && 'pointer-events-none opacity-50'">
          <ProxySelector
            v-model="proxyId"
            :proxies="proxies"
            aria-labelledby="bulk-edit-proxy-label"
          />
        </div>
      </div>

      <!-- Concurrency & Priority -->
      <div class="grid grid-cols-2 gap-4 border-t border-gray-200 pt-4 dark:border-dark-600 lg:grid-cols-4">
        <div>
          <div class="mb-3 flex items-center justify-between">
            <label
              id="bulk-edit-concurrency-label"
              class="input-label mb-0"
              for="bulk-edit-concurrency-enabled"
            >
              {{ t('admin.accounts.concurrency') }}
            </label>
            <input
              v-model="enableConcurrency"
              id="bulk-edit-concurrency-enabled"
              type="checkbox"
              aria-controls="bulk-edit-concurrency"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="concurrency"
            id="bulk-edit-concurrency"
            type="number"
            min="1"
            :disabled="!enableConcurrency"
            class="input"
            :class="!enableConcurrency && 'cursor-not-allowed opacity-50'"
            aria-labelledby="bulk-edit-concurrency-label"
            @input="concurrency = Math.max(1, concurrency || 1)"
          />
        </div>
        <div>
          <div class="mb-3 flex items-center justify-between">
            <label
              id="bulk-edit-load-factor-label"
              class="input-label mb-0"
              for="bulk-edit-load-factor-enabled"
            >
              {{ t('admin.accounts.loadFactor') }}
            </label>
            <input
              v-model="enableLoadFactor"
              id="bulk-edit-load-factor-enabled"
              type="checkbox"
              aria-controls="bulk-edit-load-factor"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="loadFactor"
            id="bulk-edit-load-factor"
            type="number"
            min="1"
            :disabled="!enableLoadFactor"
            class="input"
            :class="!enableLoadFactor && 'cursor-not-allowed opacity-50'"
            aria-labelledby="bulk-edit-load-factor-label"
            @input="loadFactor = (loadFactor &amp;&amp; loadFactor >= 1) ? loadFactor : null"
          />
          <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
        </div>
        <div>
          <div class="mb-3 flex items-center justify-between">
            <label
              id="bulk-edit-priority-label"
              class="input-label mb-0"
              for="bulk-edit-priority-enabled"
            >
              {{ t('admin.accounts.priority') }}
            </label>
            <input
              v-model="enablePriority"
              id="bulk-edit-priority-enabled"
              type="checkbox"
              aria-controls="bulk-edit-priority"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="priority"
            id="bulk-edit-priority"
            type="number"
            min="1"
            :disabled="!enablePriority"
            class="input"
            :class="!enablePriority && 'cursor-not-allowed opacity-50'"
            aria-labelledby="bulk-edit-priority-label"
          />
        </div>
        <div>
          <div class="mb-3 flex items-center justify-between">
            <label
              id="bulk-edit-rate-multiplier-label"
              class="input-label mb-0"
              for="bulk-edit-rate-multiplier-enabled"
            >
              {{ t('admin.accounts.billingRateMultiplier') }}
            </label>
            <input
              v-model="enableRateMultiplier"
              id="bulk-edit-rate-multiplier-enabled"
              type="checkbox"
              aria-controls="bulk-edit-rate-multiplier"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
          </div>
          <input
            v-model.number="rateMultiplier"
            id="bulk-edit-rate-multiplier"
            type="number"
            min="0"
            step="0.01"
            :disabled="!enableRateMultiplier"
            class="input"
            :class="!enableRateMultiplier && 'cursor-not-allowed opacity-50'"
            aria-labelledby="bulk-edit-rate-multiplier-label"
          />
          <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
        </div>
      </div>

      <!-- Status -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-status-label"
            class="input-label mb-0"
            for="bulk-edit-status-enabled"
          >
            {{ t('common.status') }}
          </label>
          <input
            v-model="enableStatus"
            id="bulk-edit-status-enabled"
            type="checkbox"
            aria-controls="bulk-edit-status"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div id="bulk-edit-status" :class="!enableStatus && 'pointer-events-none opacity-50'">
          <Select
            v-model="status"
            :options="statusOptions"
            aria-labelledby="bulk-edit-status-label"
          />
        </div>
      </div>

      <!-- OpenAI OAuth WS mode -->
      <div v-if="allOpenAIOAuth" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-openai-ws-mode-label"
            class="input-label mb-0"
            for="bulk-edit-openai-ws-mode-enabled"
          >
            {{ t('admin.accounts.openai.wsMode') }}
          </label>
          <input
            v-model="enableOpenAIWSMode"
            id="bulk-edit-openai-ws-mode-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-ws-mode"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-ws-mode"
          :class="!enableOpenAIWSMode && 'pointer-events-none opacity-50'"
        >
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.wsModeDesc') }}
          </p>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t(openAIWSModeConcurrencyHintKey) }}
          </p>
          <Select
            v-model="openaiOAuthResponsesWebSocketV2Mode"
            data-testid="bulk-edit-openai-ws-mode-select"
            :options="openAIWSModeOptions"
            aria-labelledby="bulk-edit-openai-ws-mode-label"
          />
        </div>
      </div>

      <!-- OpenAI OAuth Codex CLI only -->
      <div v-if="allOpenAIOAuth" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-openai-codex-cli-only-label"
            class="input-label mb-0"
            for="bulk-edit-openai-codex-cli-only-enabled"
          >
            {{ t('admin.accounts.openai.codexCLIOnly') }}
          </label>
          <input
            v-model="enableCodexCLIOnly"
            id="bulk-edit-openai-codex-cli-only-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-codex-cli-only"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-codex-cli-only"
          :class="!enableCodexCLIOnly && 'pointer-events-none opacity-50'"
        >
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.codexCLIOnlyDesc') }}
          </p>
          <button
            id="bulk-edit-openai-codex-cli-only-toggle"
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="codexCLIOnlyEnabled = !codexCLIOnlyEnabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- OpenAI API Key WS mode -->
      <div v-if="allOpenAIAPIKey" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-openai-apikey-ws-mode-label"
            class="input-label mb-0"
            for="bulk-edit-openai-apikey-ws-mode-enabled"
          >
            {{ t('admin.accounts.openai.wsMode') }}
          </label>
          <input
            v-model="enableOpenAIAPIKeyWSMode"
            id="bulk-edit-openai-apikey-ws-mode-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-apikey-ws-mode"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-apikey-ws-mode"
          :class="!enableOpenAIAPIKeyWSMode && 'pointer-events-none opacity-50'"
        >
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.wsModeDesc') }}
          </p>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t(openAIAPIKeyWSModeConcurrencyHintKey) }}
          </p>
          <Select
            v-model="openaiAPIKeyResponsesWebSocketV2Mode"
            data-testid="bulk-edit-openai-apikey-ws-mode-select"
            :options="openAIWSModeOptions"
            aria-labelledby="bulk-edit-openai-apikey-ws-mode-label"
          />
        </div>
      </div>

      <!-- OpenAI Compact mode -->
      <div v-if="allOpenAIPassthroughCapable" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-openai-compact-mode-label"
              class="input-label mb-0"
              for="bulk-edit-openai-compact-mode-enabled"
            >
              {{ t('admin.accounts.openai.compactMode') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.compactModeDesc') }}
            </p>
          </div>
          <input
            v-model="enableOpenAICompactMode"
            id="bulk-edit-openai-compact-mode-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-compact-mode"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-compact-mode"
          :class="!enableOpenAICompactMode && 'pointer-events-none opacity-50'"
        >
          <Select
            v-model="openAICompactMode"
            data-testid="bulk-edit-openai-compact-mode-select"
            :options="openAICompactModeOptions"
            aria-labelledby="bulk-edit-openai-compact-mode-label"
          />
        </div>
      </div>

      <!-- OpenAI Compact model mapping -->
      <div v-if="allOpenAIPassthroughCapable" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-openai-compact-model-mapping-label"
              class="input-label mb-0"
              for="bulk-edit-openai-compact-model-mapping-enabled"
            >
              {{ t('admin.accounts.openai.compactModelMapping') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.compactModelMappingDesc') }}
            </p>
          </div>
          <input
            v-model="enableOpenAICompactModelMapping"
            id="bulk-edit-openai-compact-model-mapping-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-compact-model-mapping"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-compact-model-mapping"
          :class="!enableOpenAICompactModelMapping && 'pointer-events-none opacity-50'"
        >
          <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in openAICompactModelMappings"
              :key="index"
              class="flex items-center gap-2"
            >
              <input
                v-model="mapping.from"
                type="text"
                class="input flex-1"
                :placeholder="t('admin.accounts.fromModel')"
                data-testid="bulk-edit-openai-compact-model-mapping-input"
              />
              <span class="text-gray-400">→</span>
              <input
                v-model="mapping.to"
                type="text"
                class="input flex-1"
                :placeholder="t('admin.accounts.toModel')"
                data-testid="bulk-edit-openai-compact-model-mapping-input"
              />
              <button
                type="button"
                class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                @click="removeOpenAICompactModelMapping(index)"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
          <button
            type="button"
            class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
            data-testid="bulk-edit-openai-compact-model-mapping-add"
            @click="addOpenAICompactModelMapping"
          >
            + {{ t('admin.accounts.addMapping') }}
          </button>
        </div>
      </div>

      <!-- RPM Limit (仅全部为 Anthropic OAuth/SetupToken 时显示) -->
      <div v-if="allAnthropicOAuthOrSetupToken" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-rpm-limit-label"
            class="input-label mb-0"
            for="bulk-edit-rpm-limit-enabled"
          >
            {{ t('admin.accounts.quotaControl.rpmLimit.label') }}
          </label>
          <input
            v-model="enableRpmLimit"
            id="bulk-edit-rpm-limit-enabled"
            type="checkbox"
            aria-controls="bulk-edit-rpm-limit-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div
          id="bulk-edit-rpm-limit-body"
          :class="!enableRpmLimit && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-rpm-limit-label"
        >
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.quotaControl.rpmLimit.hint') }}</span>
            <button
              type="button"
              @click="rpmLimitEnabled = !rpmLimitEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                rpmLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  rpmLimitEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="rpmLimitEnabled" class="space-y-3">
            <div>
              <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
              <input
                v-model.number="bulkBaseRpm"
                type="number"
                min="1"
                max="1000"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') }}</p>
            </div>

            <div>
              <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
              <div class="flex gap-2">
                <button
                  type="button"
                  @click="bulkRpmStrategy = 'tiered'"
                  :class="[
                    'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                    bulkRpmStrategy === 'tiered'
                      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                  ]"
                >
                  {{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') }}
                </button>
                <button
                  type="button"
                  @click="bulkRpmStrategy = 'sticky_exempt'"
                  :class="[
                    'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                    bulkRpmStrategy === 'sticky_exempt'
                      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                  ]"
                >
                  {{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') }}
                </button>
              </div>
            </div>

            <div v-if="bulkRpmStrategy === 'tiered'">
              <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
              <input
                v-model.number="bulkRpmStickyBuffer"
                type="number"
                min="1"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
            </div>

            </div>
          </div>

        <!-- 用户消息限速模式（独立于 RPM 开关，始终可见） -->
        <div class="mt-4">
          <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueue') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400 mb-2">
            {{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueueHint') }}
          </p>
          <div class="flex space-x-2">
            <button type="button" v-for="opt in umqModeOptions" :key="opt.value"
              @click="userMsgQueueMode = userMsgQueueMode === opt.value ? null : opt.value"
              :class="[
                'px-3 py-1.5 text-sm rounded-md border transition-colors',
                userMsgQueueMode === opt.value
                  ? 'bg-primary-600 text-white border-primary-600'
                  : 'bg-white dark:bg-dark-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-dark-500 hover:bg-gray-50 dark:hover:bg-dark-600'
              ]">
              {{ opt.label }}
            </button>
          </div>
        </div>
      </div>

      <!-- Groups -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-groups-label"
            class="input-label mb-0"
            for="bulk-edit-groups-enabled"
          >
            {{ t('nav.groups') }}
          </label>
          <input
            v-model="enableGroups"
            id="bulk-edit-groups-enabled"
            type="checkbox"
            aria-controls="bulk-edit-groups"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div id="bulk-edit-groups" :class="!enableGroups && 'pointer-events-none opacity-50'">
          <GroupSelector
            v-model="groupIds"
            :groups="groups"
            aria-labelledby="bulk-edit-groups-label"
          />
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="bulk-edit-account-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          <svg
            v-if="submitting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
            />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          {{
            submitting ? t('admin.accounts.bulkEdit.updating') : t('admin.accounts.bulkEdit.submit')
          }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessage"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Proxy as ProxyConfig, AdminGroup, AccountPlatform, AccountType, OpenAICompactMode } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  buildModelMappingObject as buildModelMappingPayload,
  getPresetMappingsByPlatform
} from '@/composables/useModelWhitelist'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey
} from '@/utils/openaiWsMode'
import type { OpenAIWSMode } from '@/utils/openaiWsMode'
interface Props {
  show: boolean
  accountIds: number[]
  selectedPlatforms: AccountPlatform[]
  selectedTypes: AccountType[]
  target?: {
    mode: 'selected' | 'filtered'
    filters?: Record<string, unknown>
    previewCount?: number
    selectedPlatforms?: AccountPlatform[]
    selectedTypes?: AccountType[]
  }
  proxies: ProxyConfig[]
  groups: AdminGroup[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  updated: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

// Platform awareness
const targetMode = computed(() => props.target?.mode ?? 'selected')
const targetPreviewCount = computed(() => props.target?.previewCount ?? props.accountIds.length)
const targetSelectedPlatforms = computed(() => props.target?.selectedPlatforms ?? props.selectedPlatforms)
const targetSelectedTypes = computed(() => props.target?.selectedTypes ?? props.selectedTypes)
const isMixedPlatform = computed(() => targetSelectedPlatforms.value.length > 1)

const allOpenAIPassthroughCapable = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'openai' &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedTypes.value.every(t => t === 'oauth' || t === 'apikey')
  )
})

const allOpenAIOAuth = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'openai' &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedTypes.value.every(t => t === 'oauth')
  )
})

const allOpenAIAPIKey = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'openai' &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedTypes.value.every(t => t === 'apikey')
  )
})

// 是否全部为 Anthropic OAuth/SetupToken（RPM 配置仅在此条件下显示）
const allAnthropicOAuthOrSetupToken = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'anthropic' &&
    targetSelectedTypes.value.every(t => t === 'oauth' || t === 'setup-token')
  )
})

const filteredPresets = computed(() => {
  if (targetSelectedPlatforms.value.length === 0) return []

  const dedupedPresets = new Map<string, ReturnType<typeof getPresetMappingsByPlatform>[number]>()
  for (const platform of targetSelectedPlatforms.value) {
    for (const preset of getPresetMappingsByPlatform(platform)) {
      const key = `${preset.from}=>${preset.to}`
      if (!dedupedPresets.has(key)) {
        dedupedPresets.set(key, preset)
      }
    }
  }

  return Array.from(dedupedPresets.values())
})

// Model mapping type
interface ModelMapping {
  from: string
  to: string
}

// State - field enable flags
const enableBaseUrl = ref(false)
const enableModelRestriction = ref(false)
const enableCustomErrorCodes = ref(false)
const enableInterceptWarmup = ref(false)
const enableProxy = ref(false)
const enableConcurrency = ref(false)
const enableLoadFactor = ref(false)
const enablePriority = ref(false)
const enableRateMultiplier = ref(false)
const enableStatus = ref(false)
const enableGroups = ref(false)
const enableOpenAIPassthrough = ref(false)
const enableOpenAIWSMode = ref(false)
const enableOpenAIAPIKeyWSMode = ref(false)
const enableCodexCLIOnly = ref(false)
const enableOpenAICompactMode = ref(false)
const enableOpenAICompactModelMapping = ref(false)
const enableRpmLimit = ref(false)

// State - field values
const submitting = ref(false)
const showMixedChannelWarning = ref(false)
const mixedChannelWarningMessage = ref('')
const pendingUpdatesForConfirm = ref<Record<string, unknown> | null>(null)
const baseUrl = ref('')
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const modelMappings = ref<ModelMapping[]>([])
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)
const proxyId = ref<number | null>(null)
const concurrency = ref(1)
const loadFactor = ref<number | null>(null)
const priority = ref(1)
const rateMultiplier = ref(1)
const status = ref<'active' | 'inactive'>('active')
const groupIds = ref<number[]>([])
const openaiPassthroughEnabled = ref(false)
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexCLIOnlyEnabled = ref(false)
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAICompactModelMappings = ref<ModelMapping[]>([])
const rpmLimitEnabled = ref(false)
const bulkBaseRpm = ref<number | null>(null)
const bulkRpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
const bulkRpmStickyBuffer = ref<number | null>(null)
const userMsgQueueMode = ref<string | null>(null)
const umqModeOptions = computed(() => [
  { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') },
  { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') },
  { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') },
])

// Common HTTP error codes
const commonErrorCodes = [
  { value: 401, label: 'Unauthorized' },
  { value: 403, label: 'Forbidden' },
  { value: 429, label: 'Rate Limit' },
  { value: 500, label: 'Server Error' },
  { value: 502, label: 'Bad Gateway' },
  { value: 503, label: 'Unavailable' },
  { value: 529, label: 'Overloaded' }
]

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])
const isOpenAIModelRestrictionDisabled = computed(
  () =>
    allOpenAIPassthroughCapable.value &&
    enableOpenAIPassthrough.value &&
    openaiPassthroughEnabled.value
)

const openAIWSModeOptions = computed(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') }
])
const openAICompactModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiOAuthResponsesWebSocketV2Mode.value)
)
const openAIAPIKeyWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiAPIKeyResponsesWebSocketV2Mode.value)
)

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const addOpenAICompactModelMapping = () => {
  openAICompactModelMappings.value.push({ from: '', to: '' })
}

const removeOpenAICompactModelMapping = (index: number) => {
  openAICompactModelMappings.value.splice(index, 1)
}

const addPresetMapping = (from: string, to: string) => {
  const exists = modelMappings.value.some((m) => m.from === from)
  if (exists) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  modelMappings.value.push({ from, to })
}

// Error code helpers
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    // Adding code - check for 429/529 warning
    if (code === 429) {
      if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
        return
      }
    } else if (code === 529) {
      if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
        return
      }
    }
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const addCustomErrorCode = () => {
  const code = customErrorCodeInput.value
  if (code === null || code < 100 || code > 599) {
    appStore.showError(t('admin.accounts.invalidErrorCode'))
    return
  }
  if (selectedErrorCodes.value.includes(code)) {
    appStore.showInfo(t('admin.accounts.errorCodeExists'))
    return
  }
  // Check for 429/529 warning
  if (code === 429) {
    if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
      return
    }
  } else if (code === 529) {
    if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
      return
    }
  }
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
}

const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const buildModelMappingObject = (): Record<string, string> | null => {
  return buildModelMappingPayload(
    modelRestrictionMode.value,
    allowedModels.value,
    modelMappings.value
  )
}

const buildOpenAICompactModelMapping = (): Record<string, string> | null => {
  return buildModelMappingPayload('mapping', [], openAICompactModelMappings.value)
}

const buildUpdatePayload = (): Record<string, unknown> | null => {
  const updates: Record<string, unknown> = {}
  const credentials: Record<string, unknown> = {}
  let credentialsChanged = false
  const ensureExtra = (): Record<string, unknown> => {
    if (!updates.extra) {
      updates.extra = {}
    }
    return updates.extra as Record<string, unknown>
  }

  if (enableProxy.value) {
    // 后端期望 proxy_id: 0 表示清除代理，而不是 null
    updates.proxy_id = proxyId.value === null ? 0 : proxyId.value
  }

  if (enableConcurrency.value) {
    updates.concurrency = concurrency.value
  }

  if (enableLoadFactor.value) {
    // 空值/NaN/0 时发送 0（后端约定 <= 0 表示清除）
    const lf = loadFactor.value
    updates.load_factor = (lf != null && !Number.isNaN(lf) && lf > 0) ? lf : 0
  }

  if (enablePriority.value) {
    updates.priority = priority.value
  }

  if (enableRateMultiplier.value) {
    updates.rate_multiplier = rateMultiplier.value
  }

  if (enableStatus.value) {
    updates.status = status.value
  }

  if (enableGroups.value) {
    updates.group_ids = groupIds.value
  }

  if (enableBaseUrl.value) {
    const baseUrlValue = baseUrl.value.trim()
    if (baseUrlValue) {
      credentials.base_url = baseUrlValue
      credentialsChanged = true
    }
  }

  if (enableOpenAIPassthrough.value) {
    const extra = ensureExtra()
    extra.openai_passthrough = openaiPassthroughEnabled.value
    if (!openaiPassthroughEnabled.value) {
      extra.openai_oauth_passthrough = false
    }
  }

  if (enableModelRestriction.value && !isOpenAIModelRestrictionDisabled.value) {
    // 统一使用 model_mapping 字段
    if (modelRestrictionMode.value === 'whitelist') {
      // 白名单模式：将模型转换为 model_mapping 格式（key=value）
      // 空白名单表示“支持所有模型”，需显式发送空对象以覆盖已有限制。
      const mapping: Record<string, string> = {}
      for (const m of allowedModels.value) {
        mapping[m] = m
      }
      credentials.model_mapping = mapping
      credentialsChanged = true
    } else {
      // 映射模式下空配置同样表示“支持所有模型”。
      const modelMapping = buildModelMappingObject()
      credentials.model_mapping = modelMapping ?? {}
      credentialsChanged = true
    }
  }

  if (enableCustomErrorCodes.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
    credentialsChanged = true
  }

  if (enableInterceptWarmup.value) {
    credentials.intercept_warmup_requests = interceptWarmupRequests.value
    credentialsChanged = true
  }

  if (enableOpenAIWSMode.value) {
    const extra = ensureExtra()
    extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(
      openaiOAuthResponsesWebSocketV2Mode.value
    )
  }

  if (enableOpenAIAPIKeyWSMode.value) {
    const extra = ensureExtra()
    extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(
      openaiAPIKeyResponsesWebSocketV2Mode.value
    )
  }

  if (enableCodexCLIOnly.value) {
    const extra = ensureExtra()
    extra.codex_cli_only = codexCLIOnlyEnabled.value
  }

  if (enableOpenAICompactMode.value) {
    const extra = ensureExtra()
    extra.openai_compact_mode = openAICompactMode.value
  }

  if (enableOpenAICompactModelMapping.value) {
    credentials.compact_model_mapping = buildOpenAICompactModelMapping() ?? {}
    credentialsChanged = true
  }

  // RPM limit settings (写入 extra 字段)
  if (enableRpmLimit.value) {
    const extra = ensureExtra()
    if (rpmLimitEnabled.value && bulkBaseRpm.value != null && bulkBaseRpm.value > 0) {
      extra.base_rpm = bulkBaseRpm.value
      extra.rpm_strategy = bulkRpmStrategy.value
      if (bulkRpmStickyBuffer.value != null && bulkRpmStickyBuffer.value > 0) {
        extra.rpm_sticky_buffer = bulkRpmStickyBuffer.value
      }
    } else {
      // 关闭 RPM 限制 - 设置 base_rpm 为 0，并用空值覆盖关联字段
      // 后端使用 JSONB || merge 语义，不会删除已有 key，
      // 所以必须显式发送空值来重置（后端读取时会 fallback 到默认值）
      extra.base_rpm = 0
      extra.rpm_strategy = ''
      extra.rpm_sticky_buffer = 0
    }
    updates.extra = extra
  }

  // UMQ mode（独立于 RPM 保存）
  if (userMsgQueueMode.value !== null) {
    const umqExtra = ensureExtra()
    umqExtra.user_msg_queue_mode = userMsgQueueMode.value  // '' = 清除账号级覆盖
    umqExtra.user_msg_queue_enabled = false  // 清理旧字段（JSONB merge）
  }

  if (credentialsChanged) {
    updates.credentials = credentials
  }

  return Object.keys(updates).length > 0 ? updates : null
}

const mixedChannelConfirmed = ref(false)

// 是否需要预检查：改了分组 + 全是单一的 antigravity 或 anthropic 平台
// 多平台混合的情况由 submitBulkUpdate 的 409 catch 兜底
const canPreCheck = () =>
  enableGroups.value &&
  groupIds.value.length > 0 &&
  targetSelectedPlatforms.value.length === 1 &&
  (targetSelectedPlatforms.value[0] === 'antigravity' || targetSelectedPlatforms.value[0] === 'anthropic')

const handleClose = () => {
  showMixedChannelWarning.value = false
  mixedChannelWarningMessage.value = ''
  pendingUpdatesForConfirm.value = null
  mixedChannelConfirmed.value = false
  emit('close')
}

// 预检查：提交前调接口检测，有风险就弹窗阻止，返回 false 表示需要用户确认
const preCheckMixedChannelRisk = async (built: Record<string, unknown>): Promise<boolean> => {
  if (!canPreCheck()) return true
  if (mixedChannelConfirmed.value) return true

  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({
      platform: targetSelectedPlatforms.value[0],
      group_ids: groupIds.value
    })
    if (!result.has_risk) return true

    pendingUpdatesForConfirm.value = built
    mixedChannelWarningMessage.value = result.message || t('admin.accounts.bulkEdit.failed')
    showMixedChannelWarning.value = true
    return false
  } catch (error: any) {
    appStore.showError(error.message || t('admin.accounts.bulkEdit.failed'))
    return false
  }
}

const handleSubmit = async () => {
  if (targetMode.value === 'selected' && props.accountIds.length === 0) {
    appStore.showError(t('admin.accounts.bulkEdit.noSelection'))
    return
  }

  const hasAnyFieldEnabled =
    enableBaseUrl.value ||
    enableOpenAIPassthrough.value ||
    enableModelRestriction.value ||
    enableCustomErrorCodes.value ||
    enableInterceptWarmup.value ||
    enableProxy.value ||
    enableConcurrency.value ||
    enableLoadFactor.value ||
    enablePriority.value ||
    enableRateMultiplier.value ||
    enableStatus.value ||
    enableGroups.value ||
    enableOpenAIWSMode.value ||
    enableOpenAIAPIKeyWSMode.value ||
    enableCodexCLIOnly.value ||
    enableOpenAICompactMode.value ||
    enableOpenAICompactModelMapping.value ||
    enableRpmLimit.value ||
    userMsgQueueMode.value !== null

  if (!hasAnyFieldEnabled) {
    appStore.showError(t('admin.accounts.bulkEdit.noFieldsSelected'))
    return
  }

  const built = buildUpdatePayload()
  if (!built) {
    appStore.showError(t('admin.accounts.bulkEdit.noFieldsSelected'))
    return
  }

  const canContinue = await preCheckMixedChannelRisk(built)
  if (!canContinue) return

  await submitBulkUpdate(built)
}

const submitBulkUpdate = async (baseUpdates: Record<string, unknown>) => {
  // 无论是预检查确认还是 409 兜底确认，只要 mixedChannelConfirmed 为 true 就带上 flag
  const updates = mixedChannelConfirmed.value
    ? { ...baseUpdates, confirm_mixed_channel_risk: true }
    : baseUpdates

  submitting.value = true

  try {
    const res = targetMode.value === 'filtered' && props.target?.filters
      ? await adminAPI.accounts.bulkUpdate({
        filters: props.target.filters,
        ...updates
      })
      : await adminAPI.accounts.bulkUpdate(props.accountIds, updates)
    const success = res.success || 0
    const failed = res.failed || 0

    if (success > 0 && failed === 0) {
      appStore.showSuccess(t('admin.accounts.bulkEdit.success', { count: success }))
    } else if (success > 0) {
      appStore.showError(t('admin.accounts.bulkEdit.partialSuccess', { success, failed }))
    } else {
      appStore.showError(t('admin.accounts.bulkEdit.failed'))
    }

    if (success > 0) {
      pendingUpdatesForConfirm.value = null
      emit('updated')
      handleClose()
    }
  } catch (error: any) {
    // 兜底：多平台混合场景下，预检查跳过，由后端 409 触发确认框
    if (error.status === 409 && error.error === 'mixed_channel_warning') {
      pendingUpdatesForConfirm.value = baseUpdates
      mixedChannelWarningMessage.value = error.message
      showMixedChannelWarning.value = true
    } else {
      appStore.showError(error.message || t('admin.accounts.bulkEdit.failed'))
      console.error('Error bulk updating accounts:', error)
    }
  } finally {
    submitting.value = false
  }
}

const handleMixedChannelConfirm = async () => {
  showMixedChannelWarning.value = false
  mixedChannelConfirmed.value = true
  if (pendingUpdatesForConfirm.value) {
    await submitBulkUpdate(pendingUpdatesForConfirm.value)
  }
}

const handleMixedChannelCancel = () => {
  showMixedChannelWarning.value = false
  pendingUpdatesForConfirm.value = null
}

// Reset form when modal closes
watch(
  () => props.show,
  (newShow) => {
    if (!newShow) {
      // Reset all enable flags
      enableBaseUrl.value = false
      enableModelRestriction.value = false
      enableCustomErrorCodes.value = false
      enableInterceptWarmup.value = false
      enableProxy.value = false
      enableConcurrency.value = false
      enableLoadFactor.value = false
      enablePriority.value = false
      enableRateMultiplier.value = false
      enableStatus.value = false
      enableGroups.value = false
      enableOpenAIPassthrough.value = false
      enableOpenAIWSMode.value = false
      enableOpenAIAPIKeyWSMode.value = false
      enableCodexCLIOnly.value = false
      enableOpenAICompactMode.value = false
      enableOpenAICompactModelMapping.value = false
      enableRpmLimit.value = false

      // Reset all values
      baseUrl.value = ''
      openaiPassthroughEnabled.value = false
      modelRestrictionMode.value = 'whitelist'
      allowedModels.value = []
      modelMappings.value = []
      selectedErrorCodes.value = []
      customErrorCodeInput.value = null
      interceptWarmupRequests.value = false
      proxyId.value = null
      concurrency.value = 1
      loadFactor.value = null
      priority.value = 1
      rateMultiplier.value = 1
      status.value = 'active'
      groupIds.value = []
      openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      codexCLIOnlyEnabled.value = false
      openAICompactMode.value = 'auto'
      openAICompactModelMappings.value = []
      rpmLimitEnabled.value = false
      bulkBaseRpm.value = null
      bulkRpmStrategy.value = 'tiered'
      bulkRpmStickyBuffer.value = null
      userMsgQueueMode.value = null

      // Reset mixed channel warning state
      showMixedChannelWarning.value = false
      mixedChannelWarningMessage.value = ''
      pendingUpdatesForConfirm.value = null
      mixedChannelConfirmed.value = false
    }
  }
)
</script>
