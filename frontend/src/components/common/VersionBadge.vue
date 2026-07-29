<template>
  <div class="relative">
    <!-- Admin: Full version badge with dropdown -->
    <template v-if="isAdmin">
      <button
        data-testid="version-badge"
        @click="toggleDropdown"
        class="flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs transition-colors"
        :class="[
          hasDangerousDeploymentState
            ? 'bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-400 dark:hover:bg-red-900/50'
            : hasDeploymentWarning || versionWarning || hasUpdate
            ? 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400 dark:hover:bg-amber-900/50'
            : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-400 dark:hover:bg-dark-700'
        ]"
        :title="
          hasDangerousDeploymentState
            ? displayedUpdateError
            : hasDeploymentWarning
              ? deploymentJob?.cleanup_warning
              : versionWarning
              ? t('version.updateCheckWarning')
            : hasUpdate
              ? t('version.updateAvailable')
              : t('version.upToDate')
        "
      >
        <span v-if="currentVersion" class="font-medium">v{{ currentVersion }}</span>
        <span
          v-else
          class="h-3 w-12 animate-pulse rounded bg-gray-200 font-medium dark:bg-dark-600"
        ></span>
        <!-- Update indicator -->
        <span v-if="hasDangerousDeploymentState" class="relative flex h-2 w-2">
          <span class="relative inline-flex h-2 w-2 rounded-full bg-red-500"></span>
        </span>
        <span v-else-if="hasDeploymentWarning || versionWarning || hasUpdate" class="relative flex h-2 w-2">
          <span
            class="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-75"
          ></span>
          <span class="relative inline-flex h-2 w-2 rounded-full bg-amber-500"></span>
        </span>
      </button>

      <!-- Dropdown -->
      <transition name="dropdown">
        <div
          v-if="dropdownOpen"
          ref="dropdownRef"
          class="absolute left-0 z-50 mt-2 overflow-hidden whitespace-normal rounded-xl border border-gray-200 bg-white shadow-lg transition-all duration-200 dark:border-dark-700 dark:bg-dark-800"
          :class="(rollbackPanelOpen && isReleaseBuild) || deploymentJob ? 'w-80' : 'w-64'"
        >
          <!-- Header with refresh button -->
          <div
            class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700"
          >
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{
              t('version.currentVersion')
            }}</span>
            <button
              data-testid="version-refresh"
              @click="refreshVersion(true)"
              class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-dark-200"
              :disabled="loading"
              :title="t('version.refresh')"
            >
              <Icon
                name="refresh"
                size="sm"
                :stroke-width="2"
                :class="{ 'animate-spin': loading }"
              />
            </button>
          </div>

          <div class="p-4">
            <!-- Loading state -->
            <div v-if="loading" class="flex items-center justify-center py-6">
              <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
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
            </div>

            <!-- Content -->
            <template v-else>
              <!-- Version display - centered and prominent -->
              <div class="mb-4 text-center">
                <div class="inline-flex items-center gap-2">
                  <span
                    v-if="currentVersion"
                    class="text-2xl font-bold text-gray-900 dark:text-white"
                    >v{{ currentVersion }}</span
                  >
                  <span v-else class="text-2xl font-bold text-gray-400 dark:text-dark-500">--</span>
                  <!-- Show check mark when up to date -->
                  <span
                    v-if="!hasUpdate && !versionWarning"
                    class="flex h-5 w-5 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-900/30"
                  >
                    <svg
                      class="h-3 w-3 text-gray-600 dark:text-gray-400"
                      fill="currentColor"
                      viewBox="0 0 20 20"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                        clip-rule="evenodd"
                      />
                    </svg>
                  </span>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  {{
                    versionWarning
                      ? t('version.updateCheckWarning')
                      : hasUpdate
                      ? t('version.latestVersion') + ': v' + latestVersion
                      : t('version.upToDate')
                  }}
                </p>
              </div>

              <!-- Deployment progress persists while Nginx switches between containers. -->
              <div
                v-if="deploymentJob?.status === 'running'"
                class="space-y-3 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800/50 dark:bg-blue-900/20"
              >
                <div class="flex items-start gap-3">
                  <svg
                    class="mt-0.5 h-5 w-5 flex-shrink-0 animate-spin text-blue-600 dark:text-blue-400"
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
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-blue-700 dark:text-blue-300">
                      {{ deploymentStageLabel }}
                    </p>
                    <p class="mt-0.5 text-xs text-blue-600/70 dark:text-blue-400/70">
                      v{{ deploymentJob.target_version }}
                    </p>
                  </div>
                  <span class="text-xs tabular-nums text-blue-600 dark:text-blue-400">
                    {{ deploymentProgress }}%
                  </span>
                </div>
                <div class="h-1.5 overflow-hidden rounded bg-blue-100 dark:bg-blue-950/50">
                  <div
                    class="h-full rounded bg-blue-500 transition-all duration-500"
                    :style="{ width: deploymentProgress + '%' }"
                  ></div>
                </div>
                <p class="text-[11px] leading-4 text-blue-600/70 dark:text-blue-400/70">
                  {{ t('version.deploymentRefresh') }}
                </p>
              </div>

              <!-- Priority 1: Update error (must check before hasUpdate) -->
              <div v-else-if="displayedUpdateError" data-testid="deployment-error" class="space-y-2">
                <div
                  class="flex items-center gap-3 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800/50 dark:bg-red-900/20"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/50"
                  >
                    <Icon
                      name="x"
                      size="sm"
                      :stroke-width="2"
                      class="text-red-600 dark:text-red-400"
                    />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-red-700 dark:text-red-300">
                      {{ t('version.updateFailed') }}
                    </p>
                    <p class="break-words text-xs text-red-600/70 dark:text-red-400/70">
                      {{ displayedUpdateError }}
                    </p>
                  </div>
                </div>

                <!-- Retry button -->
                <button
                  v-if="!hasDangerousDeploymentState"
                  @click="retryFailedOperation"
                  :disabled="updating"
                  class="flex w-full items-center justify-center gap-2 rounded-lg bg-red-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-600 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {{ t('version.retry') }}
                </button>
              </div>

              <div
                v-else-if="versionWarning"
                data-testid="version-warning"
                class="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800/50 dark:bg-amber-900/20"
              >
                <div class="flex items-start gap-3">
                  <Icon
                    name="exclamationTriangle"
                    size="sm"
                    :stroke-width="2"
                    class="mt-0.5 flex-shrink-0 text-amber-600 dark:text-amber-400"
                  />
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-amber-700 dark:text-amber-300">
                      {{ t('version.updateCheckWarning') }}
                    </p>
                    <p class="break-words text-xs text-amber-600/80 dark:text-amber-400/80">
                      {{ versionWarning }}
                    </p>
                  </div>
                </div>
              </div>

              <div
                v-else-if="deploymentJob?.status === 'succeeded' && deploymentJob.cleanup_warning"
                data-testid="deployment-warning"
                class="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800/50 dark:bg-amber-900/20"
              >
                <div class="flex items-start gap-3">
                  <Icon
                    name="exclamationTriangle"
                    size="sm"
                    :stroke-width="2"
                    class="mt-0.5 flex-shrink-0 text-amber-600 dark:text-amber-400"
                  />
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-amber-700 dark:text-amber-300">
                      {{ t('version.updateCompletedWithWarning') }}
                    </p>
                    <p class="break-words text-xs text-amber-600/80 dark:text-amber-400/80">
                      {{ deploymentJob.cleanup_warning }}
                    </p>
                  </div>
                </div>
              </div>

              <div v-else-if="updateSuccess && !needRestart" class="space-y-2">
                <div
                  class="flex items-center gap-3 rounded-lg border border-green-200 bg-green-50 p-3 dark:border-green-800/50 dark:bg-green-900/20"
                >
                  <Icon
                    name="check"
                    size="sm"
                    :stroke-width="2"
                    class="flex-shrink-0 text-green-600 dark:text-green-400"
                  />
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-green-700 dark:text-green-300">
                      {{
                        successKind === 'rollback'
                          ? t('version.rollbackComplete')
                          : t('version.updateComplete')
                      }}
                    </p>
                    <p class="text-xs text-green-600/70 dark:text-green-400/70">
                      {{ t('version.deploymentRefresh') }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Priority 2: Update success - need restart -->
              <div v-else-if="updateSuccess && needRestart" class="space-y-2">
                <div
                  class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-800/50 dark:bg-gray-900/20"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-900/50"
                  >
                    <svg
                      class="h-4 w-4 text-gray-600 dark:text-gray-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {{
                        successKind === 'rollback'
                          ? t('version.rollbackComplete')
                          : t('version.updateComplete')
                      }}
                    </p>
                    <p class="text-xs text-gray-600/70 dark:text-gray-400/70">
                      {{ t('version.restartRequired') }}
                    </p>
                  </div>
                </div>

                <!-- Restart button with countdown -->
                <button
                  @click="handleRestart"
                  :disabled="restarting"
                  class="flex w-full items-center justify-center gap-2 rounded-lg bg-gray-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-600 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <svg
                    v-if="restarting"
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
                  <svg
                    v-else
                    class="h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                    />
                  </svg>
                  <template v-if="restarting">
                    <span>{{ t('version.restarting') }}</span>
                    <span v-if="restartCountdown > 0" class="tabular-nums"
                      >({{ restartCountdown }}s)</span
                    >
                  </template>
                  <span v-else>{{ t('version.restartNow') }}</span>
                </button>
              </div>

              <!-- Priority 3: Update available for source build - show git pull hint -->
              <div v-else-if="hasUpdate && !isReleaseBuild" class="space-y-2">
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="group flex items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 transition-colors hover:bg-amber-100 dark:border-amber-800/50 dark:bg-amber-900/20 dark:hover:bg-amber-900/30"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-amber-100 dark:bg-amber-900/50"
                  >
                    <Icon
                      name="download"
                      size="sm"
                      :stroke-width="2"
                      class="text-amber-600 dark:text-amber-400"
                    />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-amber-700 dark:text-amber-300">
                      {{ t('version.updateAvailable') }}
                    </p>
                    <p class="text-xs text-amber-600/70 dark:text-amber-400/70">
                      v{{ latestVersion }}
                    </p>
                  </div>
                  <svg
                    class="h-4 w-4 text-amber-500 transition-transform group-hover:translate-x-0.5 dark:text-amber-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                </a>
                <!-- Source build hint -->
                <div
                  class="flex items-center gap-2 rounded-lg border border-blue-200 bg-blue-50 p-2 dark:border-blue-800/50 dark:bg-blue-900/20"
                >
                  <svg
                    class="h-3.5 w-3.5 flex-shrink-0 text-blue-500 dark:text-blue-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <p class="text-xs text-blue-600 dark:text-blue-400">
                    {{ t('version.sourceModeHint') }}
                  </p>
                </div>
              </div>

              <!-- Priority 4: Update available for release build - show update button -->
              <div v-else-if="hasUpdate && isReleaseBuild" class="space-y-2">
                <!-- Update info card -->
                <div
                  class="flex items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800/50 dark:bg-amber-900/20"
                >
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-amber-100 dark:bg-amber-900/50"
                >
                  <Icon
                    name="download"
                    size="sm"
                    :stroke-width="2"
                    class="text-amber-600 dark:text-amber-400"
                  />
                </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-amber-700 dark:text-amber-300">
                      {{ t('version.updateAvailable') }}
                    </p>
                    <p class="text-xs text-amber-600/70 dark:text-amber-400/70">
                      v{{ latestVersion }}
                    </p>
                  </div>
                </div>

                <!-- Update button -->
                <button
                  @click="handleUpdate"
                  :disabled="updating || !canOnlineUpdate"
                  class="flex w-full items-center justify-center gap-2 rounded-lg bg-primary-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <svg v-if="updating" class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
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
                  <Icon v-else name="download" size="sm" :stroke-width="2" />
                  {{ updating ? t('version.updating') : t('version.updateNow') }}
                </button>

                <p
                  v-if="!canOnlineUpdate"
                  class="rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs leading-4 text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-300"
                >
                  {{ deploymentMessage || t('version.deploymentUnavailable') }}
                </p>

                <!-- View release link -->
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="flex items-center justify-center gap-1 text-xs text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200"
                >
                  {{ t('version.viewChangelog') }}
                  <Icon name="externalLink" size="xs" :stroke-width="2" />
                </a>
              </div>

              <!-- Priority 5: Up to date - GitHub link + version rollback -->
              <div v-else class="space-y-2">
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="flex items-center justify-center gap-2 py-2 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200"
                >
                  <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                    <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z"
                    />
                  </svg>
                  {{ t('version.viewRelease') }}
                </a>

                <!-- Version rollback entry -->
                <div class="border-t border-gray-100 pt-2 dark:border-dark-700">
                  <button
                    @click="toggleRollbackPanel"
                    class="group flex w-full items-center justify-between rounded-lg px-2 py-1.5 text-xs text-gray-400 transition-colors hover:bg-gray-50 hover:text-gray-600 dark:text-dark-500 dark:hover:bg-dark-700/50 dark:hover:text-dark-300"
                  >
                    <span class="flex items-center gap-1.5">
                      <Icon name="clock" size="xs" :stroke-width="2" />
                      {{ t('version.rollback') }}
                    </span>
                    <Icon
                      name="chevronDown"
                      size="xs"
                      :stroke-width="2"
                      class="transition-transform duration-200"
                      :class="{ 'rotate-180': rollbackPanelOpen }"
                    />
                  </button>

                  <transition name="rollback">
                    <div v-if="rollbackPanelOpen" class="mt-2 space-y-2">
                      <!-- Source build: online rollback unavailable, use git instead -->
                      <div
                        v-if="!isReleaseBuild"
                        class="flex items-center gap-2 rounded-lg border border-blue-200 bg-blue-50 p-2 dark:border-blue-800/50 dark:bg-blue-900/20"
                      >
                        <svg
                          class="h-3.5 w-3.5 flex-shrink-0 text-blue-500 dark:text-blue-400"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                          />
                        </svg>
                        <p class="min-w-0 flex-1 text-xs leading-4 text-blue-600 dark:text-blue-400">
                          {{ t('version.rollbackSourceHint') }}
                        </p>
                      </div>

                      <!-- Loading versions -->
                      <div
                        v-else-if="rollbackVersionsLoading"
                        class="flex items-center justify-center py-4"
                      >
                        <svg
                          class="h-5 w-5 animate-spin text-primary-500"
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
                      </div>

                      <!-- Load error + retry -->
                      <div v-else-if="rollbackVersionsError" class="space-y-2">
                        <p
                          class="rounded-lg border border-red-200 bg-red-50 p-2.5 text-xs text-red-600 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-400"
                        >
                          {{ rollbackVersionsError }}
                        </p>
                        <button
                          @click="loadRollbackVersions"
                          class="w-full rounded-lg border border-gray-200 py-1.5 text-xs text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-700 dark:border-dark-700 dark:text-dark-400 dark:hover:bg-dark-700/50 dark:hover:text-dark-200"
                        >
                          {{ t('version.retry') }}
                        </button>
                      </div>

                      <!-- No versions available -->
                      <p
                        v-else-if="rollbackVersions.length === 0"
                        class="py-3 text-center text-xs text-gray-400 dark:text-dark-500"
                      >
                        {{ t('version.noRollbackVersions') }}
                      </p>

                      <!-- Version list -->
                      <template v-else>
                        <p class="px-0.5 text-[11px] text-gray-400 dark:text-dark-500">
                          {{ t('version.rollbackSelectVersion') }}
                        </p>

                        <button
                          v-for="item in rollbackVersions"
                          :key="item.version"
                          @click="selectRollbackVersion(item.version)"
                          :disabled="rollingBack"
                          class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left transition-all disabled:cursor-not-allowed disabled:opacity-60"
                          :class="
                            selectedRollbackVersion === item.version
                              ? 'border-amber-300 bg-amber-50 shadow-sm dark:border-amber-700 dark:bg-amber-900/20'
                              : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-700/40'
                          "
                        >
                          <span class="flex items-center gap-2">
                            <span
                              class="flex h-3.5 w-3.5 items-center justify-center rounded-full border transition-colors"
                              :class="
                                selectedRollbackVersion === item.version
                                  ? 'border-amber-500'
                                  : 'border-gray-300 dark:border-dark-500'
                              "
                            >
                              <span
                                v-if="selectedRollbackVersion === item.version"
                                class="h-1.5 w-1.5 rounded-full bg-amber-500"
                              ></span>
                            </span>
                            <span
                              class="text-sm font-semibold"
                              :class="
                                selectedRollbackVersion === item.version
                                  ? 'text-amber-700 dark:text-amber-300'
                                  : 'text-gray-700 dark:text-dark-200'
                              "
                              >v{{ item.version }}</span
                            >
                          </span>
                          <span class="text-[11px] tabular-nums text-gray-400 dark:text-dark-500">
                            {{ formatPublishedAt(item.published_at) }}
                          </span>
                        </button>

                        <!-- Selected version: manual command (per deploy method) + confirm -->
                        <transition name="rollback">
                          <div v-if="selectedRollbackVersion" class="space-y-2">
                            <template v-if="!isManagedDocker">
                              <p class="px-0.5 text-[11px] text-gray-400 dark:text-dark-500">
                                {{ t('version.manualRollbackCommand') }}
                              </p>

                              <!-- Terminal-style block with deploy-method tabs -->
                              <div
                                class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"
                              >
                                <div
                                  class="flex items-center justify-between border-b border-gray-200 bg-gray-100 px-2 py-1.5 dark:border-dark-600 dark:bg-dark-700"
                                >
                                  <div
                                    class="flex items-center gap-0.5 rounded-md bg-gray-200/70 p-0.5 dark:bg-dark-600/70"
                                  >
                                    <button
                                      v-for="tab in manualTabs"
                                      :key="tab.key"
                                      @click="manualTab = tab.key"
                                      class="rounded px-2 py-0.5 text-[11px] font-medium transition-colors"
                                      :class="
                                        manualTab === tab.key
                                          ? 'bg-white text-gray-700 shadow-sm dark:bg-dark-800 dark:text-dark-100'
                                          : 'text-gray-400 hover:text-gray-600 dark:text-dark-400 dark:hover:text-dark-200'
                                      "
                                    >
                                      {{ tab.label }}
                                    </button>
                                  </div>
                                  <button
                                    @click="copyToClipboard(activeManualCommand)"
                                    class="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-gray-400 transition-colors hover:bg-gray-200 hover:text-gray-600 dark:text-dark-400 dark:hover:bg-dark-600 dark:hover:text-dark-200"
                                  >
                                    <Icon
                                      :name="copied ? 'check' : 'copy'"
                                      size="xs"
                                      :stroke-width="2"
                                      :class="copied ? 'text-gray-500' : ''"
                                    />
                                    {{ copied ? t('version.copied') : t('version.copyCommand') }}
                                  </button>
                                </div>
                                <code
                                  class="block select-all whitespace-pre-wrap break-all bg-gray-50 p-2.5 font-mono text-[10px] leading-relaxed text-gray-600 dark:bg-dark-900 dark:text-dark-300"
                                  >{{ activeManualCommand }}</code
                                >
                              </div>
                            </template>

                            <p
                              v-if="canOnlineUpdate"
                              class="flex items-start gap-1.5 px-0.5 text-[11px] leading-4 text-amber-600 dark:text-amber-400"
                            >
                              <Icon
                                name="exclamationTriangle"
                                size="xs"
                                :stroke-width="2"
                                class="mt-px flex-shrink-0"
                              />
                              {{ t('version.rollbackWarning') }}
                            </p>

                            <p
                              v-else
                              class="rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs leading-4 text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-300"
                            >
                              {{ deploymentMessage || t('version.deploymentUnavailable') }}
                            </p>

                            <p
                              v-if="rollbackError"
                              class="rounded-lg border border-red-200 bg-red-50 p-2 text-xs text-red-600 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-400"
                            >
                              {{ rollbackError }}
                            </p>

                            <button
                              v-if="canOnlineUpdate"
                              @click="handleRollback"
                              :disabled="rollingBack"
                              class="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-500 px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-amber-600 disabled:cursor-not-allowed disabled:opacity-50"
                            >
                              <svg
                                v-if="rollingBack"
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
                              <Icon v-else name="clock" size="sm" :stroke-width="2" />
                              <span>{{
                                rollingBack
                                  ? t('version.rollingBack')
                                  : t('version.rollbackConfirm', {
                                      version: 'v' + selectedRollbackVersion
                                    })
                              }}</span>
                            </button>
                          </div>
                        </transition>
                      </template>
                    </div>
                  </transition>
                </div>
              </div>
            </template>
          </div>
        </div>
      </transition>
    </template>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import {
  performUpdate,
  restartService,
  getVersion,
  getRollbackVersions,
  getDeploymentJob,
  getCurrentDeploymentJob,
  replayOrRecoverCurrentDeployment,
  reconcileDeploymentVersion,
  rollback as rollbackAPI,
  type RollbackVersionInfo,
  type DeploymentJob
} from '@/api/admin/system'
import { useClipboard } from '@/composables/useClipboard'
import Icon from '@/components/icons/Icon.vue'

