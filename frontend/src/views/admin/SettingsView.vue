<template>
  <AppLayout>
    <div class="max-w-4xl mx-auto space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>

      <!-- Settings Form -->
      <form v-else @submit.prevent="saveSettings" class="space-y-6">
        <!-- Admin API Key Settings -->
        <div class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.adminApiKey.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.adminApiKey.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6 space-y-4">
            <!-- Security Warning -->
            <div class="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4">
              <div class="flex items-start">
                <svg class="w-5 h-5 text-amber-500 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd"/>
                </svg>
                <p class="ml-3 text-sm text-amber-700 dark:text-amber-300">
                  {{ t('admin.settings.adminApiKey.securityWarning') REDACTEDREDACTED
                </p>
              </div>
            </div>

            <!-- Loading State -->
            <div v-if="adminApiKeyLoading" class="flex items-center gap-2 text-gray-500">
              <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary-600"></div>
              {{ t('common.loading') REDACTEDREDACTED
            </div>

            <!-- No Key Configured -->
            <div v-else-if="!adminApiKeyExists" class="flex items-center justify-between">
              <span class="text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.adminApiKey.notConfigured') REDACTEDREDACTED
              </span>
              <button
                type="button"
                @click="createAdminApiKey"
                :disabled="adminApiKeyOperating"
                class="btn btn-primary btn-sm"
              >
                <svg v-if="adminApiKeyOperating" class="animate-spin h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                {{ adminApiKeyOperating ? t('admin.settings.adminApiKey.creating') : t('admin.settings.adminApiKey.create') REDACTEDREDACTED
              </button>
            </div>

            <!-- Key Exists -->
            <div v-else class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    {{ t('admin.settings.adminApiKey.currentKey') REDACTEDREDACTED
                  </label>
                  <code class="text-sm font-mono text-gray-900 dark:text-gray-100 bg-gray-100 dark:bg-dark-700 px-2 py-1 rounded">
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
                    {{ adminApiKeyOperating ? t('admin.settings.adminApiKey.regenerating') : t('admin.settings.adminApiKey.regenerate') REDACTEDREDACTED
                  </button>
                  <button
                    type="button"
                    @click="deleteAdminApiKey"
                    :disabled="adminApiKeyOperating"
                    class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                  >
                    {{ t('admin.settings.adminApiKey.delete') REDACTEDREDACTED
                  </button>
                </div>
              </div>

              <!-- Newly Generated Key Display -->
              <div v-if="newAdminApiKey" class="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-4 space-y-3">
                <p class="text-sm font-medium text-green-700 dark:text-green-300">
                  {{ t('admin.settings.adminApiKey.keyWarning') REDACTEDREDACTED
                </p>
                <div class="flex items-center gap-2">
                  <code class="flex-1 text-sm font-mono bg-white dark:bg-dark-800 px-3 py-2 rounded border border-green-300 dark:border-green-700 break-all select-all">
                    {{ newAdminApiKey REDACTEDREDACTED
                  </code>
                  <button
                    type="button"
                    @click="copyNewKey"
                    class="btn btn-primary btn-sm flex-shrink-0"
                  >
                    {{ t('admin.settings.adminApiKey.copyKey') REDACTEDREDACTED
                  </button>
                </div>
                <p class="text-xs text-green-600 dark:text-green-400">
                  {{ t('admin.settings.adminApiKey.usage') REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Registration Settings -->
        <div class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.registration.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.registration.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6 space-y-5">
            <!-- Enable Registration -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.registration.enableRegistration') REDACTEDREDACTED</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.registration.enableRegistrationHint') REDACTEDREDACTED</p>
              </div>
              <Toggle v-model="form.registration_enabled" />
            </div>

            <!-- Email Verification -->
            <div class="flex items-center justify-between pt-4 border-t border-gray-100 dark:border-dark-700">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.registration.emailVerification') REDACTEDREDACTED</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.registration.emailVerificationHint') REDACTEDREDACTED</p>
              </div>
              <Toggle v-model="form.email_verify_enabled" />
            </div>
          </div>
        </div>

        <!-- Cloudflare Turnstile Settings -->
        <div class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.turnstile.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.turnstile.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6 space-y-5">
            <!-- Enable Turnstile -->
            <div class="flex items-center justify-between">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.turnstile.enableTurnstile') REDACTEDREDACTED</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.turnstile.enableTurnstileHint') REDACTEDREDACTED</p>
              </div>
              <Toggle v-model="form.turnstile_enabled" />
            </div>

            <!-- Turnstile Keys - Only show when enabled -->
            <div v-if="form.turnstile_enabled" class="pt-4 border-t border-gray-100 dark:border-dark-700">
              <div class="grid grid-cols-1 gap-6">
                <div>
                  <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                    {{ t('admin.settings.turnstile.siteKey') REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.turnstile_site_key"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="0x4AAAAAAA..."
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.turnstile.siteKeyHint') REDACTEDREDACTED
                    <a href="https://dash.cloudflare.com/" target="_blank" class="text-primary-600 hover:text-primary-500">Cloudflare Dashboard</a>
                  </p>
                </div>
                <div>
                  <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                    {{ t('admin.settings.turnstile.secretKey') REDACTEDREDACTED
                  </label>
                  <input
                    v-model="form.turnstile_secret_key"
                    type="password"
                    class="input font-mono text-sm"
                    placeholder="0x4AAAAAAA..."
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.turnstile.secretKeyHint') REDACTEDREDACTED</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Default Settings -->
        <div class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.defaults.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.defaults.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.defaults.defaultBalance') REDACTEDREDACTED
                </label>
                <input
                  v-model.number="form.default_balance"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input"
                  placeholder="0.00"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.defaults.defaultBalanceHint') REDACTEDREDACTED</p>
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.defaults.defaultConcurrency') REDACTEDREDACTED
                </label>
                <input
                  v-model.number="form.default_concurrency"
                  type="number"
                  min="1"
                  class="input"
                  placeholder="1"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.defaults.defaultConcurrencyHint') REDACTEDREDACTED</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Site Settings -->
        <div class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.site.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.site.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6 space-y-6">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.site.siteName') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.site_name"
                  type="text"
                  class="input"
                  placeholder="Sub2API"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.site.siteNameHint') REDACTEDREDACTED</p>
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.site.siteSubtitle') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.site_subtitle"
                  type="text"
                  class="input"
                  placeholder="Subscription to API Conversion Platform"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.site.siteSubtitleHint') REDACTEDREDACTED</p>
              </div>
            </div>

            <!-- API Base URL -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {{ t('admin.settings.site.apiBaseUrl') REDACTEDREDACTED
              </label>
              <input
                v-model="form.api_base_url"
                type="text"
                class="input font-mono text-sm"
                placeholder="https://api.example.com"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.site.apiBaseUrlHint') REDACTEDREDACTED</p>
            </div>

            <!-- Contact Info -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {{ t('admin.settings.site.contactInfo') REDACTEDREDACTED
              </label>
              <input
                v-model="form.contact_info"
                type="text"
                class="input"
                :placeholder="t('admin.settings.site.contactInfoPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.site.contactInfoHint') REDACTEDREDACTED</p>
            </div>

            <!-- Doc URL -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {{ t('admin.settings.site.docUrl') REDACTEDREDACTED
              </label>
              <input
                v-model="form.doc_url"
                type="url"
                class="input font-mono text-sm"
                placeholder="https://docs.example.com"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.site.docUrlHint') REDACTEDREDACTED</p>
            </div>

            <!-- Site Logo Upload -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {{ t('admin.settings.site.siteLogo') REDACTEDREDACTED
              </label>
              <div class="flex items-start gap-6">
                <!-- Logo Preview -->
                <div class="flex-shrink-0">
                  <div
                    class="w-20 h-20 rounded-xl border-2 border-dashed border-gray-300 dark:border-dark-600 flex items-center justify-center overflow-hidden bg-gray-50 dark:bg-dark-800"
                    :class="{ 'border-solid': form.site_logo REDACTED"
                  >
                    <img
                      v-if="form.site_logo"
                      :src="form.site_logo"
                      alt="Site Logo"
                      class="w-full h-full object-contain"
                    />
                    <svg v-else class="w-8 h-8 text-gray-400 dark:text-dark-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                  </div>
                </div>
                <!-- Upload Controls -->
                <div class="flex-1 space-y-3">
                  <div class="flex items-center gap-3">
                    <label class="btn btn-secondary btn-sm cursor-pointer">
                      <input
                        type="file"
                        accept="image/*"
                        class="hidden"
                        @change="handleLogoUpload"
                      />
                      <svg class="w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                      </svg>
                      {{ t('admin.settings.site.uploadImage') REDACTEDREDACTED
                    </label>
                    <button
                      v-if="form.site_logo"
                      type="button"
                      @click="form.site_logo = ''"
                      class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                    >
                      <svg class="w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                      {{ t('admin.settings.site.remove') REDACTEDREDACTED
                    </button>
                  </div>
                  <p class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.site.logoHint') REDACTEDREDACTED
                  </p>
                  <p v-if="logoError" class="text-xs text-red-500">{{ logoError REDACTEDREDACTED</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- SMTP Settings - Only show when email verification is enabled -->
        <div v-if="form.email_verify_enabled" class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700 flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.smtp.title') REDACTEDREDACTED</h2>
              <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.smtp.description') REDACTEDREDACTED</p>
            </div>
            <button
              type="button"
              @click="testSmtpConnection"
              :disabled="testingSmtp"
              class="btn btn-secondary btn-sm"
            >
              <svg v-if="testingSmtp" class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              {{ testingSmtp ? t('admin.settings.smtp.testing') : t('admin.settings.smtp.testConnection') REDACTEDREDACTED
            </button>
          </div>
          <div class="p-6 space-y-6">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.host') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.smtp_host"
                  type="text"
                  class="input"
                  placeholder="smtp.gmail.com"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.port') REDACTEDREDACTED
                </label>
                <input
                  v-model.number="form.smtp_port"
                  type="number"
                  min="1"
                  max="65535"
                  class="input"
                  placeholder="587"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.username') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.smtp_username"
                  type="text"
                  class="input"
                  placeholder="your-email@gmail.com"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.password') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.smtp_password"
                  type="password"
                  class="input"
                  placeholder="********"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.smtp.passwordHint') REDACTEDREDACTED</p>
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.fromEmail') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.smtp_from_email"
                  type="email"
                  class="input"
                  placeholder="noreply@example.com"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.smtp.fromName') REDACTEDREDACTED
                </label>
                <input
                  v-model="form.smtp_from_name"
                  type="text"
                  class="input"
                  placeholder="Sub2API"
                />
              </div>
            </div>

            <!-- Use TLS Toggle -->
            <div class="flex items-center justify-between pt-4 border-t border-gray-100 dark:border-dark-700">
              <div>
                <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.smtp.useTls') REDACTEDREDACTED</label>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.smtp.useTlsHint') REDACTEDREDACTED</p>
              </div>
              <Toggle v-model="form.smtp_use_tls" />
            </div>
          </div>
        </div>

        <!-- Send Test Email - Only show when email verification is enabled -->
        <div v-if="form.email_verify_enabled" class="card">
          <div class="px-6 py-4 border-b border-gray-100 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.testEmail.title') REDACTEDREDACTED</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ t('admin.settings.testEmail.description') REDACTEDREDACTED</p>
          </div>
          <div class="p-6">
            <div class="flex items-end gap-4">
              <div class="flex-1">
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  {{ t('admin.settings.testEmail.recipientEmail') REDACTEDREDACTED
                </label>
                <input
                  v-model="testEmailAddress"
                  type="email"
                  class="input"
                  placeholder="test@example.com"
                />
              </div>
              <button
                type="button"
                @click="sendTestEmail"
                :disabled="sendingTestEmail || !testEmailAddress"
                class="btn btn-secondary"
              >
                <svg v-if="sendingTestEmail" class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                {{ sendingTestEmail ? t('admin.settings.testEmail.sending') : t('admin.settings.testEmail.sendTestEmail') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>

        <!-- Save Button -->
        <div class="flex justify-end">
          <button
            type="submit"
            :disabled="saving"
            class="btn btn-primary"
          >
            <svg v-if="saving" class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ saving ? t('admin.settings.saving') : t('admin.settings.saveSettings') REDACTEDREDACTED
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted REDACTED from 'vue';
import { useI18n REDACTED from 'vue-i18n';
import { adminAPI REDACTED from '@/api';
import type { SystemSettings REDACTED from '@/api/admin/settings';
import AppLayout from '@/components/layout/AppLayout.vue';
import Toggle from '@/components/common/Toggle.vue';
import { useAppStore REDACTED from '@/stores';

const { t REDACTED = useI18n();
const appStore = useAppStore();

const loading = ref(true);
const saving = ref(false);
const testingSmtp = ref(false);
const sendingTestEmail = ref(false);
const testEmailAddress = ref('');
const logoError = ref('');

// Admin API Key 状态
const adminApiKeyLoading = ref(true);
const adminApiKeyExists = ref(false);
const adminApiKeyMasked = ref('');
const adminApiKeyOperating = ref(false);
const newAdminApiKey = ref('');

const form = reactive<SystemSettings>({
  registration_enabled: true,
  email_verify_enabled: false,
  default_balance: 0,
  default_concurrency: 1,
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: 'Subscription to API Conversion Platform',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password: '',
  smtp_from_email: '',
  smtp_from_name: '',
  smtp_use_tls: true,
  // Cloudflare Turnstile
  turnstile_enabled: false,
  turnstile_site_key: '',
  turnstile_secret_key: '',
REDACTED);

function handleLogoUpload(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  logoError.value = '';

  if (!file) return;

  // Check file size (300KB = 307200 bytes)
  const maxSize = 300 * 1024;
  if (file.size > maxSize) {
    logoError.value = t('admin.settings.site.logoSizeError', { size: (file.size / 1024).toFixed(1) REDACTED);
    input.value = '';
    return;
  REDACTED

  // Check file type
  if (!file.type.startsWith('image/')) {
    logoError.value = t('admin.settings.site.logoTypeError');
    input.value = '';
    return;
  REDACTED

  // Convert to base64
  const reader = new FileReader();
  reader.onload = (e) => {
    form.site_logo = e.target?.result as string;
  REDACTED;
  reader.onerror = () => {
    logoError.value = t('admin.settings.site.logoReadError');
  REDACTED;
  reader.readAsDataURL(file);

  // Reset input
  input.value = '';
REDACTED

async function loadSettings() {
  loading.value = true;
  try {
    const settings = await adminAPI.settings.getSettings();
    Object.assign(form, settings);
  REDACTED catch (error: any) {
    appStore.showError(t('admin.settings.failedToLoad') + ': ' + (error.message || t('common.unknownError')));
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

async function saveSettings() {
  saving.value = true;
  try {
    await adminAPI.settings.updateSettings(form);
    // Refresh cached public settings so sidebar/header update immediately
    await appStore.fetchPublicSettings(true);
    appStore.showSuccess(t('admin.settings.settingsSaved'));
  REDACTED catch (error: any) {
    appStore.showError(t('admin.settings.failedToSave') + ': ' + (error.message || t('common.unknownError')));
  REDACTED finally {
    saving.value = false;
  REDACTED
REDACTED

async function testSmtpConnection() {
  testingSmtp.value = true;
  try {
    const result = await adminAPI.settings.testSmtpConnection({
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: form.smtp_password,
      smtp_use_tls: form.smtp_use_tls,
    REDACTED);
    // API returns { message: "..." REDACTED on success, errors are thrown as exceptions
    appStore.showSuccess(result.message || t('admin.settings.smtpConnectionSuccess'));
  REDACTED catch (error: any) {
    appStore.showError(t('admin.settings.failedToTestSmtp') + ': ' + (error.message || t('common.unknownError')));
  REDACTED finally {
    testingSmtp.value = false;
  REDACTED
REDACTED

async function sendTestEmail() {
  if (!testEmailAddress.value) {
    appStore.showError(t('admin.settings.testEmail.enterRecipientHint'));
    return;
  REDACTED

  sendingTestEmail.value = true;
  try {
    const result = await adminAPI.settings.sendTestEmail({
      email: testEmailAddress.value,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_password: form.smtp_password,
      smtp_from_email: form.smtp_from_email,
      smtp_from_name: form.smtp_from_name,
      smtp_use_tls: form.smtp_use_tls,
    REDACTED);
    // API returns { message: "..." REDACTED on success, errors are thrown as exceptions
    appStore.showSuccess(result.message || t('admin.settings.testEmailSent'));
  REDACTED catch (error: any) {
    appStore.showError(t('admin.settings.failedToSendTestEmail') + ': ' + (error.message || t('common.unknownError')));
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
  REDACTED catch (error: any) {
    console.error('Failed to load admin API key status:', error);
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
    adminApiKeyMasked.value = result.key.substring(0, 10) + '...' + result.key.slice(-4);
    appStore.showSuccess(t('admin.settings.adminApiKey.keyGenerated'));
  REDACTED catch (error: any) {
    appStore.showError(error.message || t('common.error'));
  REDACTED finally {
    adminApiKeyOperating.value = false;
  REDACTED
REDACTED

async function regenerateAdminApiKey() {
  if (!confirm(t('admin.settings.adminApiKey.regenerateConfirm'))) return;
  await createAdminApiKey();
REDACTED

async function deleteAdminApiKey() {
  if (!confirm(t('admin.settings.adminApiKey.deleteConfirm'))) return;
  adminApiKeyOperating.value = true;
  try {
    await adminAPI.settings.deleteAdminApiKey();
    adminApiKeyExists.value = false;
    adminApiKeyMasked.value = '';
    newAdminApiKey.value = '';
    appStore.showSuccess(t('admin.settings.adminApiKey.keyDeleted'));
  REDACTED catch (error: any) {
    appStore.showError(error.message || t('common.error'));
  REDACTED finally {
    adminApiKeyOperating.value = false;
  REDACTED
REDACTED

function copyNewKey() {
  navigator.clipboard.writeText(newAdminApiKey.value).then(() => {
    appStore.showSuccess(t('admin.settings.adminApiKey.keyCopied'));
  REDACTED).catch(() => {
    appStore.showError(t('common.copyFailed'));
  REDACTED);
REDACTED

onMounted(() => {
  loadSettings();
  loadAdminApiKey();
REDACTED);
</script>
