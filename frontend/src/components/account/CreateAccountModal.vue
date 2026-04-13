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
          }}</span>
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
          }}</span>
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
        <label class="input-label">{{ t('admin.accounts.accountName') }}</label>
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
        <label class="input-label">{{ t('admin.accounts.notes') }}</label>
        <textarea
          v-model="form.notes"
          rows="3"
          class="input"
          :placeholder="t('admin.accounts.notesPlaceholder')"
        ></textarea>
        <p class="input-hint">{{ t('admin.accounts.notesHint') }}</p>
      </div>

      <!-- Platform Selection - Segmented Control Style -->
      <div>
        <label class="input-label">{{ t('admin.accounts.platform') }}</label>
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
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
        <div class="mt-2 grid grid-cols-3 gap-3" data-tour="account-form-type">
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
              }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.oauthSetupToken')
              }}</span>
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
              }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.apiKey')
              }}</span>
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
              }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.bedrockDesc')
              }}</span>
            </div>
          </button>

        </div>
      </div>

      <!-- Account Type Selection (OpenAI) -->
      <div v-if="form.platform === 'openai'">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
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
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.chatgptOauth') }}</span>
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
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.responsesApi') }}</span>
            </div>
          </button>
        </div>
      </div>

      <!-- Account Type Selection (Gemini) -->
      <div v-if="form.platform === 'gemini'">
        <div class="flex items-center justify-between">
          <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
          <button
            type="button"
            @click="showGeminiHelpDialog = true"
            class="flex items-center gap-1 rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z" />
            </svg>
            {{ t('admin.accounts.gemini.helpButton') }}
          </button>
        </div>
        <div class="mt-2 grid grid-cols-2 gap-3" data-tour="account-form-type">
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
                {{ t('admin.accounts.gemini.accountType.oauthTitle') }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.gemini.accountType.oauthDesc') }}
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
                {{ t('admin.accounts.gemini.accountType.apiKeyTitle') }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.gemini.accountType.apiKeyDesc') }}
              </span>
            </div>
          </button>
        </div>

        <div
          v-if="accountCategory === 'apikey'"
          class="mt-3 rounded-lg border border-purple-200 bg-purple-50 px-3 py-2 text-xs text-purple-800 dark:border-purple-800/40 dark:bg-purple-900/20 dark:text-purple-200"
        >
          <p>{{ t('admin.accounts.gemini.accountType.apiKeyNote') }}</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <a
              :href="geminiHelpLinks.apiKey"
              class="font-medium text-blue-600 hover:underline dark:text-blue-400"
              target="_blank"
              rel="noreferrer"
            >
              {{ t('admin.accounts.gemini.accountType.apiKeyLink') }}
            </a>
          </div>
        </div>

        <!-- OAuth Type Selection (only show when oauth-based is selected) -->
        <div v-if="accountCategory === 'oauth-based'" class="mt-4">
          <label class="input-label">{{ t('admin.accounts.oauth.gemini.oauthTypeLabel') }}</label>
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
                    {{ t('admin.accounts.gemini.oauthType.gcpProjectLink') }}
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
              <span>{{ showAdvancedOAuth ? '隐藏' : '显示' }}高级选项（自建 OAuth Client）</span>
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
                  {{ t('admin.accounts.gemini.oauthType.customTitle') }}
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.oauthType.customDesc') }}
                </span>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.oauthType.customRequirement') }}
                </div>
                <div class="mt-2 flex flex-wrap gap-1">
                  <span
                    class="rounded bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                  >
                    {{ t('admin.accounts.gemini.oauthType.badges.orgManaged') }}
                  </span>
                  <span
                    class="rounded bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                  >
                    {{ t('admin.accounts.gemini.oauthType.badges.adminRequired') }}
                  </span>
                </div>
              </div>
              <span
                v-if="!geminiAIStudioOAuthEnabled"
                class="ml-auto shrink-0 rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
              >
                {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredShort') }}
              </span>
            </button>

            <div
              v-if="!geminiAIStudioOAuthEnabled"
              class="pointer-events-none absolute right-0 top-full z-50 mt-2 w-80 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:border-amber-700 dark:bg-amber-900/40 dark:text-amber-200"
            >
              {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredTip') }}
            </div>
          </div>
        </div>

        <!-- Tier selection (used as fallback when auto-detection is unavailable/fails) -->
        <div class="mt-4">
          <label class="input-label">{{ t('admin.accounts.gemini.tier.label') }}</label>
          <div class="mt-2">
            <select
              v-if="geminiOAuthType === 'google_one'"
              v-model="geminiTierGoogleOne"
              class="input"
            >
              <option value="google_one_free">{{ t('admin.accounts.gemini.tier.googleOne.free') }}</option>
              <option value="google_ai_pro">{{ t('admin.accounts.gemini.tier.googleOne.pro') }}</option>
              <option value="google_ai_ultra">{{ t('admin.accounts.gemini.tier.googleOne.ultra') }}</option>
            </select>

            <select
              v-else-if="geminiOAuthType === 'code_assist'"
              v-model="geminiTierGcp"
              class="input"
            >
              <option value="gcp_standard">{{ t('admin.accounts.gemini.tier.gcp.standard') }}</option>
              <option value="gcp_enterprise">{{ t('admin.accounts.gemini.tier.gcp.enterprise') }}</option>
            </select>

            <select
              v-else
              v-model="geminiTierAIStudio"
              class="input"
            >
              <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') }}</option>
              <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') }}</option>
            </select>
          </div>
          <p class="input-hint">{{ t('admin.accounts.gemini.tier.hint') }}</p>
        </div>
      </div>

      <!-- Account Type Selection (Antigravity - OAuth or Upstream) -->
      <div v-if="form.platform === 'antigravity'">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
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
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.antigravityOauth') }}</span>
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
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.types.antigravityApikey') }}</span>
            </div>
          </button>
        </div>
      </div>

      <!-- Upstream config (only for Antigravity upstream type) -->
      <div v-if="form.platform === 'antigravity' && antigravityAccountType === 'upstream'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.baseUrl') }}</label>
          <input
            v-model="upstreamBaseUrl"
            type="text"
            required
            class="input"
            placeholder="https://cloudcode-pa.googleapis.com"
          />
          <p class="input-hint">{{ t('admin.accounts.upstream.baseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.apiKey') }}</label>
          <input
            v-model="upstreamApiKey"
            type="password"
            required
            class="input font-mono"
            placeholder="sk-..."
          />
          <p class="input-hint">{{ t('admin.accounts.upstream.apiKeyHint') }}</p>
        </div>
      </div>

      <!-- Antigravity model restriction (applies to OAuth + Upstream) -->
      <!-- Antigravity 只支持模型映射模式，不支持白名单模式 -->
      <div v-if="form.platform === 'antigravity'" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

        <!-- Mapping Mode Only (no toggle for Antigravity) -->
        <div>
          <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
            <p class="text-xs text-purple-700 dark:text-purple-400">
              {{ t('admin.accounts.mapRequestModels') }}
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
                {{ t('admin.accounts.wildcardOnlyAtEnd') }}
              </p>
              <p v-if="mapping.to.includes('*')" class="text-xs text-red-500">
                {{ t('admin.accounts.targetNoWildcard') }}
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
            {{ t('admin.accounts.addMapping') }}
          </button>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in antigravityPresetMappings"
              :key="preset.label"
              type="button"
              @click="addAntigravityPresetMapping(preset.from, preset.to)"
              :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
            >
              + {{ preset.label }}
            </button>
          </div>
        </div>
      </div>

      <!-- Add Method (only for Anthropic OAuth-based type) -->
      <div v-if="form.platform === 'anthropic' && isOAuthFlow">
        <label class="input-label">{{ t('admin.accounts.addMethod') }}</label>
        <div class="mt-2 flex gap-4">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.types.oauth') }}</span>
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
            }}</span>
          </label>
        </div>
      </div>

      <!-- API Key input (only for apikey type, excluding Antigravity which has its own fields) -->
      <div v-if="form.type === 'apikey' && form.platform !== 'antigravity'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
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
          <p class="input-hint">{{ baseUrlHint }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKeyRequired') }}</label>
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
          <p class="input-hint">{{ apiKeyHint }}</p>
        </div>

        <!-- Gemini API Key tier selection -->
        <div v-if="form.platform === 'gemini'">
          <label class="input-label">{{ t('admin.accounts.gemini.tier.label') }}</label>
          <select v-model="geminiTierAIStudio" class="input">
            <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') }}</option>
            <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') }}</option>
          </select>
          <p class="input-hint">{{ t('admin.accounts.gemini.tier.aiStudioHint') }}</p>
        </div>

        <!-- Model Restriction Section (Antigravity 已在上层条件排除) -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

          <div
            v-if="isOpenAIModelRestrictionDisabled"
            class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
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
              <ModelWhitelistSelector v-model="allowedModels" :platform="form.platform" />
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
              {{ t('admin.accounts.addMapping') }}
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
                  + {{ preset.label }}
                </button>
              </div>
            </div>
          </template>
        </div>

        <!-- Pool Mode Section -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.poolMode') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.poolModeHint') }}
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
              {{ t('admin.accounts.poolModeInfo') }}
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') }}</label>
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
                })
              }}
            </p>
          </div>
        </div>

        <!-- Custom Error Codes Section -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.customErrorCodesHint') }}
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
                    ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                {{ code.value }} {{ code.label }}
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
                {{ code }}
                <button
                  type="button"
                  @click="removeErrorCode(code)"
                  class="hover:text-red-900 dark:hover:text-red-300"
                >
                  <Icon name="x" size="sm" :stroke-width="2" />
                </button>
              </span>
              <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
                {{ t('admin.accounts.noneSelectedUsesDefault') }}
              </span>
            </div>
          </div>
        </div>

      </div>

      <!-- Bedrock credentials (only for Anthropic Bedrock type) -->
      <div v-if="form.platform === 'anthropic' && accountCategory === 'bedrock'" class="space-y-4">
        <!-- Auth Mode Radio -->
        <div>
          <label class="input-label">{{ t('admin.accounts.bedrockAuthMode') }}</label>
          <div class="mt-2 flex gap-4">
            <label class="flex cursor-pointer items-center">
              <input
                v-model="bedrockAuthMode"
                type="radio"
                value="sigv4"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockAuthModeSigv4') }}</span>
            </label>
            <label class="flex cursor-pointer items-center">
              <input
                v-model="bedrockAuthMode"
                type="radio"
                value="apikey"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockAuthModeApikey') }}</span>
            </label>
          </div>
        </div>

        <!-- SigV4 fields -->
        <template v-if="bedrockAuthMode === 'sigv4'">
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockAccessKeyId') }}</label>
            <input
              v-model="bedrockAccessKeyId"
              type="text"
              required
              class="input font-mono"
              placeholder="AKIA..."
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockSecretAccessKey') }}</label>
            <input
              v-model="bedrockSecretAccessKey"
              type="password"
              required
              class="input font-mono"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.bedrockSessionToken') }}</label>
            <input
              v-model="bedrockSessionToken"
              type="password"
              class="input font-mono"
            />
            <p class="input-hint">{{ t('admin.accounts.bedrockSessionTokenHint') }}</p>
          </div>
        </template>

        <!-- API Key field -->
        <div v-if="bedrockAuthMode === 'apikey'">
          <label class="input-label">{{ t('admin.accounts.bedrockApiKeyInput') }}</label>
          <input
            v-model="bedrockApiKeyValue"
            type="password"
            required
            class="input font-mono"
          />
        </div>

        <!-- Shared: Region -->
        <div>
          <label class="input-label">{{ t('admin.accounts.bedrockRegion') }}</label>
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
          <p class="input-hint">{{ t('admin.accounts.bedrockRegionHint') }}</p>
        </div>

        <!-- Shared: Force Global -->
        <div>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              v-model="bedrockForceGlobal"
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockForceGlobal') }}</span>
          </label>
          <p class="input-hint mt-1">{{ t('admin.accounts.bedrockForceGlobalHint') }}</p>
        </div>

        <!-- Model Restriction Section for Bedrock -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

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
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <ModelWhitelistSelector v-model="allowedModels" platform="anthropic" />
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
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
            <button type="button" @click="modelMappings.push({ from: '', to: '' })" class="btn btn-secondary text-sm">
              + {{ t('admin.accounts.addMapping') }}
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
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </div>

        <!-- Pool Mode Section for Bedrock -->
        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.poolMode') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.poolModeHint') }}
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
              {{ t('admin.accounts.poolModeInfo') }}
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') }}</label>
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
                })
              }}
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
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.hint') }}
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
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaLimitHint') }}
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
        <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

        <div
          v-if="isOpenAIModelRestrictionDisabled"
          class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
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
              @click="modelRestrictionMode = 'whitelist'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'whitelist'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
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
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <ModelWhitelistSelector v-model="allowedModels" :platform="form.platform" />
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
                {{ t('admin.accounts.mapRequestModels') }}
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
              + {{ t('admin.accounts.addMapping') }}
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
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </template>
      </div>

      <!-- Temp Unschedulable Rules -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.hint') }}
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
                {{ t('admin.accounts.tempUnschedulable.notice') }}
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
              + {{ preset.label }}
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
                  {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 }) }}
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
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') }}</label>
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
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') }}</label>
                  <input
                    v-model.number="rule.duration_minutes"
                    type="number"
                    min="1"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
                  />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') }}</label>
                  <input
                    v-model="rule.keywords"
                    type="text"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
                  />
                  <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') }}</p>
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') }}</label>
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
            {{ t('admin.accounts.tempUnschedulable.addRule') }}
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
            }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
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
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.hint') }}
          </p>
        </div>

        <!-- Window Cost Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.windowCost.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.windowCost.hint') }}
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
              <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.limit') }}</label>
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
              <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.limitHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.stickyReserve') }}</label>
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
              <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.stickyReserveHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Session Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionLimit.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.sessionLimit.hint') }}
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
              <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessions') }}</label>
              <input
                v-model.number="maxSessions"
                type="number"
                min="1"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessionsPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessionsHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeout') }}</label>
              <div class="relative">
                <input
                  v-model.number="sessionIdleTimeout"
                  type="number"
                  min="1"
                  step="1"
                  class="input pr-12"
                  :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeoutPlaceholder')"
                />
                <span class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">{{ t('common.minutes') }}</span>
              </div>
              <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeoutHint') }}</p>
            </div>
          </div>
        </div>

        <!-- RPM Limit -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.rpmLimit.hint') }}
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
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
              <input
                v-model.number="baseRpm"
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
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
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
                    <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') }}</div>
                    <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyTieredHint') }}</div>
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
                    <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') }}</div>
                    <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExemptHint') }}</div>
                  </div>
                </button>
              </div>
            </div>

            <div v-if="rpmStrategy === 'tiered'">
              <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
              <input
                v-model.number="rpmStickyBuffer"
                type="number"
                min="1"
                step="1"
                class="input"
                :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
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
                @click="userMsgQueueMode = opt.value"
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

        <!-- TLS Fingerprint -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.tlsFingerprint.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.tlsFingerprint.hint') }}
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
              <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
              <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}</option>
              <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
          </div>
        </div>

        <!-- Session ID Masking -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionIdMasking.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.sessionIdMasking.hint') }}
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
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.cacheTTLOverride.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.cacheTTLOverride.hint') }}
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
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.cacheTTLOverride.target') }}</label>
            <select
              v-model="cacheTTLOverrideTarget"
              class="mt-1 block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700 dark:text-white"
            >
              <option value="5m">5m</option>
              <option value="1h">1h</option>
            </select>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.quotaControl.cacheTTLOverride.targetHint') }}
            </p>
          </div>
        </div>

        <!-- Custom Base URL Relay -->
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.customBaseUrl.label') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.quotaControl.customBaseUrl.hint') }}
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
        <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
        <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
      </div>

      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input v-model.number="form.concurrency" type="number" min="1" class="input"
            @input="form.concurrency = Math.max(1, form.concurrency || 1)" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
          <input v-model.number="form.load_factor" type="number" min="1"
            class="input" :placeholder="String(form.concurrency || 1)"
            @input="form.load_factor = (form.load_factor &amp;&amp; form.load_factor >= 1) ? form.load_factor : null" />
          <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input
            v-model.number="form.priority"
            type="number"
            min="1"
            class="input"
            data-tour="account-form-priority"
          />
          <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
          <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
          <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
        </div>
      </div>
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
        <input v-model="expiresAtInput" type="datetime-local" class="input" />
        <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
      </div>

      <!-- OpenAI 自动透传开关（OAuth/API Key） -->
      <div
        v-if="form.platform === 'openai'"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.oauthPassthrough') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
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
            <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.wsModeDesc') }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t(openAIWSModeConcurrencyHintKey) }}
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
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyPassthrough') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.apiKeyPassthroughDesc') }}
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
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.webSearchEmulation') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.webSearchEmulationDesc') }}
            </p>
          </div>
          <select v-model="webSearchEmulationMode" class="input w-24 text-sm">
            <option value="default">{{ t('admin.accounts.anthropic.webSearchDefault') }}</option>
            <option value="enabled">{{ t('admin.accounts.anthropic.webSearchEnabled') }}</option>
            <option value="disabled">{{ t('admin.accounts.anthropic.webSearchDisabled') }}</option>
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
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnly') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexCLIOnlyDesc') }}
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
      </div>

      <div>
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{
              t('admin.accounts.autoPauseOnExpired')
            }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.autoPauseOnExpiredDesc') }}
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
              {{ t('admin.accounts.mixedScheduling') }}
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
              {{ t('admin.accounts.mixedSchedulingTooltip') }}
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
              {{ t('admin.accounts.allowOverages') }}
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
              {{ t('admin.accounts.allowOveragesTooltip') }}
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
        :platform="form.platform"
        :show-project-id="geminiOAuthType === 'code_assist'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleValidateRefreshToken"
        @validate-mobile-refresh-token="handleOpenAIValidateMobileRT"
        @validate-session-token="handleValidateSessionToken"
      />

    </div>

    <template #footer>
      <div v-if="step === 1" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
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
          }}
        </button>
      </div>
      <div v-else class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="goBackToBasicInfo">
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
          }}
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
          {{ t('admin.accounts.gemini.setupGuide.title') }}
        </h3>
        <div class="space-y-4">
          <div>
            <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.gemini.setupGuide.checklistTitle') }}
            </p>
            <ul class="list-inside list-disc space-y-1 text-sm text-gray-600 dark:text-gray-400">
              <li>{{ t('admin.accounts.gemini.setupGuide.checklistItems.usIp') }}</li>
              <li>{{ t('admin.accounts.gemini.setupGuide.checklistItems.age') }}</li>
            </ul>
          </div>
          <div>
            <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.gemini.setupGuide.activationTitle') }}
            </p>
            <ul class="list-inside list-disc space-y-1 text-sm text-gray-600 dark:text-gray-400">
              <li>{{ t('admin.accounts.gemini.setupGuide.activationItems.geminiWeb') }}</li>
              <li>{{ t('admin.accounts.gemini.setupGuide.activationItems.gcpProject') }}</li>
            </ul>
            <div class="mt-2 flex flex-wrap gap-2">
              <a
                href="https://policies.google.com/terms"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ t('admin.accounts.gemini.setupGuide.links.countryCheck') }}
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
                {{ t('admin.accounts.gemini.setupGuide.links.geminiWebActivation') }}
              </a>
              <span class="text-gray-400">·</span>
              <a
                href="https://console.cloud.google.com"
                target="_blank"
                rel="noreferrer"
                class="text-sm text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ t('admin.accounts.gemini.setupGuide.links.gcpProject') }}
              </a>
            </div>
          </div>
        </div>
      </div>

      <!-- Quota Policy Section -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-600">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.gemini.quotaPolicy.title') }}
        </h3>
        <p class="mb-4 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.accounts.gemini.quotaPolicy.note') }}
        </p>
        <div class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead class="bg-gray-50 dark:bg-dark-600">
              <tr>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.channel') }}
                </th>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.account') }}
                </th>
                <th class="px-3 py-2 text-left font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.limits') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.channel') }}
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Free</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsFree') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Pro</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsPro') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Ultra</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsUltra') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.channel') }}
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Standard</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsStandard') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Enterprise</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsEnterprise') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel') }}
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Free</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree') }}
                </td>
              </tr>
              <tr>
                <td class="px-3 py-2 text-gray-900 dark:text-white"></td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">Paid</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-400">
                  {{ t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsPaid') }}
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
            {{ t('admin.accounts.gemini.quotaPolicy.docs.codeAssist') }}
          </a>
          <a
            :href="geminiQuotaDocs.aiStudio"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.quotaPolicy.docs.aiStudio') }}
          </a>
          <a
            :href="geminiQuotaDocs.vertex"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.quotaPolicy.docs.vertex') }}
          </a>
        </div>
      </div>

      <!-- API Key Links Section -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-600">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.gemini.helpDialog.apiKeySection') }}
        </h3>
        <div class="flex flex-wrap gap-3">
          <a
            :href="geminiHelpLinks.apiKey"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.accountType.apiKeyLink') }}
          </a>
          <a
            :href="geminiHelpLinks.aiStudioPricing"
            target="_blank"
            rel="noreferrer"
            class="text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('admin.accounts.gemini.accountType.quotaLink') }}
          </a>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="showGeminiHelpDialog = false" type="button" class="btn btn-primary">
          {{ t('common.close') }}
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
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  claudeModels,
  getPresetMappingsByPlatform,
  getModelsByPlatform,
  commonErrorCodes,
  buildModelMappingObject,
  fetchAntigravityDefaultMappings,
  isValidWildcardPattern
} from '@/composables/useModelWhitelist'
import { useAuthStore } from '@/stores/auth'
import { adminAPI } from '@/api/admin'
import { useQuotaNotifyState } from '@/composables/useQuotaNotifyState'
import {
  useAccountOAuth,
  type AddMethod,
  type AuthInputMethod
} from '@/composables/useAccountOAuth'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import { useGeminiOAuth } from '@/composables/useGeminiOAuth'
import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'
import type {
  Proxy,
  AdminGroup,
  AccountPlatform,
  AccountType,
  CheckMixedChannelResponse,
  CreateAccountRequest
} from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import QuotaLimitCard from '@/components/account/QuotaLimitCard.vue'
import { applyInterceptWarmup } from '@/components/account/credentialsBuilder'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import {
  // OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode
} from '@/utils/openaiWsMode'
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
  inputMethod: AuthInputMethod
  reset: () => void
}