const GITHUB_REPO = 'ssharkkky/sub2api'
// Fork releases are published to GHCR without the leading "v" in image tags.
const DOCKER_IMAGE = 'ghcr.io/ssharkkky/sub2api'
const CONTROL_PLANE_UPGRADE_TIMEOUT_MS = 5 * 60 * 1000

const { t } = useI18n()

const props = defineProps<{
  version?: string
}>()

const authStore = useAuthStore()
const appStore = useAppStore()

const isAdmin = computed(() => authStore.isAdmin)

const dropdownOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

// Use store's cached version state
const loading = computed(() => appStore.versionLoading)
const currentVersion = computed(() => appStore.currentVersion || props.version || '')
const latestVersion = computed(() => appStore.latestVersion)
const hasUpdate = computed(() => appStore.hasUpdate)
const versionWarning = computed(() => appStore.versionWarning)
const releaseInfo = computed(() => appStore.releaseInfo)
const buildType = computed(() => appStore.buildType)
const deploymentMode = computed(() => appStore.deploymentMode)
const deploymentReady = computed(() => appStore.deploymentReady)
const deploymentMessage = computed(() => appStore.deploymentMessage)
const isManagedDocker = computed(() => deploymentMode.value === 'docker-managed')
const canOnlineUpdate = computed(
  () => deploymentMode.value === 'standalone-binary' || (isManagedDocker.value && deploymentReady.value)
)

