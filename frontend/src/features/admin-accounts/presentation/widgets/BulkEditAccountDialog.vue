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

      <BulkEditRoutingPolicyFields :context="bulkEditRoutingPolicyContext" />
      <BulkEditCPAFields v-if="allTargetsAPIKey" :context="bulkEditCPAContext" />

      <div v-if="allOpenAIPassthroughCapable" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between gap-4">
          <div class="flex-1">
            <label class="input-label mb-0">{{ t('admin.accounts.openai.longContextBilling') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.longContextBillingDesc') }}</p>
          </div>
          <input v-model="enableOpenAILongContextBilling" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </div>
        <label v-if="enableOpenAILongContextBilling" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="openAILongContextBillingEnabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.accounts.openai.longContextBilling') }}
        </label>
      </div>

      <div v-if="allOpenAIAPIKey" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between gap-4">
          <div class="flex-1">
            <label id="bulk-edit-openai-endpoint-capabilities-label" class="input-label mb-0">
              {{ t('admin.accounts.openai.endpointCapabilities') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.endpointCapabilitiesDesc') }}</p>
          </div>
          <input v-model="enableOpenAIEndpointCapabilities" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </div>
        <div v-if="enableOpenAIEndpointCapabilities" class="grid grid-cols-1 gap-2 sm:grid-cols-2" role="group" aria-labelledby="bulk-edit-openai-endpoint-capabilities-label">
          <label v-for="option in openAIEndpointCapabilityOptions" :key="option.value" class="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600">
            <input type="checkbox" :checked="openAIEndpointCapabilities.includes(option.value)" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="toggleOpenAIEndpointCapability(option.value)" />
            <span class="text-gray-700 dark:text-gray-200">{{ option.label }}</span>
          </label>
        </div>
      </div>

      <div v-if="allOpenAIAPIKey" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between gap-4">
          <div class="flex-1">
            <label class="input-label mb-0">{{ t('admin.accounts.openai.responsesMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.responsesModeDesc') }}</p>
          </div>
          <input v-model="enableOpenAIResponsesMode" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </div>
        <Select v-model="openAIResponsesMode" :disabled="!enableOpenAIResponsesMode" :options="openAIResponsesModeOptions" />
      </div>

      <BulkEditOpenAIImageSizeOption v-if="allOpenAIPassthroughCapable" v-model="openAIImageSizeTiersUpdate" />

      <BulkEditExcelBPSOption v-if="allOpenAIOAuthOnly" v-model="excelBpsMode" />

      <BulkEditOpenAICustomRelayOption
        v-if="allOpenAIOAuthOnly"
        v-model:enable-update="enableOpenAICustomRelay"
        v-model:enabled="openAICustomRelayEnabled"
        v-model:url="openAICustomRelayURL"
      />

      <BulkEditTLSFingerprintOption
        v-if="allOpenAIOAuthOnly"
        v-model:enable-update="enableTLSFingerprint"
        v-model:enabled="tlsFingerprintEnabled"
        v-model:profile-id="tlsFingerprintProfileId"
      />

      <BulkEditCapacityFields :context="bulkEditCapacityContext" :proxies="proxies" />

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

      <BulkEditCodexOptions
        v-if="allOpenAIOAuthOnly"
        v-model:enable-fingerprint="enableCodexFingerprintMode"
        v-model:fingerprint-mode="codexFingerprintMode"
        v-model:enable-prewarm="enableCodexPrewarmContinuation"
        v-model:prewarm-enabled="codexPrewarmContinuationEnabled"
      />
      <BulkEditCodexThinkingTagOption v-if="allOpenAIAPIKey"
        v-model:enable-update="enableCodexThinkingTagNormalization"
        v-model:normalization-enabled="codexThinkingTagNormalizationEnabled" />

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

      <!-- OpenAI OAuth: Codex app-server -->
      <div v-if="allOpenAIOAuth" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-openai-codex-app-server-label"
            class="input-label mb-0"
            for="bulk-edit-openai-codex-app-server-enabled"
          >
            {{ t('admin.accounts.openai.codexCLIOnlyAppServer') }}
          </label>
          <input
            v-model="enableCodexCLIOnlyAppServer"
            id="bulk-edit-openai-codex-app-server-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-codex-app-server"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-codex-app-server"
          :class="!enableCodexCLIOnlyAppServer && 'pointer-events-none opacity-50'"
        >
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.codexCLIOnlyAppServerDesc') }}
          </p>
          <button
            id="bulk-edit-openai-codex-app-server-toggle"
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyAppServerEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="codexCLIOnlyAppServerEnabled = !codexCLIOnlyAppServerEnabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyAppServerEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Upstream billing auto probe -->
      <div v-if="allBillingProbeCapable" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-upstream-billing-auto-probe-label"
              class="input-label mb-0"
              for="bulk-edit-upstream-billing-auto-probe-enabled"
            >
              {{ t('admin.accounts.upstreamBilling.autoProbe') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.upstreamBilling.autoProbeHint') }}
            </p>
          </div>
          <input
            v-model="enableUpstreamBillingAutoProbe"
            id="bulk-edit-upstream-billing-auto-probe-enabled"
            type="checkbox"
            aria-controls="bulk-edit-upstream-billing-auto-probe"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-upstream-billing-auto-probe"
          :class="!enableUpstreamBillingAutoProbe && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-upstream-billing-auto-probe-label"
        >
          <Select
            v-model="upstreamBillingAutoProbeMode"
            :disabled="!enableUpstreamBillingAutoProbe"
            data-testid="bulk-edit-upstream-billing-auto-probe-select"
            :options="upstreamBillingAutoProbeOptions"
            aria-labelledby="bulk-edit-upstream-billing-auto-probe-label"
          />
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
import { useAppStore } from '@/core/stores/appStore'
import {
  bulkUpdate,
  checkMixedChannelRisk
} from '@/features/admin-accounts/data/datasources/adminAccountActions'
import type { Proxy as ProxyConfig, AdminGroup, AccountPlatform, AccountType, OpenAICompactMode, OpenAIEndpointCapability, OpenAIResponsesMode } from '@/types'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import Select from '@/common/widgets/forms/Select.vue'
import GroupSelector from '@/common/widgets/data/GroupSelector.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import {
  getPresetMappingsByPlatform
} from '@/features/admin-accounts/presentation/composables/useModelWhitelist'
import {
  isHeaderOverrideCapable,
  validateHeaderOverrideRows,
  type HeaderOverrideRow
} from '@/features/admin-accounts/presentation/credentialsBuilder'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  OPENAI_WS_MODE_HTTP_BRIDGE,
  resolveOpenAIWSModeConcurrencyHintKey
} from '@/core/utils/openaiWsMode'
import type { OpenAIWSMode } from '@/core/utils/openaiWsMode'
import { buildBulkAccountUpdatePayload } from '@/features/admin-accounts/presentation/accountBulkUpdatePayload'
import { type CodexFingerprintMode, type ModelMapping } from '@/features/admin-accounts/presentation/accountFormPolicy'
import type { BulkEditCPAContext, BulkEditCapacityContext, BulkEditRoutingPolicyContext } from '@/features/admin-accounts/presentation/bulkEditAccountContext'
import { areUpstreamBillingProbeTargetsEligible } from '@/features/admin-accounts/presentation/upstreamBillingProbeEligibility'
import BulkEditRoutingPolicyFields from './BulkEditRoutingPolicyFields.vue'
import BulkEditCPAFields from './BulkEditCPAFields.vue'
import BulkEditCapacityFields from './BulkEditCapacityFields.vue'
import BulkEditCodexOptions from './BulkEditCodexOptions.vue'
import BulkEditCodexThinkingTagOption from './BulkEditCodexThinkingTagOption.vue'
import BulkEditExcelBPSOption from './BulkEditExcelBPSOption.vue'
import BulkEditOpenAIImageSizeOption from './BulkEditOpenAIImageSizeOption.vue'
import BulkEditOpenAICustomRelayOption from './BulkEditOpenAICustomRelayOption.vue'
import BulkEditTLSFingerprintOption from './BulkEditTLSFingerprintOption.vue'

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
// Grok 快捷端点仅在所选账号全部为 grok 平台时展示（其他平台不显示）
const allTargetsGrok = computed(
  () =>
    targetSelectedPlatforms.value.length > 0 &&
    targetSelectedPlatforms.value.every((p) => p === 'grok')
)
const isMixedPlatform = computed(() => targetSelectedPlatforms.value.length > 1)

const allOpenAIPassthroughCapable = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'openai' &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedTypes.value.every(t => t === 'oauth' || t === 'setup-token' || t === 'apikey')
  )
})

const allOpenAIOAuth = computed(() => {
  return (
    targetSelectedPlatforms.value.length === 1 &&
    targetSelectedPlatforms.value[0] === 'openai' &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedTypes.value.every(t => t === 'oauth' || t === 'setup-token')
  )
})

const allOpenAIOAuthOnly = computed(() => {
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
const allTargetsAPIKey = computed(() =>
  targetSelectedTypes.value.length > 0 && targetSelectedTypes.value.every(type => type === 'apikey')
)

const allBillingProbeCapable = computed(() =>
  areUpstreamBillingProbeTargetsEligible(
    targetSelectedPlatforms.value,
    targetSelectedTypes.value
  )
)

// 是否全部为 anthropic/openai 平台的 apikey 账号（请求头覆写仅在此条件下显示）
// 所选平台 × 所选类型的全组合均需具备覆写资格（实际选中账号是该组合的子集，
// 按交叉积判定偏保守但绝不放行不合资格的账号）
const allHeaderOverrideCapable = computed(() => {
  return (
    targetSelectedPlatforms.value.length > 0 &&
    targetSelectedTypes.value.length > 0 &&
    targetSelectedPlatforms.value.every(p =>
      targetSelectedTypes.value.every(ty => isHeaderOverrideCapable(p, ty))
    )
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

// State - field enable flags
const enableBaseUrl = ref(false)
const enableModelRestriction = ref(false)
const enableCustomErrorCodes = ref(false)
const enableInterceptWarmup = ref(false)
const enableHeaderOverride = ref(false)
const enableProxy = ref(false)
const enableConcurrency = ref(false)
const enableLoadFactor = ref(false)
const enablePriority = ref(false)
const enableRateMultiplier = ref(false)
const enableStatus = ref(false)
const enableGroups = ref(false)
const enableOpenAIPassthrough = ref(false)
const openAIImageSizeTiersUpdate = ref<string[] | null>(null)
const enableOpenAIFlattenNamespaces = ref(false)
const excelBpsMode = ref<'unchanged' | 'enabled' | 'disabled'>('unchanged')
const enableOpenAICustomRelay = ref(false)
const enableOpenAILongContextBilling = ref(false)
const enableTLSFingerprint = ref(false)
const enableOpenAIEndpointCapabilities = ref(false)
const enableOpenAIResponsesMode = ref(false)
const enableOpenAIWSMode = ref(false)
const enableOpenAIAPIKeyWSMode = ref(false)
const enableCodexFingerprintMode = ref(false)
const enableCodexPrewarmContinuation = ref(false)
const enableCodexThinkingTagNormalization = ref(false)
const enableUpstreamBillingAutoProbe = ref(false)
const enableCodexCLIOnly = ref(false)
const enableCodexCLIOnlyAppServer = ref(false)
const enableOpenAICompactMode = ref(false)
const enableOpenAICompactModelMapping = ref(false)
const enableRpmLimit = ref(false)
const enableCPA = ref(false)

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
const headerOverrideEnabled = ref(false)
const headerOverrideRows = ref<HeaderOverrideRow[]>([])
const proxyId = ref<number | null>(null)
const concurrency = ref(1)
const loadFactor = ref<number | null>(null)
const priority = ref(1)
const rateMultiplier = ref(1)
const status = ref<'active' | 'inactive'>('active')
const groupIds = ref<number[]>([])
const openaiPassthroughEnabled = ref(false)
const openaiFlattenNamespacesEnabled = ref(false)
const openAICustomRelayEnabled = ref(false)
const openAICustomRelayURL = ref('')
const openAILongContextBillingEnabled = ref(false)
const tlsFingerprintEnabled = ref(false)
const tlsFingerprintProfileId = ref<number | null>(-1)
const openAIEndpointCapabilities = ref<OpenAIEndpointCapability[]>(['chat_completions', 'embeddings', 'decisions'])
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexFingerprintMode = ref<CodexFingerprintMode>('off')
const codexPrewarmContinuationEnabled = ref(false)
const codexThinkingTagNormalizationEnabled = ref(false)
const upstreamBillingAutoProbeMode = ref<'enabled' | 'disabled'>('enabled')
const codexCLIOnlyEnabled = ref(false)
const codexCLIOnlyAppServerEnabled = ref(false)
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAICompactModelMappings = ref<ModelMapping[]>([])
const rpmLimitEnabled = ref(false)
const bulkBaseRpm = ref<number | null>(null)
const bulkRpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
const bulkRpmStickyBuffer = ref<number | null>(null)
const userMsgQueueMode = ref<string | null>(null)
const cpaModeEnabled = ref(true)
const cpaUseBaseUrl = ref(true)
const cpaManagementUrl = ref('')
const cpaManagementPassword = ref('')
const cpaConcurrencyPerCredential = ref(10)
const cpaExcludeAbnormalCredentials = ref(false)
const MAX_CPA_CONCURRENCY_PER_CREDENTIAL = 10000
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
const upstreamBillingAutoProbeOptions = computed(() => [
  { value: 'enabled', label: t('common.enabled') },
  { value: 'disabled', label: t('common.disabled') }
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
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') },
  { value: OPENAI_WS_MODE_HTTP_BRIDGE, label: t('admin.accounts.openai.wsModeHttpBridge') }
])
const openAICompactModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
const openAIResponsesModeOptions = computed(() => [
  { value: 'auto' as OpenAIResponsesMode, label: t('admin.accounts.openai.responsesModeAuto') },
  { value: 'force_responses' as OpenAIResponsesMode, label: t('admin.accounts.openai.responsesModeForceResponses') },
  { value: 'force_chat_completions' as OpenAIResponsesMode, label: t('admin.accounts.openai.responsesModeForceChatCompletions') }
])
const openAIEndpointCapabilityOptions = computed(() => [
  { value: 'chat_completions' as OpenAIEndpointCapability, label: t('admin.accounts.openai.capabilityChatCompletions') },
  { value: 'embeddings' as OpenAIEndpointCapability, label: t('admin.accounts.openai.capabilityEmbeddings') },
  { value: 'decisions' as OpenAIEndpointCapability, label: t('admin.accounts.openai.capabilityDecisions') }
])

function toggleOpenAIEndpointCapability(capability: OpenAIEndpointCapability) {
  if (openAIEndpointCapabilities.value.includes(capability)) {
    if (openAIEndpointCapabilities.value.length <= 1) return
    openAIEndpointCapabilities.value = openAIEndpointCapabilities.value.filter(value => value !== capability)
    return
  }
  openAIEndpointCapabilities.value = [...openAIEndpointCapabilities.value, capability]
}
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

const bulkEditRoutingPolicyContext = {
  addCustomErrorCode,
  addModelMapping,
  addPresetMapping,
  allHeaderOverrideCapable,
  allOpenAIOAuthOnly,
  allowedModels,
  allOpenAIPassthroughCapable,
  allTargetsGrok,
  baseUrl,
  commonErrorCodes,
  customErrorCodeInput,
  enableBaseUrl,
  enableCustomErrorCodes,
  enableHeaderOverride,
  enableInterceptWarmup,
  enableModelRestriction,
  enableOpenAIFlattenNamespaces,
  enableOpenAIPassthrough,
  filteredPresets,
  headerOverrideEnabled,
  headerOverrideRows,
  interceptWarmupRequests,
  isOpenAIModelRestrictionDisabled,
  modelMappings,
  modelRestrictionMode,
  openaiFlattenNamespacesEnabled,
  openaiPassthroughEnabled,
  removeErrorCode,
  removeModelMapping,
  selectedErrorCodes,
  t,
  targetSelectedPlatforms,
  toggleErrorCode,
} satisfies BulkEditRoutingPolicyContext

const bulkEditCPAContext = {
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL,
  cpaConcurrencyPerCredential,
  cpaExcludeAbnormalCredentials,
  cpaManagementPassword,
  cpaManagementUrl,
  cpaModeEnabled,
  cpaUseBaseUrl,
  enableCPA,
  t,
} satisfies BulkEditCPAContext

const bulkEditCapacityContext = {
  concurrency,
  enableConcurrency,
  enableLoadFactor,
  enablePriority,
  enableProxy,
  enableRateMultiplier,
  loadFactor,
  priority,
  proxyId,
  rateMultiplier,
  t,
} satisfies BulkEditCapacityContext

const buildUpdatePayload = (): Record<string, unknown> | null => {
  return buildBulkAccountUpdatePayload({
    openAIImageSizeTiersUpdate: allOpenAIPassthroughCapable.value ? openAIImageSizeTiersUpdate.value : null,
    enableProxy: enableProxy.value,
    proxyId: proxyId.value,
    enableConcurrency: enableConcurrency.value,
    concurrency: concurrency.value,
    enableLoadFactor: enableLoadFactor.value,
    loadFactor: loadFactor.value,
    enablePriority: enablePriority.value,
    priority: priority.value,
    enableRateMultiplier: enableRateMultiplier.value,
    rateMultiplier: rateMultiplier.value,
    enableStatus: enableStatus.value,
    status: status.value,
    enableGroups: enableGroups.value,
    groupIds: groupIds.value,
    enableBaseUrl: enableBaseUrl.value,
    baseUrl: baseUrl.value,
    enableOpenAIPassthrough: enableOpenAIPassthrough.value,
    openaiPassthroughEnabled: openaiPassthroughEnabled.value,
    enableOpenAIFlattenNamespaces: enableOpenAIFlattenNamespaces.value,
    excelBpsMode: allOpenAIOAuthOnly.value ? excelBpsMode.value : 'unchanged',
    openaiFlattenNamespacesEligible: allOpenAIOAuthOnly.value,
    openaiFlattenNamespacesEnabled: openaiFlattenNamespacesEnabled.value,
    enableOpenAICustomRelay: enableOpenAICustomRelay.value,
    openAICustomRelayEnabled: openAICustomRelayEnabled.value,
    openAICustomRelayURL: openAICustomRelayURL.value,
    enableOpenAILongContextBilling: enableOpenAILongContextBilling.value,
    openAILongContextBillingEnabled: openAILongContextBillingEnabled.value,
    enableTLSFingerprint: enableTLSFingerprint.value,
    tlsFingerprintEnabled: tlsFingerprintEnabled.value,
    tlsFingerprintProfileId: tlsFingerprintProfileId.value,
    enableOpenAIEndpointCapabilities: enableOpenAIEndpointCapabilities.value,
    openAIEndpointCapabilities: openAIEndpointCapabilities.value,
    enableOpenAIResponsesMode: enableOpenAIResponsesMode.value,
    openAIResponsesMode: openAIResponsesMode.value,
    enableModelRestriction: enableModelRestriction.value,
    isOpenAIModelRestrictionDisabled: isOpenAIModelRestrictionDisabled.value,
    modelRestrictionMode: modelRestrictionMode.value,
    allowedModels: allowedModels.value,
    modelMappings: modelMappings.value,
    enableCustomErrorCodes: enableCustomErrorCodes.value,
    selectedErrorCodes: selectedErrorCodes.value,
    enableInterceptWarmup: enableInterceptWarmup.value,
    interceptWarmupRequests: interceptWarmupRequests.value,
    enableHeaderOverride: enableHeaderOverride.value,
    headerOverrideEnabled: headerOverrideEnabled.value,
    headerOverrideRows: headerOverrideRows.value,
    enableOpenAIWSMode: enableOpenAIWSMode.value,
    openaiOAuthResponsesWebSocketV2Mode: openaiOAuthResponsesWebSocketV2Mode.value,
    enableOpenAIAPIKeyWSMode: enableOpenAIAPIKeyWSMode.value,
    openaiAPIKeyResponsesWebSocketV2Mode: openaiAPIKeyResponsesWebSocketV2Mode.value,
    enableCodexFingerprintMode: enableCodexFingerprintMode.value,
    codexFingerprintMode: codexFingerprintMode.value,
    enableCodexPrewarmContinuation: enableCodexPrewarmContinuation.value,
    codexPrewarmContinuationEnabled: codexPrewarmContinuationEnabled.value,
    enableCodexThinkingTagNormalization: enableCodexThinkingTagNormalization.value,
    codexThinkingTagNormalizationEnabled: codexThinkingTagNormalizationEnabled.value,
    enableUpstreamBillingAutoProbe: enableUpstreamBillingAutoProbe.value,
    upstreamBillingAutoProbeMode: upstreamBillingAutoProbeMode.value,
    enableCodexCLIOnly: enableCodexCLIOnly.value,
    codexCLIOnlyEnabled: codexCLIOnlyEnabled.value,
    enableCodexCLIOnlyAppServer: enableCodexCLIOnlyAppServer.value,
    codexCLIOnlyAppServerEnabled: codexCLIOnlyAppServerEnabled.value,
    enableOpenAICompactMode: enableOpenAICompactMode.value,
    openAICompactMode: openAICompactMode.value,
    enableOpenAICompactModelMapping: enableOpenAICompactModelMapping.value,
    openAICompactModelMappings: openAICompactModelMappings.value,
    enableRpmLimit: enableRpmLimit.value,
    rpmLimitEnabled: rpmLimitEnabled.value,
    bulkBaseRpm: bulkBaseRpm.value,
    bulkRpmStrategy: bulkRpmStrategy.value,
    bulkRpmStickyBuffer: bulkRpmStickyBuffer.value,
    userMsgQueueMode: userMsgQueueMode.value,
    enableCPA: enableCPA.value,
    cpaModeEnabled: cpaModeEnabled.value,
    cpaUseBaseUrl: cpaUseBaseUrl.value,
    cpaManagementUrl: cpaManagementUrl.value,
    cpaManagementPassword: cpaManagementPassword.value,
    cpaConcurrencyPerCredential: cpaConcurrencyPerCredential.value,
    cpaExcludeAbnormalCredentials: cpaExcludeAbnormalCredentials.value,
  })
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
    const result = await checkMixedChannelRisk({
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
    (allOpenAIPassthroughCapable.value && openAIImageSizeTiersUpdate.value !== null) ||
    enableBaseUrl.value ||
    enableOpenAIPassthrough.value ||
    enableOpenAIFlattenNamespaces.value ||
    (allOpenAIOAuthOnly.value && excelBpsMode.value !== 'unchanged') ||
    enableOpenAICustomRelay.value ||
    enableOpenAILongContextBilling.value ||
    enableTLSFingerprint.value ||
    enableOpenAIEndpointCapabilities.value ||
    enableOpenAIResponsesMode.value ||
    enableModelRestriction.value ||
    enableCustomErrorCodes.value ||
    enableInterceptWarmup.value ||
    enableHeaderOverride.value ||
    enableProxy.value ||
    enableConcurrency.value ||
    enableLoadFactor.value ||
    enablePriority.value ||
    enableRateMultiplier.value ||
    enableStatus.value ||
    enableGroups.value ||
    enableOpenAIWSMode.value ||
    enableOpenAIAPIKeyWSMode.value ||
    enableCodexFingerprintMode.value ||
    enableCodexPrewarmContinuation.value ||
    enableCodexThinkingTagNormalization.value ||
    enableUpstreamBillingAutoProbe.value ||
    enableCodexCLIOnly.value ||
    enableCodexCLIOnlyAppServer.value ||
    enableOpenAICompactMode.value ||
    enableOpenAICompactModelMapping.value ||
    enableRpmLimit.value ||
    enableCPA.value ||
    userMsgQueueMode.value !== null

  if (!hasAnyFieldEnabled) {
    appStore.showError(t('admin.accounts.bulkEdit.noFieldsSelected'))
    return
  }

  // base_url 现在也会作用于 Grok OAuth 订阅账号的转发端点；坏值会让请求期
  // 校验失败、账号请求全挂，因此保存前强制格式校验（与单账号编辑一致）。
  if (enableBaseUrl.value) {
    const trimmedBaseUrl = baseUrl.value.trim()
    if (trimmedBaseUrl && !/^https?:\/\//i.test(trimmedBaseUrl)) {
      appStore.showError(t('admin.accounts.grokCustomBaseUrl.invalid'))
      return
    }
  }

  if (enableOpenAICustomRelay.value && openAICustomRelayEnabled.value) {
    const relayURL = openAICustomRelayURL.value.trim()
    if (!/^https?:\/\//i.test(relayURL)) {
      appStore.showError(t('admin.accounts.grokCustomBaseUrl.invalid'))
      return
    }
  }

  if (enableHeaderOverride.value && headerOverrideEnabled.value) {
    // 批量保存对 header_overrides 是整键替换：开启但没有任何有效行会把所选账号的
    // 既有覆写配置静默清空，必须显式拦截（清空请走关闭开关的路径，有专门提示）
    if (!headerOverrideRows.value.some((row) => row.name.trim())) {
      appStore.showError(t('admin.accounts.headerOverride.bulkEmptyRows'))
      return
    }
    const headerError = validateHeaderOverrideRows(headerOverrideRows.value)
    if (headerError) {
      appStore.showError(t(`admin.accounts.headerOverride.${headerError}`))
      return
    }
  }

  if (enableCPA.value && cpaModeEnabled.value) {
    const perCredential = Number(cpaConcurrencyPerCredential.value)
    if (!Number.isInteger(perCredential) || perCredential < 1 || perCredential > MAX_CPA_CONCURRENCY_PER_CREDENTIAL) {
      appStore.showError(t('admin.accounts.cpaConcurrencyInvalid', { max: MAX_CPA_CONCURRENCY_PER_CREDENTIAL }))
      return
    }
    if (!cpaUseBaseUrl.value) {
      const managementURL = cpaManagementUrl.value.trim()
      if (!/^https?:\/\//i.test(managementURL)) {
        appStore.showError(t('admin.accounts.cpaManagementUrlRequired'))
        return
      }
    }
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
      ? await bulkUpdate({
        filters: props.target.filters,
        ...updates
      })
      : await bulkUpdate(props.accountIds, updates)
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
    } else if (error.reason === 'UPSTREAM_BILLING_RATE_SYNC_BULK_CONFLICT') {
      appStore.showError(t('admin.accounts.bulkEdit.rateSyncConflict', {
        count: error.metadata?.count ?? 1
      }))
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
      enableHeaderOverride.value = false
      enableProxy.value = false
      enableConcurrency.value = false
      enableLoadFactor.value = false
      enablePriority.value = false
      enableRateMultiplier.value = false
      enableStatus.value = false
      enableGroups.value = false
      enableOpenAIPassthrough.value = false
      openAIImageSizeTiersUpdate.value = null
      enableOpenAIFlattenNamespaces.value = false
      excelBpsMode.value = 'unchanged'
      enableOpenAICustomRelay.value = false
      enableOpenAILongContextBilling.value = false
      enableTLSFingerprint.value = false
      enableOpenAIEndpointCapabilities.value = false
      enableOpenAIResponsesMode.value = false
      enableOpenAIWSMode.value = false
      enableOpenAIAPIKeyWSMode.value = false
      enableCodexFingerprintMode.value = false
      enableCodexPrewarmContinuation.value = false
      enableCodexThinkingTagNormalization.value = false
      enableUpstreamBillingAutoProbe.value = false
      enableCodexCLIOnly.value = false
      enableCodexCLIOnlyAppServer.value = false
      enableOpenAICompactMode.value = false
      enableOpenAICompactModelMapping.value = false
      enableRpmLimit.value = false
      enableCPA.value = false

      // Reset all values
      baseUrl.value = ''
      openaiPassthroughEnabled.value = false
      openaiFlattenNamespacesEnabled.value = false
      openAICustomRelayEnabled.value = false
      openAICustomRelayURL.value = ''
      openAILongContextBillingEnabled.value = false
      tlsFingerprintEnabled.value = false
      tlsFingerprintProfileId.value = -1
      openAIEndpointCapabilities.value = ['chat_completions', 'embeddings', 'decisions']
      openAIResponsesMode.value = 'auto'
      modelRestrictionMode.value = 'whitelist'
      allowedModels.value = []
      modelMappings.value = []
      selectedErrorCodes.value = []
      customErrorCodeInput.value = null
      interceptWarmupRequests.value = false
      headerOverrideEnabled.value = false
      headerOverrideRows.value = []
      proxyId.value = null
      concurrency.value = 1
      loadFactor.value = null
      priority.value = 1
      rateMultiplier.value = 1
      status.value = 'active'
      groupIds.value = []
      openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      codexFingerprintMode.value = 'off'
      codexPrewarmContinuationEnabled.value = false
      codexThinkingTagNormalizationEnabled.value = false
      upstreamBillingAutoProbeMode.value = 'enabled'
      codexCLIOnlyEnabled.value = false
      codexCLIOnlyAppServerEnabled.value = false
      openAICompactMode.value = 'auto'
      openAICompactModelMappings.value = []
      rpmLimitEnabled.value = false
      bulkBaseRpm.value = null
      bulkRpmStrategy.value = 'tiered'
      bulkRpmStickyBuffer.value = null
      userMsgQueueMode.value = null
      cpaModeEnabled.value = true
      cpaUseBaseUrl.value = true
      cpaManagementUrl.value = ''
      cpaManagementPassword.value = ''
      cpaConcurrencyPerCredential.value = 10
      cpaExcludeAbnormalCredentials.value = false

      // Reset mixed channel warning state
      showMixedChannelWarning.value = false
      mixedChannelWarningMessage.value = ''
      pendingUpdatesForConfirm.value = null
      mixedChannelConfirmed.value = false
    }
  }
)
</script>