const { t } = useI18n()
const authStore = useAuthStore()

const oauthStepTitle = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.oauth.openai.title')
  if (form.platform === 'gemini') return t('admin.accounts.oauth.gemini.title')
  if (form.platform === 'antigravity') return t('admin.accounts.oauth.antigravity.title')
  return t('admin.accounts.oauth.title')
})

// Platform-specific hints for API Key type
const baseUrlHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.baseUrlHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.baseUrlHint')
  return t('admin.accounts.baseUrlHint')
})

const apiKeyHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.apiKeyHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.apiKeyHint')
  return t('admin.accounts.apiKeyHint')
})

interface Props {
  show: boolean
  proxies: Proxy[]
  groups: AdminGroup[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  created: []
}>()

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
})

const currentSessionId = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.sessionId.value
  if (form.platform === 'gemini') return geminiOAuth.sessionId.value
  if (form.platform === 'antigravity') return antigravityOAuth.sessionId.value
  return oauth.sessionId.value
})

const currentOAuthLoading = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.loading.value
  if (form.platform === 'gemini') return geminiOAuth.loading.value
  if (form.platform === 'antigravity') return antigravityOAuth.loading.value
  return oauth.loading.value
})

const currentOAuthError = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.error.value
  if (form.platform === 'gemini') return geminiOAuth.error.value
  if (form.platform === 'antigravity') return antigravityOAuth.error.value
  return oauth.error.value
})

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// Model mapping type
interface ModelMapping {
  from: string
  to: string
}

interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

// State
const step = ref(1)
const submitting = ref(false)
const accountCategory = ref<'oauth-based' | 'apikey' | 'bedrock'>('oauth-based') // UI selection for account category
const addMethod = ref<AddMethod>('oauth') // For oauth-based: 'oauth' or 'setup-token'
const apiKeyBaseUrl = ref('https://api.anthropic.com')
const apiKeyValue = ref('')
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
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const DEFAULT_POOL_MODE_RETRY_COUNT = 3
const MAX_POOL_MODE_RETRY_COUNT = 10
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)
const autoPauseOnExpired = ref(true)
const openaiPassthroughEnabled = ref(false)
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexCLIOnlyEnabled = ref(false)
const anthropicPassthroughEnabled = ref(false)
const webSearchEmulationMode = ref('default')
const webSearchGlobalEnabled = ref(false)
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  writeToExtra: writeQuotaNotifyToExtra,
} = useQuotaNotifyState()

// Load global feature states once
adminAPI.settings.getWebSearchEmulationConfig().then(cfg => {
  webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
}).catch(() => { webSearchGlobalEnabled.value = false })

loadQuotaNotifyGlobal()
const mixedScheduling = ref(false) // For antigravity accounts: enable mixed scheduling
const allowOverages = ref(false) // For antigravity accounts: enable AI Credits overages
const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth') // For antigravity: oauth or upstream
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
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const getModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-model-mapping')
const getAntigravityModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-antigravity-model-mapping')
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('create-temp-unsched-rule')
const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')
const geminiAIStudioOAuthEnabled = ref(false)