// Update process states (local to this component)
const updating = ref(false)
const restarting = ref(false)
const needRestart = ref(false)
const updateError = ref('')
const updateSuccess = ref(false)
const restartCountdown = ref(0)
// Distinguishes the success + restart panel between update and rollback flows
const successKind = ref<'update' | 'rollback'>('update')
const deploymentJob = ref<DeploymentJob | null>(null)
const hasDangerousDeploymentState = computed(() => isDangerousDeploymentJob(deploymentJob.value))
const hasDeploymentWarning = computed(
  () => deploymentJob.value?.status === 'succeeded' && Boolean(deploymentJob.value.cleanup_warning)
)
const displayedUpdateError = computed(() => {
  if (hasDangerousDeploymentState.value && deploymentJob.value) {
    return deploymentFailureMessage(deploymentJob.value)
  }
  return updateError.value
})
let deploymentPollToken = 0
let reloadTimer: ReturnType<typeof setTimeout> | null = null

const deploymentStages = [
  'pulling',
  'preparing',
  'starting_candidate',
  'checking_candidate',
  'switching_traffic',
  'stabilizing',
  'draining',
  'activating_background',
  'completed'
]
const deploymentProgress = computed(() => {
  if (!deploymentJob.value) return 0
  const index = deploymentStages.indexOf(deploymentJob.value.stage)
  if (index < 0) return 5
  return Math.round(((index + 1) / deploymentStages.length) * 100)
})
const deploymentStageLabel = computed(() => {
  const stage = deploymentJob.value?.stage
  return stage ? t(`version.deploymentStages.${stage}`) : t('version.deploymentStarting')
})

