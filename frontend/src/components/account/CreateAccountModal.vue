<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.createAccount')"
    size="2xl"
    @close="handleClose"
  >
    <!-- Step Indicator for OAuth accounts -->
    <div v-if="isOAuthFlow" class="mb-6 flex items-center justify-center">
      <div class="flex items-center space-x-4">
        <div class="flex items-center">
          <div
            :class="[
              'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold',
              step >= 1 ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600'
            ]"
          >
            1
          </div>
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.oauth.authMethod') }}</span>
        </div>
        <div class="h-0.5 w-8 bg-gray-300 dark:bg-dark-600" />
        <div class="flex items-center">
          <div
            :class="[
              'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold',
              step >= 2 ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600'
            ]"
          >
            2
          </div>
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.oauth.title') }}</span>
        </div>
      </div>
    </div>

    <!-- Step 1: Basic Info -->
    <form v-if="step === 1" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.accounts.accountName') }}</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.accounts.enterAccountName')"
        />
      </div>

      <!-- Platform Selection - Segmented Control Style -->
      <div>
        <label class="input-label">{{ t('admin.accounts.platform') }}</label>
        <div class="flex rounded-lg bg-gray-100 dark:bg-dark-700 p-1 mt-2">
          <button
            type="button"
            @click="form.platform = 'anthropic'"
            :class="[
              'flex-1 flex items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'anthropic'
                ? 'bg-white dark:bg-dark-600 text-orange-600 dark:text-orange-400 shadow-sm'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
            ]"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456z" />
            </svg>
            Anthropic
          </button>
          <button
            type="button"
            @click="form.platform = 'openai'"
            :class="[
              'flex-1 flex items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'openai'
                ? 'bg-white dark:bg-dark-600 text-green-600 dark:text-green-400 shadow-sm'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
            ]"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
            </svg>
            OpenAI
          </button>
        </div>
      </div>

      <!-- Account Type Selection (Anthropic) -->
      <div v-if="form.platform === 'anthropic'">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
        <div class="grid grid-cols-2 gap-3 mt-2">
          <button
            type="button"
            @click="accountCategory = 'oauth-based'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 transition-all text-left',
              accountCategory === 'oauth-based'
                ? 'border-orange-500 bg-orange-50 dark:bg-orange-900/20'
                : 'border-gray-200 dark:border-dark-600 hover:border-orange-300 dark:hover:border-orange-700'
            ]"
          >
            <div :class="[
              'flex h-8 w-8 items-center justify-center rounded-lg',
              accountCategory === 'oauth-based'
                ? 'bg-orange-500 text-white'
                : 'bg-gray-100 dark:bg-dark-600 text-gray-500 dark:text-gray-400'
            ]">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456z" />
              </svg>
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.claudeCode') }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.oauthSetupToken') }}</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'apikey'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 transition-all text-left',
              accountCategory === 'apikey'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 dark:border-dark-600 hover:border-purple-300 dark:hover:border-purple-700'
            ]"
          >
            <div :class="[
              'flex h-8 w-8 items-center justify-center rounded-lg',
              accountCategory === 'apikey'
                ? 'bg-purple-500 text-white'
                : 'bg-gray-100 dark:bg-dark-600 text-gray-500 dark:text-gray-400'
            ]">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
              </svg>
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.claudeConsole') }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.apiKey') }}</span>
            </div>
          </button>
        </div>
      </div>

      <!-- Account Type Selection (OpenAI) -->
      <div v-if="form.platform === 'openai'">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
        <div class="grid grid-cols-2 gap-3 mt-2">
          <button
            type="button"
            @click="accountCategory = 'oauth-based'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 transition-all text-left',
              accountCategory === 'oauth-based'
                ? 'border-green-500 bg-green-50 dark:bg-green-900/20'
                : 'border-gray-200 dark:border-dark-600 hover:border-green-300 dark:hover:border-green-700'
            ]"
          >
            <div :class="[
              'flex h-8 w-8 items-center justify-center rounded-lg',
              accountCategory === 'oauth-based'
                ? 'bg-green-500 text-white'
                : 'bg-gray-100 dark:bg-dark-600 text-gray-500 dark:text-gray-400'
            ]">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
              </svg>
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">OAuth</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">ChatGPT OAuth</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'apikey'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 transition-all text-left',
              accountCategory === 'apikey'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 dark:border-dark-600 hover:border-purple-300 dark:hover:border-purple-700'
            ]"
          >
            <div :class="[
              'flex h-8 w-8 items-center justify-center rounded-lg',
              accountCategory === 'apikey'
                ? 'bg-purple-500 text-white'
                : 'bg-gray-100 dark:bg-dark-600 text-gray-500 dark:text-gray-400'
            ]">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
              </svg>
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">API Key</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">Responses API</span>
            </div>
          </button>
        </div>
      </div>

      <!-- Add Method (only for Anthropic OAuth-based type) -->
      <div v-if="form.platform === 'anthropic' && isOAuthFlow">
        <label class="input-label">{{ t('admin.accounts.addMethod') }}</label>
        <div class="flex gap-4 mt-2">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">Oauth</span>
          </label>
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="setup-token"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenLongLived') }}</span>
          </label>
        </div>
      </div>

      <!-- API Key input (only for apikey type) -->
      <div v-if="form.type === 'apikey'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
          <input
            v-model="apiKeyBaseUrl"
            type="text"
            class="input"
            :placeholder="form.platform === 'openai' ? 'https://api.openai.com' : 'https://api.anthropic.com'"
          />
          <p class="input-hint">{{ t('admin.accounts.baseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKeyRequired') }}</label>
          <input
            v-model="apiKeyValue"
            type="password"
            required
            class="input font-mono"
            :placeholder="form.platform === 'openai' ? 'sk-proj-...' : 'sk-ant-...'"
          />
          <p class="input-hint">{{ t('admin.accounts.apiKeyHint') }}</p>
        </div>

        <!-- Model Restriction Section -->
        <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

          <!-- Mode Toggle -->
          <div class="flex gap-2 mb-4">
            <button
              type="button"
              @click="modelRestrictionMode = 'whitelist'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'whitelist'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <svg class="w-4 h-4 inline mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              {{ t('admin.accounts.modelWhitelist') }}
            </button>
            <button
              type="button"
              @click="modelRestrictionMode = 'mapping'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'mapping'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <svg class="w-4 h-4 inline mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
              </svg>
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <div class="mb-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 p-3">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {{ t('admin.accounts.selectAllowedModels') }}
              </p>
            </div>

            <!-- Model Checkbox List -->
            <div class="grid grid-cols-2 gap-2 mb-3">
              <label
                v-for="model in commonModels"
                :key="model.value"
                class="flex cursor-pointer items-center rounded-lg border p-3 transition-all hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                :class="allowedModels.includes(model.value) ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200'"
              >
                <input
                  type="checkbox"
                  :value="model.value"
                  v-model="allowedModels"
                  class="mr-2 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="text-sm text-gray-700 dark:text-gray-300">{{ model.label }}</span>
              </label>
            </div>

            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else>
            <div class="mb-3 rounded-lg bg-purple-50 dark:bg-purple-900/20 p-3">
              <p class="text-xs text-purple-700 dark:text-purple-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {{ t('admin.accounts.mapRequestModels') }}
              </p>
            </div>

            <!-- Model Mapping List -->
            <div v-if="modelMappings.length > 0" class="space-y-2 mb-3">
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
                <svg class="w-4 h-4 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeModelMapping(index)"
                  class="p-2 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              @click="addModelMapping"
              class="w-full rounded-lg border-2 border-dashed border-gray-300 dark:border-dark-500 px-4 py-2 text-gray-600 dark:text-gray-400 transition-colors hover:border-gray-400 hover:text-gray-700 dark:hover:border-dark-400 dark:hover:text-gray-300 mb-3"
            >
              <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              {{ t('admin.accounts.addMapping') }}
            </button>

            <!-- Quick Add Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in presetMappings"
                :key="preset.label"
                type="button"
                @click="addPresetMapping(preset.from, preset.to)"
                :class="[
                  'rounded-lg px-3 py-1 text-xs transition-colors',
                  preset.color
                ]"
              >
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </div>

        <!-- Custom Error Codes Section -->
        <div class="border-t border-gray-200 dark:border-dark-600 pt-4">
          <div class="flex items-center justify-between mb-3">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.accounts.customErrorCodesHint') }}</p>
            </div>
            <button
              type="button"
              @click="customErrorCodesEnabled = !customErrorCodesEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                customErrorCodesEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  customErrorCodesEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="customErrorCodesEnabled" class="space-y-3">
            <div class="rounded-lg bg-amber-50 dark:bg-amber-900/20 p-3">
              <p class="text-xs text-amber-700 dark:text-amber-400">
                <svg class="w-4 h-4 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
                {{ t('admin.accounts.customErrorCodesWarning') }}
              </p>
            </div>

            <!-- Error Code Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="code in commonErrorCodes"
                :key="code.value"
                type="button"
                @click="toggleErrorCode(code.value)"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                  selectedErrorCodes.includes(code.value)
                    ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 ring-1 ring-red-500'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                {{ code.value }} {{ code.label }}
              </button>
            </div>

            <!-- Manual input -->
            <div class="flex items-center gap-2">
              <input
                v-model="customErrorCodeInput"
                type="number"
                min="100"
                max="599"
                class="input flex-1"
                :placeholder="t('admin.accounts.enterErrorCode')"
                @keyup.enter="addCustomErrorCode"
              />
              <button
                type="button"
                @click="addCustomErrorCode"
                class="btn btn-secondary px-3"
              >
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
              </button>
            </div>

            <!-- Selected codes summary -->
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="code in selectedErrorCodes.sort((a, b) => a - b)"
                :key="code"
                class="inline-flex items-center gap-1 rounded-full bg-red-100 dark:bg-red-900/30 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:text-red-400"
              >
                {{ code }}
                <button
                  type="button"
                  @click="removeErrorCode(code)"
                  class="hover:text-red-900 dark:hover:text-red-300"
                >
                  <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </span>
              <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
                {{ t('admin.accounts.noneSelectedUsesDefault') }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Intercept Warmup Requests (Anthropic only) -->
      <div v-if="form.platform === 'anthropic'" class="border-t border-gray-200 dark:border-dark-600 pt-4">
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.interceptWarmupRequests') }}</label>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.accounts.interceptWarmupRequestsDesc') }}</p>
          </div>
          <button
            type="button"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
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

      <div>
        <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
        <ProxySelector
          v-model="form.proxy_id"
          :proxies="proxies"
        />
      </div>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input
            v-model.number="form.concurrency"
            type="number"
            min="1"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input
            v-model.number="form.priority"
            type="number"
            min="1"
            class="input"
          />
          <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
        </div>
      </div>

      <!-- Group Selection -->
      <GroupSelector
        v-model="form.group_ids"
        :groups="groups"
        :platform="form.platform"
      />

      <div class="flex justify-end gap-3 pt-4">
        <button
          @click="handleClose"
          type="button"
          class="btn btn-secondary"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          :disabled="submitting"
          class="btn btn-primary"
        >
          <svg
            v-if="submitting"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ isOAuthFlow ? t('common.next') : (submitting ? t('admin.accounts.creating') : t('common.create')) }}
        </button>
      </div>
    </form>

    <!-- Step 2: OAuth Authorization -->
    <div v-else class="space-y-5">
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="form.platform === 'openai' ? 'oauth' : addMethod"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentOAuthLoading"
        :error="currentOAuthError"
        :show-help="form.platform !== 'openai'"
        :show-proxy-warning="!!form.proxy_id"
        :allow-multiple="form.platform !== 'openai'"
        :show-cookie-option="form.platform !== 'openai'"
        :platform="form.platform"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
      />

      <div class="flex justify-between gap-3 pt-4">
        <button
          type="button"
          class="btn btn-secondary"
          @click="goBackToBasicInfo"
        >
          {{ t('common.back') }}
        </button>
        <button
          v-if="isManualInputMethod"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="currentOAuthLoading"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ currentOAuthLoading ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.completeAuth') }}
        </button>
      </div>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { useAccountOAuth, type AddMethod, type AuthInputMethod } from '@/composables/useAccountOAuth'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import type { Proxy, Group, AccountPlatform, AccountType } from '@/types'