function buildAntigravityExtra(): Record<string, unknown> | undefined {
  const extra: Record<string, unknown> = {}
  if (mixedScheduling.value) extra.mixed_scheduling = true
  if (allowOverages.value) extra.allow_overages = true
  return Object.keys(extra).length > 0 ? extra : undefined
}

const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string } | null>(
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
  { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') },
  { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') },
  { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') },
])
const tlsFingerprintEnabled = ref(false)
const tlsFingerprintProfileId = ref<number | null>(null)
const tlsFingerprintProfiles = ref<{ id: number; name: string }[]>([])
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
  }
})

const openAIWSModeOptions = computed(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  // TODO: ctx_pool 选项暂时隐藏，待测试完成后恢复
  // { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') }
])

const openaiResponsesWebSocketV2Mode = computed({
  get: () => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      return openaiAPIKeyResponsesWebSocketV2Mode.value
    }
    return openaiOAuthResponsesWebSocketV2Mode.value
  },
  set: (mode: OpenAIWSMode) => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      openaiAPIKeyResponsesWebSocketV2Mode.value = mode
      return
    }
    openaiOAuthResponsesWebSocketV2Mode.value = mode
  }
})

const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2Mode.value)
)

const isOpenAIModelRestrictionDisabled = computed(() =>
  form.platform === 'openai' && openaiPassthroughEnabled.value
)

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value) {
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  }
  return mixedChannelWarningRawMessage.value
})

const geminiQuotaDocs = {
  codeAssist: 'https://developers.google.com/gemini-code-assist/resources/quotas',
  aiStudio: 'https://ai.google.dev/pricing',
  vertex: 'https://cloud.google.com/vertex-ai/generative-ai/docs/quotas'
}

const geminiHelpLinks = {
  apiKey: 'https://aistudio.google.com/app/apikey',
  aiStudioPricing: 'https://ai.google.dev/pricing',
  gcpProject: 'https://console.cloud.google.com/welcome/new',
  geminiWebActivation: 'https://gemini.google.com/gems/create?hl=en-US&pli=1',
  countryCheck: 'https://policies.google.com/terms',
  countryChange: 'https://policies.google.com/country-association-form'
}

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
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    }
  }
])

const form = reactive({
  name: '',
  notes: '',
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType, // Will be 'oauth', 'setup-token', or 'apikey'
  credentials: {} as Record<string, unknown>,
  proxy_id: null as number | null,
  concurrency: 10,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  group_ids: [] as number[],
  expires_at: null as number | null
})