// Rollback states
const rollbackPanelOpen = ref(false)
const rollbackVersions = ref<RollbackVersionInfo[]>([])
const rollbackVersionsLoading = ref(false)
const rollbackVersionsError = ref('')
const selectedRollbackVersion = ref('')
const rollingBack = ref(false)
const rollbackError = ref('')

const { copied, copyToClipboard } = useClipboard()

// Manual rollback methods differ by deployment: script installs use install.sh,
// docker deployments pin the image tag instead
const manualTab = ref<'script' | 'docker'>('script')

const manualTabs = computed(() => [
  { key: 'script' as const, label: t('version.deployScript') },
  { key: 'docker' as const, label: t('version.deployDocker') }
])

const scriptRollbackCommand = computed(() => {
  if (!selectedRollbackVersion.value) return ''
  const tag = `v${selectedRollbackVersion.value}`
  return `curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/${tag}/deploy/install.sh | sudo bash -s -- rollback ${tag}`
})

const dockerRollbackCommand = computed(() => {
  if (!selectedRollbackVersion.value) return ''
  return [
    `# ${t('version.dockerEditCompose')}`,
    `image: ${DOCKER_IMAGE}:${selectedRollbackVersion.value}`,
    '',
    `# ${t('version.dockerRecreate')}`,
    'docker compose up -d'
  ].join('\n')
})