import Modal from '@/components/common/Modal.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'

// Type for exposed OAuthAuthorizationFlow component
// Note: defineExpose automatically unwraps refs, so we use the unwrapped types
interface OAuthFlowExposed {
  authCode: string
  sessionKey: string
  inputMethod: AuthInputMethod
  reset: () => void
}

const { t } = useI18n()

interface Props {
  show: boolean
  proxies: Proxy[]
  groups: Group[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  created: []
}>()

const appStore = useAppStore()

// OAuth composables
const oauth = useAccountOAuth()           // For Anthropic OAuth
const openaiOAuth = useOpenAIOAuth()      // For OpenAI OAuth

// Computed: current OAuth state for template binding
const currentAuthUrl = computed(() => {
  return form.platform === 'openai' ? openaiOAuth.authUrl.value : oauth.authUrl.value
})

const currentSessionId = computed(() => {
  return form.platform === 'openai' ? openaiOAuth.sessionId.value : oauth.sessionId.value
})

const currentOAuthLoading = computed(() => {
  return form.platform === 'openai' ? openaiOAuth.loading.value : oauth.loading.value
})

const currentOAuthError = computed(() => {
  return form.platform === 'openai' ? openaiOAuth.error.value : oauth.error.value
})

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// Model mapping type
interface ModelMapping {
  from: string
  to: string
}

// State
const step = ref(1)
const submitting = ref(false)
const accountCategory = ref<'oauth-based' | 'apikey'>('oauth-based') // UI selection for account category
const addMethod = ref<AddMethod>('oauth') // For oauth-based: 'oauth' or 'setup-token'
const apiKeyBaseUrl = ref('https://api.anthropic.com')
const apiKeyValue = ref('')
const modelMappings = ref<ModelMapping[]>([])
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)