// Helper to check if current type needs OAuth flow
const isOAuthFlow = computed(() => {
  // Antigravity upstream 类型不需要 OAuth 流程
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    return false
  }
  // Bedrock 类型不需要 OAuth 流程
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    return false
  }
  return accountCategory.value === 'oauth-based'
})

const isManualInputMethod = computed(() => {
  return oauthFlowRef.value?.inputMethod === 'manual'
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocal(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocal(value)
  }
})

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  if (form.platform === 'openai') {
    return authCode.trim() && openaiOAuth.sessionId.value && !openaiOAuth.loading.value
  }
  if (form.platform === 'gemini') {
    return authCode.trim() && geminiOAuth.sessionId.value && !geminiOAuth.loading.value
  }
  if (form.platform === 'antigravity') {
    return authCode.trim() && antigravityOAuth.sessionId.value && !antigravityOAuth.loading.value
  }
  return authCode.trim() && oauth.sessionId.value && !oauth.loading.value
})

// Watchers
watch(
  () => props.show,
  (newVal) => {
    if (newVal) {
      // Load TLS fingerprint profiles
      adminAPI.tlsFingerprintProfiles.list()
        .then(profiles => { tlsFingerprintProfiles.value = profiles.map(p => ({ id: p.id, name: p.name })) })
        .catch(() => { tlsFingerprintProfiles.value = [] })
      // Modal opened - fill related models
      allowedModels.value = [...getModelsByPlatform(form.platform)]
      // Antigravity: 默认使用映射模式并填充默认映射
      if (form.platform === 'antigravity') {
        antigravityModelRestrictionMode.value = 'mapping'
        fetchAntigravityDefaultMappings().then(mappings => {
          antigravityModelMappings.value = [...mappings]
        })
        antigravityWhitelistModels.value = []
      } else {
        antigravityWhitelistModels.value = []
        antigravityModelMappings.value = []
        antigravityModelRestrictionMode.value = 'mapping'
      }
    } else {
      resetForm()
    }
  }
)

// Sync form.type based on accountCategory, addMethod, and platform-specific type
watch(
  [accountCategory, addMethod, antigravityAccountType],
  ([category, method, agType]) => {
    // Antigravity upstream 类型（实际创建为 apikey）
    if (form.platform === 'antigravity' && agType === 'upstream') {
      form.type = 'apikey'
      return
    }
    // Bedrock 类型
    if (form.platform === 'anthropic' && category === 'bedrock') {
      form.type = 'bedrock' as AccountType
      return
    }
    if (category === 'oauth-based') {
      form.type = method as AccountType // 'oauth' or 'setup-token'
    } else {
      form.type = 'apikey'
    }
  },
  { immediate: true }
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
      })
      antigravityWhitelistModels.value = []
      accountCategory.value = 'oauth-based'
      antigravityAccountType.value = 'oauth'
    } else {
      allowOverages.value = false
      antigravityWhitelistModels.value = []
      antigravityModelMappings.value = []
      antigravityModelRestrictionMode.value = 'mapping'
    }
    // Reset Bedrock fields when switching platforms
    bedrockAccessKeyId.value = ''
    bedrockSecretAccessKey.value = ''
    bedrockSessionToken.value = ''
    bedrockRegion.value = 'us-east-1'
    bedrockForceGlobal.value = false
    bedrockAuthMode.value = 'sigv4'
    bedrockApiKeyValue.value = ''
    // Reset Anthropic/Antigravity-specific settings when switching to other platforms
    if (newPlatform !== 'anthropic' && newPlatform !== 'antigravity') {
      interceptWarmupRequests.value = false
    }
    if (newPlatform !== 'openai') {
      openaiPassthroughEnabled.value = false
      openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
      codexCLIOnlyEnabled.value = false
    }
    if (newPlatform !== 'anthropic') {
      anthropicPassthroughEnabled.value = false
      webSearchEmulationMode.value = 'default'
    }
    // Reset OAuth states
    oauth.resetState()
    openaiOAuth.resetState()

    geminiOAuth.resetState()
    antigravityOAuth.resetState()
  }
)

// Gemini AI Studio OAuth availability (requires operator-configured OAuth client)
watch(
  [accountCategory, () => form.platform],
  ([category, platform]) => {
    if (platform === 'openai' && category !== 'oauth-based') {
      codexCLIOnlyEnabled.value = false
    }
    if (platform !== 'anthropic' || category !== 'apikey') {
      anthropicPassthroughEnabled.value = false
      webSearchEmulationMode.value = 'default'
    }
  }
)

watch(
  [() => props.show, () => form.platform, accountCategory],
  async ([show, platform, category]) => {
    if (!show || platform !== 'gemini' || category !== 'oauth-based') {
      geminiAIStudioOAuthEnabled.value = false
      return
    }
    const caps = await geminiOAuth.getCapabilities()
    geminiAIStudioOAuthEnabled.value = !!caps?.ai_studio_oauth_enabled
    if (!geminiAIStudioOAuthEnabled.value && geminiOAuthType.value === 'ai_studio') {
      geminiOAuthType.value = 'code_assist'
    }
  },
  { immediate: true }
)

const handleSelectGeminiOAuthType = (oauthType: 'code_assist' | 'google_one' | 'ai_studio') => {
  if (oauthType === 'ai_studio' && !geminiAIStudioOAuthEnabled.value) {
    appStore.showError(t('admin.accounts.oauth.gemini.aiStudioNotConfigured'))
    return
  }
  geminiOAuthType.value = oauthType
}

// Auto-fill related models when switching to whitelist mode or changing platform
watch(
  [modelRestrictionMode, () => form.platform],
  ([newMode]) => {
    if (newMode === 'whitelist') {
      allowedModels.value = [...getModelsByPlatform(form.platform)]
    }
  }
)

watch(
  [antigravityModelRestrictionMode, () => form.platform],
  ([, platform]) => {
    if (platform !== 'antigravity') return
    // Antigravity 默认不做限制：白名单留空表示允许所有（包含未来新增模型）。
    // 如果需要快速填充常用模型，可在组件内点“填充相关模型”。
  }
)

// Model mapping helpers
const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}

const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const addPresetMapping = (from: string, to: string) => {
  if (modelMappings.value.some((m) => m.from === from)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  modelMappings.value.push({ from, to })
}

const addAntigravityModelMapping = () => {
  antigravityModelMappings.value.push({ from: '', to: '' })
}

const removeAntigravityModelMapping = (index: number) => {
  antigravityModelMappings.value.splice(index, 1)
}

const addAntigravityPresetMapping = (from: string, to: string) => {
  if (antigravityModelMappings.value.some((m) => m.from === from)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  antigravityModelMappings.value.push({ from, to })
}

// Error code toggle helper
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

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
  if (preset) {
    tempUnschedRules.value.push({ ...preset })
    return
  }
  tempUnschedRules.value.push({
    error_code: null,
    keywords: '',
    duration_minutes: 30,
    description: ''
  })
}

const removeTempUnschedRule = (index: number) => {
  tempUnschedRules.value.splice(index, 1)
}

const moveTempUnschedRule = (index: number, direction: number) => {
  const target = index + direction
  if (target < 0 || target >= tempUnschedRules.value.length) return
  const rules = tempUnschedRules.value
  const current = rules[index]
  rules[index] = rules[target]
  rules[target] = current
}

const buildTempUnschedRules = (rules: TempUnschedRuleForm[]) => {
  const out: Array<{
    error_code: number
    keywords: string[]
    duration_minutes: number
    description: string
  }> = []

  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) {
      continue
    }
    if (!Number.isFinite(duration) || duration <= 0) {
      continue
    }
    if (keywords.length === 0) {
      continue
    }
    out.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim()
    })
  }

  return out
}