const activeManualCommand = computed(() =>
  manualTab.value === 'docker' ? dockerRollbackCommand.value : scriptRollbackCommand.value
)

// Only show update check for release builds (binary/docker deployment)
const isReleaseBuild = computed(() => buildType.value === 'release')

function toggleDropdown() {
  dropdownOpen.value = !dropdownOpen.value
}

function closeDropdown() {
  dropdownOpen.value = false
}

async function refreshVersion(force = true) {
  if (!isAdmin.value) return

  // Reset update states when refreshing
  updateError.value = ''
  updateSuccess.value = false
  needRestart.value = false
  resetRollbackState()

  await appStore.fetchVersion(force)
  await recoverCurrentDeployment()
}

async function handleUpdate() {
  if (updating.value || !canOnlineUpdate.value) return

  updating.value = true
  updateError.value = ''
  updateSuccess.value = false
  const attemptStartedAt = Date.now()

  try {
    applyUpdateResult(await performUpdate())
  } catch (error: unknown) {
    if (isAmbiguousSystemRequestError(error)) {
      const recovered = await replayOrRecoverCurrentDeployment(
        performUpdate,
        'update',
        latestVersion.value,
        attemptStartedAt
      )
      if (recovered?.result) {
        applyUpdateResult(recovered.result)
        return
      }
      if (recovered?.job) {
        trackDeployment(recovered.job, 'update')
        return
      }
    }
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    updateError.value = err.response?.data?.message || err.message || t('version.updateFailed')
  } finally {
    if (!deploymentJob.value || deploymentJob.value.status !== 'running') {
      updating.value = false
    }
  }
}