// Common models for whitelist - Anthropic
const anthropicModels = [
  { value: 'claude-opus-4-5-20251101', label: 'Claude Opus 4.5' },
  { value: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4' },
  { value: 'claude-sonnet-4-5-20250929', label: 'Claude Sonnet 4.5' },
  { value: 'claude-3-5-haiku-20241022', label: 'Claude 3.5 Haiku' },
  { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5' },
  { value: 'claude-3-opus-20240229', label: 'Claude 3 Opus' },
  { value: 'claude-3-5-sonnet-20241022', label: 'Claude 3.5 Sonnet' },
  { value: 'claude-3-haiku-20240307', label: 'Claude 3 Haiku' }
]

// Common models for whitelist - OpenAI
const openaiModels = [
  { value: 'gpt-5.2-2025-12-11', label: 'GPT-5.2' },
  { value: 'gpt-5.2-codex', label: 'GPT-5.2 Codex' },
  { value: 'gpt-5.1-codex-max', label: 'GPT-5.1 Codex Max' },
  { value: 'gpt-5.1-codex', label: 'GPT-5.1 Codex' },
  { value: 'gpt-5.1-2025-11-13', label: 'GPT-5.1' },
  { value: 'gpt-5.1-codex-mini', label: 'GPT-5.1 Codex Mini' },
  { value: 'gpt-5-2025-08-07', label: 'GPT-5' }
]

// Computed: current models based on platform
const commonModels = computed(() => {
  return form.platform === 'openai' ? openaiModels : anthropicModels
})

// Preset mappings for quick add - Anthropic
const anthropicPresetMappings = [
  { label: 'Sonnet 4', from: 'claude-sonnet-4-20250514', to: 'claude-sonnet-4-20250514', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: 'Sonnet 4.5', from: 'claude-sonnet-4-5-20250929', to: 'claude-sonnet-4-5-20250929', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
  { label: 'Opus 4.5', from: 'claude-opus-4-5-20251101', to: 'claude-opus-4-5-20251101', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: 'Haiku 3.5', from: 'claude-3-5-haiku-20241022', to: 'claude-3-5-haiku-20241022', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
  { label: 'Haiku 4.5', from: 'claude-haiku-4-5-20251001', to: 'claude-haiku-4-5-20251001', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
  { label: 'Opus->Sonnet', from: 'claude-opus-4-5-20251101', to: 'claude-sonnet-4-5-20250929', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' }
]

// Preset mappings for quick add - OpenAI
const openaiPresetMappings = [
  { label: 'GPT-5.2', from: 'gpt-5.2-2025-12-11', to: 'gpt-5.2-2025-12-11', color: 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400' },
  { label: 'GPT-5.2 Codex', from: 'gpt-5.2-codex', to: 'gpt-5.2-codex', color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400' },
  { label: 'GPT-5.1 Codex', from: 'gpt-5.1-codex', to: 'gpt-5.1-codex', color: 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400' },
  { label: 'Codex Max', from: 'gpt-5.1-codex-max', to: 'gpt-5.1-codex-max', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: 'Codex Mini', from: 'gpt-5.1-codex-mini', to: 'gpt-5.1-codex-mini', color: 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400' },
  { label: 'Max->Codex', from: 'gpt-5.1-codex-max', to: 'gpt-5.1-codex', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' }
]

// Computed: current preset mappings based on platform
const presetMappings = computed(() => {
  return form.platform === 'openai' ? openaiPresetMappings : anthropicPresetMappings
})

// Common HTTP error codes for quick selection
const commonErrorCodes = [
  { value: 401, label: 'Unauthorized' },
  { value: 403, label: 'Forbidden' },
  { value: 429, label: 'Rate Limit' },
  { value: 500, label: 'Server Error' },
  { value: 502, label: 'Bad Gateway' },
  { value: 503, label: 'Unavailable' },
  { value: 529, label: 'Overloaded' }
]

const form = reactive({
  name: '',
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType, // Will be 'oauth', 'setup-token', or 'apikey'
  credentials: {} as Record<string, unknown>,
  proxy_id: null as number | null,
  concurrency: 10,
  priority: 1,
  group_ids: [] as number[]
})

// Helper to check if current type needs OAuth flow
const isOAuthFlow = computed(() => accountCategory.value === 'oauth-based')

const isManualInputMethod = computed(() => {
  return oauthFlowRef.value?.inputMethod === 'manual'
})

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  if (form.platform === 'openai') {
    return authCode.trim() && openaiOAuth.sessionId.value && !openaiOAuth.loading.value
  }
  return authCode.trim() && oauth.sessionId.value && !oauth.loading.value
})

// Watchers
watch(() => props.show, (newVal) => {
  if (!newVal) {
    resetForm()
  }
})

// Sync form.type based on accountCategory and addMethod
watch([accountCategory, addMethod], ([category, method]) => {
  if (category === 'oauth-based') {
    form.type = method as AccountType // 'oauth' or 'setup-token'
  } else {
    form.type = 'apikey'
  }
}, { immediate: true })

// Reset platform-specific settings when platform changes
watch(() => form.platform, (newPlatform) => {
  // Reset base URL based on platform
  apiKeyBaseUrl.value = newPlatform === 'openai'
    ? 'https://api.openai.com'
    : 'https://api.anthropic.com'
  // Clear model-related settings
  allowedModels.value = []
  modelMappings.value = []
  // Reset OAuth states
  oauth.resetState()
  openaiOAuth.resetState()
})

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const addPresetMapping = (from: string, to: string) => {
  // Check if mapping already exists
  const exists = modelMappings.value.some(m => m.from === from)
  if (exists) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  modelMappings.value.push({ from, to })
}

// Error code toggle helper
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

// Add custom error code from input
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
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
}

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const buildModelMappingObject = (): Record<string, string> | null => {
  const mapping: Record<string, string> = {}

  if (modelRestrictionMode.value === 'whitelist') {
    // Whitelist mode: map model to itself
    for (const model of allowedModels.value) {
      mapping[model] = model
    }
  } else {
    // Mapping mode: use custom mappings
    for (const m of modelMappings.value) {
      const from = m.from.trim()
      const to = m.to.trim()
      if (from && to) {
        mapping[from] = to
      }
    }
  }

  return Object.keys(mapping).length > 0 ? mapping : null
}

// Methods
const resetForm = () => {
  step.value = 1
  form.name = ''
  form.platform = 'anthropic'
  form.type = 'oauth'
  form.credentials = {}
  form.proxy_id = null
  form.concurrency = 10
  form.priority = 1
  form.group_ids = []
  accountCategory.value = 'oauth-based'
  addMethod.value = 'oauth'
  apiKeyBaseUrl.value = 'https://api.anthropic.com'
  apiKeyValue.value = ''
  modelMappings.value = []
  modelRestrictionMode.value = 'whitelist'
  allowedModels.value = []
  customErrorCodesEnabled.value = false
  selectedErrorCodes.value = []
  customErrorCodeInput.value = null
  interceptWarmupRequests.value = false
  oauth.resetState()
  openaiOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const handleClose = () => {
  emit('close')
}

const handleSubmit = async () => {
  // For OAuth-based type, handle OAuth flow (goes to step 2)
  if (isOAuthFlow.value) {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    step.value = 2
    return
  }

  // For apikey type, create directly
  if (!apiKeyValue.value.trim()) {
    appStore.showError(t('admin.accounts.pleaseEnterApiKey'))
    return
  }

  // Determine default base URL based on platform
  const defaultBaseUrl = form.platform === 'openai'
    ? 'https://api.openai.com'
    : 'https://api.anthropic.com'

  // Build credentials with optional model mapping
  const credentials: Record<string, unknown> = {
    base_url: apiKeyBaseUrl.value.trim() || defaultBaseUrl,
    api_key: apiKeyValue.value.trim()
  }

  // Add model mapping if configured
  const modelMapping = buildModelMappingObject()
  if (modelMapping) {
    credentials.model_mapping = modelMapping
  }

  // Add custom error codes if enabled
  if (customErrorCodesEnabled.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
  }

  // Add intercept warmup requests setting
  if (interceptWarmupRequests.value) {
    credentials.intercept_warmup_requests = true
  }

  form.credentials = credentials

  submitting.value = true
  try {
    await adminAPI.accounts.create({
      ...form,
      group_ids: form.group_ids
    })
    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToCreate'))
  } finally {
    submitting.value = false
  }
}

const goBackToBasicInfo = () => {
  step.value = 1
  oauth.resetState()
  openaiOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const handleGenerateUrl = async () => {
  if (form.platform === 'openai') {
    await openaiOAuth.generateAuthUrl(form.proxy_id)
  } else {
    await oauth.generateAuthUrl(addMethod.value, form.proxy_id)
  }
}

const handleExchangeCode = async () => {
  const authCode = oauthFlowRef.value?.authCode || ''

  // For OpenAI
  if (form.platform === 'openai') {
    if (!authCode.trim() || !openaiOAuth.sessionId.value) return

    openaiOAuth.loading.value = true
    openaiOAuth.error.value = ''

    try {
      const tokenInfo = await openaiOAuth.exchangeAuthCode(
        authCode.trim(),
        openaiOAuth.sessionId.value,
        form.proxy_id
      )

      if (!tokenInfo) {
        return // Error already handled by composable
      }

      const credentials = openaiOAuth.buildCredentials(tokenInfo)
      const extra = openaiOAuth.buildExtraInfo(tokenInfo)

      // Merge interceptWarmupRequests into credentials
      if (interceptWarmupRequests.value) {
        credentials.intercept_warmup_requests = true
      }

      await adminAPI.accounts.create({
        name: form.name,
        platform: 'openai',
        type: 'oauth',
        credentials,
        extra,
        proxy_id: form.proxy_id,
        concurrency: form.concurrency,
        priority: form.priority,
        group_ids: form.group_ids
      })

      appStore.showSuccess(t('admin.accounts.accountCreated'))
      emit('created')
      handleClose()
    } catch (error: any) {
      openaiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(openaiOAuth.error.value)
    } finally {
      openaiOAuth.loading.value = false
    }
    return
  }

  // For Anthropic
  if (!authCode.trim() || !oauth.sessionId.value) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id } : {}
    const endpoint = addMethod.value === 'oauth'
      ? '/admin/accounts/exchange-code'
      : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: oauth.sessionId.value,
      code: authCode.trim(),
      ...proxyConfig
    })

    const extra = oauth.buildExtraInfo(tokenInfo)

    // Merge interceptWarmupRequests into credentials
    const credentials = {
      ...tokenInfo,
      ...(interceptWarmupRequests.value ? { intercept_warmup_requests: true } : {})
    }

    await adminAPI.accounts.create({
      name: form.name,
      platform: form.platform,
      type: addMethod.value, // Use addMethod as type: 'oauth' or 'setup-token'
      credentials,
      extra,
      proxy_id: form.proxy_id,
      concurrency: form.concurrency,
      priority: form.priority,
      group_ids: form.group_ids
    })

    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  } catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauth.error.value)
  } finally {
    oauth.loading.value = false
  }
}

const handleCookieAuth = async (sessionKey: string) => {
  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id } : {}
    const keys = oauth.parseSessionKeys(sessionKey)

    if (keys.length === 0) {
      oauth.error.value = t('admin.accounts.oauth.pleaseEnterSessionKey')
      return
    }

    const endpoint = addMethod.value === 'oauth'
      ? '/admin/accounts/cookie-auth'
      : '/admin/accounts/setup-token-cookie-auth'

    let successCount = 0
    let failedCount = 0
    const errors: string[] = []

    for (let i = 0; i < keys.length; i++) {
      try {
        const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
          session_id: '',
          code: keys[i],
          ...proxyConfig
        })

        const extra = oauth.buildExtraInfo(tokenInfo)
        const accountName = keys.length > 1 ? `${form.name} #${i + 1}` : form.name

        // Merge interceptWarmupRequests into credentials
        const credentials = {
          ...tokenInfo,
          ...(interceptWarmupRequests.value ? { intercept_warmup_requests: true } : {})
        }

        await adminAPI.accounts.create({
          name: accountName,
          platform: form.platform,
          type: addMethod.value, // Use addMethod as type: 'oauth' or 'setup-token'
          credentials,
          extra,
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          priority: form.priority
        })

        successCount++
      } catch (error: any) {
        failedCount++
        errors.push(t('admin.accounts.oauth.keyAuthFailed', { index: i + 1, error: error.response?.data?.detail || t('admin.accounts.oauth.authFailed') }))
      }
    }

    if (successCount > 0) {
      appStore.showSuccess(t('admin.accounts.oauth.successCreated', { count: successCount }))
      if (failedCount === 0) {
        emit('created')
        handleClose()
      } else {
        emit('created')
      }
    }

    if (failedCount > 0) {
      oauth.error.value = errors.join('\n')
    }
  } catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  } finally {
    oauth.loading.value = false
  }
}
</script>