const applyTempUnschedConfig = (credentials: Record<string, unknown>) => {
  if (!tempUnschedEnabled.value) {
    delete credentials.temp_unschedulable_enabled
    delete credentials.temp_unschedulable_rules
    return true
  }

  const rules = buildTempUnschedRules(tempUnschedRules.value)
  if (rules.length === 0) {
    appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
    return false
  }

  credentials.temp_unschedulable_enabled = true
  credentials.temp_unschedulable_rules = rules
  return true
}

const splitTempUnschedKeywords = (value: string) => {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

const needsMixedChannelCheck = (platform: AccountPlatform) => platform === 'antigravity' || platform === 'anthropic'

const buildMixedChannelDetails = (resp?: CheckMixedChannelResponse) => {
  const details = resp?.details
  if (!details) {
    return null
  }
  return {
    groupName: details.group_name || 'Unknown',
    currentPlatform: details.current_platform || 'Unknown',
    otherPlatform: details.other_platform || 'Unknown'
  }
}

const clearMixedChannelDialog = () => {
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
}

const openMixedChannelDialog = (opts: {
  response?: CheckMixedChannelResponse
  message?: string
  onConfirm: () => Promise<void>
}) => {
  mixedChannelWarningDetails.value = buildMixedChannelDetails(opts.response)
  mixedChannelWarningRawMessage.value =
    opts.message || opts.response?.message || t('admin.accounts.failedToCreate')
  mixedChannelWarningAction.value = opts.onConfirm
  showMixedChannelWarning.value = true
}

const withAntigravityConfirmFlag = (payload: CreateAccountRequest): CreateAccountRequest => {
  if (needsMixedChannelCheck(payload.platform) && antigravityMixedChannelConfirmed.value) {
    return {
      ...payload,
      confirm_mixed_channel_risk: true
    }
  }
  const cloned = { ...payload }
  delete cloned.confirm_mixed_channel_risk
  return cloned
}

const ensureAntigravityMixedChannelConfirmed = async (onConfirm: () => Promise<void>): Promise<boolean> => {
  if (!needsMixedChannelCheck(form.platform)) {
    return true
  }
  if (antigravityMixedChannelConfirmed.value) {
    return true
  }

  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({
      platform: form.platform,
      group_ids: form.group_ids
    })
    if (!result.has_risk) {
      return true
    }
    openMixedChannelDialog({
      response: result,
      onConfirm: async () => {
        antigravityMixedChannelConfirmed.value = true
        await onConfirm()
      }
    })
    return false
  } catch (error: any) {
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
    return false
  }
}

const submitCreateAccount = async (payload: CreateAccountRequest) => {
  submitting.value = true
  try {
    await adminAPI.accounts.create(withAntigravityConfirmFlag(payload))
    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  } catch (error: any) {
    if (error.response?.status === 409 && error.response?.data?.error === 'mixed_channel_warning' && needsMixedChannelCheck(form.platform)) {
      openMixedChannelDialog({
        message: error.response?.data?.message,
        onConfirm: async () => {
          antigravityMixedChannelConfirmed.value = true
          await submitCreateAccount(payload)
        }
      })
      return
    }
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
  } finally {
    submitting.value = false
  }
}

// Methods
const resetForm = () => {
  step.value = 1
  form.name = ''
  form.notes = ''
  form.platform = 'anthropic'
  form.type = 'oauth'
  form.credentials = {}
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
  modelRestrictionMode.value = 'whitelist'
  allowedModels.value = [...claudeModels] // Default fill related models

  antigravityModelRestrictionMode.value = 'mapping'
  antigravityWhitelistModels.value = []
  fetchAntigravityDefaultMappings().then(mappings => {
    antigravityModelMappings.value = [...mappings]
  })
  poolModeEnabled.value = false
  poolModeRetryCount.value = DEFAULT_POOL_MODE_RETRY_COUNT
  customErrorCodesEnabled.value = false
  selectedErrorCodes.value = []
  customErrorCodeInput.value = null
  interceptWarmupRequests.value = false
  autoPauseOnExpired.value = true
  openaiPassthroughEnabled.value = false
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexCLIOnlyEnabled.value = false
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
  upstreamBaseUrl.value = ''
  upstreamApiKey.value = ''
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
}

const handleClose = () => {
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
  emit('close')
}

const buildOpenAIExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'openai') {
    return base
  }

  const extra: Record<string, unknown> = { ...(base || {}) }
  if (accountCategory.value === 'oauth-based') {
    extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthResponsesWebSocketV2Mode.value)
  } else if (accountCategory.value === 'apikey') {
    extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyResponsesWebSocketV2Mode.value)
  }
  // 清理兼容旧键，统一改用分类型开关。
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled
  if (openaiPassthroughEnabled.value) {
    extra.openai_passthrough = true
  } else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  }

  if (accountCategory.value === 'oauth-based' && codexCLIOnlyEnabled.value) {
    extra.codex_cli_only = true
  } else {
    delete extra.codex_cli_only
  }

  return Object.keys(extra).length > 0 ? extra : undefined
}

const buildAnthropicExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'anthropic' || accountCategory.value !== 'apikey') {
    return base
  }

  const extra: Record<string, unknown> = { ...(base || {}) }
  if (anthropicPassthroughEnabled.value) {
    extra.anthropic_passthrough = true
  } else {
    delete extra.anthropic_passthrough
  }
  if (webSearchEmulationMode.value === 'default') {
    delete extra.web_search_emulation
  } else {
    extra.web_search_emulation = webSearchEmulationMode.value
  }

  return Object.keys(extra).length > 0 ? extra : undefined
}

// Helper function to create account with mixed channel warning handling
const doCreateAccount = async (payload: CreateAccountRequest) => {
  const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
    await submitCreateAccount(payload)
  })
  if (!canContinue) {
    return
  }
  await submitCreateAccount(payload)
}

// Handle mixed channel warning confirmation
const handleMixedChannelConfirm = async () => {
  const action = mixedChannelWarningAction.value
  if (!action) {
    clearMixedChannelDialog()
    return
  }
  clearMixedChannelDialog()
  submitting.value = true
  try {
    await action()
  } finally {
    submitting.value = false
  }
}

const handleMixedChannelCancel = () => {
  clearMixedChannelDialog()
}