function applyUpdateResult(result: Awaited<ReturnType<typeof performUpdate>>) {
  if (result.job) {
    trackDeployment(result.job, 'update')
    return
  }
  successKind.value = 'update'
  updateSuccess.value = true
  needRestart.value = result.need_restart
  appStore.clearVersionCache()
}

function retryFailedOperation() {
  if (successKind.value === 'rollback') {
    if (!selectedRollbackVersion.value) {
      selectedRollbackVersion.value = deploymentJob.value?.target_version || ''
    }
    void handleRollback()
    return
  }
  void handleUpdate()
}

function resetRollbackState() {
  rollbackPanelOpen.value = false
  rollbackVersions.value = []
  rollbackVersionsError.value = ''
  selectedRollbackVersion.value = ''
  rollbackError.value = ''
  manualTab.value = 'script'
}

async function toggleRollbackPanel() {
  if (!isAdmin.value) return
  rollbackPanelOpen.value = !rollbackPanelOpen.value
  // Source builds only show a hint, no version list to fetch
  if (
    rollbackPanelOpen.value &&
    isReleaseBuild.value &&
    rollbackVersions.value.length === 0 &&
    !rollbackVersionsLoading.value
  ) {
    await loadRollbackVersions()
  }
}

async function loadRollbackVersions() {
  if (!isAdmin.value) return
  rollbackVersionsLoading.value = true
  rollbackVersionsError.value = ''
  try {
    const data = await getRollbackVersions()
    rollbackVersions.value = data.versions || []
  } catch (error: unknown) {
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    rollbackVersionsError.value =
      err.response?.data?.message || err.message || t('version.loadVersionsFailed')
  } finally {
    rollbackVersionsLoading.value = false
  }
}

function selectRollbackVersion(version: string) {
  if (rollingBack.value) return
  rollbackError.value = ''
  selectedRollbackVersion.value = selectedRollbackVersion.value === version ? '' : version
}

function formatPublishedAt(publishedAt: string): string {
  if (!publishedAt) return ''
  const date = new Date(publishedAt)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString()
}

