<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.createAccount')"
    width="wide"
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
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
            t('admin.accounts.oauth.authMethod')
          REDACTEDREDACTED</span>
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
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
            oauthStepTitle
          REDACTEDREDACTED</span>
        </div>
      </div>
    </div>

    <!-- Step 1: Basic Info -->
    <form
      v-if="step === 1"
      id="create-account-form"
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <div>
        <label class="input-label">{{ t('admin.accounts.accountName') REDACTEDREDACTED</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.accounts.enterAccountName')"
          data-tour="account-form-name"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.notes') REDACTEDREDACTED</label>
        <textarea
          v-model="form.notes"
          rows="3"
          class="input"
          :placeholder="t('admin.accounts.notesPlaceholder')"
        ></textarea>
        <p class="input-hint">{{ t('admin.accounts.notesHint') REDACTEDREDACTED</p>
      </div>

      <!-- Platform Selection - Segmented Control Style -->
      <div>
        <label class="input-label">{{ t('admin.accounts.platform') REDACTEDREDACTED</label>
        <div class="mt-2 flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700" data-tour="account-form-platform">
          <button
            type="button"
            @click="form.platform = 'anthropic'"
            :class="[
              'flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'anthropic'
                ? 'bg-white text-orange-600 shadow-sm dark:bg-dark-600 dark:text-orange-400'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
          >
            <Icon name="sparkles" size="sm" />
            Anthropic
          </button>
          <button
            type="button"
            @click="form.platform = 'openai'"
            :class="[
              'flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'openai'
                ? 'bg-white text-green-600 shadow-sm dark:bg-dark-600 dark:text-green-400'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z"
              />
            </svg>
            OpenAI
          </button>
          <button
            type="button"
            @click="form.platform = 'gemini'"
            :class="[
              'flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'gemini'
                ? 'bg-white text-blue-600 shadow-sm dark:bg-dark-600 dark:text-blue-400'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M12 2l1.5 6.5L20 10l-6.5 1.5L12 18l-1.5-6.5L4 10l6.5-1.5L12 2z"
              />
            </svg>
            Gemini
          </button>
          <button
            type="button"
            @click="form.platform = 'antigravity'"
            :class="[
              'flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === 'antigravity'
                ? 'bg-white text-purple-600 shadow-sm dark:bg-dark-600 dark:text-purple-400'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
          >
            <Icon name="cloud" size="sm" />
            Antigravity
          </button>
        </div>
      </div>

      <!-- Account Type Selection (Anthropic) -->
      <div v-if="form.platform === 'anthropic'">
        <label class="input-label">{{ t('admin.accounts.accountType') REDACTEDREDACTED</label>
        <div class="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-4" data-tour="account-form-type">
          <button
            type="button"
            @click="accountCategory = 'oauth-based'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'oauth-based'
                ? 'border-orange-500 bg-orange-50 dark:bg-orange-900/20'
                : 'border-gray-200 hover:border-orange-300 dark:border-dark-600 dark:hover:border-orange-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'oauth-based'
                  ? 'bg-orange-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="sparkles" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{
                t('admin.accounts.claudeCode')
              REDACTEDREDACTED</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.oauthSetupToken')
              REDACTEDREDACTED</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'apikey'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'apikey'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'apikey'
                  ? 'bg-purple-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{
                t('admin.accounts.claudeConsole')
              REDACTEDREDACTED</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.apiKey')
              REDACTEDREDACTED</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'bedrock'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'bedrock'
                ? 'border-amber-500 bg-amber-50 dark:bg-amber-900/20'
                : 'border-gray-200 hover:border-amber-300 dark:border-dark-600 dark:hover:border-amber-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'bedrock'
                  ? 'bg-amber-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="cloud" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{
                t('admin.accounts.bedrockLabel')
              REDACTEDREDACTED</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.bedrockDesc')
              REDACTEDREDACTED</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'service_account'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'service_account'
                ? 'border-sky-500 bg-sky-50 dark:bg-sky-900/20'
                : 'border-gray-200 hover:border-sky-300 dark:border-dark-600 dark:hover:border-sky-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'service_account'
                  ? 'bg-sky-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="cloud" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">Vertex</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">Service Account</span>
            </div>
          </button>

        </div>

        <div
          v-if="accountCategory === 'service_account'"
          class="mt-3 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800 dark:border-sky-800/40 dark:bg-sky-900/20 dark:text-sky-200"
        >
          <p>{{ t('admin.accounts.vertexAnthropicHint') REDACTEDREDACTED</p>
        </div>
      </div>

      <!-- Account Type Selection (OpenAI) -->
      <div v-if="form.platform === 'openai'">
        <label class="input-label">{{ t('admin.accounts.accountType') REDACTEDREDACTED</label>
        <div class="mt-2 grid grid-cols-2 gap-3" data-tour="account-form-type">
          <button
            type="button"
            @click="accountCategory = 'oauth-based'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'oauth-based'
                ? 'border-green-500 bg-green-50 dark:bg-green-900/20'
                : 'border-gray-200 hover:border-green-300 dark:border-dark-600 dark:hover:border-green-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'oauth-based'
                  ? 'bg-green-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">OAuth</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.chatgptOauth') REDACTEDREDACTED</span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'apikey'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'apikey'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'apikey'
                  ? 'bg-purple-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">API Key</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.responsesApi') REDACTEDREDACTED</span>
            </div>
          </button>

        </div>
      </div>

      <!-- Account Type Selection (Gemini) -->
      <div v-if="form.platform === 'gemini'">
        <div class="flex items-center justify-between">
          <label class="input-label">{{ t('admin.accounts.accountType') REDACTEDREDACTED</label>
          <button
            type="button"
            @click="showGeminiHelpDialog = true"
            class="flex items-center gap-1 rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z" />
            </svg>
            {{ t('admin.accounts.gemini.helpButton') REDACTEDREDACTED
          </button>
        </div>
        <div class="mt-2 grid grid-cols-3 gap-3" data-tour="account-form-type">
          <button
            type="button"
            @click="accountCategory = 'oauth-based'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'oauth-based'
                ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                : 'border-gray-200 hover:border-blue-300 dark:border-dark-600 dark:hover:border-blue-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'oauth-based'
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.accounts.gemini.accountType.oauthTitle') REDACTEDREDACTED
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.gemini.accountType.oauthDesc') REDACTEDREDACTED
              </span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'apikey'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'apikey'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'apikey'
                  ? 'bg-purple-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1721.75 8.25z"
                />
              </svg>
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.accounts.gemini.accountType.apiKeyTitle') REDACTEDREDACTED
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.gemini.accountType.apiKeyDesc') REDACTEDREDACTED
              </span>
            </div>
          </button>

          <button
            type="button"
            @click="accountCategory = 'service_account'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === 'service_account'
                ? 'border-sky-500 bg-sky-50 dark:bg-sky-900/20'
                : 'border-gray-200 hover:border-sky-300 dark:border-dark-600 dark:hover:border-sky-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                accountCategory === 'service_account'
                  ? 'bg-sky-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="cloud" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                Vertex
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                Service Account
              </span>
            </div>
          </button>
        </div>

        <div
          v-if="accountCategory === 'apikey'"
          class="mt-3 rounded-lg border border-purple-200 bg-purple-50 px-3 py-2 text-xs text-purple-800 dark:border-purple-800/40 dark:bg-purple-900/20 dark:text-purple-200"
        >
          <p>{{ t('admin.accounts.gemini.accountType.apiKeyNote') REDACTEDREDACTED</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <a
              :href="geminiHelpLinks.apiKey"
              class="font-medium text-blue-600 hover:underline dark:text-blue-400"
              target="_blank"
              rel="noreferrer"
            >
              {{ t('admin.accounts.gemini.accountType.apiKeyLink') REDACTEDREDACTED
            </a>
          </div>
        </div>

        <div
          v-if="accountCategory === 'service_account'"
          class="mt-3 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800 dark:border-sky-800/40 dark:bg-sky-900/20 dark:text-sky-200"
        >
          <p>{{ t('admin.accounts.vertexGeminiHint') REDACTEDREDACTED</p>
        </div>

        <!-- OAuth Type Selection (only show when oauth-based is selected) -->
        <div v-if="accountCategory === 'oauth-based'" class="mt-4">
          <label class="input-label">{{ t('admin.accounts.oauth.gemini.oauthTypeLabel') REDACTEDREDACTED</label>
          <div class="mt-2 grid grid-cols-2 gap-3">
            <!-- Google One OAuth -->
            <button
              type="button"
              @click="handleSelectGeminiOAuthType('google_one')"
              :class="[
                'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
                geminiOAuthType === 'google_one'
                  ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                  : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
              ]"
            >
              <div
                :class="[
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                  geminiOAuthType === 'google_one'
                    ? 'bg-purple-500 text-white'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
                ]"
              >
                <Icon name="user" size="sm" />
              </div>
              <div class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  Google One
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  个人账号，享受 Google One 订阅配额
                </span>
                <div class="mt-2 flex flex-wrap gap-1">
                  <span
                    class="rounded bg-purple-100 px-2 py-0.5 text-[10px] font-semibold text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
                  >
                    推荐个人用户
                  </span>
                  <span
                    class="rounded bg-emerald-100 px-2 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
                  >
                    无需 GCP
                  </span>
                </div>
              </div>
            </button>

            <!-- GCP Code Assist OAuth -->
            <button
              type="button"
              @click="handleSelectGeminiOAuthType('code_assist')"
              :class="[
                'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
                geminiOAuthType === 'code_assist'
                  ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                  : 'border-gray-200 hover:border-blue-300 dark:border-dark-600 dark:hover:border-blue-700'
              ]"
            >
              <div
                :class="[
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                  geminiOAuthType === 'code_assist'
                    ? 'bg-blue-500 text-white'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
                ]"
              >
                <Icon name="cloud" size="sm" />
              </div>
              <div class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  GCP Code Assist
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  企业级，需要 GCP 项目
                </span>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  需要激活 GCP 项目并绑定信用卡
                  <a
                    :href="geminiHelpLinks.gcpProject"
                    class="ml-1 text-blue-600 hover:underline dark:text-blue-400"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {{ t('admin.accounts.gemini.oauthType.gcpProjectLink') REDACTEDREDACTED
                  </a>
                </div>
                <div class="mt-2 flex flex-wrap gap-1">
                  <span
                    class="rounded bg-blue-100 px-2 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                  >
                    企业用户
                  </span>
                  <span
                    class="rounded bg-emerald-100 px-2 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
                  >
                    高并发
                  </span>
                </div>
              </div>
            </button>
          </div>

          <!-- Advanced Options Toggle -->
          <div class="mt-3">
            <button
              type="button"
              @click="showAdvancedOAuth = !showAdvancedOAuth"
              class="flex items-center gap-2 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
            >
              <svg
                :class="['h-4 w-4 transition-transform', showAdvancedOAuth ? 'rotate-90' : '']"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
              </svg>
              <span>{{ showAdvancedOAuth ? '隐藏' : '显示' REDACTEDREDACTED高级选项（自建 OAuth Client）</span>
            </button>
          </div>

          <!-- Custom OAuth Client (Advanced) -->
          <div v-if="showAdvancedOAuth" class="mt-3 group relative">
            <button
              type="button"
              :disabled="!geminiAIStudioOAuthEnabled"
              @click="handleSelectGeminiOAuthType('ai_studio')"
              :class="[
                'flex w-full items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
                !geminiAIStudioOAuthEnabled ? 'cursor-not-allowed opacity-60' : '',
                geminiOAuthType === 'ai_studio'
                  ? 'border-amber-500 bg-amber-50 dark:bg-amber-900/20'
                  : 'border-gray-200 hover:border-amber-300 dark:border-dark-600 dark:hover:border-amber-700'
              ]"
            >
              <div
                :class="[
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                  geminiOAuthType === 'ai_studio'
                    ? 'bg-amber-500 text-white'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
                ]"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z"
                  />
                </svg>
              </div>
              <div class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.oauthType.customTitle') REDACTEDREDACTED
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.oauthType.customDesc') REDACTEDREDACTED
                </span>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.oauthType.customRequirement') REDACTEDREDACTED
                </div>
                <div class="mt-2 flex flex-wrap gap-1">
                  <span
                    class="rounded bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                  >
                    {{ t('admin.accounts.gemini.oauthType.badges.orgManaged') REDACTEDREDACTED
                  </span>
                  <span
                    class="rounded bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                  >
                    {{ t('admin.accounts.gemini.oauthType.badges.adminRequired') REDACTEDREDACTED
                  </span>
                </div>
              </div>
              <span
                v-if="!geminiAIStudioOAuthEnabled"
                class="ml-auto shrink-0 rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
              >
                {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredShort') REDACTEDREDACTED
              </span>
            </button>

            <div
              v-if="!geminiAIStudioOAuthEnabled"
              class="pointer-events-none absolute right-0 top-full z-50 mt-2 w-80 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:border-amber-700 dark:bg-amber-900/40 dark:text-amber-200"
            >
              {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredTip') REDACTEDREDACTED
            </div>
          </div>
        </div>

        <!-- Tier selection (used as fallback when auto-detection is unavailable/fails) -->
        <div v-if="accountCategory !== 'service_account'" class="mt-4">
          <label class="input-label">{{ t('admin.accounts.gemini.tier.label') REDACTEDREDACTED</label>
          <div class="mt-2">
            <select
              v-if="geminiOAuthType === 'google_one'"
              v-model="geminiTierGoogleOne"
              class="input"
            >
              <option value="google_one_free">{{ t('admin.accounts.gemini.tier.googleOne.free') REDACTEDREDACTED</option>
              <option value="google_ai_pro">{{ t('admin.accounts.gemini.tier.googleOne.pro') REDACTEDREDACTED</option>
              <option value="google_ai_ultra">{{ t('admin.accounts.gemini.tier.googleOne.ultra') REDACTEDREDACTED</option>
            </select>

            <select
              v-else-if="geminiOAuthType === 'code_assist'"
              v-model="geminiTierGcp"
              class="input"
            >
              <option value="gcp_standard">{{ t('admin.accounts.gemini.tier.gcp.standard') REDACTEDREDACTED</option>
              <option value="gcp_enterprise">{{ t('admin.accounts.gemini.tier.gcp.enterprise') REDACTEDREDACTED</option>
            </select>

            <select
              v-else
              v-model="geminiTierAIStudio"
              class="input"
            >
              <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') REDACTEDREDACTED</option>
              <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') REDACTEDREDACTED</option>
            </select>
          </div>
          <p class="input-hint">{{ t('admin.accounts.gemini.tier.hint') REDACTEDREDACTED</p>
        </div>
      </div>

      <!-- Account Type Selection (Antigravity - OAuth or Upstream) -->
      <div v-if="form.platform === 'antigravity'">
        <label class="input-label">{{ t('admin.accounts.accountType') REDACTEDREDACTED</label>
        <div class="mt-2 grid grid-cols-2 gap-3">
          <button
            type="button"
            @click="antigravityAccountType = 'oauth'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              antigravityAccountType === 'oauth'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                antigravityAccountType === 'oauth'
                  ? 'bg-purple-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">OAuth</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.antigravityOauth') REDACTEDREDACTED</span>
            </div>
          </button>

          <button
            type="button"
            @click="antigravityAccountType = 'upstream'"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              antigravityAccountType === 'upstream'
                ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
                : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
            ]"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                antigravityAccountType === 'upstream'
                  ? 'bg-purple-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="cloud" size="sm" />
            </div>
            <div>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">API Key</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.antigravityApikey') REDACTEDREDACTED</span>
            </div>
          </button>
        </div>
      </div>

      <div v-if="form.platform === 'antigravity' && antigravityAccountType === 'oauth'">
        <label class="input-label">{{ t('admin.accounts.antigravityProjectIdLabel') REDACTEDREDACTED</label>
        <input
          v-model="antigravityProjectId"
          data-testid="antigravity-project-id-input"
          type="text"
          class="input font-mono"
          :placeholder="t('admin.accounts.antigravityProjectIdPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.accounts.antigravityProjectIdHint') REDACTEDREDACTED</p>
      </div>

      <!-- Upstream config (only for Antigravity upstream type) -->
      <div v-if="form.platform === 'antigravity' && antigravityAccountType === 'upstream'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.baseUrl') REDACTEDREDACTED</label>
          <input
            v-model="upstreamBaseUrl"
            type="text"
            required
            class="input"
            placeholder="https://cloudcode-pa.googleapis.com"
          />
          <p class="input-hint">{{ t('admin.accounts.upstream.baseUrlHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.apiKey') REDACTEDREDACTED</label>
          <input
            v-model="upstreamApiKey"
            type="password"
            required
            class="input font-mono"
            placeholder="sk-..."
          />
          <p class="input-hint">{{ t('admin.accounts.upstream.apiKeyHint') REDACTEDREDACTED</p>
        </div>
      </div>

      <!-- Vertex Service Account -->
      <div v-if="(form.platform === 'gemini' || form.platform === 'anthropic') && accountCategory === 'service_account'" class="space-y-4">
        <div>
          <label class="input-label">Service Account JSON</label>
          <input
            ref="vertexServiceAccountFileInput"
            type="file"
            accept="application/json,.json"
            class="hidden"
            @change="handleVertexServiceAccountFile"
          />
          <div
            :class="[
              'rounded-lg border-2 border-dashed px-4 py-5 transition-colors',
              vertexServiceAccountDragActive
                ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-900/20'
                : 'border-gray-300 bg-gray-50 hover:border-sky-400 hover:bg-sky-50/60 dark:border-dark-500 dark:bg-dark-700/40 dark:hover:border-sky-600 dark:hover:bg-sky-900/10'
            ]"
            @dragenter.prevent="vertexServiceAccountDragActive = true"
            @dragover.prevent="vertexServiceAccountDragActive = true"
            @dragleave.prevent="vertexServiceAccountDragActive = false"
            @drop.prevent="handleVertexServiceAccountDrop"
          >
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div class="min-w-0">
                <div class="flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
                  <Icon name="upload" size="sm" />
                  <span>{{ vertexClientEmail ? t('admin.accounts.vertexSaJsonLoaded') : t('admin.accounts.vertexSaJsonDrop') REDACTEDREDACTED</span>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ vertexClientEmail ? t('admin.accounts.vertexSaJsonKeyHidden') : t('admin.accounts.vertexSaJsonDropHint') REDACTEDREDACTED
                </p>
              </div>
              <button
                type="button"
                class="btn btn-secondary shrink-0"
                @click="vertexServiceAccountFileInput?.click()"
              >
                <Icon name="upload" size="sm" />
                {{ t('admin.accounts.vertexSaJsonSelectBtn') REDACTEDREDACTED
              </button>
            </div>
            <div
              v-if="vertexClientEmail"
              class="mt-3 rounded-md border border-sky-200 bg-white px-3 py-2 text-xs text-sky-900 dark:border-sky-800/50 dark:bg-dark-800 dark:text-sky-200"
            >
              <div class="truncate">Project ID: <span class="font-mono">{{ vertexProjectId REDACTEDREDACTED</span></div>
              <div class="truncate">Client Email: <span class="font-mono">{{ vertexClientEmail REDACTEDREDACTED</span></div>
            </div>
          </div>
          <p class="input-hint">{{ t('admin.accounts.vertexSaJsonUploadHint') REDACTEDREDACTED</p>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">Project ID</label>
            <input
              v-model="vertexProjectId"
              type="text"
              class="input font-mono"
              readonly
              :placeholder="t('admin.accounts.vertexProjectIdPlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">Location</label>
            <select
              v-model="vertexLocation"
              required
              class="input font-mono"
            >
              <optgroup
                v-for="group in VERTEX_LOCATION_OPTIONS"
                :key="group.label"
                :label="group.label"
              >
                <option
                  v-for="option in group.options"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label REDACTEDREDACTED
                </option>
              </optgroup>
            </select>
            <p class="input-hint">{{ t('admin.accounts.vertexLocationHint') REDACTEDREDACTED</p>
          </div>
        </div>
      </div>

      <!-- Antigravity model restriction (applies to OAuth + Upstream) -->
      <!-- Antigravity 只支持模型映射模式，不支持白名单模式 -->
      <div v-if="form.platform === 'antigravity'" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.modelRestriction') REDACTEDREDACTED</label>

        <!-- Mapping Mode Only (no toggle for Antigravity) -->
        <div>
          <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
            <p class="text-xs text-purple-700 dark:text-purple-400">
              {{ t('admin.accounts.mapRequestModels') REDACTEDREDACTED
            </p>
          </div>

          <div v-if="antigravityModelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in antigravityModelMappings"
              :key="getAntigravityModelMappingKey(mapping)"
              class="space-y-1"
            >
              <div class="flex items-center gap-2">
                <input
                  v-model="mapping.from"
                  type="text"
                  :class="[
                    'input flex-1',
                    !isValidWildcardPattern(mapping.from) ? 'border-red-500 dark:border-red-500' : ''
                  ]"
                  :placeholder="t('admin.accounts.requestModel')"
                />
                <svg class="h-4 w-4 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  :class="[
                    'input flex-1',
                    mapping.to.includes('*') ? 'border-red-500 dark:border-red-500' : ''
                  ]"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeAntigravityModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
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
              <!-- 校验错误提示 -->
              <p v-if="!isValidWildcardPattern(mapping.from)" class="text-xs text-red-500">
                {{ t('admin.accounts.wildcardOnlyAtEnd') REDACTEDREDACTED
              </p>
              <p v-if="mapping.to.includes('*')" class="text-xs text-red-500">
                {{ t('admin.accounts.targetNoWildcard') REDACTEDREDACTED
              </p>
            </div>
          </div>

          <button
            type="button"
            @click="addAntigravityModelMapping"
            class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
          >
            <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            {{ t('admin.accounts.addMapping') REDACTEDREDACTED
          </button>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in antigravityPresetMappings"
              :key="preset.label"
              type="button"
              @click="addAntigravityPresetMapping(preset.from, preset.to)"
              :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
            >
              + {{ preset.label REDACTEDREDACTED
            </button>
          </div>
        </div>
      </div>

      <!-- Add Method (only for Anthropic OAuth-based type) -->
      <div v-if="form.platform === 'anthropic' && isOAuthFlow">
        <label class="input-label">{{ t('admin.accounts.addMethod') REDACTEDREDACTED</label>
        <div class="mt-2 flex gap-4">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.types.oauth') REDACTEDREDACTED</span>
          </label>
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="setup-token"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{
              t('admin.accounts.setupTokenLongLived')
            REDACTEDREDACTED</span>
          </label>
        </div>
      </div>

      <!-- API Key input (only for apikey type, excluding Antigravity which has its own fields) -->
      <div v-if="form.type === 'apikey' && form.platform !== 'antigravity'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') REDACTEDREDACTED</label>
          <input
            v-model="apiKeyBaseUrl"
            type="text"
            class="input"
            :placeholder="
              form.platform === 'openai'
                ? 'https://api.openai.com'
                : form.platform === 'gemini'
                  ? 'https://generativelanguage.googleapis.com'
                  : 'https://api.anthropic.com'
            "
          />
          <p class="input-hint">{{ baseUrlHint REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKeyRequired') REDACTEDREDACTED</label>
          <input
            v-model="apiKeyValue"
            type="password"
            required
            class="input font-mono"
            :placeholder="
              form.platform === 'openai'
                ? 'sk-proj-...'
                : form.platform === 'gemini'
                  ? 'AIza...'
                  : 'sk-ant-...'
            "
          />
          <p class="input-hint">{{ apiKeyHint REDACTEDREDACTED</p>
        </div>

        <!-- Gemini API Key tier selection -->
        <div v-if="form.platform === 'gemini'">
          <label class="input-label">{{ t('admin.accounts.gemini.tier.label') REDACTEDREDACTED</label>
          <select v-model="geminiTierAIStudio" class="input">
            <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') REDACTEDREDACTED</option>
            <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') REDACTEDREDACTED</option>
          </select>
          <p class="input-hint">{{ t('admin.accounts.gemini.tier.aiStudioHint') REDACTEDREDACTED</p>
        </div>

        <!-- Model Restriction Section (Antigravity 已在上层条件排除) -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') REDACTEDREDACTED</label>

          <div
            v-if="isOpenAIModelRestrictionDisabled"
            class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
          >
            <p class="text-xs text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') REDACTEDREDACTED
            </p>
          </div>

          <template v-else>
            <!-- Mode Toggle -->
            <div class="mb-4 flex gap-2">
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
                {{ t('admin.accounts.modelWhitelist') REDACTEDREDACTED
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
                {{ t('admin.accounts.modelMapping') REDACTEDREDACTED
              </button>
            </div>

            <!-- Whitelist Mode -->
            <div v-if="modelRestrictionMode === 'whitelist'">
              <ModelWhitelistSelector v-model="allowedModels" :platform="form.platform" :sync-credentials="syncPreviewCredentials" />
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.selectedModels', { count: allowedModels.length REDACTED) REDACTEDREDACTED
                <span v-if="allowedModels.length === 0">{{
                  t('admin.accounts.supportsAllModels')
                REDACTEDREDACTED</span>
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
                  {{ t('admin.accounts.mapRequestModels') REDACTEDREDACTED
                </p>
              </div>

            <!-- Model Mapping List -->
            <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
              <div
                v-for="(mapping, index) in modelMappings"
                :key="getModelMappingKey(mapping)"
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
                  @click="removeModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
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
              @click="addModelMapping"
              class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
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
              {{ t('admin.accounts.addMapping') REDACTEDREDACTED
            </button>

              <!-- Quick Add Buttons -->
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="preset in presetMappings"
                  :key="preset.label"
                  type="button"
                  @click="addPresetMapping(preset.from, preset.to)"
                  :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
                >
                  + {{ preset.label REDACTEDREDACTED
                </button>
              </div>
            </div>
          </template>
        </div>

        <!-- Pool Mode Section -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.poolMode') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.poolModeHint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="poolModeEnabled = !poolModeEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                poolModeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  poolModeEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="poolModeEnabled" class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
            <p class="text-xs text-blue-700 dark:text-blue-400">
              <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.poolModeInfo') REDACTEDREDACTED
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') REDACTEDREDACTED</label>
            <input
              v-model.number="poolModeRetryCount"
              type="number"
              min="0"
              :max="MAX_POOL_MODE_RETRY_COUNT"
              step="1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t('admin.accounts.poolModeRetryCountHint', {
                  default: DEFAULT_POOL_MODE_RETRY_COUNT,
                  max: MAX_POOL_MODE_RETRY_COUNT
                REDACTED)
              REDACTEDREDACTED
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryStatusCodes') REDACTEDREDACTED</label>
            <input
              v-model="poolModeRetryStatusCodesInput"
              type="text"
              class="input"
              :placeholder="DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ')"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.poolModeRetryStatusCodesHint', { default: DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ') REDACTED) REDACTEDREDACTED
            </p>
          </div>
        </div>

        <!-- Custom Error Codes Section -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.customErrorCodesHint') REDACTEDREDACTED
              </p>
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
            <div class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
              <p class="text-xs text-amber-700 dark:text-amber-400">
                <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
                {{ t('admin.accounts.customErrorCodesWarning') REDACTEDREDACTED
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
                    ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                {{ code.value REDACTEDREDACTED {{ code.label REDACTEDREDACTED
              </button>
            </div>

            <!-- Manual input -->
            <div class="flex items-center gap-2">
              <input
                v-model.number="customErrorCodeInput"
                type="number"
                min="100"
                max="599"
                class="input flex-1"
                :placeholder="t('admin.accounts.enterErrorCode')"
                @keyup.enter="addCustomErrorCode"
              />
              <button type="button" @click="addCustomErrorCode" class="btn btn-secondary px-3">
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
                {{ code REDACTEDREDACTED
                <button
                  type="button"
                  @click="removeErrorCode(code)"
                  class="hover:text-red-900 dark:hover:text-red-300"
                >
                  <Icon name="x" size="sm" :stroke-width="2" />
                </button>
              </span>
              <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
                {{ t('admin.accounts.noneSelectedUsesDefault') REDACTEDREDACTED
              </span>
            </div>
          </div>
        </div>

      </div>

      <!-- Bedrock credentials (only for Anthropic Bedrock type) -->
      <div v-if="form.platform === 'anthropic' && accountCategory === 'bedrock'" class="space-y-4">
        <!-- Auth Mode Radio -->
        <div>
          <label class="input-label">{{ t('admin.accounts.bedrockAuthMode') REDACTEDREDACTED</label>
          <div class="mt-2 flex gap-4">
            <label class="flex cursor-pointer items-center">
              <input
                v-model="bedrockAuthMode"
                type="radio"
                value="sigv4"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockAuthModeSigv4') REDACTEDREDACTED</span>
            </label>
            <label class="flex cursor-pointer items-center">
              <input
                v-model="bedrockAuthMode"
                type="radio"
                value="apikey"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockAuthModeApikey') REDACTEDREDACTED</span>
            </label>
          </div>
        </div>

        <!-- SigV4 fields -->
        <template v-if="bedrockAuthMode === 'sigv4'">
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockAccessKeyId') REDACTEDREDACTED</label>
            <input
              v-model="bedrockAccessKeyId"
              type="text"
              required
              class="input font-mono"
              placeholder="AKIA..."
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockSecretAccessKey') REDACTEDREDACTED</label>
            <input
              v-model="bedrockSecretAccessKey"
              type="password"
              required
              class="input font-mono"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockSessionToken') REDACTEDREDACTED</label>
            <input
              v-model="bedrockSessionToken"
              type="password"
              class="input font-mono"
            />
            <p class="input-hint">{{ t('admin.accounts.bedrockSessionTokenHint') REDACTEDREDACTED</p>
          </div>
        </template>

        <!-- API Key field -->
        <div v-if="bedrockAuthMode === 'apikey'">
          <label class="input-label">{{ t('admin.accounts.bedrockApiKeyInput') REDACTEDREDACTED</label>
          <input
            v-model="bedrockApiKeyValue"
            type="password"
            required
            class="input font-mono"
          />
        </div>

        <!-- Shared: Region -->
        <div>
          <label class="input-label">{{ t('admin.accounts.bedrockRegion') REDACTEDREDACTED</label>
          <select v-model="bedrockRegion" class="input">
            <optgroup label="US">
              <option value="us-east-1">us-east-1 (N. Virginia)</option>
              <option value="us-east-2">us-east-2 (Ohio)</option>
              <option value="us-west-1">us-west-1 (N. California)</option>
              <option value="us-west-2">us-west-2 (Oregon)</option>
              <option value="us-gov-east-1">us-gov-east-1 (GovCloud US-East)</option>
              <option value="us-gov-west-1">us-gov-west-1 (GovCloud US-West)</option>
            </optgroup>
            <optgroup label="Europe">
              <option value="eu-west-1">eu-west-1 (Ireland)</option>
              <option value="eu-west-2">eu-west-2 (London)</option>
              <option value="eu-west-3">eu-west-3 (Paris)</option>
              <option value="eu-central-1">eu-central-1 (Frankfurt)</option>
              <option value="eu-central-2">eu-central-2 (Zurich)</option>
              <option value="eu-south-1">eu-south-1 (Milan)</option>
              <option value="eu-south-2">eu-south-2 (Spain)</option>
              <option value="eu-north-1">eu-north-1 (Stockholm)</option>
            </optgroup>
            <optgroup label="Asia Pacific">
              <option value="ap-northeast-1">ap-northeast-1 (Tokyo)</option>
              <option value="ap-northeast-2">ap-northeast-2 (Seoul)</option>
              <option value="ap-northeast-3">ap-northeast-3 (Osaka)</option>
              <option value="ap-south-1">ap-south-1 (Mumbai)</option>
              <option value="ap-south-2">ap-south-2 (Hyderabad)</option>
              <option value="ap-southeast-1">ap-southeast-1 (Singapore)</option>
              <option value="ap-southeast-2">ap-southeast-2 (Sydney)</option>
            </optgroup>
            <optgroup label="Canada">
              <option value="ca-central-1">ca-central-1 (Canada)</option>
            </optgroup>
            <optgroup label="South America">
              <option value="sa-east-1">sa-east-1 (São Paulo)</option>
            </optgroup>
          </select>
          <p class="input-hint">{{ t('admin.accounts.bedrockRegionHint') REDACTEDREDACTED</p>
        </div>

        <!-- Shared: Force Global -->
        <div>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              v-model="bedrockForceGlobal"
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockForceGlobal') REDACTEDREDACTED</span>
          </label>
          <p class="input-hint mt-1">{{ t('admin.accounts.bedrockForceGlobalHint') REDACTEDREDACTED</p>
        </div>

        <!-- Model Restriction Section for Bedrock -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') REDACTEDREDACTED</label>

          <!-- Mode Toggle -->
          <div class="mb-4 flex gap-2">
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
              {{ t('admin.accounts.modelWhitelist') REDACTEDREDACTED
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
              {{ t('admin.accounts.modelMapping') REDACTEDREDACTED
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <ModelWhitelistSelector v-model="allowedModels" platform="anthropic" :sync-credentials="syncPreviewCredentials" />
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length REDACTED) REDACTEDREDACTED
              <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') REDACTEDREDACTED</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else class="space-y-3">
            <div v-for="(mapping, index) in modelMappings" :key="index" class="flex items-center gap-2">
              <input v-model="mapping.from" type="text" class="input flex-1" :placeholder="t('admin.accounts.fromModel')" />
              <span class="text-gray-400">→</span>
              <input v-model="mapping.to" type="text" class="input flex-1" :placeholder="t('admin.accounts.toModel')" />
              <button type="button" @click="modelMappings.splice(index, 1)" class="text-red-500 hover:text-red-700">
                <Icon name="trash" size="sm" />
              </button>
            </div>
            <button type="button" @click="modelMappings.push({ from: '', to: '' REDACTED)" class="btn btn-secondary text-sm">
              + {{ t('admin.accounts.addMapping') REDACTEDREDACTED
            </button>
            <!-- Bedrock Preset Mappings -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in bedrockPresets"
                :key="preset.from"
                type="button"
                @click="addPresetMapping(preset.from, preset.to)"
                :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
              >
                + {{ preset.label REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>

        <!-- Pool Mode Section for Bedrock -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.poolMode') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.poolModeHint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="poolModeEnabled = !poolModeEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                poolModeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  poolModeEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="poolModeEnabled" class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
            <p class="text-xs text-blue-700 dark:text-blue-400">
              <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.poolModeInfo') REDACTEDREDACTED
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') REDACTEDREDACTED</label>
            <input
              v-model.number="poolModeRetryCount"
              type="number"
              min="0"
              :max="MAX_POOL_MODE_RETRY_COUNT"
              step="1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t('admin.accounts.poolModeRetryCountHint', {
                  default: DEFAULT_POOL_MODE_RETRY_COUNT,
                  max: MAX_POOL_MODE_RETRY_COUNT
                REDACTED)
              REDACTEDREDACTED
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryStatusCodes') REDACTEDREDACTED</label>
            <input
              v-model="poolModeRetryStatusCodesInput"
              type="text"
              class="input"
              :placeholder="DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ')"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.poolModeRetryStatusCodesHint', { default: DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ') REDACTED) REDACTEDREDACTED
            </p>
          </div>
        </div>
      </div>

      <!-- 配额控制 (Anthropic apikey/bedrock: 配额限制 + 亲和) -->
      <div
        v-if="form.platform === 'anthropic' && (form.type === 'apikey' || form.type === 'bedrock')"
        class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
      >
        <div class="mb-3">
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') REDACTEDREDACTED</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.hint') REDACTEDREDACTED
          </p>
        </div>
        <QuotaLimitCard
          :totalLimit="editQuotaLimit"
          :dailyLimit="editQuotaDailyLimit"
          :weeklyLimit="editQuotaWeeklyLimit"
          :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
          :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled"
          :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold"
          :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
          :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled"
          :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold"
          :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
          :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled"
          :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold"
          :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
          :dailyResetMode="editDailyResetMode"
          :dailyResetHour="editDailyResetHour"
          :weeklyResetMode="editWeeklyResetMode"
          :weeklyResetDay="editWeeklyResetDay"
          :weeklyResetHour="editWeeklyResetHour"
          :resetTimezone="editResetTimezone"
          @update:totalLimit="editQuotaLimit = $event"
          @update:dailyLimit="editQuotaDailyLimit = $event"
          @update:weeklyLimit="editQuotaWeeklyLimit = $event"
          @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event"
          @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event"
          @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
          @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event"
          @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event"
          @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
          @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event"
          @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event"
          @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
          @update:dailyResetMode="editDailyResetMode = $event"
          @update:dailyResetHour="editDailyResetHour = $event"
          @update:weeklyResetMode="editWeeklyResetMode = $event"
          @update:weeklyResetDay="editWeeklyResetDay = $event"
          @update:weeklyResetHour="editWeeklyResetHour = $event"
          @update:resetTimezone="editResetTimezone = $event"
        />
      </div>

      <!-- 配额控制 (非 Anthropic apikey/bedrock) -->
      <div
        v-else-if="form.type === 'apikey' || form.type === 'bedrock'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
      >
        <div class="mb-3">
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') REDACTEDREDACTED</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaLimitHint') REDACTEDREDACTED
          </p>
        </div>
        <QuotaLimitCard
          :totalLimit="editQuotaLimit"
          :dailyLimit="editQuotaDailyLimit"
          :weeklyLimit="editQuotaWeeklyLimit"
          :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
          :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled"
          :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold"
          :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
          :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled"
          :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold"
          :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
          :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled"
          :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold"
          :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
          :dailyResetMode="editDailyResetMode"
          :dailyResetHour="editDailyResetHour"
          :weeklyResetMode="editWeeklyResetMode"
          :weeklyResetDay="editWeeklyResetDay"
          :weeklyResetHour="editWeeklyResetHour"
          :resetTimezone="editResetTimezone"
          @update:totalLimit="editQuotaLimit = $event"
          @update:dailyLimit="editQuotaDailyLimit = $event"
          @update:weeklyLimit="editQuotaWeeklyLimit = $event"
          @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event"
          @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event"
          @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
          @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event"
          @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event"
          @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
          @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event"
          @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event"
          @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
          @update:dailyResetMode="editDailyResetMode = $event"
          @update:dailyResetHour="editDailyResetHour = $event"
          @update:weeklyResetMode="editWeeklyResetMode = $event"
          @update:weeklyResetDay="editWeeklyResetDay = $event"
          @update:weeklyResetHour="editWeeklyResetHour = $event"
          @update:resetTimezone="editResetTimezone = $event"
        />
      </div>

      <!-- OpenAI OAuth Model Mapping (OAuth 类型没有 apikey 容器，需要独立的模型映射区域) -->
      <div
        v-if="form.platform === 'openai' && accountCategory === 'oauth-based'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <label class="input-label">{{ t('admin.accounts.modelRestriction') REDACTEDREDACTED</label>

        <div
          v-if="isOpenAIModelRestrictionDisabled"
          class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
        >
          <p class="text-xs text-amber-700 dark:text-amber-400">
            {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') REDACTEDREDACTED
          </p>
        </div>

        <template v-else>
          <!-- Mode Toggle -->
          <div class="mb-4 flex gap-2">
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
              {{ t('admin.accounts.modelWhitelist') REDACTEDREDACTED
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
              {{ t('admin.accounts.modelMapping') REDACTEDREDACTED
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <ModelWhitelistSelector v-model="allowedModels" :platform="form.platform" :sync-credentials="syncPreviewCredentials" />
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length REDACTED) REDACTEDREDACTED
              <span v-if="allowedModels.length === 0">{{
                t('admin.accounts.supportsAllModels')
              REDACTEDREDACTED</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else>
            <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
              <p class="text-xs text-purple-700 dark:text-purple-400">
                {{ t('admin.accounts.mapRequestModels') REDACTEDREDACTED
              </p>
            </div>

            <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
              <div
                v-for="(mapping, index) in modelMappings"
                :key="'oauth-' + getModelMappingKey(mapping)"
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
                  @click="removeModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
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
              @click="addModelMapping"
              class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
            >
              + {{ t('admin.accounts.addMapping') REDACTEDREDACTED
            </button>

            <!-- Quick Add Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in presetMappings"
                :key="'oauth-' + preset.label"
                type="button"
                @click="addPresetMapping(preset.from, preset.to)"
                :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
              >
                + {{ preset.label REDACTEDREDACTED
              </button>
            </div>
          </div>
        </template>
      </div>

      <!-- Temp Unschedulable Rules -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.hint') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="tempUnschedEnabled = !tempUnschedEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              tempUnschedEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                tempUnschedEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>

        <div v-if="tempUnschedEnabled" class="space-y-3">
          <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
                {{ t('admin.accounts.tempUnschedulable.notice') REDACTEDREDACTED
              </p>
            </div>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in tempUnschedPresets"
              :key="preset.label"
              type="button"
              @click="addTempUnschedRule(preset.rule)"
              class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
            >
              + {{ preset.label REDACTEDREDACTED
            </button>
          </div>

          <div v-if="tempUnschedRules.length > 0" class="space-y-3">
            <div
              v-for="(rule, index) in tempUnschedRules"
              :key="getTempUnschedRuleKey(rule)"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="mb-2 flex items-center justify-between">
                <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 REDACTED) REDACTEDREDACTED
                </span>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    :disabled="index === 0"
                    @click="moveTempUnschedRule(index, -1)"
                    class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                  >
                    <Icon name="chevronUp" size="sm" :stroke-width="2" />
                  </button>
                  <button
                    type="button"
                    :disabled="index === tempUnschedRules.length - 1"
                    @click="moveTempUnschedRule(index, 1)"
                    class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    @click="removeTempUnschedRule(index)"
                    class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
                  >
                    <Icon name="x" size="sm" :stroke-width="2" />
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') REDACTEDREDACTED</label>
                  <input
                    v-model.number="rule.error_code"
                    type="number"
                    min="100"
                    max="599"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.errorCodePlaceholder')"
                  />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') REDACTEDREDACTED</label>
                  <input
                    v-model.number="rule.duration_minutes"
                    type="number"
                    min="1"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
                  />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') REDACTEDREDACTED</label>
                  <input
                    v-model="rule.keywords"
                    type="text"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
                  />
                  <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') REDACTEDREDACTED</p>
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') REDACTEDREDACTED</label>
                  <input
                    v-model="rule.description"
                    type="text"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.descriptionPlaceholder')"
                  />
                </div>
              </div>
            </div>
          </div>

          <button
            type="button"
            @click="addTempUnschedRule()"
            class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
          >
            <svg
              class="mr-1 inline h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            {{ t('admin.accounts.tempUnschedulable.addRule') REDACTEDREDACTED
          </button>
        </div>
      </div>

      <!-- Intercept Warmup Requests (Anthropic/Antigravity) -->
      <div
        v-if="form.platform === 'anthropic' || form.platform === 'antigravity'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{
              t('admin.accounts.interceptWarmupRequests')
            REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') REDACTEDREDACTED
            </p>
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

      <!-- 配额控制 (Anthropic OAuth/SetupToken: 亲和 + 窗口费用 + 会话 + RPM 等) -->
      <div
        v-if="form.platform === 'anthropic' && accountCategory === 'oauth-based'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
      >
        <div class="mb-3">
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') REDACTEDREDACTED</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.hint') REDACTEDREDACTED
          </p>
        </div>

        <!-- Window Cost Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.windowCost.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.windowCost.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="windowCostEnabled = !windowCostEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                windowCostEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  windowCostEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="windowCostEnabled" class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.limit') REDACTEDREDACTED</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
                <input
                  v-model.number="windowCostLimit"
                  type="number"
                  min="0"
                  step="1"
                  class="input pl-7"
                  :placeholder="t('admin.accounts.quotaControl.windowCost.limitPlaceholder')"
                />
              </div>
              <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.limitHint') REDACTEDREDACTED</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.stickyReserve') REDACTEDREDACTED</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
                <input
                  v-model.number="windowCostStickyReserve"
                  type="number"
                  min="0"
                  step="1"
                  class="input pl-7"
                  :placeholder="t('admin.accounts.quotaControl.windowCost.stickyReservePlaceholder')"
                />
              </div>
              <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.stickyReserveHint') REDACTEDREDACTED</p>
            </div>
          </div>
        </div>

        <!-- Session Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionLimit.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.sessionLimit.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="sessionLimitEnabled = !sessionLimitEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                sessionLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  sessionLimitEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="sessionLimitEnabled" class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessions') REDACTEDREDACTED</label>
              <input
                v-model.number="maxSessions"
                type="number"
                min="1"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessionsPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessionsHint') REDACTEDREDACTED</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeout') REDACTEDREDACTED</label>
              <div class="relative">
                <input
                  v-model.number="sessionIdleTimeout"
                  type="number"
                  min="1"
                  step="1"
                  class="input pr-12"
                  :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeoutPlaceholder')"
                />
                <span class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">{{ t('common.minutes') REDACTEDREDACTED</span>
              </div>
              <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeoutHint') REDACTEDREDACTED</p>
            </div>
          </div>
        </div>

        <!-- RPM Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.rpmLimit.hint') REDACTEDREDACTED
              </p>
            </div>
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

          <div v-if="rpmLimitEnabled" class="space-y-4">
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') REDACTEDREDACTED</label>
              <input
                v-model.number="baseRpm"
                type="number"
                min="1"
                max="1000"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') REDACTEDREDACTED</p>
            </div>

            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') REDACTEDREDACTED</label>
              <div class="flex gap-2">
                <button
                  type="button"
                  @click="rpmStrategy = 'tiered'"
                  :class="[
                    'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                    rpmStrategy === 'tiered'
                      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                  ]"
                >
                  <div class="text-center">
                    <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') REDACTEDREDACTED</div>
                    <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyTieredHint') REDACTEDREDACTED</div>
                  </div>
                </button>
                <button
                  type="button"
                  @click="rpmStrategy = 'sticky_exempt'"
                  :class="[
                    'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                    rpmStrategy === 'sticky_exempt'
                      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                  ]"
                >
                  <div class="text-center">
                    <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') REDACTEDREDACTED</div>
                    <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExemptHint') REDACTEDREDACTED</div>
                  </div>
                </button>
              </div>
            </div>

            <div v-if="rpmStrategy === 'tiered'">
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') REDACTEDREDACTED</label>
              <input
                v-model.number="rpmStickyBuffer"
                type="number"
                min="1"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') REDACTEDREDACTED</p>
            </div>

          </div>

          <!-- 用户消息限速模式（独立于 RPM 开关，始终可见） -->
          <div class="mt-4">
            <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueue') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400 mb-2">
              {{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueueHint') REDACTEDREDACTED
            </p>
            <div class="flex space-x-2">
              <button type="button" v-for="opt in umqModeOptions" :key="opt.value"
                @click="userMsgQueueMode = opt.value"
                :class="[
                  'px-3 py-1.5 text-sm rounded-md border transition-colors',
                  userMsgQueueMode === opt.value
                    ? 'bg-primary-600 text-white border-primary-600'
                    : 'bg-white dark:bg-dark-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-dark-500 hover:bg-gray-50 dark:hover:bg-dark-600'
                ]">
                {{ opt.label REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>

        <!-- TLS Fingerprint -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.tlsFingerprint.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.tlsFingerprint.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="tlsFingerprintEnabled = !tlsFingerprintEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                tlsFingerprintEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  tlsFingerprintEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <!-- Profile selector -->
          <div v-if="tlsFingerprintEnabled" class="mt-3">
            <select v-model="tlsFingerprintProfileId" class="input">
              <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') REDACTEDREDACTED</option>
              <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') REDACTEDREDACTED</option>
              <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name REDACTEDREDACTED</option>
            </select>
          </div>
        </div>

        <!-- Session ID Masking -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionIdMasking.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.sessionIdMasking.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="sessionIdMaskingEnabled = !sessionIdMaskingEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                sessionIdMaskingEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  sessionIdMaskingEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
        </div>

        <!-- Cache TTL Override -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.cacheTTLOverride.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.cacheTTLOverride.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="cacheTTLOverrideEnabled = !cacheTTLOverrideEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                cacheTTLOverrideEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  cacheTTLOverrideEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="cacheTTLOverrideEnabled" class="mt-3">
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.cacheTTLOverride.target') REDACTEDREDACTED</label>
            <select
              v-model="cacheTTLOverrideTarget"
              class="mt-1 block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700 dark:text-white"
            >
              <option value="5m">5m</option>
              <option value="1h">1h</option>
            </select>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.quotaControl.cacheTTLOverride.targetHint') REDACTEDREDACTED
            </p>
          </div>
        </div>

        <!-- Custom Base URL Relay -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.customBaseUrl.label') REDACTEDREDACTED</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.customBaseUrl.hint') REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              @click="customBaseUrlEnabled = !customBaseUrlEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                customBaseUrlEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  customBaseUrlEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="customBaseUrlEnabled" class="mt-3">
            <input
              v-model="customBaseUrl"
              type="text"
              class="input"
              :placeholder="t('admin.accounts.quotaControl.customBaseUrl.urlHint')"
            />
          </div>
        </div>
      </div>

      <div>
        <div class="mb-1 flex items-center gap-2">
          <label class="input-label mb-0">{{ t('admin.accounts.proxy') REDACTEDREDACTED</label>
          <ProxyAdBanner />
        </div>
        <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
      </div>

      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') REDACTEDREDACTED</label>
          <input v-model.number="form.concurrency" type="number" min="1" class="input"
            @input="form.concurrency = Math.max(1, form.concurrency || 1)" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.loadFactor') REDACTEDREDACTED</label>
          <input v-model.number="form.load_factor" type="number" min="1"
            class="input" :placeholder="String(form.concurrency || 1)"
            @input="form.load_factor = (form.load_factor &amp;&amp; form.load_factor >= 1) ? form.load_factor : null" />
          <p class="input-hint">{{ t('admin.accounts.loadFactorHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') REDACTEDREDACTED</label>
          <input
            v-model.number="form.priority"
            type="number"
            min="1"
            class="input"
            data-tour="account-form-priority"
          />
          <p class="input-hint">{{ t('admin.accounts.priorityHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') REDACTEDREDACTED</label>
          <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
          <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') REDACTEDREDACTED</p>
        </div>
      </div>
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.expiresAt') REDACTEDREDACTED</label>
        <input v-model="expiresAtInput" type="datetime-local" class="input" />
        <p class="input-hint">{{ t('admin.accounts.expiresAtHint') REDACTEDREDACTED</p>
      </div>

      <!-- OpenAI 自动透传开关（OAuth/API Key） -->
      <div
        v-if="form.platform === 'openai'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.oauthPassthrough') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.oauthPassthroughDesc') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="openaiPassthroughEnabled = !openaiPassthroughEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              openaiPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
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

      <!-- OpenAI WS Mode 三态（off/ctx_pool/passthrough） -->
      <div
        v-if="form.platform === 'openai' && (accountCategory === 'oauth-based' || accountCategory === 'apikey')"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.wsModeDesc') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t(openAIWSModeConcurrencyHintKey) REDACTEDREDACTED
            </p>
          </div>
          <div class="w-52">
            <Select v-model="openaiResponsesWebSocketV2Mode" :options="openAIWSModeOptions" />
          </div>
        </div>
      </div>

      <!-- Anthropic API Key 自动透传开关 -->
      <div
        v-if="form.platform === 'anthropic' && accountCategory === 'apikey'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyPassthrough') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.apiKeyPassthroughDesc') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="anthropicPassthroughEnabled = !anthropicPassthroughEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              anthropicPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                anthropicPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Anthropic API Key: Web Search Emulation (hidden when global disabled) -->
      <div
        v-if="form.platform === 'anthropic' && accountCategory === 'apikey' && webSearchGlobalEnabled"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.webSearchEmulation') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.webSearchEmulationDesc') REDACTEDREDACTED
            </p>
          </div>
          <select v-model="webSearchEmulationMode" class="input w-24 text-sm">
            <option value="default">{{ t('admin.accounts.anthropic.webSearchDefault') REDACTEDREDACTED</option>
            <option value="enabled">{{ t('admin.accounts.anthropic.webSearchEnabled') REDACTEDREDACTED</option>
            <option value="disabled">{{ t('admin.accounts.anthropic.webSearchDisabled') REDACTEDREDACTED</option>
          </select>
        </div>
      </div>

      <!-- OpenAI OAuth Codex 官方客户端限制开关 -->
      <div
        v-if="form.platform === 'openai' && accountCategory === 'oauth-based'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnly') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexCLIOnlyDesc') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="codexCLIOnlyEnabled = !codexCLIOnlyEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <div
          v-if="codexCLIOnlyEnabled"
          class="mt-4 flex items-center justify-between border-l-2 border-gray-200 pl-4 dark:border-dark-600"
        >
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnlyAllowClaudeCode') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexCLIOnlyAllowClaudeCodeDesc') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="codexCLIOnlyAllowClaudeCodeEnabled = !codexCLIOnlyAllowClaudeCodeEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyAllowClaudeCodeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyAllowClaudeCodeEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- OpenAI Compact 能力配置 -->
      <div
        v-if="form.platform === 'openai' && (accountCategory === 'oauth-based' || accountCategory === 'apikey')"
        class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.compactModeDesc') REDACTEDREDACTED
            </p>
          </div>
          <div class="w-44">
            <Select v-model="openAICompactMode" :options="openAICompactModeOptions" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') REDACTEDREDACTED</label>
          <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') REDACTEDREDACTED</p>
          <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in openAICompactModelMappings"
              :key="getOpenAICompactModelMappingKey(mapping)"
              class="flex items-center gap-2"
            >
              <input v-model="mapping.from" type="text" class="input flex-1" :placeholder="t('admin.accounts.fromModel')" />
              <span class="text-gray-400">→</span>
              <input v-model="mapping.to" type="text" class="input flex-1" :placeholder="t('admin.accounts.toModel')" />
              <button type="button" @click="removeOpenAICompactModelMapping(index)" class="text-red-500 hover:text-red-700">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
          <button type="button" @click="addOpenAICompactModelMapping" class="btn btn-secondary text-sm">
            + {{ t('admin.accounts.addMapping') REDACTEDREDACTED
          </button>
        </div>
      </div>

      <!-- OpenAI APIKey Responses API support mode -->
      <div
        v-if="form.platform === 'openai' && accountCategory === 'apikey'"
        class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.responsesMode') REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.responsesModeDesc') REDACTEDREDACTED
            </p>
          </div>
          <div class="w-56">
            <Select
              v-model="openAIResponsesMode"
              :options="openAIResponsesModeOptions"
              :disabled="!openAITextGenerationCapabilityEnabled"
              data-testid="openai-responses-mode-select"
            />
          </div>
        </div>
        <p
          v-if="!openAITextGenerationCapabilityEnabled"
          class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
          data-testid="openai-responses-mode-not-applicable"
        >
          {{ t('admin.accounts.openai.responsesModeTextDisabledHint') REDACTEDREDACTED
        </p>
        <div>
          <label class="input-label mb-2 block">{{ t('admin.accounts.openai.endpointCapabilities') REDACTEDREDACTED</label>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label
              v-for="option in openAIEndpointCapabilityOptions"
              :key="option.value"
              class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
            >
              <input
                type="checkbox"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
                :data-testid="`openai-endpoint-capability-${option.valueREDACTED`"
                :checked="openAIEndpointCapabilities.includes(option.value)"
                @change="toggleOpenAIEndpointCapability(option.value, $event)"
              />
              <span class="text-gray-700 dark:text-gray-200">{{ option.label REDACTEDREDACTED</span>
            </label>
          </div>
          <p class="input-hint">{{ t('admin.accounts.openai.endpointCapabilitiesDesc') REDACTEDREDACTED</p>
        </div>
      </div>

      <div>
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{
              t('admin.accounts.autoPauseOnExpired')
            REDACTEDREDACTED</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.autoPauseOnExpiredDesc') REDACTEDREDACTED
            </p>
          </div>
          <button
            type="button"
            @click="autoPauseOnExpired = !autoPauseOnExpired"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              autoPauseOnExpired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                autoPauseOnExpired ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <!-- Mixed Scheduling (only for antigravity accounts) -->
        <div v-if="form.platform === 'antigravity'" class="flex items-center gap-2">
          <label class="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              v-model="mixedScheduling"
              class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.mixedScheduling') REDACTEDREDACTED
            </span>
          </label>
          <div class="group relative">
            <span
              class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
            >
              ?
            </span>
            <!-- Tooltip（向下显示避免被弹窗裁剪） -->
            <div
              class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
            >
              {{ t('admin.accounts.mixedSchedulingTooltip') REDACTEDREDACTED
              <div
                class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
              ></div>
            </div>
          </div>
        </div>
        <div v-if="form.platform === 'antigravity'" class="mt-3 flex items-center gap-2">
          <label class="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              v-model="allowOverages"
              class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.allowOverages') REDACTEDREDACTED
            </span>
          </label>
          <div class="group relative">
            <span
              class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
            >
              ?
            </span>
            <div
              class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
            >
              {{ t('admin.accounts.allowOveragesTooltip') REDACTEDREDACTED
              <div
                class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
              ></div>
            </div>
          </div>
        </div>

        <!-- Group Selection - 仅标准模式显示 -->
        <GroupSelector
          v-if="!authStore.isSimpleMode"
          v-model="form.group_ids"
          :groups="groups"
          :platform="form.platform"
          :mixed-scheduling="mixedScheduling"
          data-tour="account-form-groups"
        />
      </div>

    </form>

    <!-- Step 2: OAuth Authorization -->
    <div v-else class="space-y-5">
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="form.platform === 'anthropic' ? addMethod : 'oauth'"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentOAuthLoading"
        :error="currentOAuthError"
        :show-help="form.platform === 'anthropic'"
        :show-proxy-warning="form.platform !== 'openai' && !!form.proxy_id"
        :allow-multiple="form.platform === 'anthropic'"
        :show-cookie-option="form.platform === 'anthropic'"
        :show-refresh-token-option="form.platform === 'openai' || form.platform === 'antigravity'"
        :show-mobile-refresh-token-option="form.platform === 'openai'"
        :show-session-token-option="false"
        :show-access-token-option="false"
        :show-codex-session-import-option="form.platform === 'openai'"
        :platform="form.platform"
        :show-project-id="geminiOAuthType === 'code_assist'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleValidateRefreshToken"
        @validate-mobile-refresh-token="handleOpenAIValidateMobileRT"
        @validate-session-token="handleValidateSessionToken"
        @import-codex-session="handleOpenAIImportCodexSession"
      />

    </div>

    <template #footer>
      <div v-if="step === 1" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          type="submit"
          form="create-account-form"
          :disabled="submitting"
          class="btn btn-primary"
          data-tour="account-form-submit"
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
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{
            isOAuthFlow
              ? t('common.next')
              : submitting
                ? t('admin.accounts.creating')
                : t('common.create')
          REDACTEDREDACTED
        </button>
      </div>
      <div v-else class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="goBackToBasicInfo">
          {{ t('common.back') REDACTEDREDACTED
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
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{
            currentOAuthLoading
              ? t('admin.accounts.oauth.verifying')
              : t('admin.accounts.oauth.completeAuth')
          REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Gemini Help Dialog -->
  <BaseDialog
    :show="showGeminiHelpDialog"
    :title="t('admin.accounts.gemini.helpDialog.title')"
    @close="showGeminiHelpDialog = false"
    max-width="max-w-3xl"
  >
    <div class="space-y-6">
      <!-- Setup Guide Section -->
      <div>
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.gemini.setupGuide.title') REDACTEDREDACTED
        </h3>
        <div class="space-y-4">
          <div>
            <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.gemini.setupGuide.checklistTitle') REDACTEDREDACTED
            </p>
            <ul class="list-inside list-disc space-y-1 text-sm text-gray-600 dark:text-gray-400">
              <li>{{ t('admin.accounts.gemini.setupGuide.checklistItems.usIp') REDACTEDREDACTED</li>
              <li>{{ t('admin.accounts.gemini.setupGuide.checklistItems.age') REDACTEDREDACTED</li>
            </ul>
          </div>
          <div>
            <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.gemini.setupGuide.activationTitle') REDACTEDREDACTED
            </p>
            <ul class="list-inside list-disc space-y-1 text-sm text-gray-600 dark:text-gray-400">
              <li>{{ t('admin.accounts.gemini.setupGuide.activationItems.geminiWeb') REDACTEDREDACTED</li>
              <li>{{ t('admin.accounts.gemini.setupGuide.activationItems.gcpProject') REDACTEDREDACTED</li>
            </ul>
            <div class="mt-2 flex flex-wrap gap-2">
              <a
                href="https://policies.google.com/terms"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ t('admin.accounts.gemini.setupGuide.links.countryCheck') REDACTEDREDACTED
              </a>
              <span class="text-gray-400">·</span>
              <a
                href="https://policies.google.com/country-association-form"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                修改归属地
              </a>
              <span class="text-gray-400">·</span>
              <a
                href="https://gemini.google.com/gems/create?hl=en-US&pli=1"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ t('admin.accounts.gemini.setupGuide.links.geminiWebActivation') REDACTEDREDACTED
              </a>
              <span class="text-gray-400">·</span>
              <a
                href="https://console.cloud.google.com"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ t('admin.accounts.gemini.setupGuide.links.gcpProject') REDACTEDREDACTED
              </a>
            </div>
          </div>
        </div>
      </div>

      <!-- Quota Policy Section -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-600">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.gemini.quotaPolicy.title') REDACTEDREDACTED
        </h3>
        <p class="mb-4 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.accounts.gemini.quotaPolicy.note') REDACTEDREDACTED
        </p>
        <div class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead class="bg-gray-50 dark:bg-dark-600">
              <tr>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.channel') REDACTEDREDACTED
                </th>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.account') REDACTEDREDACTED
                </th>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.limits') REDACTEDREDACTED
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.channel') REDACTEDREDACTED
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Free</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsFree') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Pro</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsPro') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Ultra</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsUltra') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.channel') REDACTEDREDACTED
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Standard</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsStandard') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Enterprise</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsEnterprise') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel') REDACTEDREDACTED
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Free</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree') REDACTEDREDACTED
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Paid</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsPaid') REDACTEDREDACTED
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="mt-4 flex flex-wrap gap-3">
          <a
            :href="geminiQuotaDocs.codeAssist"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.quotaPolicy.docs.codeAssist') REDACTEDREDACTED
          </a>
          <a
            :href="geminiQuotaDocs.aiStudio"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.quotaPolicy.docs.aiStudio') REDACTEDREDACTED
          </a>
          <a
            :href="geminiQuotaDocs.vertex"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.quotaPolicy.docs.vertex') REDACTEDREDACTED
          </a>
        </div>
      </div>

      <!-- API Key Links Section -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-600">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.gemini.helpDialog.apiKeySection') REDACTEDREDACTED
        </h3>
        <div class="flex flex-wrap gap-3">
          <a
            :href="geminiHelpLinks.apiKey"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.accountType.apiKeyLink') REDACTEDREDACTED
          </a>
          <a
            :href="geminiHelpLinks.aiStudioPricing"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.accountType.quotaLink') REDACTEDREDACTED
          </a>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="showGeminiHelpDialog = false" type="button" class="btn btn-primary">
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Mixed Channel Warning Dialog -->
  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessageText"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import {
  claudeModels,
  getPresetMappingsByPlatform,
  getModelsByPlatform,
  commonErrorCodes,
  buildModelMappingObject,
  fetchAntigravityDefaultMappings,
  isValidWildcardPattern
REDACTED from '@/composables/useModelWhitelist'
import { useAuthStore REDACTED from '@/stores/auth'
import { adminAPI REDACTED from '@/api/admin'
import { useQuotaNotifyState REDACTED from '@/composables/useQuotaNotifyState'
import {
  useAccountOAuth,
  type AddMethod,
  type AuthInputMethod
REDACTED from '@/composables/useAccountOAuth'
import { useOpenAIOAuth REDACTED from '@/composables/useOpenAIOAuth'
import { useGeminiOAuth REDACTED from '@/composables/useGeminiOAuth'
import { useAntigravityOAuth REDACTED from '@/composables/useAntigravityOAuth'
import type {
  Proxy,
  AdminGroup,
  AccountPlatform,
  AccountType,
  CheckMixedChannelResponse,
  CreateAccountRequest,
  CodexSessionImportMessage,
  OpenAICompactMode,
  OpenAIResponsesMode,
  OpenAIEndpointCapability
REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import ProxyAdBanner from '@/components/common/ProxyAdBanner.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import QuotaLimitCard from '@/components/account/QuotaLimitCard.vue'
import {
  applyAntigravityProjectID,
  applyInterceptWarmup
REDACTED from '@/components/account/credentialsBuilder'
import { formatDateTimeLocalInput, parseDateTimeLocalInput REDACTED from '@/utils/format'
import { createStableObjectKeyResolver REDACTED from '@/utils/stableObjectKey'
import { VERTEX_LOCATION_OPTIONS REDACTED from '@/constants/account'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode
REDACTED from '@/utils/openaiWsMode'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'

// Type for exposed OAuthAuthorizationFlow component
// Note: defineExpose automatically unwraps refs, so we use the unwrapped types
interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  projectId: string
  sessionKey: string
  refreshToken: string
  sessionToken: string
  codexSession: string
  inputMethod: AuthInputMethod
  reset: () => void
REDACTED

const { t REDACTED = useI18n()
const authStore = useAuthStore()

const oauthStepTitle = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.oauth.openai.title')
  if (form.platform === 'gemini') return t('admin.accounts.oauth.gemini.title')
  if (form.platform === 'antigravity') return t('admin.accounts.oauth.antigravity.title')
  return t('admin.accounts.oauth.title')
REDACTED)

// Platform-specific hints for API Key type
const baseUrlHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.baseUrlHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.baseUrlHint')
  return t('admin.accounts.baseUrlHint')
REDACTED)

const apiKeyHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.apiKeyHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.apiKeyHint')
  return t('admin.accounts.apiKeyHint')
REDACTED)

interface Props {
  show: boolean
  proxies: Proxy[]
  groups: AdminGroup[]
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  created: []
REDACTED>()

const appStore = useAppStore()

// OAuth composables
const oauth = useAccountOAuth() // For Anthropic OAuth
const openaiOAuth = useOpenAIOAuth() // For OpenAI OAuth
const geminiOAuth = useGeminiOAuth() // For Gemini OAuth
const antigravityOAuth = useAntigravityOAuth() // For Antigravity OAuth

// Computed: current OAuth state for template binding
const currentAuthUrl = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.authUrl.value
  if (form.platform === 'gemini') return geminiOAuth.authUrl.value
  if (form.platform === 'antigravity') return antigravityOAuth.authUrl.value
  return oauth.authUrl.value
REDACTED)

const currentSessionId = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.sessionId.value
  if (form.platform === 'gemini') return geminiOAuth.sessionId.value
  if (form.platform === 'antigravity') return antigravityOAuth.sessionId.value
  return oauth.sessionId.value
REDACTED)

const currentOAuthLoading = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.loading.value
  if (form.platform === 'gemini') return geminiOAuth.loading.value
  if (form.platform === 'antigravity') return antigravityOAuth.loading.value
  return oauth.loading.value
REDACTED)

const currentOAuthError = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.error.value
  if (form.platform === 'gemini') return geminiOAuth.error.value
  if (form.platform === 'antigravity') return antigravityOAuth.error.value
  return oauth.error.value
REDACTED)

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// Model mapping type
interface ModelMapping {
  from: string
  to: string
REDACTED

interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
REDACTED

// State
const step = ref(1)
const submitting = ref(false)
const accountCategory = ref<'oauth-based' | 'apikey' | 'bedrock' | 'service_account'>('oauth-based') // UI selection for account category
const addMethod = ref<AddMethod>('oauth') // For oauth-based: 'oauth' or 'setup-token'
const apiKeyBaseUrl = ref('https://api.anthropic.com')
const apiKeyValue = ref('')

const syncPreviewCredentials = computed(() => {
  if (!apiKeyValue.value) return undefined
  return {
    platform: form.platform,
    type: form.type,
    base_url: apiKeyBaseUrl.value || undefined,
    api_key: apiKeyValue.value
  REDACTED
REDACTED)

const editQuotaLimit = ref<number | null>(null)
const editQuotaDailyLimit = ref<number | null>(null)
const editQuotaWeeklyLimit = ref<number | null>(null)
const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editDailyResetHour = ref<number | null>(null)
const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editWeeklyResetDay = ref<number | null>(null)
const editWeeklyResetHour = ref<number | null>(null)
const editResetTimezone = ref<string | null>(null)
const modelMappings = ref<ModelMapping[]>([])
const openAICompactModelMappings = ref<ModelMapping[]>([])
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const DEFAULT_POOL_MODE_RETRY_COUNT = 3
const MAX_POOL_MODE_RETRY_COUNT = 10
const DEFAULT_POOL_MODE_RETRY_STATUS_CODES = [401, 403, 429]
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const poolModeRetryStatusCodesInput = ref('')

function parsePoolModeRetryStatusCodes(input: string): number[] {
  if (!input || !input.trim()) return []
  const seen = new Set<number>()
  const out: number[] = []
  for (const token of input.split(/[,\s]+/)) {
    const trimmed = token.trim()
    if (!trimmed) continue
    const n = Number(trimmed)
    if (!Number.isFinite(n) || !Number.isInteger(n)) continue
    if (n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  REDACTED
  return out.sort((a, b) => a - b)
REDACTED
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)
const autoPauseOnExpired = ref(true)
const openaiPassthroughEnabled = ref(false)
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openAIEndpointCapabilities = ref<OpenAIEndpointCapability[]>(['chat_completions', 'embeddings'])
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexCLIOnlyEnabled = ref(false)
const codexCLIOnlyAllowClaudeCodeEnabled = ref(false)
const anthropicPassthroughEnabled = ref(false)
const webSearchEmulationMode = ref('default')
const webSearchGlobalEnabled = ref(false)
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  writeToExtra: writeQuotaNotifyToExtra,
REDACTED = useQuotaNotifyState()

// Load global feature states once
adminAPI.settings.getWebSearchEmulationConfig().then(cfg => {
  webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
REDACTED).catch(() => { webSearchGlobalEnabled.value = false REDACTED)

loadQuotaNotifyGlobal()
const mixedScheduling = ref(false) // For antigravity accounts: enable mixed scheduling
const allowOverages = ref(false) // For antigravity accounts: enable AI Credits overages
const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth') // For antigravity: oauth or upstream
const antigravityProjectId = ref('')
const upstreamBaseUrl = ref('') // For upstream type: base URL
const upstreamApiKey = ref('') // For upstream type: API key
const antigravityModelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const antigravityWhitelistModels = ref<string[]>([])
const antigravityModelMappings = ref<ModelMapping[]>([])
const antigravityPresetMappings = computed(() => getPresetMappingsByPlatform('antigravity'))
const bedrockPresets = computed(() => getPresetMappingsByPlatform('bedrock'))

// Bedrock credentials
const bedrockAuthMode = ref<'sigv4' | 'apikey'>('sigv4')
const bedrockAccessKeyId = ref('')
const bedrockSecretAccessKey = ref('')
const bedrockSessionToken = ref('')
const bedrockRegion = ref('us-east-1')
const bedrockForceGlobal = ref(false)
const bedrockApiKeyValue = ref('')
const vertexServiceAccountFileInput = ref<HTMLInputElement | null>(null)
const vertexServiceAccountJson = ref('')
const vertexProjectId = ref('')
const vertexClientEmail = ref('')
const vertexLocation = ref('global')
const vertexServiceAccountDragActive = ref(false)
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const getModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-model-mapping')
const getOpenAICompactModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-openai-compact-model-mapping')
const getAntigravityModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-antigravity-model-mapping')
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('create-temp-unsched-rule')
const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')
const geminiAIStudioOAuthEnabled = ref(false)
const openAICompactModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') REDACTED,
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') REDACTED,
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') REDACTED
])
const openAIResponsesModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.responsesModeAuto') REDACTED,
  { value: 'force_responses', label: t('admin.accounts.openai.responsesModeForceResponses') REDACTED,
  { value: 'force_chat_completions', label: t('admin.accounts.openai.responsesModeForceChatCompletions') REDACTED
])
const openAITextEndpointCapabilityLabel = computed(() => {
  if (openAIResponsesMode.value === 'force_responses') {
    return t('admin.accounts.openai.capabilityResponses')
  REDACTED
  if (openAIResponsesMode.value === 'force_chat_completions') {
    return t('admin.accounts.openai.capabilityChatCompletions')
  REDACTED
  return t('admin.accounts.openai.capabilityTextAuto')
REDACTED)
const openAIEndpointCapabilityOptions = computed<{ value: OpenAIEndpointCapability; label: string REDACTED[]>(() => [
  { value: 'chat_completions', label: openAITextEndpointCapabilityLabel.value REDACTED,
  { value: 'embeddings', label: t('admin.accounts.openai.capabilityEmbeddings') REDACTED
])
const openAITextGenerationCapabilityEnabled = computed(() =>
  openAIEndpointCapabilities.value.includes('chat_completions')
)

const normalizeOpenAIEndpointCapabilities = (values: OpenAIEndpointCapability[]) => {
  const allowed: OpenAIEndpointCapability[] = ['chat_completions', 'embeddings']
  const selected = allowed.filter((value) => values.includes(value))
  return selected.length > 0 ? selected : allowed
REDACTED

const toggleOpenAIEndpointCapability = (capability: OpenAIEndpointCapability, event?: Event) => {
  if (openAIEndpointCapabilities.value.includes(capability)) {
    if (openAIEndpointCapabilities.value.length <= 1) {
      const input = event?.target as HTMLInputElement | null
      if (input) input.checked = true
      return
    REDACTED
    openAIEndpointCapabilities.value = openAIEndpointCapabilities.value.filter(
      (value) => value !== capability
    )
    if (!openAITextGenerationCapabilityEnabled.value) {
      openAIResponsesMode.value = 'auto'
    REDACTED
    return
  REDACTED
  openAIEndpointCapabilities.value = normalizeOpenAIEndpointCapabilities([
    ...openAIEndpointCapabilities.value,
    capability
  ])
REDACTED

const applyOpenAIEndpointCapabilities = (credentials: Record<string, unknown>) => {
  const capabilities = normalizeOpenAIEndpointCapabilities(openAIEndpointCapabilities.value)
  if (capabilities.length === 2) {
    delete credentials.openai_capabilities
    return
  REDACTED
  credentials.openai_capabilities = capabilities
REDACTED

function buildAntigravityExtra(): Record<string, unknown> | undefined {
  const extra: Record<string, unknown> = {REDACTED
  if (mixedScheduling.value) extra.mixed_scheduling = true
  if (allowOverages.value) extra.allow_overages = true
  return Object.keys(extra).length > 0 ? extra : undefined
REDACTED

const buildOpenAICompactModelMapping = () =>
  buildModelMappingObject('mapping', [], openAICompactModelMappings.value)

const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string REDACTED | null>(
  null
)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)
const showAdvancedOAuth = ref(false)
const showGeminiHelpDialog = ref(false)

// Quota control state (Anthropic OAuth/SetupToken only)
const windowCostEnabled = ref(false)
const windowCostLimit = ref<number | null>(null)
const windowCostStickyReserve = ref<number | null>(null)
const sessionLimitEnabled = ref(false)
const maxSessions = ref<number | null>(null)
const sessionIdleTimeout = ref<number | null>(null)
const rpmLimitEnabled = ref(false)
const baseRpm = ref<number | null>(null)
const rpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
const rpmStickyBuffer = ref<number | null>(null)
const userMsgQueueMode = ref('')
const umqModeOptions = computed(() => [
  { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') REDACTED,
  { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') REDACTED,
  { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') REDACTED,
])
const tlsFingerprintEnabled = ref(false)
const tlsFingerprintProfileId = ref<number | null>(null)
const tlsFingerprintProfiles = ref<{ id: number; name: string REDACTED[]>([])
const sessionIdMaskingEnabled = ref(false)
const cacheTTLOverrideEnabled = ref(false)
const cacheTTLOverrideTarget = ref<string>('5m')
const customBaseUrlEnabled = ref(false)
const customBaseUrl = ref('')

// Gemini tier selection (used as fallback when auto-detection is unavailable/fails)
const geminiTierGoogleOne = ref<'google_one_free' | 'google_ai_pro' | 'google_ai_ultra'>('google_one_free')
const geminiTierGcp = ref<'gcp_standard' | 'gcp_enterprise'>('gcp_standard')
const geminiTierAIStudio = ref<'aistudio_free' | 'aistudio_paid'>('aistudio_free')

const geminiSelectedTier = computed(() => {
  if (form.platform !== 'gemini') return ''
  if (accountCategory.value === 'apikey') return geminiTierAIStudio.value
  switch (geminiOAuthType.value) {
    case 'google_one':
      return geminiTierGoogleOne.value
    case 'code_assist':
      return geminiTierGcp.value
    default:
      return geminiTierAIStudio.value
  REDACTED
REDACTED)

const openAIWSModeOptions = computed(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') REDACTED,
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') REDACTED,
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') REDACTED
])

const openaiResponsesWebSocketV2Mode = computed({
  get: () => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      return openaiAPIKeyResponsesWebSocketV2Mode.value
    REDACTED
    return openaiOAuthResponsesWebSocketV2Mode.value
  REDACTED,
  set: (mode: OpenAIWSMode) => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      openaiAPIKeyResponsesWebSocketV2Mode.value = mode
      return
    REDACTED
    openaiOAuthResponsesWebSocketV2Mode.value = mode
  REDACTED
REDACTED)

const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2Mode.value)
)

const isOpenAIModelRestrictionDisabled = computed(() =>
  form.platform === 'openai' && openaiPassthroughEnabled.value
)

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value) {
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  REDACTED
  return mixedChannelWarningRawMessage.value
REDACTED)

const geminiQuotaDocs = {
  codeAssist: 'https://developers.google.com/gemini-code-assist/resources/quotas',
  aiStudio: 'https://ai.google.dev/pricing',
  vertex: 'https://cloud.google.com/vertex-ai/generative-ai/docs/quotas'
REDACTED

const geminiHelpLinks = {
  apiKey: 'https://aistudio.google.com/app/apikey',
  aiStudioPricing: 'https://ai.google.dev/pricing',
  gcpProject: 'https://console.cloud.google.com/welcome/new',
  geminiWebActivation: 'https://gemini.google.com/gems/create?hl=en-US&pli=1',
  countryCheck: 'https://policies.google.com/terms',
  countryChange: 'https://policies.google.com/country-association-form'
REDACTED

// Computed: current preset mappings based on platform
const presetMappings = computed(() => getPresetMappingsByPlatform(form.platform))
const tempUnschedPresets = computed(() => [
  {
    label: t('admin.accounts.tempUnschedulable.presets.overloadLabel'),
    rule: {
      error_code: 529,
      keywords: 'overloaded, too many',
      duration_minutes: 60,
      description: t('admin.accounts.tempUnschedulable.presets.overloadDesc')
    REDACTED
  REDACTED,
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    REDACTED
  REDACTED,
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    REDACTED
  REDACTED
])

const form = reactive({
  name: '',
  notes: '',
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType, // Will be 'oauth', 'setup-token', or 'apikey'
  credentials: {REDACTED as Record<string, unknown>,
  proxy_id: null as number | null,
  concurrency: 10,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  group_ids: [] as number[],
  expires_at: null as number | null
REDACTED)

// Helper to check if current type needs OAuth flow
const isOAuthFlow = computed(() => {
  // Antigravity upstream 类型不需要 OAuth 流程
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    return false
  REDACTED
  // Bedrock 类型不需要 OAuth 流程
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    return false
  REDACTED
  return accountCategory.value === 'oauth-based'
REDACTED)

const isManualInputMethod = computed(() => {
  return oauthFlowRef.value?.inputMethod === 'manual'
REDACTED)

const expiresAtInput = computed({
  get: () => formatDateTimeLocal(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocal(value)
  REDACTED
REDACTED)

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  if (form.platform === 'openai') {
    return authCode.trim() && openaiOAuth.sessionId.value && !openaiOAuth.loading.value
  REDACTED
  if (form.platform === 'gemini') {
    return authCode.trim() && geminiOAuth.sessionId.value && !geminiOAuth.loading.value
  REDACTED
  if (form.platform === 'antigravity') {
    return authCode.trim() && antigravityOAuth.sessionId.value && !antigravityOAuth.loading.value
  REDACTED
  return authCode.trim() && oauth.sessionId.value && !oauth.loading.value
REDACTED)

// Watchers
watch(
  () => props.show,
  (newVal) => {
    if (newVal) {
      // Load TLS fingerprint profiles
      adminAPI.tlsFingerprintProfiles.list()
        .then(profiles => { tlsFingerprintProfiles.value = profiles.map(p => ({ id: p.id, name: p.name REDACTED)) REDACTED)
        .catch(() => { tlsFingerprintProfiles.value = [] REDACTED)
      // Modal opened - fill related models
      allowedModels.value = [...getModelsByPlatform(form.platform)]
      // Antigravity: 默认使用映射模式并填充默认映射
      if (form.platform === 'antigravity') {
        antigravityModelRestrictionMode.value = 'mapping'
        fetchAntigravityDefaultMappings().then(mappings => {
          antigravityModelMappings.value = [...mappings]
        REDACTED)
        antigravityWhitelistModels.value = []
      REDACTED else {
        antigravityWhitelistModels.value = []
        antigravityModelMappings.value = []
        antigravityModelRestrictionMode.value = 'mapping'
      REDACTED
    REDACTED else {
      resetForm()
    REDACTED
  REDACTED
)

// Sync form.type based on accountCategory, addMethod, and platform-specific type
watch(
  [accountCategory, addMethod, antigravityAccountType, () => form.platform],
  ([category, method, agType]) => {
    // Antigravity upstream 类型（实际创建为 apikey）
    if (form.platform === 'antigravity' && agType === 'upstream') {
      form.type = 'apikey'
      return
    REDACTED
    // Bedrock 类型
    if (form.platform === 'anthropic' && category === 'bedrock') {
      form.type = 'bedrock' as AccountType
      return
    REDACTED
    if ((form.platform === 'gemini' || form.platform === 'anthropic') && category === 'service_account') {
      form.type = 'service_account' as AccountType
    REDACTED else if (category === 'oauth-based') {
      form.type = method as AccountType // 'oauth' or 'setup-token'
    REDACTED else {
      form.type = 'apikey'
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

// Reset platform-specific settings when platform changes
watch(
  () => form.platform,
  (newPlatform) => {
    // Reset base URL based on platform
    apiKeyBaseUrl.value =
      (newPlatform === 'openai')
        ? 'https://api.openai.com'
        : newPlatform === 'gemini'
          ? 'https://generativelanguage.googleapis.com'
          : 'https://api.anthropic.com'
    // Clear model-related settings
    allowedModels.value = []
    modelMappings.value = []
    // Antigravity: 默认使用映射模式并填充默认映射
    if (newPlatform === 'antigravity') {
      antigravityModelRestrictionMode.value = 'mapping'
      fetchAntigravityDefaultMappings().then(mappings => {
        antigravityModelMappings.value = [...mappings]
      REDACTED)
      antigravityWhitelistModels.value = []
      accountCategory.value = 'oauth-based'
      antigravityAccountType.value = 'oauth'
    REDACTED else {
      allowOverages.value = false
      antigravityProjectId.value = ''
      antigravityWhitelistModels.value = []
      antigravityModelMappings.value = []
      antigravityModelRestrictionMode.value = 'mapping'
    REDACTED
    if (newPlatform !== 'gemini' && newPlatform !== 'anthropic' && accountCategory.value === 'service_account') {
      accountCategory.value = 'oauth-based'
    REDACTED
    if (newPlatform !== 'anthropic' && accountCategory.value === 'bedrock') {
      accountCategory.value = 'oauth-based'
    REDACTED
    // Reset Bedrock fields when switching platforms
    bedrockAccessKeyId.value = ''
    bedrockSecretAccessKey.value = ''
    bedrockSessionToken.value = ''
    bedrockRegion.value = 'us-east-1'
    bedrockForceGlobal.value = false
    bedrockAuthMode.value = 'sigv4'
    bedrockApiKeyValue.value = ''
    vertexServiceAccountJson.value = ''
    vertexProjectId.value = ''
    vertexClientEmail.value = ''
    vertexLocation.value = 'global'
    // Reset Anthropic/Antigravity-specific settings when switching to other platforms
    if (newPlatform !== 'anthropic' && newPlatform !== 'antigravity') {
      interceptWarmupRequests.value = false
    REDACTED
    if (newPlatform !== 'openai') {
      openaiPassthroughEnabled.value = false
      openAIEndpointCapabilities.value = ['chat_completions', 'embeddings']
      openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      codexCLIOnlyEnabled.value = false
      codexCLIOnlyAllowClaudeCodeEnabled.value = false
    REDACTED
    if (newPlatform !== 'anthropic') {
      anthropicPassthroughEnabled.value = false
      webSearchEmulationMode.value = 'default'
    REDACTED
    // Reset OAuth states
    oauth.resetState()
    openaiOAuth.resetState()

    geminiOAuth.resetState()
    antigravityOAuth.resetState()
  REDACTED
)

// Gemini AI Studio OAuth availability (requires operator-configured OAuth client)
watch(
  [accountCategory, () => form.platform],
  ([category, platform]) => {
    if (platform === 'openai' && category !== 'oauth-based') {
      codexCLIOnlyEnabled.value = false
      codexCLIOnlyAllowClaudeCodeEnabled.value = false
    REDACTED
    if (platform !== 'anthropic' || category !== 'apikey') {
      anthropicPassthroughEnabled.value = false
      webSearchEmulationMode.value = 'default'
    REDACTED
  REDACTED
)

watch(
  [() => props.show, () => form.platform, accountCategory],
  async ([show, platform, category]) => {
    if (!show || platform !== 'gemini' || category !== 'oauth-based') {
      geminiAIStudioOAuthEnabled.value = false
      return
    REDACTED
    const caps = await geminiOAuth.getCapabilities()
    geminiAIStudioOAuthEnabled.value = !!caps?.ai_studio_oauth_enabled
    if (!geminiAIStudioOAuthEnabled.value && geminiOAuthType.value === 'ai_studio') {
      geminiOAuthType.value = 'code_assist'
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

const handleSelectGeminiOAuthType = (oauthType: 'code_assist' | 'google_one' | 'ai_studio') => {
  if (oauthType === 'ai_studio' && !geminiAIStudioOAuthEnabled.value) {
    appStore.showError(t('admin.accounts.oauth.gemini.aiStudioNotConfigured'))
    return
  REDACTED
  geminiOAuthType.value = oauthType
REDACTED

// Auto-fill related models when switching to whitelist mode or changing platform
watch(
  [modelRestrictionMode, () => form.platform],
  ([newMode]) => {
    if (newMode === 'whitelist') {
      allowedModels.value = [...getModelsByPlatform(form.platform)]
    REDACTED
  REDACTED
)

watch(
  [antigravityModelRestrictionMode, () => form.platform],
  ([, platform]) => {
    if (platform !== 'antigravity') return
    // Antigravity 默认不做限制：白名单留空表示允许所有（包含未来新增模型）。
    // 如果需要快速填充常用模型，可在组件内点“填充相关模型”。
  REDACTED
)

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' REDACTED)
REDACTED

const addOpenAICompactModelMapping = () => {
  openAICompactModelMappings.value.push({ from: '', to: '' REDACTED)
REDACTED

const removeOpenAICompactModelMapping = (index: number) => {
  openAICompactModelMappings.value.splice(index, 1)
REDACTED

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
REDACTED

const addPresetMapping = (from: string, to: string) => {
  if (modelMappings.value.some((m) => m.from === from)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from REDACTED))
    return
  REDACTED
  modelMappings.value.push({ from, to REDACTED)
REDACTED

const addAntigravityModelMapping = () => {
  antigravityModelMappings.value.push({ from: '', to: '' REDACTED)
REDACTED

const removeAntigravityModelMapping = (index: number) => {
  antigravityModelMappings.value.splice(index, 1)
REDACTED

const addAntigravityPresetMapping = (from: string, to: string) => {
  if (antigravityModelMappings.value.some((m) => m.from === from)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from REDACTED))
    return
  REDACTED
  antigravityModelMappings.value.push({ from, to REDACTED)
REDACTED

// Error code toggle helper
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    // Adding code - check for 429/529 warning
    if (code === 429) {
      if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
        return
      REDACTED
    REDACTED else if (code === 529) {
      if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
        return
      REDACTED
    REDACTED
    selectedErrorCodes.value.push(code)
  REDACTED else {
    selectedErrorCodes.value.splice(index, 1)
  REDACTED
REDACTED

// Add custom error code from input
const addCustomErrorCode = () => {
  const code = customErrorCodeInput.value
  if (code === null || code < 100 || code > 599) {
    appStore.showError(t('admin.accounts.invalidErrorCode'))
    return
  REDACTED
  if (selectedErrorCodes.value.includes(code)) {
    appStore.showInfo(t('admin.accounts.errorCodeExists'))
    return
  REDACTED
  // Check for 429/529 warning
  if (code === 429) {
    if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
      return
    REDACTED
  REDACTED else if (code === 529) {
    if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
      return
    REDACTED
  REDACTED
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
REDACTED

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  REDACTED
REDACTED

const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
  if (preset) {
    tempUnschedRules.value.push({ ...preset REDACTED)
    return
  REDACTED
  tempUnschedRules.value.push({
    error_code: null,
    keywords: '',
    duration_minutes: 30,
    description: ''
  REDACTED)
REDACTED

const removeTempUnschedRule = (index: number) => {
  tempUnschedRules.value.splice(index, 1)
REDACTED

const moveTempUnschedRule = (index: number, direction: number) => {
  const target = index + direction
  if (target < 0 || target >= tempUnschedRules.value.length) return
  const rules = tempUnschedRules.value
  const current = rules[index]
  rules[index] = rules[target]
  rules[target] = current
REDACTED

const buildTempUnschedRules = (rules: TempUnschedRuleForm[]) => {
  const out: Array<{
    error_code: number
    keywords: string[]
    duration_minutes: number
    description: string
  REDACTED> = []

  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) {
      continue
    REDACTED
    if (!Number.isFinite(duration) || duration <= 0) {
      continue
    REDACTED
    if (keywords.length === 0) {
      continue
    REDACTED
    out.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim()
    REDACTED)
  REDACTED

  return out
REDACTED

const applyTempUnschedConfig = (credentials: Record<string, unknown>) => {
  if (!tempUnschedEnabled.value) {
    delete credentials.temp_unschedulable_enabled
    delete credentials.temp_unschedulable_rules
    return true
  REDACTED

  const rules = buildTempUnschedRules(tempUnschedRules.value)
  if (rules.length === 0) {
    appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
    return false
  REDACTED

  credentials.temp_unschedulable_enabled = true
  credentials.temp_unschedulable_rules = rules
  return true
REDACTED

const splitTempUnschedKeywords = (value: string) => {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
REDACTED

const needsMixedChannelCheck = (platform: AccountPlatform) => platform === 'antigravity' || platform === 'anthropic'

const buildMixedChannelDetails = (resp?: CheckMixedChannelResponse) => {
  const details = resp?.details
  if (!details) {
    return null
  REDACTED
  return {
    groupName: details.group_name || 'Unknown',
    currentPlatform: details.current_platform || 'Unknown',
    otherPlatform: details.other_platform || 'Unknown'
  REDACTED
REDACTED

const clearMixedChannelDialog = () => {
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
REDACTED

const openMixedChannelDialog = (opts: {
  response?: CheckMixedChannelResponse
  message?: string
  onConfirm: () => Promise<void>
REDACTED) => {
  mixedChannelWarningDetails.value = buildMixedChannelDetails(opts.response)
  mixedChannelWarningRawMessage.value =
    opts.message || opts.response?.message || t('admin.accounts.failedToCreate')
  mixedChannelWarningAction.value = opts.onConfirm
  showMixedChannelWarning.value = true
REDACTED

const withAntigravityConfirmFlag = (payload: CreateAccountRequest): CreateAccountRequest => {
  if (needsMixedChannelCheck(payload.platform) && antigravityMixedChannelConfirmed.value) {
    return {
      ...payload,
      confirm_mixed_channel_risk: true
    REDACTED
  REDACTED
  const cloned = { ...payload REDACTED
  delete cloned.confirm_mixed_channel_risk
  return cloned
REDACTED

const ensureAntigravityMixedChannelConfirmed = async (onConfirm: () => Promise<void>): Promise<boolean> => {
  if (!needsMixedChannelCheck(form.platform)) {
    return true
  REDACTED
  if (antigravityMixedChannelConfirmed.value) {
    return true
  REDACTED

  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({
      platform: form.platform,
      group_ids: form.group_ids
    REDACTED)
    if (!result.has_risk) {
      return true
    REDACTED
    openMixedChannelDialog({
      response: result,
      onConfirm: async () => {
        antigravityMixedChannelConfirmed.value = true
        await onConfirm()
      REDACTED
    REDACTED)
    return false
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
    return false
  REDACTED
REDACTED

const submitCreateAccount = async (payload: CreateAccountRequest) => {
  submitting.value = true
  try {
    await adminAPI.accounts.create(withAntigravityConfirmFlag(payload))
    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  REDACTED catch (error: any) {
    if (error.response?.status === 409 && error.response?.data?.error === 'mixed_channel_warning' && needsMixedChannelCheck(form.platform)) {
      openMixedChannelDialog({
        message: error.response?.data?.message,
        onConfirm: async () => {
          antigravityMixedChannelConfirmed.value = true
          await submitCreateAccount(payload)
        REDACTED
      REDACTED)
      return
    REDACTED
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

// Methods
const resetForm = () => {
  step.value = 1
  form.name = ''
  form.notes = ''
  form.platform = 'anthropic'
  form.type = 'oauth'
  form.credentials = {REDACTED
  form.proxy_id = null
  form.concurrency = 10
  form.load_factor = null
  form.priority = 1
  form.rate_multiplier = 1
  form.group_ids = []
  form.expires_at = null
  accountCategory.value = 'oauth-based'
  addMethod.value = 'oauth'
  apiKeyBaseUrl.value = 'https://api.anthropic.com'
  apiKeyValue.value = ''
  editQuotaLimit.value = null
  editQuotaDailyLimit.value = null
  editQuotaWeeklyLimit.value = null
  editDailyResetMode.value = null
  editDailyResetHour.value = null
  editWeeklyResetMode.value = null
  editWeeklyResetDay.value = null
  editWeeklyResetHour.value = null
  editResetTimezone.value = null
  modelMappings.value = []
  openAICompactModelMappings.value = []
  modelRestrictionMode.value = 'whitelist'
  allowedModels.value = [...claudeModels] // Default fill related models

  antigravityModelRestrictionMode.value = 'mapping'
  antigravityWhitelistModels.value = []
  fetchAntigravityDefaultMappings().then(mappings => {
    antigravityModelMappings.value = [...mappings]
  REDACTED)
  poolModeEnabled.value = false
  poolModeRetryCount.value = DEFAULT_POOL_MODE_RETRY_COUNT
  poolModeRetryStatusCodesInput.value = ''
  customErrorCodesEnabled.value = false
  selectedErrorCodes.value = []
  customErrorCodeInput.value = null
  interceptWarmupRequests.value = false
  autoPauseOnExpired.value = true
  openaiPassthroughEnabled.value = false
  openAICompactMode.value = 'auto'
  openAIResponsesMode.value = 'auto'
  openAIEndpointCapabilities.value = ['chat_completions', 'embeddings']
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexCLIOnlyEnabled.value = false
  codexCLIOnlyAllowClaudeCodeEnabled.value = false
  anthropicPassthroughEnabled.value = false
  webSearchEmulationMode.value = 'default'
  // Reset quota control state
  windowCostEnabled.value = false
  windowCostLimit.value = null
  windowCostStickyReserve.value = null
  sessionLimitEnabled.value = false
  maxSessions.value = null
  sessionIdleTimeout.value = null
  rpmLimitEnabled.value = false
  baseRpm.value = null
  rpmStrategy.value = 'tiered'
  rpmStickyBuffer.value = null
  userMsgQueueMode.value = ''
  tlsFingerprintEnabled.value = false
  tlsFingerprintProfileId.value = null
  sessionIdMaskingEnabled.value = false
  cacheTTLOverrideEnabled.value = false
  cacheTTLOverrideTarget.value = '5m'
  customBaseUrlEnabled.value = false
  customBaseUrl.value = ''
  allowOverages.value = false
  antigravityAccountType.value = 'oauth'
  antigravityProjectId.value = ''
  upstreamBaseUrl.value = ''
  upstreamApiKey.value = ''
  vertexServiceAccountJson.value = ''
  vertexProjectId.value = ''
  vertexClientEmail.value = ''
  vertexLocation.value = 'global'
  tempUnschedEnabled.value = false
  tempUnschedRules.value = []
  geminiOAuthType.value = 'code_assist'
  geminiTierGoogleOne.value = 'google_one_free'
  geminiTierGcp.value = 'gcp_standard'
  geminiTierAIStudio.value = 'aistudio_free'
  oauth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  oauthFlowRef.value?.reset()
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
REDACTED

const handleClose = () => {
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
  emit('close')
REDACTED

const buildOpenAIExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'openai') {
    return base
  REDACTED

  const extra: Record<string, unknown> = { ...(base || {REDACTED) REDACTED
  if (accountCategory.value === 'oauth-based') {
    extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthResponsesWebSocketV2Mode.value)
  REDACTED else if (accountCategory.value === 'apikey') {
    extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyResponsesWebSocketV2Mode.value)
  REDACTED
  // 清理兼容旧键，统一改用分类型开关。
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled
  if (openaiPassthroughEnabled.value) {
    extra.openai_passthrough = true
  REDACTED else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  REDACTED

  if (accountCategory.value === 'oauth-based' && codexCLIOnlyEnabled.value) {
    extra.codex_cli_only = true
  REDACTED else {
    delete extra.codex_cli_only
  REDACTED
  if (
    accountCategory.value === 'oauth-based' &&
    codexCLIOnlyEnabled.value &&
    codexCLIOnlyAllowClaudeCodeEnabled.value
  ) {
    extra.codex_cli_only_allowed_clients = ['claude_code']
  REDACTED else {
    delete extra.codex_cli_only_allowed_clients
  REDACTED
  if (openAICompactMode.value !== 'auto') {
    extra.openai_compact_mode = openAICompactMode.value
  REDACTED else {
    delete extra.openai_compact_mode
  REDACTED

  if (
    accountCategory.value === 'apikey' &&
    openAITextGenerationCapabilityEnabled.value &&
    openAIResponsesMode.value !== 'auto'
  ) {
    extra.openai_responses_mode = openAIResponsesMode.value
  REDACTED else {
    delete extra.openai_responses_mode
  REDACTED

  return Object.keys(extra).length > 0 ? extra : undefined
REDACTED

const buildAnthropicExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'anthropic' || accountCategory.value !== 'apikey') {
    return base
  REDACTED

  const extra: Record<string, unknown> = { ...(base || {REDACTED) REDACTED
  if (anthropicPassthroughEnabled.value) {
    extra.anthropic_passthrough = true
  REDACTED else {
    delete extra.anthropic_passthrough
  REDACTED
  if (webSearchEmulationMode.value === 'default') {
    delete extra.web_search_emulation
  REDACTED else {
    extra.web_search_emulation = webSearchEmulationMode.value
  REDACTED

  return Object.keys(extra).length > 0 ? extra : undefined
REDACTED

// Helper function to create account with mixed channel warning handling
const doCreateAccount = async (payload: CreateAccountRequest) => {
  const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
    await submitCreateAccount(payload)
  REDACTED)
  if (!canContinue) {
    return
  REDACTED
  await submitCreateAccount(payload)
REDACTED

// Handle mixed channel warning confirmation
const handleMixedChannelConfirm = async () => {
  const action = mixedChannelWarningAction.value
  if (!action) {
    clearMixedChannelDialog()
    return
  REDACTED
  clearMixedChannelDialog()
  submitting.value = true
  try {
    await action()
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleMixedChannelCancel = () => {
  clearMixedChannelDialog()
REDACTED

const normalizePoolModeRetryCount = (value: number) => {
  if (!Number.isFinite(value)) {
    return DEFAULT_POOL_MODE_RETRY_COUNT
  REDACTED
  const normalized = Math.trunc(value)
  if (normalized < 0) {
    return 0
  REDACTED
  if (normalized > MAX_POOL_MODE_RETRY_COUNT) {
    return MAX_POOL_MODE_RETRY_COUNT
  REDACTED
  return normalized
REDACTED

const applyVertexServiceAccountJson = (value: string) => {
  const raw = value.trim()
  if (!raw) {
    vertexProjectId.value = ''
    vertexClientEmail.value = ''
    return false
  REDACTED
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const projectId = typeof parsed.project_id === 'string' ? parsed.project_id.trim() : ''
    const clientEmail = typeof parsed.client_email === 'string' ? parsed.client_email.trim() : ''
    const privateKey = typeof parsed.private_key === 'string' ? parsed.private_key.trim() : ''
    if (!projectId || !clientEmail || !privateKey) {
      appStore.showError(t('admin.accounts.vertexSaJsonMissingFields'))
      return false
    REDACTED
    vertexProjectId.value = projectId
    vertexClientEmail.value = clientEmail
    vertexServiceAccountJson.value = JSON.stringify(parsed)
    return true
  REDACTED catch {
    appStore.showError(t('admin.accounts.vertexSaJsonInvalid'))
    return false
  REDACTED
REDACTED

const parseVertexServiceAccountJson = () => applyVertexServiceAccountJson(vertexServiceAccountJson.value)

const handleVertexServiceAccountFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    applyVertexServiceAccountJson(await file.text())
  REDACTED finally {
    input.value = ''
  REDACTED
REDACTED

const handleVertexServiceAccountDrop = async (event: DragEvent) => {
  vertexServiceAccountDragActive.value = false
  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  applyVertexServiceAccountJson(await file.text())
REDACTED

const handleSubmit = async () => {
  // For OAuth-based type, handle OAuth flow (goes to step 2)
  if (isOAuthFlow.value) {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    REDACTED
    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
      step.value = 2
    REDACTED)
    if (!canContinue) {
      return
    REDACTED
    step.value = 2
    return
  REDACTED

  // For Bedrock type, create directly
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    REDACTED

    const credentials: Record<string, unknown> = {
      auth_mode: bedrockAuthMode.value,
      aws_region: bedrockRegion.value.trim() || 'us-east-1',
    REDACTED

    if (bedrockAuthMode.value === 'sigv4') {
      if (!bedrockAccessKeyId.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockAccessKeyIdRequired'))
        return
      REDACTED
      if (!bedrockSecretAccessKey.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockSecretAccessKeyRequired'))
        return
      REDACTED
      credentials.aws_access_key_id = bedrockAccessKeyId.value.trim()
      credentials.aws_secret_access_key = bedrockSecretAccessKey.value.trim()
      if (bedrockSessionToken.value.trim()) {
        credentials.aws_session_token = bedrockSessionToken.value.trim()
      REDACTED
    REDACTED else {
      if (!bedrockApiKeyValue.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockApiKeyRequired'))
        return
      REDACTED
      credentials.api_key = bedrockApiKeyValue.value.trim()
    REDACTED

    if (bedrockForceGlobal.value) {
      credentials.aws_force_global = 'true'
    REDACTED

    // Model mapping
    const modelMapping = buildModelMappingObject(
      modelRestrictionMode.value, allowedModels.value, modelMappings.value
    )
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    REDACTED

    // Pool mode
    if (poolModeEnabled.value) {
      credentials.pool_mode = true
      credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
      const parsedRetryStatusCodes = parsePoolModeRetryStatusCodes(poolModeRetryStatusCodesInput.value)
      if (parsedRetryStatusCodes.length > 0) {
        credentials.pool_mode_retry_status_codes = parsedRetryStatusCodes
      REDACTED
    REDACTED

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    await createAccountAndFinish('anthropic', 'bedrock' as AccountType, credentials)
    return
  REDACTED

  // For Antigravity upstream type, create directly
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    REDACTED
    if (!upstreamBaseUrl.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterBaseUrl'))
      return
    REDACTED
    if (!upstreamApiKey.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterApiKey'))
      return
    REDACTED

    // Build upstream credentials (and optional model restriction)
    const credentials: Record<string, unknown> = {
      base_url: upstreamBaseUrl.value.trim(),
      api_key: upstreamApiKey.value.trim()
    REDACTED

    // Antigravity 只使用映射模式
    const antigravityModelMapping = buildModelMappingObject(
      'mapping',
      [],
      antigravityModelMappings.value
    )
    if (antigravityModelMapping) {
      credentials.model_mapping = antigravityModelMapping
    REDACTED

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    const extra = buildAntigravityExtra()
    await createAccountAndFinish(form.platform, 'apikey', credentials, extra)
    return
  REDACTED

  if ((form.platform === 'gemini' || form.platform === 'anthropic') && accountCategory.value === 'service_account') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    REDACTED
    if (!parseVertexServiceAccountJson()) {
      return
    REDACTED
    if (!vertexLocation.value.trim()) {
      appStore.showError(t('admin.accounts.vertexLocationRequired'))
      return
    REDACTED
    const credentials: Record<string, unknown> = {
      service_account_json: vertexServiceAccountJson.value.trim(),
      project_id: vertexProjectId.value.trim(),
      client_email: vertexClientEmail.value.trim(),
      location: vertexLocation.value.trim(),
      tier_id: 'vertex'
    REDACTED
    await createAccountAndFinish(form.platform, 'service_account' as AccountType, credentials)
    return
  REDACTED

  // For apikey type, create directly
  if (!apiKeyValue.value.trim()) {
    appStore.showError(t('admin.accounts.pleaseEnterApiKey'))
    return
  REDACTED

  // Determine default base URL based on platform
  const defaultBaseUrl =
    form.platform === 'openai'
      ? 'https://api.openai.com'
      : form.platform === 'gemini'
        ? 'https://generativelanguage.googleapis.com'
        : 'https://api.anthropic.com'

  // Build credentials with optional model mapping
  const credentials: Record<string, unknown> = {
    base_url: apiKeyBaseUrl.value.trim() || defaultBaseUrl,
    api_key: apiKeyValue.value.trim()
  REDACTED
  if (form.platform === 'gemini') {
    credentials.tier_id = geminiTierAIStudio.value
  REDACTED

  // Add model mapping if configured（OpenAI 开启自动透传时不应用）
  if (!isOpenAIModelRestrictionDisabled.value) {
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    REDACTED
  REDACTED
  if (form.platform === 'openai') {
    applyOpenAIEndpointCapabilities(credentials)
    const compactModelMapping = buildOpenAICompactModelMapping()
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    REDACTED
  REDACTED

  // Add pool mode if enabled
  if (poolModeEnabled.value) {
    credentials.pool_mode = true
    credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
    const parsedRetryStatusCodes = parsePoolModeRetryStatusCodes(poolModeRetryStatusCodesInput.value)
    if (parsedRetryStatusCodes.length > 0) {
      credentials.pool_mode_retry_status_codes = parsedRetryStatusCodes
    REDACTED
  REDACTED

  // Add custom error codes if enabled
  if (customErrorCodesEnabled.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
  REDACTED

  applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
  if (!applyTempUnschedConfig(credentials)) {
    return
  REDACTED

  form.credentials = credentials
  const extra = buildAnthropicExtra(buildOpenAIExtra())

  await doCreateAccount({
    ...form,
    group_ids: form.group_ids,
    extra,
    auto_pause_on_expired: autoPauseOnExpired.value
  REDACTED)
REDACTED

const goBackToBasicInfo = () => {
  step.value = 1
  oauth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  oauthFlowRef.value?.reset()
REDACTED

const handleGenerateUrl = async () => {
  if (form.platform === 'openai') {
    await openaiOAuth.generateAuthUrl(form.proxy_id)
  REDACTED else if (form.platform === 'gemini') {
    await geminiOAuth.generateAuthUrl(
      form.proxy_id,
      oauthFlowRef.value?.projectId,
      geminiOAuthType.value,
      geminiSelectedTier.value
    )
  REDACTED else if (form.platform === 'antigravity') {
    await antigravityOAuth.generateAuthUrl(form.proxy_id)
  REDACTED else {
    await oauth.generateAuthUrl(addMethod.value, form.proxy_id)
  REDACTED
REDACTED

const handleValidateRefreshToken = (rt: string) => {
  if (form.platform === 'openai') {
    handleOpenAIValidateRT(rt)
  REDACTED else if (form.platform === 'antigravity') {
    handleAntigravityValidateRT(rt)
  REDACTED
REDACTED

const handleValidateSessionToken = (_sessionToken: string) => {
  // Session token validation removed
REDACTED

const formatDateTimeLocal = formatDateTimeLocalInput
const parseDateTimeLocal = parseDateTimeLocalInput

// Create account and handle success/failure
const createAccountAndFinish = async (
  platform: AccountPlatform,
  type: AccountType,
  credentials: Record<string, unknown>,
  extra?: Record<string, unknown>
) => {
  if (!applyTempUnschedConfig(credentials)) {
    return
  REDACTED
  // Inject quota limits for apikey/bedrock accounts
  let finalExtra = extra
  if (type === 'apikey' || type === 'bedrock') {
    const quotaExtra: Record<string, unknown> = { ...(extra || {REDACTED) REDACTED
    if (editQuotaLimit.value != null && editQuotaLimit.value > 0) {
      quotaExtra.quota_limit = editQuotaLimit.value
    REDACTED
    if (editQuotaDailyLimit.value != null && editQuotaDailyLimit.value > 0) {
      quotaExtra.quota_daily_limit = editQuotaDailyLimit.value
    REDACTED
    if (editQuotaWeeklyLimit.value != null && editQuotaWeeklyLimit.value > 0) {
      quotaExtra.quota_weekly_limit = editQuotaWeeklyLimit.value
    REDACTED
    // Quota reset mode config
    if (editDailyResetMode.value === 'fixed') {
      quotaExtra.quota_daily_reset_mode = 'fixed'
      quotaExtra.quota_daily_reset_hour = editDailyResetHour.value ?? 0
    REDACTED
    if (editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_weekly_reset_mode = 'fixed'
      quotaExtra.quota_weekly_reset_day = editWeeklyResetDay.value ?? 1
      quotaExtra.quota_weekly_reset_hour = editWeeklyResetHour.value ?? 0
    REDACTED
    if (editDailyResetMode.value === 'fixed' || editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_reset_timezone = editResetTimezone.value || 'UTC'
    REDACTED
    // Quota notify config
    writeQuotaNotifyToExtra(quotaExtra, 'create')
    if (Object.keys(quotaExtra).length > 0) {
      finalExtra = quotaExtra
    REDACTED
  REDACTED
  if (platform === 'openai') {
    if (type === 'apikey') {
      applyOpenAIEndpointCapabilities(credentials)
    REDACTED
    const compactModelMapping = buildOpenAICompactModelMapping()
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    REDACTED else {
      delete credentials.compact_model_mapping
    REDACTED
  REDACTED
  await doCreateAccount({
    name: form.name,
    notes: form.notes,
    platform,
    type,
    credentials,
    extra: finalExtra,
    proxy_id: form.proxy_id,
    concurrency: form.concurrency,
    load_factor: form.load_factor ?? undefined,
    priority: form.priority,
    rate_multiplier: form.rate_multiplier,
    group_ids: form.group_ids,
    expires_at: form.expires_at,
    auto_pause_on_expired: autoPauseOnExpired.value
  REDACTED)
REDACTED

// OpenAI OAuth 授权码兑换
const handleOpenAIExchange = async (authCode: string) => {
  const oauthClient = openaiOAuth
  if (!authCode.trim() || !oauthClient.sessionId.value) return

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  try {
    const stateToUse = (oauthFlowRef.value?.oauthState || oauthClient.oauthState.value || '').trim()
    if (!stateToUse) {
      oauthClient.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(oauthClient.error.value)
      return
    REDACTED

    const tokenInfo = await oauthClient.exchangeAuthCode(
      authCode.trim(),
      oauthClient.sessionId.value,
      stateToUse,
      form.proxy_id
    )
    if (!tokenInfo) return

    const credentials = oauthClient.buildCredentials(tokenInfo)
    const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
    const extra = buildOpenAIExtra(oauthExtra)
    const shouldCreateOpenAI = form.platform === 'openai'

    // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
    if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
      const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      if (modelMapping) {
        credentials.model_mapping = modelMapping
      REDACTED
    REDACTED
    if (shouldCreateOpenAI) {
      const compactModelMapping = buildOpenAICompactModelMapping()
      if (compactModelMapping) {
        credentials.compact_model_mapping = compactModelMapping
      REDACTED
    REDACTED

    // 应用临时不可调度配置
    if (!applyTempUnschedConfig(credentials)) {
      return
    REDACTED

    if (shouldCreateOpenAI) {
      await adminAPI.accounts.create({
        name: form.name,
        notes: form.notes,
        platform: 'openai',
        type: 'oauth',
        credentials,
        extra,
        proxy_id: form.proxy_id,
        concurrency: form.concurrency,
        load_factor: form.load_factor ?? undefined,
        priority: form.priority,
        rate_multiplier: form.rate_multiplier,
        group_ids: form.group_ids,
        expires_at: form.expires_at,
        auto_pause_on_expired: autoPauseOnExpired.value
      REDACTED)
      appStore.showSuccess(t('admin.accounts.accountCreated'))
    REDACTED

    emit('created')
    handleClose()
  REDACTED catch (error: any) {
    oauthClient.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauthClient.error.value)
  REDACTED finally {
    oauthClient.loading.value = false
  REDACTED
REDACTED

// OpenAI 手动 RT 批量验证和创建
// OpenAI Mobile RT client_id
const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

const buildOpenAICodexImportCredentialExtras = (): Record<string, unknown> | null => {
  const credentials: Record<string, unknown> = {REDACTED
  if (!isOpenAIModelRestrictionDisabled.value) {
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    REDACTED
  REDACTED

  const compactModelMapping = buildOpenAICompactModelMapping()
  if (compactModelMapping) {
    credentials.compact_model_mapping = compactModelMapping
  REDACTED

  if (!applyTempUnschedConfig(credentials)) {
    return null
  REDACTED
  return credentials
REDACTED

const formatCodexImportMessages = (messages?: CodexSessionImportMessage[]) => {
  return (messages || [])
    .map((item) => {
      const name = item.name ? ` ${item.nameREDACTED` : ''
      return `#${item.indexREDACTED${nameREDACTED: ${item.messageREDACTED`
    REDACTED)
    .join('\n')
REDACTED

const handleOpenAIImportCodexSession = async (content: string) => {
  const oauthClient = openaiOAuth
  const trimmed = content.trim()
  if (!trimmed) {
    oauthClient.error.value = t('admin.accounts.oauth.openai.codexSessionEmpty')
    return
  REDACTED

  const credentialExtras = buildOpenAICodexImportCredentialExtras()
  if (credentialExtras === null) {
    return
  REDACTED

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  try {
    const extra = buildOpenAIExtra()
    const result = await adminAPI.accounts.importCodexSession({
      content: trimmed,
      name: form.name,
      notes: form.notes || null,
      proxy_id: form.proxy_id,
      concurrency: form.concurrency,
      load_factor: form.load_factor ?? undefined,
      priority: form.priority,
      rate_multiplier: form.rate_multiplier,
      group_ids: form.group_ids,
      expires_at: form.expires_at,
      auto_pause_on_expired: autoPauseOnExpired.value,
      credential_extras: Object.keys(credentialExtras).length > 0 ? credentialExtras : undefined,
      extra,
      update_existing: true
    REDACTED)

    const successCount = result.created + result.updated
    const params = {
      created: result.created,
      updated: result.updated,
      skipped: result.skipped,
      failed: result.failed
    REDACTED

    if (successCount > 0 && result.failed === 0) {
      appStore.showSuccess(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
      emit('created')
      handleClose()
      return
    REDACTED

    const errorText = formatCodexImportMessages(result.errors)
    const warningText = formatCodexImportMessages(result.warnings)
    oauthClient.error.value = [errorText, warningText].filter(Boolean).join('\n')

    if (result.failed === 0) {
      appStore.showWarning(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
      return
    REDACTED

    if (successCount > 0) {
      appStore.showWarning(t('admin.accounts.oauth.openai.codexSessionImportPartial', params))
      emit('created')
      return
    REDACTED

    appStore.showError(t('admin.accounts.oauth.openai.codexSessionImportFailed'))
  REDACTED catch (error: any) {
    oauthClient.error.value =
      error.response?.data?.detail ||
      error.response?.data?.message ||
      error.message ||
      t('admin.accounts.oauth.openai.codexSessionImportFailed')
    appStore.showError(oauthClient.error.value)
  REDACTED finally {
    oauthClient.loading.value = false
  REDACTED
REDACTED

// OpenAI RT 批量验证和创建（共享逻辑）
const handleOpenAIBatchRT = async (refreshTokenInput: string, clientId?: string) => {
  const oauthClient = openaiOAuth
  if (!refreshTokenInput.trim()) return

  const refreshTokens = refreshTokenInput
    .split('\n')
    .map((rt) => rt.trim())
    .filter((rt) => rt)

  if (refreshTokens.length === 0) {
    oauthClient.error.value = t('admin.accounts.oauth.openai.pleaseEnterRefreshToken')
    return
  REDACTED

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  let successCount = 0
  let failedCount = 0
  const errors: string[] = []
  const shouldCreateOpenAI = form.platform === 'openai'

  try {
    for (let i = 0; i < refreshTokens.length; i++) {
      try {
        const tokenInfo = await oauthClient.validateRefreshToken(
          refreshTokens[i],
          form.proxy_id,
          clientId
        )
        if (!tokenInfo) {
          failedCount++
          errors.push(`#${i + 1REDACTED: ${oauthClient.error.value || 'Validation failed'REDACTED`)
          oauthClient.error.value = ''
          continue
        REDACTED

        const credentials = oauthClient.buildCredentials(tokenInfo)
        if (clientId) {
          credentials.client_id = clientId
        REDACTED
        const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
        const extra = buildOpenAIExtra(oauthExtra)

        // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
        if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
          const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
          if (modelMapping) {
            credentials.model_mapping = modelMapping
          REDACTED
        REDACTED
        if (shouldCreateOpenAI) {
          const compactModelMapping = buildOpenAICompactModelMapping()
          if (compactModelMapping) {
            credentials.compact_model_mapping = compactModelMapping
          REDACTED
        REDACTED

        // Generate account name; fallback to email if name is empty (ent schema requires NotEmpty)
        const baseName = form.name || tokenInfo.email || 'OpenAI OAuth Account'
        const accountName = refreshTokens.length > 1 ? `${baseNameREDACTED #${i + 1REDACTED` : baseName

        if (shouldCreateOpenAI) {
          await adminAPI.accounts.create({
            name: accountName,
            notes: form.notes,
            platform: 'openai',
            type: 'oauth',
            credentials,
            extra,
            proxy_id: form.proxy_id,
            concurrency: form.concurrency,
            load_factor: form.load_factor ?? undefined,
            priority: form.priority,
            rate_multiplier: form.rate_multiplier,
            group_ids: form.group_ids,
            expires_at: form.expires_at,
            auto_pause_on_expired: autoPauseOnExpired.value
          REDACTED)
        REDACTED

        successCount++
      REDACTED catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1REDACTED: ${errMsgREDACTED`)
      REDACTED
    REDACTED

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount REDACTED)
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    REDACTED else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount REDACTED)
      )
      oauthClient.error.value = errors.join('\n')
      emit('created')
    REDACTED else {
      oauthClient.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    REDACTED
  REDACTED finally {
    oauthClient.loading.value = false
  REDACTED
REDACTED

// 手动输入 RT（Codex CLI client_id，默认）
const handleOpenAIValidateRT = (rt: string) => handleOpenAIBatchRT(rt)

// 手动输入 Mobile RT
const handleOpenAIValidateMobileRT = (rt: string) => handleOpenAIBatchRT(rt, OPENAI_MOBILE_RT_CLIENT_ID)

// Antigravity 手动 RT 批量验证和创建
const handleAntigravityValidateRT = async (refreshTokenInput: string) => {
  if (!refreshTokenInput.trim()) return

  // Parse multiple refresh tokens (one per line)
  const refreshTokens = refreshTokenInput
    .split('\n')
    .map((rt) => rt.trim())
    .filter((rt) => rt)

  if (refreshTokens.length === 0) {
    antigravityOAuth.error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
    return
  REDACTED

  antigravityOAuth.loading.value = true
  antigravityOAuth.error.value = ''

  let successCount = 0
  let failedCount = 0
  const errors: string[] = []

  try {
    for (let i = 0; i < refreshTokens.length; i++) {
      try {
        const tokenInfo = await antigravityOAuth.validateRefreshToken(
          refreshTokens[i],
          form.proxy_id
        )
        if (!tokenInfo) {
          failedCount++
          errors.push(`#${i + 1REDACTED: ${antigravityOAuth.error.value || 'Validation failed'REDACTED`)
          antigravityOAuth.error.value = ''
          continue
        REDACTED

        const credentials = antigravityOAuth.buildCredentials(tokenInfo)
        applyAntigravityProjectID(credentials, antigravityProjectId.value, 'create')
        
        // Generate account name with index for batch
        const accountName = refreshTokens.length > 1 ? `${form.nameREDACTED #${i + 1REDACTED` : form.name

        // Note: Antigravity doesn't have buildExtraInfo, so we pass empty extra or rely on credentials
        const createPayload = withAntigravityConfirmFlag({
          name: accountName,
          notes: form.notes,
          platform: 'antigravity',
          type: 'oauth',
          credentials,
          extra: {REDACTED,
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        REDACTED)
        await adminAPI.accounts.create(createPayload)
        successCount++
      REDACTED catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1REDACTED: ${errMsgREDACTED`)
      REDACTED
    REDACTED

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount REDACTED)
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    REDACTED else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount REDACTED)
      )
      antigravityOAuth.error.value = errors.join('\n')
      emit('created')
    REDACTED else {
      antigravityOAuth.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    REDACTED
  REDACTED finally {
    antigravityOAuth.loading.value = false
  REDACTED
REDACTED

// Gemini OAuth 授权码兑换
const handleGeminiExchange = async (authCode: string) => {
  if (!authCode.trim() || !geminiOAuth.sessionId.value) return

  geminiOAuth.loading.value = true
  geminiOAuth.error.value = ''

  try {
    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || geminiOAuth.state.value
    if (!stateToUse) {
      geminiOAuth.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(geminiOAuth.error.value)
      return
    REDACTED

    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: geminiOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id,
      oauthType: geminiOAuthType.value,
      tierId: geminiSelectedTier.value
    REDACTED)
    if (!tokenInfo) return

    const credentials = geminiOAuth.buildCredentials(tokenInfo)
    const extra = geminiOAuth.buildExtraInfo(tokenInfo)
    await createAccountAndFinish('gemini', 'oauth', credentials, extra)
  REDACTED catch (error: any) {
    geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(geminiOAuth.error.value)
  REDACTED finally {
    geminiOAuth.loading.value = false
  REDACTED
REDACTED

// Antigravity OAuth 授权码兑换
const handleAntigravityExchange = async (authCode: string) => {
  if (!authCode.trim() || !antigravityOAuth.sessionId.value) return

  antigravityOAuth.loading.value = true
  antigravityOAuth.error.value = ''

  try {
    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || antigravityOAuth.state.value
    if (!stateToUse) {
      antigravityOAuth.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(antigravityOAuth.error.value)
      return
    REDACTED

    const tokenInfo = await antigravityOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: antigravityOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id
    REDACTED)
		if (!tokenInfo) return

		const credentials = antigravityOAuth.buildCredentials(tokenInfo)
		applyAntigravityProjectID(credentials, antigravityProjectId.value, 'create')
		applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
		// Antigravity 只使用映射模式
		const antigravityModelMapping = buildModelMappingObject(
			'mapping',
			[],
			antigravityModelMappings.value
		)
		if (antigravityModelMapping) {
			credentials.model_mapping = antigravityModelMapping
	REDACTED
		const extra = buildAntigravityExtra()
		await createAccountAndFinish('antigravity', 'oauth', credentials, extra)
  REDACTED catch (error: any) {
    antigravityOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(antigravityOAuth.error.value)
  REDACTED finally {
    antigravityOAuth.loading.value = false
  REDACTED
REDACTED

// Anthropic OAuth 授权码兑换
const handleAnthropicExchange = async (authCode: string) => {
  if (!authCode.trim() || !oauth.sessionId.value) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id REDACTED : {REDACTED
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/exchange-code'
        : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: oauth.sessionId.value,
      code: authCode.trim(),
      ...proxyConfig
    REDACTED)

    // Build extra with quota control settings
    const baseExtra = oauth.buildExtraInfo(tokenInfo) || {REDACTED
    const extra: Record<string, unknown> = { ...baseExtra REDACTED

    // Add window cost limit settings
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      extra.window_cost_limit = windowCostLimit.value
      extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    REDACTED

    // Add session limit settings
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      extra.max_sessions = maxSessions.value
      extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    REDACTED

    // Add RPM limit settings
    if (rpmLimitEnabled.value) {
      const DEFAULT_BASE_RPM = 15
      extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
        ? baseRpm.value
        : DEFAULT_BASE_RPM
      extra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
        extra.rpm_sticky_buffer = rpmStickyBuffer.value
      REDACTED
    REDACTED

    // UMQ mode（独立于 RPM）
    if (userMsgQueueMode.value) {
      extra.user_msg_queue_mode = userMsgQueueMode.value
    REDACTED

    // Add TLS fingerprint settings
    if (tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) {
        extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
      REDACTED
    REDACTED

    // Add session ID masking settings
    if (sessionIdMaskingEnabled.value) {
      extra.session_id_masking_enabled = true
    REDACTED

    // Add cache TTL override settings
    if (cacheTTLOverrideEnabled.value) {
      extra.cache_ttl_override_enabled = true
      extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    REDACTED

    // Add custom base URL settings
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = customBaseUrl.value.trim()
    REDACTED

    const credentials: Record<string, unknown> = { ...tokenInfo REDACTED
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
    await createAccountAndFinish(form.platform, addMethod.value as AccountType, credentials, extra)
  REDACTED catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauth.error.value)
  REDACTED finally {
    oauth.loading.value = false
  REDACTED
REDACTED

// 主入口：根据平台路由到对应处理函数
const handleExchangeCode = async () => {
  const authCode = oauthFlowRef.value?.authCode || ''

  switch (form.platform) {
    case 'openai':
      return handleOpenAIExchange(authCode)
    case 'gemini':
      return handleGeminiExchange(authCode)
    case 'antigravity':
      return handleAntigravityExchange(authCode)
    default:
      return handleAnthropicExchange(authCode)
  REDACTED
REDACTED

const handleCookieAuth = async (sessionKey: string) => {
  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id REDACTED : {REDACTED
    const keys = oauth.parseSessionKeys(sessionKey)

    if (keys.length === 0) {
      oauth.error.value = t('admin.accounts.oauth.pleaseEnterSessionKey')
      return
    REDACTED

    const tempUnschedPayload = tempUnschedEnabled.value
      ? buildTempUnschedRules(tempUnschedRules.value)
      : []
    if (tempUnschedEnabled.value && tempUnschedPayload.length === 0) {
      appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
      return
    REDACTED

    const endpoint =
      addMethod.value === 'oauth'
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
        REDACTED)

        // Build extra with quota control settings
        const baseExtra = oauth.buildExtraInfo(tokenInfo) || {REDACTED
        const extra: Record<string, unknown> = { ...baseExtra REDACTED

        // Add window cost limit settings
        if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
          extra.window_cost_limit = windowCostLimit.value
          extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
        REDACTED

        // Add session limit settings
        if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
          extra.max_sessions = maxSessions.value
          extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
        REDACTED

        // Add RPM limit settings
        if (rpmLimitEnabled.value) {
          const DEFAULT_BASE_RPM = 15
          extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
            ? baseRpm.value
            : DEFAULT_BASE_RPM
          extra.rpm_strategy = rpmStrategy.value
          if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
            extra.rpm_sticky_buffer = rpmStickyBuffer.value
          REDACTED
        REDACTED

        // UMQ mode（独立于 RPM）
        if (userMsgQueueMode.value) {
          extra.user_msg_queue_mode = userMsgQueueMode.value
        REDACTED

        // Add TLS fingerprint settings
        if (tlsFingerprintEnabled.value) {
          extra.enable_tls_fingerprint = true
          if (tlsFingerprintProfileId.value) {
            extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
          REDACTED
        REDACTED

        // Add session ID masking settings
        if (sessionIdMaskingEnabled.value) {
          extra.session_id_masking_enabled = true
        REDACTED

        // Add cache TTL override settings
        if (cacheTTLOverrideEnabled.value) {
          extra.cache_ttl_override_enabled = true
          extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
        REDACTED

        // Add custom base URL settings
        if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
          extra.custom_base_url_enabled = true
          extra.custom_base_url = customBaseUrl.value.trim()
        REDACTED

        const accountName = keys.length > 1 ? `${form.nameREDACTED #${i + 1REDACTED` : form.name

        const credentials: Record<string, unknown> = { ...tokenInfo REDACTED
        applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
        if (tempUnschedEnabled.value) {
          credentials.temp_unschedulable_enabled = true
          credentials.temp_unschedulable_rules = tempUnschedPayload
        REDACTED

        await adminAPI.accounts.create({
          name: accountName,
          notes: form.notes,
          platform: form.platform,
          type: addMethod.value, // Use addMethod as type: 'oauth' or 'setup-token'
          credentials,
          extra,
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        REDACTED)

        successCount++
      REDACTED catch (error: any) {
        failedCount++
        errors.push(
          t('admin.accounts.oauth.keyAuthFailed', {
            index: i + 1,
            error: error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
          REDACTED)
        )
      REDACTED
    REDACTED

    if (successCount > 0) {
      appStore.showSuccess(t('admin.accounts.oauth.successCreated', { count: successCount REDACTED))
      if (failedCount === 0) {
        emit('created')
        handleClose()
      REDACTED else {
        emit('created')
      REDACTED
    REDACTED

    if (failedCount > 0) {
      oauth.error.value = errors.join('\n')
    REDACTED
  REDACTED catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  REDACTED finally {
    oauth.loading.value = false
  REDACTED
REDACTED
</script>