const normalizePoolModeRetryCount = (value: number) => {
  if (!Number.isFinite(value)) {
    return DEFAULT_POOL_MODE_RETRY_COUNT
  }
  const normalized = Math.trunc(value)
  if (normalized < 0) {
    return 0
  }
  if (normalized > MAX_POOL_MODE_RETRY_COUNT) {
    return MAX_POOL_MODE_RETRY_COUNT
  }
  return normalized
}

const handleSubmit = async () => {
  // For OAuth-based type, handle OAuth flow (goes to step 2)
  if (isOAuthFlow.value) {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
      step.value = 2
    })
    if (!canContinue) {
      return
    }
    step.value = 2
    return
  }

  // For Bedrock type, create directly
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }

    const credentials: Record<string, unknown> = {
      auth_mode: bedrockAuthMode.value,
      aws_region: bedrockRegion.value.trim() || 'us-east-1',
    }

    if (bedrockAuthMode.value === 'sigv4') {
      if (!bedrockAccessKeyId.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockAccessKeyIdRequired'))
        return
      }
      if (!bedrockSecretAccessKey.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockSecretAccessKeyRequired'))
        return
      }
      credentials.aws_access_key_id = bedrockAccessKeyId.value.trim()
      credentials.aws_secret_access_key = bedrockSecretAccessKey.value.trim()
      if (bedrockSessionToken.value.trim()) {
        credentials.aws_session_token = bedrockSessionToken.value.trim()
      }
    } else {
      if (!bedrockApiKeyValue.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockApiKeyRequired'))
        return
      }
      credentials.api_key = bedrockApiKeyValue.value.trim()
    }

    if (bedrockForceGlobal.value) {
      credentials.aws_force_global = 'true'
    }

    // Model mapping
    const modelMapping = buildModelMappingObject(
      modelRestrictionMode.value, allowedModels.value, modelMappings.value
    )
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }

    // Pool mode
    if (poolModeEnabled.value) {
      credentials.pool_mode = true
      credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
    }

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    await createAccountAndFinish('anthropic', 'bedrock' as AccountType, credentials)
    return
  }

  // For Antigravity upstream type, create directly
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    if (!upstreamBaseUrl.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterBaseUrl'))
      return
    }
    if (!upstreamApiKey.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterApiKey'))
      return
    }

    // Build upstream credentials (and optional model restriction)
    const credentials: Record<string, unknown> = {
      base_url: upstreamBaseUrl.value.trim(),
      api_key: upstreamApiKey.value.trim()
    }

    // Antigravity 只使用映射模式
    const antigravityModelMapping = buildModelMappingObject(
      'mapping',
      [],
      antigravityModelMappings.value
    )
    if (antigravityModelMapping) {
      credentials.model_mapping = antigravityModelMapping
    }

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    const extra = buildAntigravityExtra()
    await createAccountAndFinish(form.platform, 'apikey', credentials, extra)
    return
  }

  // For apikey type, create directly
  if (!apiKeyValue.value.trim()) {
    appStore.showError(t('admin.accounts.pleaseEnterApiKey'))
    return
  }

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
  }
  if (form.platform === 'gemini') {
    credentials.tier_id = geminiTierAIStudio.value
  }

  // Add model mapping if configured（OpenAI 开启自动透传时不应用）
  if (!isOpenAIModelRestrictionDisabled.value) {
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }
  }

  // Add pool mode if enabled
  if (poolModeEnabled.value) {
    credentials.pool_mode = true
    credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
  }

  // Add custom error codes if enabled
  if (customErrorCodesEnabled.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
  }

  applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
  if (!applyTempUnschedConfig(credentials)) {
    return
  }

  form.credentials = credentials
  const extra = buildAnthropicExtra(buildOpenAIExtra())

  await doCreateAccount({
    ...form,
    group_ids: form.group_ids,
    extra,
    auto_pause_on_expired: autoPauseOnExpired.value
  })
}

const goBackToBasicInfo = () => {
  step.value = 1
  oauth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const handleGenerateUrl = async () => {
  if (form.platform === 'openai') {
    await openaiOAuth.generateAuthUrl(form.proxy_id)
  } else if (form.platform === 'gemini') {
    await geminiOAuth.generateAuthUrl(
      form.proxy_id,
      oauthFlowRef.value?.projectId,
      geminiOAuthType.value,
      geminiSelectedTier.value
    )
  } else if (form.platform === 'antigravity') {
    await antigravityOAuth.generateAuthUrl(form.proxy_id)
  } else {
    await oauth.generateAuthUrl(addMethod.value, form.proxy_id)
  }
}

const handleValidateRefreshToken = (rt: string) => {
  if (form.platform === 'openai') {
    handleOpenAIValidateRT(rt)
  } else if (form.platform === 'antigravity') {
    handleAntigravityValidateRT(rt)
  }
}

const handleValidateSessionToken = (_sessionToken: string) => {
  // Session token validation removed
}

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
  }
  // Inject quota limits for apikey/bedrock accounts
  let finalExtra = extra
  if (type === 'apikey' || type === 'bedrock') {
    const quotaExtra: Record<string, unknown> = { ...(extra || {}) }
    if (editQuotaLimit.value != null && editQuotaLimit.value > 0) {
      quotaExtra.quota_limit = editQuotaLimit.value
    }
    if (editQuotaDailyLimit.value != null && editQuotaDailyLimit.value > 0) {
      quotaExtra.quota_daily_limit = editQuotaDailyLimit.value
    }
    if (editQuotaWeeklyLimit.value != null && editQuotaWeeklyLimit.value > 0) {
      quotaExtra.quota_weekly_limit = editQuotaWeeklyLimit.value
    }
    // Quota reset mode config
    if (editDailyResetMode.value === 'fixed') {
      quotaExtra.quota_daily_reset_mode = 'fixed'
      quotaExtra.quota_daily_reset_hour = editDailyResetHour.value ?? 0
    }
    if (editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_weekly_reset_mode = 'fixed'
      quotaExtra.quota_weekly_reset_day = editWeeklyResetDay.value ?? 1
      quotaExtra.quota_weekly_reset_hour = editWeeklyResetHour.value ?? 0
    }
    if (editDailyResetMode.value === 'fixed' || editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_reset_timezone = editResetTimezone.value || 'UTC'
    }
    // Quota notify config
    writeQuotaNotifyToExtra(quotaExtra, 'create')
    if (Object.keys(quotaExtra).length > 0) {
      finalExtra = quotaExtra
    }
  }
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
  })
}

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
    }

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
      }
    }

    // 应用临时不可调度配置
    if (!applyTempUnschedConfig(credentials)) {
      return
    }

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
      })
      appStore.showSuccess(t('admin.accounts.accountCreated'))
    }

    emit('created')
    handleClose()
  } catch (error: any) {
    oauthClient.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauthClient.error.value)
  } finally {
    oauthClient.loading.value = false
  }
}