async function handleRollback() {
  if (!isAdmin.value) return
  if (rollingBack.value || !selectedRollbackVersion.value || !canOnlineUpdate.value) return

  rollingBack.value = true
  rollbackError.value = ''
  const attemptStartedAt = Date.now()

  try {
    applyRollbackResult(await rollbackAPI(selectedRollbackVersion.value))
  } catch (error: unknown) {
    if (isAmbiguousSystemRequestError(error)) {
      const recovered = await replayOrRecoverCurrentDeployment(
        () => rollbackAPI(selectedRollbackVersion.value),
        'rollback',
        selectedRollbackVersion.value,
        attemptStartedAt
      )
      if (recovered?.result) {
        applyRollbackResult(recovered.result)
        return
      }
      if (recovered?.job) {
        trackDeployment(recovered.job, 'rollback')
        return
      }
    }
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    rollbackError.value = err.response?.data?.message || err.message || t('version.rollbackFailed')
  } finally {
    if (!deploymentJob.value || deploymentJob.value.status !== 'running') {
      rollingBack.value = false
    }
  }
}

function applyRollbackResult(result: Awaited<ReturnType<typeof rollbackAPI>>) {
  if (result.job) {
    trackDeployment(result.job, 'rollback')
    return
  }
  successKind.value = 'rollback'
  updateSuccess.value = true
  needRestart.value = result.need_restart
  rollbackPanelOpen.value = false
  appStore.clearVersionCache()
}

function isAmbiguousSystemRequestError(error: unknown): boolean {
  const status = (error as { response?: { status?: number } }).response?.status
  return status === undefined || status === 408 || status >= 500
}

function isDangerousDeploymentJob(job: DeploymentJob | null): boolean {
  return (
    job?.status === 'degraded' ||
    job?.status === 'rollback_failed' ||
    job?.control_plane_upgrade_status === 'failed' ||
    job?.control_plane_upgrade_status === 'unknown' ||
    Boolean(job?.rollback_error)
  )
}

function deploymentFailureMessage(job: DeploymentJob): string {
  const fallback =
    job.status === 'degraded'
      ? t('version.deploymentDegraded')
      : job.status === 'rollback_failed'
        ? t('version.deploymentRollbackFailed')
        : job.rollback_performed
          ? t('version.deploymentRolledBack')
          : t('version.updateFailed')
  const controlPlaneFallback =
    job.control_plane_upgrade_status === 'failed'
      ? t('version.controlPlaneUpgradeFailed')
      : job.control_plane_upgrade_status === 'unknown'
        ? t('version.controlPlaneUpgradeUnknown')
        : ''
  const details = [controlPlaneFallback, job.error, job.rollback_error, job.control_plane_upgrade_error].filter(
    (value, index, values): value is string => Boolean(value) && values.indexOf(value) === index
  )
  return details.length > 0 ? `${fallback}: ${details.join('; ')}` : fallback
}

function trackDeployment(job: DeploymentJob, kind: 'update' | 'rollback') {
  deploymentJob.value = job
  successKind.value = kind
  updating.value = kind === 'update'
  rollingBack.value = kind === 'rollback'
  rollbackPanelOpen.value = false
  const token = ++deploymentPollToken
  void pollDeployment(job.id, token)
}

async function pollDeployment(jobID: string, token: number) {
  let transientFailures = 0
  let controlPlanePendingSince: number | null = null
  while (token === deploymentPollToken) {
    try {
      const job = await getDeploymentJob(jobID)
      if (token !== deploymentPollToken) return
      deploymentJob.value = job
      transientFailures = 0
      const controlPlanePending =
        job.status === 'succeeded' &&
        (job.control_plane_upgrade_status === 'pending' ||
          job.control_plane_upgrade_status === 'retrying')
      if (controlPlanePending) {
        controlPlanePendingSince ??= Date.now()
        if (Date.now() - controlPlanePendingSince >= CONTROL_PLANE_UPGRADE_TIMEOUT_MS) {
          updating.value = false
          rollingBack.value = false
          updateError.value = t('version.controlPlaneUpgradeTimeout')
          return
        }
      } else {
        controlPlanePendingSince = null
      }
      if (
        job.status === 'running' ||
        controlPlanePending
      ) {
        await new Promise((resolve) => setTimeout(resolve, 1500))
        continue
      }
      updating.value = false
      rollingBack.value = false
      if (job.status === 'succeeded') {
        updateSuccess.value = true
        needRestart.value = false
        updateError.value = ''
        appStore.clearVersionCache()
        reloadTimer = setTimeout(() => window.location.reload(), 2500)
      } else {
        updateError.value = deploymentFailureMessage(job)
      }
      return
    } catch (error) {
      transientFailures++
      const status = (error as { response?: { status?: number } }).response?.status
      if (status === 404 && transientFailures >= 3 && transientFailures % 3 === 0) {
        const reconciled = await reconcileLegacyDeployment(deploymentJob.value)
        if (reconciled) return
      }
      // A few failed polls are expected while Nginx replaces its upstream.
      if (transientFailures >= 40) {
        updating.value = false
        rollingBack.value = false
        updateError.value = t('version.deploymentUnavailable')
        if (deploymentJob.value?.status === 'running') {
          deploymentJob.value = {
            ...deploymentJob.value,
            status: 'failed',
            stage: 'failed',
            error: updateError.value
          }
        }
        return
      }
      await new Promise((resolve) => setTimeout(resolve, 1500))
    }
  }
}