// OpenAI 手动 RT 批量验证和创建
// OpenAI Mobile RT client_id
const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

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
  }

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
          errors.push(`#${i + 1}: ${oauthClient.error.value || 'Validation failed'}`)
          oauthClient.error.value = ''
          continue
        }

        const credentials = oauthClient.buildCredentials(tokenInfo)
        if (clientId) {
          credentials.client_id = clientId
        }
        const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
        const extra = buildOpenAIExtra(oauthExtra)

        // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
        if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
          const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
          if (modelMapping) {
            credentials.model_mapping = modelMapping
          }
        }

        // Generate account name; fallback to email if name is empty (ent schema requires NotEmpty)
        const baseName = form.name || tokenInfo.email || 'OpenAI OAuth Account'
        const accountName = refreshTokens.length > 1 ? `${baseName} #${i + 1}` : baseName

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
          })
        }

        successCount++
      } catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1}: ${errMsg}`)
      }
    }

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    } else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
      )
      oauthClient.error.value = errors.join('\n')
      emit('created')
    } else {
      oauthClient.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    }
  } finally {
    oauthClient.loading.value = false
  }
}

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
  }

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
          errors.push(`#${i + 1}: ${antigravityOAuth.error.value || 'Validation failed'}`)
          antigravityOAuth.error.value = ''
          continue
        }

        const credentials = antigravityOAuth.buildCredentials(tokenInfo)
        
        // Generate account name with index for batch
        const accountName = refreshTokens.length > 1 ? `${form.name} #${i + 1}` : form.name

        // Note: Antigravity doesn't have buildExtraInfo, so we pass empty extra or rely on credentials
        const createPayload = withAntigravityConfirmFlag({
          name: accountName,
          notes: form.notes,
          platform: 'antigravity',
          type: 'oauth',
          credentials,
          extra: {},
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        })
        await adminAPI.accounts.create(createPayload)
        successCount++
      } catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1}: ${errMsg}`)
      }
    }

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    } else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
      )
      antigravityOAuth.error.value = errors.join('\n')
      emit('created')
    } else {
      antigravityOAuth.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    }
  } finally {
    antigravityOAuth.loading.value = false
  }
}

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
    }

    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: geminiOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id,
      oauthType: geminiOAuthType.value,
      tierId: geminiSelectedTier.value
    })
    if (!tokenInfo) return

    const credentials = geminiOAuth.buildCredentials(tokenInfo)
    const extra = geminiOAuth.buildExtraInfo(tokenInfo)
    await createAccountAndFinish('gemini', 'oauth', credentials, extra)
  } catch (error: any) {
    geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(geminiOAuth.error.value)
  } finally {
    geminiOAuth.loading.value = false
  }
}

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
    }

    const tokenInfo = await antigravityOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: antigravityOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id
    })
		if (!tokenInfo) return

		const credentials = antigravityOAuth.buildCredentials(tokenInfo)
		applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
		// Antigravity 只使用映射模式
		const antigravityModelMapping = buildModelMappingObject(
			'mapping',
			[],
			antigravityModelMappings.value
		)
		if (antigravityModelMapping) {
			credentials.model_mapping = antigravityModelMapping
		}
		const extra = buildAntigravityExtra()
		await createAccountAndFinish('antigravity', 'oauth', credentials, extra)
  } catch (error: any) {
    antigravityOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(antigravityOAuth.error.value)
  } finally {
    antigravityOAuth.loading.value = false
  }
}

// Anthropic OAuth 授权码兑换
const handleAnthropicExchange = async (authCode: string) => {
  if (!authCode.trim() || !oauth.sessionId.value) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id } : {}
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/exchange-code'
        : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: oauth.sessionId.value,
      code: authCode.trim(),
      ...proxyConfig
    })

    // Build extra with quota control settings
    const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
    const extra: Record<string, unknown> = { ...baseExtra }

    // Add window cost limit settings
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      extra.window_cost_limit = windowCostLimit.value
      extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    }

    // Add session limit settings
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      extra.max_sessions = maxSessions.value
      extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    }

    // Add RPM limit settings
    if (rpmLimitEnabled.value) {
      const DEFAULT_BASE_RPM = 15
      extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
        ? baseRpm.value
        : DEFAULT_BASE_RPM
      extra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
        extra.rpm_sticky_buffer = rpmStickyBuffer.value
      }
    }

    // UMQ mode（独立于 RPM）
    if (userMsgQueueMode.value) {
      extra.user_msg_queue_mode = userMsgQueueMode.value
    }

    // Add TLS fingerprint settings
    if (tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) {
        extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
      }
    }

    // Add session ID masking settings
    if (sessionIdMaskingEnabled.value) {
      extra.session_id_masking_enabled = true
    }

    // Add cache TTL override settings
    if (cacheTTLOverrideEnabled.value) {
      extra.cache_ttl_override_enabled = true
      extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    }

    // Add custom base URL settings
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = customBaseUrl.value.trim()
    }

    const credentials: Record<string, unknown> = { ...tokenInfo }
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
    await createAccountAndFinish(form.platform, addMethod.value as AccountType, credentials, extra)
  } catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauth.error.value)
  } finally {
    oauth.loading.value = false
  }
}

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

    const tempUnschedPayload = tempUnschedEnabled.value
      ? buildTempUnschedRules(tempUnschedRules.value)
      : []
    if (tempUnschedEnabled.value && tempUnschedPayload.length === 0) {
      appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
      return
    }

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
        })

        // Build extra with quota control settings
        const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
        const extra: Record<string, unknown> = { ...baseExtra }

        // Add window cost limit settings
        if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
          extra.window_cost_limit = windowCostLimit.value
          extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
        }

        // Add session limit settings
        if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
          extra.max_sessions = maxSessions.value
          extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
        }

        // Add RPM limit settings
        if (rpmLimitEnabled.value) {
          const DEFAULT_BASE_RPM = 15
          extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
            ? baseRpm.value
            : DEFAULT_BASE_RPM
          extra.rpm_strategy = rpmStrategy.value
          if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
            extra.rpm_sticky_buffer = rpmStickyBuffer.value
          }
        }

        // UMQ mode（独立于 RPM）
        if (userMsgQueueMode.value) {
          extra.user_msg_queue_mode = userMsgQueueMode.value
        }

        // Add TLS fingerprint settings
        if (tlsFingerprintEnabled.value) {
          extra.enable_tls_fingerprint = true
          if (tlsFingerprintProfileId.value) {
            extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
          }
        }

        // Add session ID masking settings
        if (sessionIdMaskingEnabled.value) {
          extra.session_id_masking_enabled = true
        }

        // Add cache TTL override settings
        if (cacheTTLOverrideEnabled.value) {
          extra.cache_ttl_override_enabled = true
          extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
        }

        // Add custom base URL settings
        if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
          extra.custom_base_url_enabled = true
          extra.custom_base_url = customBaseUrl.value.trim()
        }

        const accountName = keys.length > 1 ? `${form.name} #${i + 1}` : form.name

        const credentials: Record<string, unknown> = { ...tokenInfo }
        applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
        if (tempUnschedEnabled.value) {
          credentials.temp_unschedulable_enabled = true
          credentials.temp_unschedulable_rules = tempUnschedPayload
        }

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
        })

        successCount++
      } catch (error: any) {
        failedCount++
        errors.push(
          t('admin.accounts.oauth.keyAuthFailed', {
            index: i + 1,
            error: error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
          })
        )
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