async function reconcileLegacyDeployment(job: DeploymentJob | null): Promise<boolean> {
  if (!job) return false
  try {
    const current = (await getVersion()).version
    const reconciliation = reconcileDeploymentVersion(job, current)
    if (reconciliation === 'succeeded') {
      deploymentJob.value = { ...job, status: 'succeeded', stage: 'completed' }
      updating.value = false
      rollingBack.value = false
      updateSuccess.value = true
      needRestart.value = false
      updateError.value = ''
      appStore.clearVersionCache()
      reloadTimer = setTimeout(() => window.location.reload(), 2500)
      return true
    }
    if (reconciliation === 'rolled_back') {
      deploymentJob.value = {
        ...job,
        status: 'failed',
        stage: 'failed',
        rollback_performed: true
      }
      updating.value = false
      rollingBack.value = false
      updateError.value = t('version.deploymentRolledBack')
      return true
    }
  } catch {
    // Keep polling while Nginx or the restored legacy container is starting.
  }
  return false
}

async function handleRestart() {
  if (restarting.value) return

  restarting.value = true
  restartCountdown.value = 8

  try {
    await restartService()
    // Service will restart, page will reload automatically or show disconnected
  } catch (error) {
    // Expected - connection will be lost during restart
    console.log('Service restarting...')
  }

  // Start countdown
  const countdownInterval = setInterval(() => {
    restartCountdown.value--
    if (restartCountdown.value <= 0) {
      clearInterval(countdownInterval)
      // Try to check if service is back before reload
      checkServiceAndReload()
    }
  }, 1000)
}

async function checkServiceAndReload() {
  const maxRetries = 5
  const retryDelay = 1000

  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch('/health', {
        method: 'GET',
        cache: 'no-cache'
      })
      if (response.ok) {
        // Service is back, reload page
        window.location.reload()
        return
      }
    } catch {
      // Service not ready yet
    }

    if (i < maxRetries - 1) {
      await new Promise((resolve) => setTimeout(resolve, retryDelay))
    }
  }

  // After retries, reload anyway
  window.location.reload()
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node
  const button = (event.target as Element).closest('button')
  if (dropdownRef.value && !dropdownRef.value.contains(target) && !button?.contains(target)) {
    closeDropdown()
  }
}

async function recoverCurrentDeployment() {
  try {
    // This request is intentionally independent of the version response. During
    // an Nginx slot switch, /check-updates can fail while the durable host job
    // remains available from the container that receives the next request.
    const job = await getCurrentDeploymentJob()
    if (
      job.status === 'running' ||
      (job.status === 'succeeded' &&
        (job.control_plane_upgrade_status === 'pending' ||
          job.control_plane_upgrade_status === 'retrying'))
    ) {
      trackDeployment(job, job.action)
    } else if (isDangerousDeploymentJob(job)) {
      deploymentJob.value = job
      successKind.value = job.action
      updating.value = false
      rollingBack.value = false
      updateSuccess.value = false
      needRestart.value = false
    } else if (job.status === 'succeeded' && job.cleanup_warning) {
      deploymentJob.value = job
      successKind.value = job.action
      updating.value = false
      rollingBack.value = false
      updateSuccess.value = false
      needRestart.value = false
    } else if (job.status === 'succeeded') {
      // The deployer retains its last terminal job indefinitely. A success is
      // only current UI state while this page is polling the operation it
      // started; after a reload, version availability is authoritative.
      deploymentJob.value = null
      updating.value = false
      rollingBack.value = false
      updateSuccess.value = false
      needRestart.value = false
      updateError.value = ''
    } else {
      deploymentJob.value = job
      successKind.value = job.action
      updating.value = false
      rollingBack.value = false
      if (job.action === 'rollback') {
        selectedRollbackVersion.value = job.target_version
      }
      updateSuccess.value = false
      needRestart.value = false
      updateError.value = deploymentFailureMessage(job)
    }
  } catch (error) {
    const status = (error as { response?: { status?: number } }).response?.status
    if (status === 404 && deploymentJob.value?.status === 'running') {
      updateError.value = t('version.deploymentUnavailable')
      deploymentJob.value = {
        ...deploymentJob.value,
        status: 'failed',
        stage: 'failed',
        error: updateError.value
      }
      updating.value = false
      rollingBack.value = false
    }
    // Source/manual deployments and a first request during a switch normally
    // have no recoverable managed job. A later manual refresh retries this.
  }
}

onMounted(async () => {
  if (isAdmin.value) {
    // Use cached version if available, otherwise fetch
    await appStore.fetchVersion(false)
    await recoverCurrentDeployment()
  }
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  deploymentPollToken++
  if (reloadTimer) clearTimeout(reloadTimer)
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}

.rollback-enter-active,
.rollback-leave-active {
  transition: all 0.2s ease;
}

.rollback-enter-from,
.rollback-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.line-clamp-3 {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
