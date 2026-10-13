<template>
  <BaseDialog
    :show="show"
    :title="t('keys.useKeyModal.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- No Group Assigned Warning -->
      <div v-if="!platform" class="flex items-start gap-3 p-4 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800">
        <svg class="w-5 h-5 text-yellow-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
        </svg>
        <div>
          <p class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            {{ t('keys.useKeyModal.noGroupTitle') }}
          </p>
          <p class="text-sm text-yellow-700 dark:text-yellow-300 mt-1">
            {{ t('keys.useKeyModal.noGroupDescription') }}
          </p>
        </div>
      </div>

      <!-- Platform-specific content -->
      <template v-else>
        <!-- Description -->
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ platformDescription }}
        </p>

        <!-- Provider-backed capability guidance -->
        <section
          v-if="showCapabilityGuidance"
          data-testid="capability-guidance"
          class="rounded-xl border border-primary-100 bg-primary-50/60 p-4 dark:border-primary-900/60 dark:bg-primary-950/20"
          aria-live="polite"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('keys.useKeyModal.capabilities.title') }}
              </h3>
              <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                {{ t('keys.useKeyModal.capabilities.description') }}
              </p>
            </div>
            <Icon name="sparkles" size="sm" class="mt-0.5 text-primary-500" />
          </div>

          <div
            v-if="showApiTab"
            data-testid="capability-test-guide"
            class="mt-3 flex flex-col gap-3 rounded-lg border border-primary-200 bg-white/80 p-3 dark:border-primary-800 dark:bg-dark-900/60 sm:flex-row sm:items-start sm:justify-between"
          >
            <div class="flex min-w-0 items-start gap-2">
              <Icon name="infoCircle" size="sm" class="mt-0.5 flex-shrink-0 text-primary-500" />
              <div>
                <p class="text-xs font-semibold text-gray-900 dark:text-white">
                  {{ t('keys.useKeyModal.capabilities.selfTestTitle') }}
                </p>
                <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                  {{ t('keys.useKeyModal.capabilities.selfTestDescription') }}
                </p>
                <ol class="mt-2 space-y-1 text-xs text-gray-600 dark:text-gray-400">
                  <li class="flex gap-2"><span class="font-semibold text-primary-600 dark:text-primary-400">1.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepSelect') }}</span></li>
                  <li class="flex gap-2"><span class="font-semibold text-primary-600 dark:text-primary-400">2.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepOpen') }}</span></li>
                  <li class="flex gap-2"><span class="font-semibold text-primary-600 dark:text-primary-400">3.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepSend') }}</span></li>
                </ol>
              </div>
            </div>
            <button
              v-if="activeClientTab !== 'api'"
              type="button"
              data-testid="open-api-test-guide"
              class="inline-flex min-h-9 flex-shrink-0 items-center justify-center rounded-md bg-primary-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-900"
              @click="focusApiTab"
            >
              {{ t('keys.useKeyModal.capabilities.openApiGuide') }}
            </button>
          </div>
          <div
            v-else-if="capabilityState === 'ready' && capabilities"
            data-testid="capability-no-api-route"
            class="mt-3 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50/80 p-3 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-200"
          >
            <Icon name="infoCircle" size="sm" class="mt-0.5 flex-shrink-0" />
            <div>
              <p class="font-semibold">{{ t('keys.useKeyModal.capabilities.noAvailableTitle') }}</p>
              <p class="mt-1">{{ t('keys.useKeyModal.capabilities.noAvailableDescription') }}</p>
            </div>
          </div>

          <p v-if="capabilityState === 'loading'" class="mt-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('keys.useKeyModal.capabilities.loading') }}
          </p>
          <p v-else-if="capabilityState === 'error'" data-testid="capability-guidance-error" class="mt-3 text-xs text-amber-700 dark:text-amber-300">
            {{ t('keys.useKeyModal.capabilities.error') }}
          </p>
          <div v-else-if="capabilities" class="mt-3 grid gap-2 sm:grid-cols-2">
            <div
              v-for="item in capabilityGuidanceItems"
              :key="item.id"
              :data-testid="`capability-${item.id}`"
              :class="selectedCapabilityExampleId === item.id ? 'ring-2 ring-primary-300 dark:ring-primary-700' : ''"
              class="rounded-lg border border-white/80 bg-white/80 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/60"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ item.label }}</span>
                <div class="flex items-center gap-2">
                  <span
                    :class="item.available ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400 dark:text-gray-500'"
                    class="text-[11px] font-semibold"
                  >
                    {{ item.available ? t('keys.useKeyModal.capabilities.available') : t('keys.useKeyModal.capabilities.unavailable') }}
                  </span>
                  <button
                    v-if="item.available && hasCapabilityExample(item.id)"
                    type="button"
                    :data-testid="`capability-example-${item.id}`"
                    class="inline-flex items-center gap-1 rounded-md border border-primary-200 bg-primary-50 px-2 py-1 text-[11px] font-medium text-primary-700 transition-colors hover:border-primary-300 hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-950/40 dark:text-primary-300 dark:hover:bg-primary-950/70"
                    @click="openCapabilityExample(item)"
                  >
                    <Icon name="document" size="sm" />
                    <span class="hidden sm:inline">{{ t('keys.useKeyModal.capabilities.viewExample') }}</span>
                    <span class="sm:hidden">{{ t('keys.useKeyModal.capabilities.exampleShort') }}</span>
                  </button>
                </div>
              </div>
              <code v-if="item.path" class="mt-1 block truncate text-[11px] text-gray-500 dark:text-gray-400">{{ item.path }}</code>
              <span v-else-if="item.reason" class="mt-1 block text-[11px] text-gray-500 dark:text-gray-400">{{ capabilityReasonLabel(item.reason) }}</span>
            </div>
          </div>

          <section
            v-if="selectedCapabilityExample && activeClientTab !== 'api'"
            data-testid="capability-example-panel"
            class="mt-3 space-y-3 rounded-lg border border-primary-200 bg-white p-3 dark:border-primary-800 dark:bg-dark-900/70"
            aria-live="polite"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('keys.useKeyModal.capabilities.exampleTitle') }}: {{ selectedCapabilityExample.label }}
                </h4>
                <code class="mt-1 block truncate text-[11px] text-gray-500 dark:text-gray-400">
                  {{ selectedCapabilityExample.method }} {{ selectedCapabilityExample.path }}
                </code>
              </div>
              <button
                type="button"
                data-testid="capability-example-close"
                class="flex-shrink-0 rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-gray-200"
                @click="selectedCapabilityExampleId = null"
              >
                {{ t('keys.useKeyModal.capabilities.closeExample') }}
              </button>
            </div>

            <div class="grid gap-3 lg:grid-cols-2">
              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                    {{ t('keys.useKeyModal.capabilities.requestExample') }}
                  </p>
                  <button
                    type="button"
                    data-testid="copy-capability-request"
                    class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    @click="copyCapabilityExample('request')"
                  >
                    {{ copiedCapabilityExamplePart === 'request' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                  </button>
                </div>
                <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-5 text-gray-100"><code>{{ selectedCapabilityExample.request }}</code></pre>
              </div>

              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                    {{ t('keys.useKeyModal.capabilities.responseExample') }}
                  </p>
                  <button
                    type="button"
                    data-testid="copy-capability-response"
                    class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    @click="copyCapabilityExample('response')"
                  >
                    {{ copiedCapabilityExamplePart === 'response' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                  </button>
                </div>
                <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-5 text-gray-100"><code>{{ selectedCapabilityExample.response }}</code></pre>
              </div>
            </div>

            <p v-if="selectedCapabilityExample.responseNote" class="text-xs text-gray-600 dark:text-gray-400">
              {{ selectedCapabilityExample.responseNote }}
            </p>
          </section>
        </section>

        <!-- Client Tabs -->
        <div v-if="clientTabs.length" class="overflow-x-auto border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex min-w-max gap-4 sm:gap-6" aria-label="Client" role="tablist">
            <button
              v-for="tab in clientTabs"
              :key="tab.id"
              type="button"
              :data-testid="`client-tab-${tab.id}`"
              :id="`client-tab-${tab.id}`"
              role="tab"
              :aria-selected="activeClientTab === tab.id"
              :aria-controls="`${tab.id}-client-panel`"
              @click="activeClientTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeClientTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label }}
              </span>
            </button>
          </nav>
        </div>

        <!-- Capability-backed API examples -->
        <section
          v-if="showApiTab && activeClientTab === 'api'"
          id="api-client-panel"
          data-testid="api-capability-panel"
          role="tabpanel"
          aria-labelledby="client-tab-api"
          class="space-y-4 rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/40"
        >
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.title') }}</h3>
            <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.api.description') }}</p>
          </div>

          <section
            data-testid="api-test-guide"
            class="space-y-3 rounded-lg border border-primary-200 bg-primary-50/60 p-3 dark:border-primary-800 dark:bg-primary-950/20"
          >
            <div class="flex items-start gap-2">
              <Icon name="infoCircle" size="sm" class="mt-0.5 flex-shrink-0 text-primary-500" />
              <div>
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('keys.useKeyModal.api.testGuideTitle') }}
                </h4>
                <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                  {{ t('keys.useKeyModal.api.testGuideDescription') }}
                </p>
              </div>
            </div>
            <ol class="grid gap-2 text-xs text-gray-700 dark:text-gray-300 sm:grid-cols-3">
              <li class="flex gap-2 rounded-md bg-white/70 p-2 dark:bg-dark-900/50"><span class="font-semibold text-primary-600 dark:text-primary-400">1.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepSelect') }}</span></li>
              <li class="flex gap-2 rounded-md bg-white/70 p-2 dark:bg-dark-900/50"><span class="font-semibold text-primary-600 dark:text-primary-400">2.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepOpen') }}</span></li>
              <li class="flex gap-2 rounded-md bg-white/70 p-2 dark:bg-dark-900/50"><span class="font-semibold text-primary-600 dark:text-primary-400">3.</span><span>{{ t('keys.useKeyModal.capabilities.selfTestStepSend') }}</span></li>
            </ol>
            <div
              v-if="capabilities && capabilities.video.generation.available && capabilities.video.status.available && capabilities.video.content.available"
              data-testid="api-video-test-flow"
              class="rounded-md border border-primary-200 bg-white/80 p-2 dark:border-primary-800 dark:bg-dark-900/60"
            >
              <p class="text-xs font-semibold text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.videoFlowTitle') }}</p>
              <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.api.videoFlowDescription') }}</p>
            </div>
          </section>

          <section
            v-if="selectedCapabilityExample && activeClientTab === 'api'"
            data-testid="api-capability-example-panel"
            class="space-y-3 rounded-lg border border-primary-200 bg-white p-3 dark:border-primary-800 dark:bg-dark-900/70"
            aria-live="polite"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('keys.useKeyModal.capabilities.exampleTitle') }}: {{ selectedCapabilityExample.label }}
                </h4>
                <code class="mt-1 block truncate text-[11px] text-gray-500 dark:text-gray-400">
                  {{ selectedCapabilityExample.method }} {{ selectedCapabilityExample.path }}
                </code>
              </div>
              <button
                type="button"
                data-testid="api-capability-example-close"
                class="flex-shrink-0 rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-gray-200"
                @click="selectedCapabilityExampleId = null"
              >
                {{ t('keys.useKeyModal.capabilities.closeExample') }}
              </button>
            </div>

            <div class="grid gap-3 lg:grid-cols-2">
              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                    {{ t('keys.useKeyModal.capabilities.requestExample') }}
                  </p>
                  <button
                    type="button"
                    data-testid="api-copy-capability-request"
                    class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    @click="copyCapabilityExample('request')"
                  >
                    {{ copiedCapabilityExamplePart === 'request' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                  </button>
                </div>
                <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-5 text-gray-100"><code>{{ selectedCapabilityExample.request }}</code></pre>
              </div>

              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                    {{ t('keys.useKeyModal.capabilities.responseExample') }}
                  </p>
                  <button
                    type="button"
                    data-testid="api-copy-capability-response"
                    class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    @click="copyCapabilityExample('response')"
                  >
                    {{ copiedCapabilityExamplePart === 'response' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                  </button>
                </div>
                <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-5 text-gray-100"><code>{{ selectedCapabilityExample.response }}</code></pre>
              </div>
            </div>

            <p v-if="selectedCapabilityExample.responseNote" class="text-xs text-gray-600 dark:text-gray-400">
              {{ selectedCapabilityExample.responseNote }}
            </p>
          </section>

          <section
            v-if="capabilities && (capabilities.image.sync.available || capabilities.image.async.generation.available || capabilities.image.async.edit.available || capabilities.image.async.poll.available)"
            data-testid="api-image-section"
            aria-labelledby="api-image-title"
            class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/50"
          >
            <h4 id="api-image-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.image.title') }}</h4>
            <div v-if="capabilities.image.sync.available" data-testid="api-image-sync">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.image.sync') }}</p>
                <button
                  v-if="hasCapabilityExample('image-sync')"
                  type="button"
                  data-testid="api-example-image-sync"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('image-sync')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">curl -X {{ capabilities.image.sync.method }} {{ apiExampleUrl(capabilities.image.sync.path) }} -H "Authorization: Bearer [REDACTED]" -H "Content-Type: application/json" -d '{"model":"{model}","prompt":"..."}'</code>
            </div>
            <div v-if="capabilities.image.async.generation.available" data-testid="api-image-async-generation">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.image.asyncSubmit') }}</p>
                <button
                  v-if="hasCapabilityExample('image-async-generation')"
                  type="button"
                  data-testid="api-example-image-async-generation"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('image-async-generation')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.image.async.generation.method }} {{ apiExampleUrl(capabilities.image.async.generation.path) }}</code>
            </div>
            <div v-if="capabilities.image.async.poll.available" data-testid="api-image-async-poll">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.image.poll') }}</p>
                <button
                  v-if="hasCapabilityExample('image-async-poll')"
                  type="button"
                  data-testid="api-example-image-async-poll"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('image-async-poll')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.image.async.poll.method }} {{ apiExampleUrl(capabilities.image.async.poll.path) }}</code>
            </div>
            <div v-if="capabilities.image.async.edit.available" data-testid="api-image-async-edit">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.image.edit') }}</p>
                <button
                  v-if="hasCapabilityExample('image-async-edit')"
                  type="button"
                  data-testid="api-example-image-async-edit"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('image-async-edit')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.image.async.edit.method }} {{ apiExampleUrl(capabilities.image.async.edit.path) }}</code>
            </div>
            <p class="text-xs text-amber-700 dark:text-amber-300">{{ t('keys.useKeyModal.api.image.streamWarning') }}</p>
          </section>

          <section
            v-if="capabilities && capabilities.gemini_native.image_generation.available"
            data-testid="api-gemini-native-section"
            aria-labelledby="api-gemini-native-title"
            class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/50"
          >
            <h4 id="api-gemini-native-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.geminiNative.title') }}</h4>
            <div v-if="capabilities.gemini_native.image_generation.generate_content.available" data-testid="api-gemini-native-generate">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.geminiNative.generate') }}</p>
                <button
                  v-if="hasCapabilityExample('gemini-native-generate-content')"
                  type="button"
                  data-testid="api-example-gemini-native-generate-content"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('gemini-native-generate-content')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.gemini_native.image_generation.generate_content.method }} {{ apiExampleUrl(capabilities.gemini_native.image_generation.generate_content.path) }}&#10;{"contents":[{"parts":[{"text":"..."}]}],"generationConfig":{"imageConfig":{"aspectRatio":"1:1"}}}</code>
            </div>
            <div v-if="capabilities.gemini_native.image_generation.stream_generate_content.available" data-testid="api-gemini-native-stream">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('keys.useKeyModal.api.geminiNative.stream') }}</p>
                <button
                  v-if="hasCapabilityExample('gemini-native-stream-generate-content')"
                  type="button"
                  data-testid="api-example-gemini-native-stream-generate-content"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('gemini-native-stream-generate-content')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="mt-1 block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.gemini_native.image_generation.stream_generate_content.method }} {{ apiExampleUrl(capabilities.gemini_native.image_generation.stream_generate_content.path) }}</code>
            </div>
          </section>

          <section
            v-if="capabilities && (capabilities.batch_image.available || capabilities.batch_image.reason)"
            data-testid="api-batch-image-section"
            aria-labelledby="api-batch-image-title"
            class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/50"
          >
            <h4 id="api-batch-image-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.batchImage.title') }}</h4>
            <template v-if="capabilities.batch_image.available">
              <div class="flex items-center justify-between gap-2">
                <p class="text-xs text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.api.batchImage.models') }}</p>
                <button
                  v-if="hasCapabilityExample('batch-image')"
                  type="button"
                  data-testid="api-example-batch-image"
                  class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('batch-image')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
              <code class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.batch_image.models.method }} {{ apiExampleUrl(capabilities.batch_image.models.path) }}</code>
            </template>
            <p v-else class="text-xs text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.api.reason.unavailable') }}</p>
          </section>

          <section
            v-if="capabilities && (capabilities.video.generation.available || capabilities.video.status.available || capabilities.video.content.available)"
            data-testid="api-video-section"
            aria-labelledby="api-video-title"
            class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/50"
          >
            <h4 id="api-video-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.video.title') }}</h4>
            <div v-if="capabilities.video.generation.available" data-testid="api-video-generation">
              <div class="flex items-center justify-between gap-2">
                <code class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.video.generation.method }} {{ apiExampleUrl(capabilities.video.generation.path) }}</code>
                <button
                  v-if="hasCapabilityExample('video-generation')"
                  type="button"
                  data-testid="api-example-video-generation"
                  class="flex-shrink-0 text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('video-generation')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
            </div>
            <div v-if="capabilities.video.status.available" data-testid="api-video-status">
              <div class="flex items-center justify-between gap-2">
                <code class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.video.status.method }} {{ apiExampleUrl(capabilities.video.status.path) }}</code>
                <button
                  v-if="hasCapabilityExample('video-status')"
                  type="button"
                  data-testid="api-example-video-status"
                  class="flex-shrink-0 text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('video-status')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
            </div>
            <div v-if="capabilities.video.content.available" data-testid="api-video-content">
              <div class="flex items-center justify-between gap-2">
                <code class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.video.content.method }} {{ apiExampleUrl(capabilities.video.content.path) }}</code>
                <button
                  v-if="hasCapabilityExample('video-content')"
                  type="button"
                  data-testid="api-example-video-content"
                  class="flex-shrink-0 text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                  @click="openCapabilityExampleById('video-content')"
                >
                  {{ t('keys.useKeyModal.capabilities.viewExample') }}
                </button>
              </div>
            </div>
            <code v-if="capabilities.video.edit.available" data-testid="api-video-edit" class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.video.edit.method }} {{ apiExampleUrl(capabilities.video.edit.path) }}</code>
            <code v-if="capabilities.video.extension.available" data-testid="api-video-extension" class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.video.extension.method }} {{ apiExampleUrl(capabilities.video.extension.path) }}</code>
            <p v-if="!capabilities.video.edit.available || !capabilities.video.extension.available" class="text-xs text-amber-700 dark:text-amber-300">{{ t('keys.useKeyModal.api.video.generationOnly') }}</p>
          </section>

          <section
            v-if="capabilities && capabilities.seedance.available"
            data-testid="api-seedance-section"
            aria-labelledby="api-seedance-title"
            class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/50"
          >
            <h4 id="api-seedance-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('keys.useKeyModal.api.seedance.title') }}</h4>
            <p class="text-xs text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.api.seedance.prefixes') }}</p>
            <div class="flex flex-wrap gap-2">
              <button
                v-if="capabilities.seedance.create.available && hasCapabilityExample('seedance-create')"
                type="button"
                data-testid="api-example-seedance-create"
                class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                @click="openCapabilityExampleById('seedance-create')"
              >
                {{ t('keys.useKeyModal.capabilities.viewExample') }}: {{ capabilities.seedance.create.method }}
              </button>
              <button
                v-if="capabilities.seedance.status.available && hasCapabilityExample('seedance-status')"
                type="button"
                data-testid="api-example-seedance-status"
                class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                @click="openCapabilityExampleById('seedance-status')"
              >
                {{ t('keys.useKeyModal.capabilities.viewExample') }}: {{ capabilities.seedance.status.method }}
              </button>
              <button
                v-if="capabilities.seedance.delete.available && hasCapabilityExample('seedance-delete')"
                type="button"
                data-testid="api-example-seedance-delete"
                class="text-[11px] font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                @click="openCapabilityExampleById('seedance-delete')"
              >
                {{ t('keys.useKeyModal.capabilities.viewExample') }}: {{ capabilities.seedance.delete.method }}
              </button>
            </div>
            <code class="block overflow-x-auto rounded bg-gray-900 px-3 py-2 text-[11px] text-gray-100">{{ capabilities.seedance.create.method }} {{ apiExampleUrl(capabilities.seedance.create.path) }}&#10;{{ capabilities.seedance.status.method }} {{ apiExampleUrl(capabilities.seedance.status.path) }}&#10;{{ capabilities.seedance.delete.method }} {{ apiExampleUrl(capabilities.seedance.delete.path) }}</code>
          </section>
        </section>

        <!-- Codex Authentication Mode -->
        <div
          v-if="showCodexAuthMode && activeClientTab !== 'api'"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
        >
          <div class="mb-2">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('keys.useKeyModal.openai.authModeTitle') }}
            </p>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.useKeyModal.openai.authModeDescription') }}
            </p>
          </div>
          <div
            class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700"
            role="radiogroup"
            :aria-label="t('keys.useKeyModal.openai.authModeTitle')"
          >
            <button
              type="button"
              role="radio"
              data-testid="codex-auth-mode-legacy"
              :aria-checked="codexAuthMode === 'legacy'"
              :class="[
                'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                codexAuthMode === 'legacy'
                  ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
                  : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
              ]"
              @click="codexAuthMode = 'legacy'"
            >
              {{ t('keys.useKeyModal.openai.authModeLegacy') }}
            </button>
            <button
              type="button"
              role="radio"
              data-testid="codex-auth-mode-api-key"
              :aria-checked="codexAuthMode === 'api-key'"
              :class="[
                'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                codexAuthMode === 'api-key'
                  ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
                  : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
              ]"
              @click="codexAuthMode = 'api-key'"
            >
              {{ t('keys.useKeyModal.openai.authModeApiKey') }}
            </button>
          </div>
          <div
            v-if="codexAuthMode === 'api-key'"
            data-testid="codex-api-key-restart-notice"
            class="mt-3 flex items-start gap-2 border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-500 dark:bg-amber-950/30 dark:text-amber-200"
          >
            <Icon name="exclamationCircle" size="sm" class="mt-0.5 flex-shrink-0" />
            <p>{{ t('keys.useKeyModal.openai.authModeApiKeyRestartNotice') }}</p>
          </div>
        </div>

        <!-- OS/Shell Tabs -->
        <div v-if="showShellTabs && activeClientTab !== 'api'" class="overflow-x-auto border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex min-w-max gap-4" aria-label="Tabs">
            <button
              v-for="tab in currentTabs"
              :key="tab.id"
              type="button"
              @click="activeTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label }}
              </span>
            </button>
          </nav>
        </div>

        <!-- Code Blocks (Stacked for multi-file platforms) -->
        <div v-if="activeClientTab !== 'api'" class="space-y-4">
          <div
            v-for="(file, index) in currentFiles"
            :key="index"
            class="relative"
          >
            <!-- File Hint (if exists) -->
            <p v-if="file.hint" class="text-xs text-amber-600 dark:text-amber-400 mb-1.5 flex items-center gap-1">
              <Icon name="exclamationCircle" size="sm" class="flex-shrink-0" />
              {{ file.hint }}
            </p>
            <div class="bg-gray-900 dark:bg-dark-900 rounded-xl overflow-hidden">
              <!-- Code Header -->
              <div class="flex items-center justify-between px-4 py-2 bg-gray-800 dark:bg-dark-800 border-b border-gray-700 dark:border-dark-700">
                <span class="min-w-0 truncate text-xs text-gray-400 font-mono">{{ file.path }}</span>
                <button
                  type="button"
                  @click="copyContent(file.content, index)"
                  class="flex flex-shrink-0 items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg transition-colors"
                  :class="copiedIndex === index
                    ? 'bg-green-500/20 text-green-400'
                    : 'bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-white'"
                >
                  <svg v-if="copiedIndex === index" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                  <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                  </svg>
                  {{ copiedIndex === index ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                </button>
              </div>
              <!-- Code Content -->
              <pre class="p-4 text-sm font-mono text-gray-100 overflow-x-auto"><code v-if="file.highlighted" v-html="file.highlighted"></code><code v-else v-text="file.content"></code></pre>
            </div>
          </div>
        </div>

        <section
          v-if="showCodexModelCatalog && activeClientTab !== 'api'"
          data-testid="codex-model-catalog"
          class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50"
        >
          <div class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <h3 class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('keys.useKeyModal.codexModelCatalog.title') }}
              </h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('keys.useKeyModal.codexModelCatalog.description') }}
              </p>
              <p class="mt-1 truncate font-mono text-xs text-gray-700 dark:text-gray-300">
                {{ codexModelCatalogMode === 'remote' ? codexModelCatalogUrl : codexModelCatalogPath }}
              </p>
              <select
                v-model="codexModelCatalogMode"
                data-testid="codex-model-catalog-mode"
                :aria-label="t('keys.useKeyModal.codexModelCatalog.mode')"
                class="input mt-2 text-sm"
              >
                <option value="remote" :disabled="codexModelCatalogOversized">{{ t('keys.useKeyModal.codexModelCatalog.remote') }}</option>
                <option value="file">{{ t('keys.useKeyModal.codexModelCatalog.local') }}</option>
              </select>
              <p v-if="codexModelCatalogOversized" class="mt-2 text-xs text-amber-700 dark:text-amber-300">
                {{ t('keys.useKeyModal.codexModelCatalog.oversized') }}
              </p>
            </div>
            <button
              v-if="codexModelManifestState === 'ready'"
              type="button"
              class="btn btn-primary min-h-9 flex-shrink-0 px-3 text-xs"
              @click="downloadCodexModelManifest"
            >
              <Icon name="download" size="sm" class="mr-1.5" />
              {{ t('keys.useKeyModal.codexModelCatalog.download') }}
            </button>
            <button
              v-else
              type="button"
              data-testid="codex-model-catalog-fetch"
              class="btn btn-primary min-h-9 flex-shrink-0 px-3 text-xs"
              :disabled="codexModelManifestState === 'loading' || !apiKey"
              @click="loadCodexModelManifest"
            >
              <Icon
                name="refresh"
                size="sm"
                class="mr-1.5"
                :class="codexModelManifestState === 'loading' ? 'animate-spin' : ''"
              />
              {{ codexModelManifestState === 'error'
                ? t('keys.useKeyModal.codexModelCatalog.retry')
                : t('keys.useKeyModal.codexModelCatalog.fetch') }}
            </button>
          </div>
          <p
            v-if="codexModelManifestState === 'ready'"
            class="border-t border-gray-200 px-4 py-2 text-xs text-emerald-700 dark:border-dark-700 dark:text-emerald-300"
          >
            {{ t('keys.useKeyModal.codexModelCatalog.modelsCount', { count: codexModelManifestModelCount }) }}
          </p>
          <p
            v-else-if="codexModelManifestState === 'error'"
            class="border-t border-red-200 px-4 py-2 text-xs text-red-700 dark:border-red-900 dark:text-red-300"
          >
            {{ t('keys.useKeyModal.codexModelCatalog.errorDescription') }}
          </p>
        </section>

        <!-- Usage Note -->
        <div v-if="showPlatformNote && activeClientTab !== 'api'" class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
          <Icon name="infoCircle" size="md" class="text-blue-500 flex-shrink-0 mt-0.5" />
          <p class="text-sm text-blue-700 dark:text-blue-300">
            {{ platformNote }}
          </p>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="emit('close')"
          class="btn btn-secondary"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, nextTick, onBeforeUnmount, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { buildCodexModelCatalogUrl, fetchCodexModelsManifest } from '@/api/codex'
import { keysAPI, type APIKeyCapabilities, type APIKeyCapabilityReason } from '@/api/keys'
import { buildCapabilityAPIExample, type CapabilityAPIExample } from './apiCapabilityExamples'
import type { GroupPlatform } from '@/types'
import {
  findCodexCatalogModel,
  formatCodexReasoningEffortTomlLine,
  parseCodexCatalogModels,
  selectCodexConfigReasoningEffort
} from '@/utils/codexCatalogConfig'

interface Props {
  show: boolean
  apiKey: string
  apiKeyId?: number | null
  baseUrl: string
  platform: GroupPlatform | null
  claudeCodeOnly?: boolean
  allowMessagesDispatch?: boolean
}

interface Emits {
  (e: 'close'): void
}

interface TabConfig {
  id: string
  label: string
  icon: Component
}

interface FileConfig {
  path: string
  content: string
  hint?: string  // Optional hint message for this file
  highlighted?: string
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()

const copiedIndex = ref<number | null>(null)
const activeTab = ref<string>('unix')

type CapabilityState = 'idle' | 'loading' | 'ready' | 'error'
interface CapabilityGuidanceItem {
  id: string
  label: string
  available: boolean
  method?: string
  path?: string
  reason?: APIKeyCapabilityReason
}
interface SelectedCapabilityExample extends CapabilityAPIExample {
  id: string
  label: string
  method: string
  path: string
}
const capabilityState = ref<CapabilityState>('idle')
const capabilities = ref<APIKeyCapabilities | null>(null)
const selectedCapabilityExampleId = ref<string | null>(null)
const copiedCapabilityExamplePart = ref<'request' | 'response' | null>(null)
let capabilityController: AbortController | null = null
let capabilityRequestID = 0
const showCapabilityGuidance = computed(() => props.show && props.apiKeyId != null)
const showApiTab = computed(() => {
  const value = capabilities.value
  if (!props.show || capabilityState.value !== 'ready' || !value) return false
  return value.image.sync.available ||
    value.image.async.generation.available ||
    value.image.async.edit.available ||
    value.image.async.poll.available ||
    value.batch_image.available ||
    value.video.generation.available ||
    value.video.status.available ||
    value.video.content.available ||
    value.seedance.available ||
    value.gemini_native.image_generation.available
})
const apiExampleUrl = (path?: string) => {
  const base = props.baseUrl.trim().replace(/\/+$/, '')
  if (!path) return `${base}/...`

  const isAbsolutePath = path.startsWith('/')
  const normalizedPath = isAbsolutePath ? path : `/${path}`
  if (!isAbsolutePath) return `${base}${normalizedPath}`

  try {
    return `${new URL(base).origin}${normalizedPath}`
  } catch {
    return normalizedPath
  }
}
const capabilityGuidanceItems = computed<CapabilityGuidanceItem[]>(() => {
  if (!capabilities.value) return []
  const image = capabilities.value.image
  const video = capabilities.value.video
  const gemini = capabilities.value.gemini_native.image_generation
  return [
    {
      id: 'image-sync',
      label: t('keys.useKeyModal.capabilities.imageSync'),
      available: image.sync.available,
      method: image.sync.method,
      path: image.sync.path,
      reason: image.sync.reason
    },
    {
      id: 'image-async-generation',
      label: t('keys.useKeyModal.capabilities.imageAsyncSubmit'),
      available: image.async.generation.available,
      method: image.async.generation.method,
      path: image.async.generation.path,
      reason: image.async.generation.reason
    },
    {
      id: 'image-async-edit',
      label: t('keys.useKeyModal.capabilities.imageAsyncEdit'),
      available: image.async.edit.available,
      method: image.async.edit.method,
      path: image.async.edit.path,
      reason: image.async.edit.reason
    },
    {
      id: 'image-async-poll',
      label: t('keys.useKeyModal.capabilities.imageAsyncPoll'),
      available: image.async.poll.available,
      method: image.async.poll.method,
      path: image.async.poll.path,
      reason: image.async.poll.reason
    },
    {
      id: 'batch-image',
      label: t('keys.useKeyModal.capabilities.batchImage'),
      available: capabilities.value.batch_image.available,
      method: capabilities.value.batch_image.models.method,
      path: capabilities.value.batch_image.models.path,
      reason: capabilities.value.batch_image.reason
    },
    {
      id: 'gemini-native-generate-content',
      label: t('keys.useKeyModal.capabilities.geminiNativeGenerateContent'),
      available: gemini.generate_content.available,
      method: gemini.generate_content.method,
      path: gemini.generate_content.path,
      reason: gemini.generate_content.reason
    },
    {
      id: 'gemini-native-stream-generate-content',
      label: t('keys.useKeyModal.capabilities.geminiNativeStreamGenerateContent'),
      available: gemini.stream_generate_content.available,
      method: gemini.stream_generate_content.method,
      path: gemini.stream_generate_content.path,
      reason: gemini.stream_generate_content.reason
    },
    {
      id: 'video-generation',
      label: t('keys.useKeyModal.capabilities.videoGeneration'),
      available: video?.generation.available ?? false,
      method: video?.generation.method,
      path: video?.generation.path,
      reason: video?.generation.reason
    },
    {
      id: 'video-status',
      label: t('keys.useKeyModal.capabilities.videoStatus'),
      available: video?.status.available ?? false,
      method: video?.status.method,
      path: video?.status.path,
      reason: video?.status.reason
    },
    {
      id: 'video-content',
      label: t('keys.useKeyModal.capabilities.videoContent'),
      available: video?.content.available ?? false,
      method: video?.content.method,
      path: video?.content.path,
      reason: video?.content.reason
    },
    {
      id: 'video-edit',
      label: t('keys.useKeyModal.capabilities.videoEdit'),
      available: video?.edit.available ?? false,
      method: video?.edit.method,
      path: video?.edit.path,
      reason: video?.edit.reason
    },
    {
      id: 'video-extension',
      label: t('keys.useKeyModal.capabilities.videoExtension'),
      available: video?.extension.available ?? false,
      method: video?.extension.method,
      path: video?.extension.path,
      reason: video?.extension.reason
    },
    {
      id: 'seedance-create',
      label: t('keys.useKeyModal.capabilities.seedanceCreate'),
      available: capabilities.value.seedance?.create?.available ?? false,
      method: capabilities.value.seedance?.create?.method,
      path: capabilities.value.seedance?.create?.path,
      reason: capabilities.value.seedance?.create?.reason
    },
    {
      id: 'seedance-status',
      label: t('keys.useKeyModal.capabilities.seedanceStatus'),
      available: capabilities.value.seedance?.status?.available ?? false,
      method: capabilities.value.seedance?.status?.method,
      path: capabilities.value.seedance?.status?.path,
      reason: capabilities.value.seedance?.status?.reason
    },
    {
      id: 'seedance-delete',
      label: t('keys.useKeyModal.capabilities.seedanceDelete'),
      available: capabilities.value.seedance?.delete?.available ?? false,
      method: capabilities.value.seedance?.delete?.method,
      path: capabilities.value.seedance?.delete?.path,
      reason: capabilities.value.seedance?.delete?.reason
    }
  ]
})

const capabilityReasonLabel = (reason: APIKeyCapabilityReason) => {
  const key = `keys.useKeyModal.capabilities.reasons.${reason}`
  const translated = t(key)
  return translated === key ? t('keys.useKeyModal.capabilities.unavailable') : translated
}

const hasCapabilityExample = (id: string) => buildCapabilityAPIExample(id, '') !== null

const selectedCapabilityExample = computed<SelectedCapabilityExample | null>(() => {
  const id = selectedCapabilityExampleId.value
  if (!id) return null
  const item = capabilityGuidanceItems.value.find((candidate) => candidate.id === id)
  if (!item?.available || !item.path) return null
  const example = buildCapabilityAPIExample(id, apiExampleUrl(item.path))
  if (!example) return null
  return {
    ...example,
    id,
    label: item.label,
    method: item.method || 'GET',
    path: item.path
  }
})

const openCapabilityExample = (item: CapabilityGuidanceItem) => {
  if (!item.available || !item.path || !hasCapabilityExample(item.id)) return
  selectedCapabilityExampleId.value = item.id
  copiedCapabilityExamplePart.value = null
}

const openCapabilityExampleById = (id: string) => {
  const item = capabilityGuidanceItems.value.find((candidate) => candidate.id === id)
  if (item) openCapabilityExample(item)
}

const copyCapabilityExample = async (part: 'request' | 'response') => {
  const example = selectedCapabilityExample.value
  if (!example) return
  const success = await clipboardCopy(example[part], t('keys.copied'))
  if (success) {
    copiedCapabilityExamplePart.value = part
    setTimeout(() => {
      if (copiedCapabilityExamplePart.value === part) copiedCapabilityExamplePart.value = null
    }, 2000)
  }
}

const activeClientTab = ref<string>('claude')
const focusApiTab = async () => {
  activeClientTab.value = 'api'
  await nextTick()
  document.getElementById('client-tab-api')?.focus()
}
watch(showApiTab, (available) => {
  if (!available && activeClientTab.value === 'api') {
    activeClientTab.value = defaultClientTab.value
  }
})
type CodexAuthMode = 'legacy' | 'api-key'
const codexAuthMode = ref<CodexAuthMode>('legacy')
type CodexModelManifestState = 'idle' | 'loading' | 'ready' | 'error'
const codexModelManifestState = ref<CodexModelManifestState>('idle')
const codexModelManifestContent = ref('')
const codexModelManifestModelCount = ref(0)
const codexModelCatalogMode = ref<'remote' | 'file'>('remote')
const codexModelManifestResponseBytes = ref(0)
const codexModelCatalogOversized = computed(() => codexModelManifestResponseBytes.value > 1024 * 1024)
const codexModelCatalogUrl = computed(() => buildCodexModelCatalogUrl(props.baseUrl))
const codexLocalCatalogToml = computed(() => codexModelCatalogMode.value === 'file'
  ? `model_catalog_json = "${CODEX_MODEL_CATALOG_CONFIG_PATH}"\n`
  : '')
let codexModelManifestController: AbortController | null = null
let codexModelManifestRequestID = 0

const showCodexModelCatalog = computed(() =>
  props.show &&
  (activeClientTab.value === 'codex' ||
    (props.platform === 'openai' && activeClientTab.value === 'codex-ws'))
)

const codexModelCatalogPath = computed(() => {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'
  return joinConfigPath(configDir, 'codex-models.json', isWindows)
})

// Codex expands a leading ~/ on every platform but not %userprofile%, which it
// resolves relative to the config directory, so config.toml always uses ~/.
const CODEX_MODEL_CATALOG_CONFIG_PATH = '~/.codex/codex-models.json'

const codexManifestContext = computed(() => {
  if (!showCodexModelCatalog.value) return ''
  return `${props.platform}|${props.baseUrl}|${props.apiKey}`
})

// Reset tabs when platform changes
const defaultClientTab = computed(() => {
  if (props.claudeCodeOnly) return 'claude'
  switch (props.platform) {
    case 'openai':
      return 'codex'
    case 'grok':
      return 'grok'
    case 'gemini':
      return 'gemini'
    case 'antigravity':
      return 'claude'
    case 'typesafe':
      return 'systemone'
    default:
      return 'claude'
  }
})

watch(() => [props.platform, props.claudeCodeOnly], () => {
  activeTab.value = 'unix'
  activeClientTab.value = defaultClientTab.value
  codexAuthMode.value = 'legacy'
}, { immediate: true })

watch(() => props.show, (show) => {
  if (show) {
    codexAuthMode.value = 'legacy'
  } else {
    resetCodexModelManifest()
  }
})

const loadCapabilities = async (id: number) => {
  capabilityController?.abort()
  const controller = new AbortController()
  capabilityController = controller
  const requestID = ++capabilityRequestID
  capabilityState.value = 'loading'
  capabilities.value = null
  try {
    const nextCapabilities = await keysAPI.getCapabilities(id, { signal: controller.signal })
    if (requestID === capabilityRequestID && !controller.signal.aborted) {
      capabilities.value = nextCapabilities
      capabilityState.value = 'ready'
    }
  } catch (error) {
    if (controller.signal.aborted || requestID !== capabilityRequestID) return
    console.error('Failed to load API key capabilities')
    capabilityState.value = 'error'
  }
}

watch(() => [props.show, props.apiKeyId] as const, ([show, id]) => {
  if (show && id != null) {
    void loadCapabilities(id)
    return
  }
  selectedCapabilityExampleId.value = null
  copiedCapabilityExamplePart.value = null
  capabilityController?.abort()
  capabilityRequestID += 1
  capabilityState.value = 'idle'
  capabilities.value = null
}, { immediate: true })

onBeforeUnmount(() => {
  capabilityController?.abort()
})

watch(codexManifestContext, (context, previousContext) => {
  if (context !== previousContext) {
    resetCodexModelManifest()
  }
})

// Reset shell tab when client changes
watch(activeClientTab, () => {
  activeTab.value = 'unix'
})

// Icon components
const AppleIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    }, [
      h('path', { d: 'M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z' })
    ])
  }
}

const WindowsIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    }, [
      h('path', { d: 'M3 12V6.75l6-1.32v6.48L3 12zm17-9v8.75l-10 .15V5.21L20 3zM3 13l6 .09v6.81l-6-1.15V13zm7 .25l10 .15V21l-10-1.91v-5.84z' })
    ])
  }
}

// Terminal icon for Claude Code
const TerminalIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'm6.75 7.5 3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0 0 21 17.25V6.75A2.25 2.25 0 0 0 18.75 4.5H5.25A2.25 2.25 0 0 0 3 6.75v10.5A2.25 2.25 0 0 0 5.25 20.25Z'
      })
    ])
  }
}

// Sparkle icon for Gemini
const SparkleIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 0 0-2.456 2.456ZM16.894 20.567 16.5 21.75l-.394-1.183a2.25 2.25 0 0 0-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 0 0 1.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 0 0 1.423 1.423l1.183.394-1.183.394a2.25 2.25 0 0 0-1.423 1.423Z'
      })
    ])
  }
}

const clientTabs = computed((): TabConfig[] => {
  if (!props.platform) return []
  const withApiTab = (tabs: TabConfig[]) => showApiTab.value
    ? [...tabs, { id: 'api', label: t('keys.useKeyModal.api.tab'), icon: SparkleIcon }]
    : tabs
  if (props.claudeCodeOnly) {
    return withApiTab([{ id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon }])
  }
  switch (props.platform) {
    case 'openai': {
      const tabs: TabConfig[] = [
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'codex-ws', label: t('keys.useKeyModal.cliTabs.codexCliWs'), icon: TerminalIcon },
      ]
      if (props.allowMessagesDispatch) {
        tabs.push({ id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon })
      }
      tabs.push({ id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon })
      return withApiTab(tabs)
    }
    case 'gemini':
      return withApiTab([
        { id: 'gemini', label: t('keys.useKeyModal.cliTabs.geminiCli'), icon: SparkleIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ])
    case 'antigravity':
      return withApiTab([
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'gemini', label: t('keys.useKeyModal.cliTabs.geminiCli'), icon: SparkleIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ])
    case 'grok':
      return withApiTab([
        { id: 'grok', label: t('keys.useKeyModal.cliTabs.grokCli'), icon: TerminalIcon },
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ])
    case 'typesafe':
      return withApiTab([
        { id: 'systemone', label: t('keys.useKeyModal.cliTabs.systemOne'), icon: TerminalIcon }
      ])
    case 'deepseek':
    case 'minimax':
    case 'composite':
      return withApiTab([
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ])
    default:
      return withApiTab([
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ])
  }
})

// Shell tabs (3 types for environment variable based configs)
const shellTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'cmd', label: 'Windows CMD', icon: WindowsIcon },
  { id: 'powershell', label: 'PowerShell', icon: WindowsIcon }
]

// OpenAI tabs (2 OS types)
const openaiTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'windows', label: 'Windows', icon: WindowsIcon }
]

const showShellTabs = computed(() => activeClientTab.value !== 'opencode' && activeClientTab.value !== 'api')

const showCodexAuthMode = computed(() =>
  props.platform === 'openai' &&
  (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws')
)

const currentTabs = computed(() => {
  if (!showShellTabs.value) return []
  if (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws' || activeClientTab.value === 'grok') {
    return openaiTabs
  }
  return shellTabs
})

const platformDescription = computed(() => {
  if (activeClientTab.value === 'api') return t('keys.useKeyModal.api.description')
  if (activeClientTab.value === 'codex' &&
    props.platform !== 'openai' &&
    props.platform !== 'grok' &&
    props.platform !== 'deepseek' &&
    props.platform !== 'minimax' &&
    props.platform !== 'composite') {
    return t('keys.useKeyModal.routedCodex.description')
  }
  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.description')
      }
      return t('keys.useKeyModal.openai.description')
    case 'gemini':
      return t('keys.useKeyModal.gemini.description')
    case 'antigravity':
      return t('keys.useKeyModal.antigravity.description')
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.grok.claudeDescription')
      }
      if (activeClientTab.value === 'codex') {
        return t('keys.useKeyModal.grok.codexDescription')
      }
      return t('keys.useKeyModal.grok.description')
    case 'deepseek':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.deepseek.codexDescription')
        : t('keys.useKeyModal.deepseek.description')
    case 'minimax':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.minimax.codexDescription')
        : t('keys.useKeyModal.minimax.description')
    case 'composite':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.composite.codexDescription')
        : t('keys.useKeyModal.composite.description')
    case 'typesafe':
      return t('keys.useKeyModal.typesafe.description')
    default:
      return t('keys.useKeyModal.description')
  }
})

const platformNote = computed(() => {
  if (activeClientTab.value === 'codex' &&
    props.platform !== 'openai' &&
    props.platform !== 'grok' &&
    props.platform !== 'deepseek' &&
    props.platform !== 'minimax' &&
    props.platform !== 'composite') {
    return t('keys.useKeyModal.routedCodex.note')
  }
  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.note')
      }
      return activeTab.value === 'windows'
        ? t('keys.useKeyModal.openai.noteWindows')
        : t('keys.useKeyModal.openai.note')
    case 'gemini':
      return t('keys.useKeyModal.gemini.note')
    case 'antigravity':
      return activeClientTab.value === 'claude'
        ? t('keys.useKeyModal.antigravity.claudeNote')
        : t('keys.useKeyModal.antigravity.geminiNote')
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.grok.claudeNote')
      }
      if (activeClientTab.value === 'codex') {
        return activeTab.value === 'windows'
          ? t('keys.useKeyModal.grok.codexNoteWindows')
          : t('keys.useKeyModal.grok.codexNote')
      }
      // Grok CLI: shell-specific path guidance (env + ~/.grok/config.toml).
      if (activeClientTab.value === 'grok' && (activeTab.value === 'cmd' || activeTab.value === 'powershell')) {
        return t('keys.useKeyModal.grok.noteWindows')
      }
      if (activeClientTab.value === 'grok' && activeTab.value === 'windows') {
        return t('keys.useKeyModal.grok.noteWindows')
      }
      return t('keys.useKeyModal.grok.note')
    case 'deepseek':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.deepseek.codexNote')
        : t('keys.useKeyModal.note')
    case 'minimax':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.minimax.codexNote')
        : t('keys.useKeyModal.note')
    case 'composite':
      return activeClientTab.value === 'codex'
        ? t('keys.useKeyModal.composite.codexNote')
        : t('keys.useKeyModal.note')
    case 'typesafe':
      return t('keys.useKeyModal.typesafe.note')
    default:
      return t('keys.useKeyModal.note')
  }
})

const showPlatformNote = computed(() => activeClientTab.value !== 'opencode')

function resetCodexModelManifest() {
  codexModelManifestController?.abort()
  codexModelManifestController = null
  codexModelManifestRequestID += 1
  codexModelManifestState.value = 'idle'
  codexModelManifestContent.value = ''
  codexModelManifestModelCount.value = 0
  codexModelManifestResponseBytes.value = 0
}

async function loadCodexModelManifest() {
  if (!showCodexModelCatalog.value || !props.apiKey) return

  codexModelManifestController?.abort()
  const controller = new AbortController()
  const requestID = ++codexModelManifestRequestID
  codexModelManifestController = controller
  codexModelManifestState.value = 'loading'

  try {
    const result = await fetchCodexModelsManifest(props.baseUrl, props.apiKey, controller.signal)
    if (requestID !== codexModelManifestRequestID) return
    codexModelManifestContent.value = result.content
    codexModelManifestModelCount.value = result.modelCount
    codexModelManifestResponseBytes.value = result.responseBytes
    if (codexModelCatalogOversized.value) codexModelCatalogMode.value = 'file'
    codexModelManifestState.value = 'ready'
  } catch (error) {
    const errorName = error && typeof error === 'object' && 'name' in error
      ? String((error as { name?: unknown }).name || '')
      : ''
    if (requestID !== codexModelManifestRequestID || errorName === 'AbortError') return
    codexModelManifestState.value = 'error'
  } finally {
    if (requestID === codexModelManifestRequestID) {
      codexModelManifestController = null
    }
  }
}

function downloadCodexModelManifest() {
  if (!codexModelManifestContent.value) return
  saveAs(
    new Blob([codexModelManifestContent.value], { type: 'application/json;charset=utf-8' }),
    'codex-models.json'
  )
}

const codexCatalogModelSlugs = computed(() =>
  parseCodexCatalogModels(codexModelManifestContent.value).map((model) => model.slug)
)

function selectCodexCatalogModel(preferredModel: string): string {
  if (codexCatalogModelSlugs.value.includes(preferredModel)) return preferredModel
  return codexCatalogModelSlugs.value[0] || preferredModel
}

function codexReasoningEffortTomlLine(modelSlug: string): string {
  return formatCodexReasoningEffortTomlLine(
    selectCodexConfigReasoningEffort(findCodexCatalogModel(codexModelManifestContent.value, modelSlug))
  )
}

const escapeHtml = (value: string) => value
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const wrapToken = (className: string, value: string) =>
  `<span class="${className}">${escapeHtml(value)}</span>`

const keyword = (value: string) => wrapToken('text-emerald-300', value)
const variable = (value: string) => wrapToken('text-sky-200', value)
const operator = (value: string) => wrapToken('text-slate-400', value)
const string = (value: string) => wrapToken('text-amber-200', value)
const comment = (value: string) => wrapToken('text-slate-500', value)

// Syntax highlighting helpers
// Generate file configs based on platform and active tab
const currentFiles = computed((): FileConfig[] => {
  const baseUrl = props.baseUrl || window.location.origin
  const apiKey = props.apiKey
  const baseRoot = baseUrl.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
  const ensureV1 = (value: string) => {
    const trimmed = value.replace(/\/+$/, '')
    return trimmed.endsWith('/v1') ? trimmed : `${trimmed}/v1`
  }
  const apiBase = ensureV1(baseRoot)
  const antigravityBase = ensureV1(`${baseRoot}/antigravity`)
  const antigravityGeminiBase = (() => {
    const trimmed = `${baseRoot}/antigravity`.replace(/\/+$/, '')
    return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
  })()
  const geminiBase = (() => {
    const trimmed = baseRoot.replace(/\/+$/, '')
    return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
  })()

  if (activeClientTab.value === 'opencode') {
    switch (props.platform) {
      case 'anthropic':
        return [generateOpenCodeConfig('anthropic', apiBase, apiKey)]
      case 'openai':
        return [generateOpenCodeConfig('openai', apiBase, apiKey)]
      case 'gemini':
        return [generateOpenCodeConfig('gemini', geminiBase, apiKey)]
      case 'antigravity':
        return [
          generateOpenCodeConfig('antigravity-claude', antigravityBase, apiKey, 'opencode.json (Claude)'),
          generateOpenCodeConfig('antigravity-gemini', antigravityGeminiBase, apiKey, 'opencode.json (Gemini)')
        ]
      case 'grok':
        return [generateOpenCodeConfig('grok', apiBase, apiKey)]
      default:
        return [generateOpenCodeConfig('openai', apiBase, apiKey)]
    }
  }

  switch (props.platform) {
    case 'typesafe':
      return [generateSystemOneCurl(baseRoot, apiKey)]
    case 'openai':
      if (activeClientTab.value === 'claude') {
        // Anthropic clients append /v1/messages themselves.
        return generateAnthropicFiles(baseRoot, apiKey)
      }
      if (activeClientTab.value === 'codex-ws') {
        return generateOpenAIWsFiles(apiBase, apiKey)
      }
      // Codex appends /responses directly and does not add /v1.
      return generateOpenAIFiles(apiBase, apiKey)
    case 'gemini':
      if (activeClientTab.value === 'codex') {
        return generateRoutedCodexFiles(apiBase, apiKey, 'gemini')
      }
      return [generateGeminiCliContent(baseUrl, apiKey)]
    case 'antigravity':
      if (activeClientTab.value === 'codex') {
        return generateRoutedCodexFiles(apiBase, apiKey, 'antigravity')
      }
      if (activeClientTab.value === 'gemini') {
        return [generateGeminiCliContent(`${baseUrl}/antigravity`, apiKey)]
      }
      return generateAnthropicFiles(`${baseUrl}/antigravity`, apiKey)
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return generateGrokClaudeFiles(baseRoot, apiKey)
      }
      if (activeClientTab.value === 'codex') {
        return generateGrokCodexFiles(apiBase, apiKey)
      }
      return generateGrokFiles(apiBase, apiKey)
    case 'deepseek':
      if (activeClientTab.value === 'codex') {
        return generateRoutedCodexFiles(apiBase, apiKey, 'deepseek')
      }
      return generateAnthropicFiles(baseRoot, apiKey)
    case 'minimax':
      if (activeClientTab.value === 'codex') {
        return generateRoutedCodexFiles(apiBase, apiKey, 'minimax')
      }
      return generateAnthropicFiles(baseRoot, apiKey)
    case 'composite':
      if (activeClientTab.value === 'codex') {
        return generateRoutedCodexFiles(apiBase, apiKey, 'composite')
      }
      return generateAnthropicFiles(baseRoot, apiKey)
    default:
      if (activeClientTab.value === 'codex' && props.platform) {
        return generateRoutedCodexFiles(apiBase, apiKey, props.platform)
      }
      return generateAnthropicFiles(baseUrl, apiKey)
  }
})

function generateSystemOneCurl(baseUrl: string, apiKey: string): FileConfig {
  const endpoint = `${baseUrl}/v1/systemone`
  const payload = `{
  "model": "jev-latest",
  "state": "Text to evaluate",
  "questions": {
    "safety": {
      "type": "noul",
      "instructions": "Evaluate whether the text is unsafe"
    }
  }
}`
  if (activeTab.value === 'powershell') {
    return {
      path: 'PowerShell',
      content: `$headers = @{ Authorization = "Bearer ${apiKey}" }
$body = @'
${payload}
'@
Invoke-RestMethod -Method Post -Uri "${endpoint}" -Headers $headers -ContentType "application/json" -Body $body`
    }
  }
  if (activeTab.value === 'cmd') {
    return {
      path: 'Command Prompt',
      content: `curl -X POST "${endpoint}" ^
  -H "Authorization: Bearer ${apiKey}" ^
  -H "Content-Type: application/json" ^
  --data "{\"model\":\"jev-latest\",\"state\":\"Text to evaluate\",\"questions\":{\"safety\":{\"type\":\"noul\",\"instructions\":\"Evaluate whether the text is unsafe\"}}}"`
    }
  }
  return {
    path: 'Terminal',
    content: `curl -X POST "${endpoint}" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -H "Content-Type: application/json" \\
  --data '${payload}'`
  }
}

function generateAnthropicFiles(baseUrl: string, apiKey: string): FileConfig[] {
  let path: string
  let content: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="${apiKey}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${apiKey}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:ANTHROPIC_BASE_URL="${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="${apiKey}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const vscodeSettingsPath = activeTab.value === 'unix'
    ? '~/.claude/settings.json'
    : '%USERPROFILE%\\.claude\\settings.json'

  const vscodeContent = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}`

  return [
    { path, content },
    {
      path: vscodeSettingsPath,
      content: vscodeContent,
      hint: t('keys.useKeyModal.claudeSettingsHint')
    }
  ]
}

function generateGrokClaudeFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const environment = {
    ANTHROPIC_BASE_URL: baseUrl,
    ANTHROPIC_AUTH_TOKEN: apiKey,
    ANTHROPIC_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_FABLE_MODEL: 'grok-4.5',
    CLAUDE_CODE_SUBAGENT_MODEL: 'grok-4.5',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1'
  }
  let path: string
  let content: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = Object.entries(environment)
        .map(([name, value]) => `export ${name}="${value}"`)
        .join('\n')
      break
    case 'cmd':
      path = 'Command Prompt'
      content = Object.entries(environment)
        .map(([name, value]) => `set ${name}=${value}`)
        .join('\n')
      break
    case 'powershell':
      path = 'PowerShell'
      content = Object.entries(environment)
        .map(([name, value]) => `$env:${name}="${value}"`)
        .join('\n')
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const settingsPath = activeTab.value === 'unix'
    ? '~/.claude/settings.json'
    : '%USERPROFILE%\\.claude\\settings.json'

  return [
    { path, content },
    {
      path: settingsPath,
      content: JSON.stringify({
        $schema: 'https://json.schemastore.org/claude-code-settings.json',
        env: environment
      }, null, 2),
      hint: t('keys.useKeyModal.claudeSettingsHint')
    }
  ]
}

function generateGeminiCliContent(baseUrl: string, apiKey: string): FileConfig {
  const model = 'gemini-2.0-flash'
  const modelComment = t('keys.useKeyModal.gemini.modelComment')
  let path: string
  let content: string
  let highlighted: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export GOOGLE_GEMINI_BASE_URL="${baseUrl}"
export GEMINI_API_KEY="${apiKey}"
export GEMINI_MODEL="${model}"  # ${modelComment}`
      highlighted = `${keyword('export')} ${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(`"${baseUrl}"`)}
${keyword('export')} ${variable('GEMINI_API_KEY')}${operator('=')}${string(`"${apiKey}"`)}
${keyword('export')} ${variable('GEMINI_MODEL')}${operator('=')}${string(`"${model}"`)}  ${comment(`# ${modelComment}`)}`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set GOOGLE_GEMINI_BASE_URL=${baseUrl}
set GEMINI_API_KEY=${apiKey}
set GEMINI_MODEL=${model}`
      highlighted = `${keyword('set')} ${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(baseUrl)}
${keyword('set')} ${variable('GEMINI_API_KEY')}${operator('=')}${string(apiKey)}
${keyword('set')} ${variable('GEMINI_MODEL')}${operator('=')}${string(model)}
${comment(`REM ${modelComment}`)}`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:GOOGLE_GEMINI_BASE_URL="${baseUrl}"
$env:GEMINI_API_KEY="${apiKey}"
$env:GEMINI_MODEL="${model}"  # ${modelComment}`
      highlighted = `${keyword('$env:')}${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(`"${baseUrl}"`)}
${keyword('$env:')}${variable('GEMINI_API_KEY')}${operator('=')}${string(`"${apiKey}"`)}
${keyword('$env:')}${variable('GEMINI_MODEL')}${operator('=')}${string(`"${model}"`)}  ${comment(`# ${modelComment}`)}`
      break
    default:
      path = 'Terminal'
      content = ''
      highlighted = ''
  }

  return { path, content, highlighted }
}

function generateOpenAIFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  const model = selectCodexCatalogModel('gpt-5.5')
  const reasoningEffortLine = codexReasoningEffortTomlLine(model)

  // config.toml content
  const configContent = `model_provider = "OpenAI"
model = "${model}"
review_model = "${model}"
${reasoningEffortLine}disable_response_storage = true
${codexLocalCatalogToml.value}network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
${codexModelCatalogMode.value === 'remote' ? `model_catalog_url = "${escapeTomlBasicString(buildCodexModelCatalogUrl(baseUrl))}"\n` : ''}wire_api = "responses"
${generateCodexProviderAuthConfig(apiKey)}

[features]
${codexModelCatalogMode.value === 'remote' ? 'api_key_model_discovery = true\n' : ''}goals = true`

  return buildOpenAICodexFileConfigs(configDir, configContent, apiKey)
}

function generateCodexProviderAuthConfig(apiKey: string): string {
  if (codexAuthMode.value === 'api-key') {
    return `requires_openai_auth = false
experimental_bearer_token = "${escapeTomlBasicString(apiKey)}"
http_headers = { "x-openai-actor-authorization" = "local-image-extension" }`
  }

  return 'requires_openai_auth = true'
}

function buildOpenAICodexFileConfigs(
  configDir: string,
  configContent: string,
  apiKey: string
): FileConfig[] {
  const files: FileConfig[] = [
    {
      path: `${configDir}/config.toml`,
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    }
  ]

  if (codexAuthMode.value === 'legacy') {
    files.push({
      path: `${configDir}/auth.json`,
      content: JSON.stringify({ OPENAI_API_KEY: apiKey }, null, 2)
    })
  }

  return files
}

function joinConfigPath(dir: string, file: string, windows: boolean): string {
  if (!windows) return `${dir}/${file}`
  return `${dir}\\${file}`
}

function escapeTomlBasicString(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

function generateGrokFiles(baseUrl: string, apiKey: string): FileConfig[] {
  // Prefer unix/cmd/powershell when shell tabs are shown; fall back to windows tab.
  const shell = activeTab.value
  const isWindowsPath = shell === 'windows' || shell === 'cmd' || shell === 'powershell'
  const configDir = isWindowsPath ? '%userprofile%\\.grok' : '~/.grok'

  let envPath: string
  let envContent: string
  switch (shell) {
    case 'cmd':
      envPath = 'Command Prompt'
      envContent = `set GROK_MODELS_BASE_URL=${baseUrl}
set XAI_API_KEY=${apiKey}`
      break
    case 'powershell':
    case 'windows':
      envPath = 'PowerShell'
      envContent = `$env:GROK_MODELS_BASE_URL="${baseUrl}"
$env:XAI_API_KEY="${apiKey}"`
      break
    default:
      envPath = 'Terminal'
      envContent = `export GROK_MODELS_BASE_URL="${baseUrl}"
export XAI_API_KEY="${apiKey}"`
  }

  // Shape follows Grok Build user guide (~/.grok/docs + custom-models) and production-ready Sub2API setups.
  // Text models only (Responses). Image/video: Imagine model IDs on media endpoints / feature overrides.
  // Credential order: api_key field → env_key → signed-in session → XAI_API_KEY global fallback.
  const modelsListUrl = `${baseUrl.replace(/\/+$/, '')}/models`
  const configContent = `# Grok Build CLI → Sub2API Grok group (API key auth).
# Docs: ~/.grok/docs/user-guide/05-configuration.md + 11-custom-models.md
# Verify after save: grok inspect
#
# IMPORTANT: api_backend must be "responses" for Sub2API Grok (POST /v1/responses).
# If omitted, Grok Build defaults to chat_completions (/v1/chat/completions).
# Keep api_backend = "responses" on every model entry.
#
# Prefer env_key over hardcoding api_key (never commit secrets).
# Also export GROK_MODELS_BASE_URL + XAI_API_KEY in the shell block above.

# Global inference / catalog endpoints (same role as env GROK_MODELS_BASE_URL).
# When models_base_url is set, Grok uses API-key Bearer auth (no grok login required).
[endpoints]
models_base_url = "${baseUrl}"              # inference base; model list defaults to {base}/models
models_list_url = "${modelsListUrl}"        # optional override (env: GROK_MODELS_LIST_URL)
xai_api_base_url = "${baseUrl}"             # public xAI API base override for gateway routing
cli_chat_proxy_base_url = "${baseUrl}"      # CLI chat-proxy base (env: GROK_CLI_CHAT_PROXY_BASE_URL)

# Prefer API key when using a custom gateway (matches Sub2API).
# Requires XAI_API_KEY env or per-model env_key / api_key.
[auth]
preferred_method = "api_key"

[model."grok-4.5"]
model = "grok-4.5"                          # id sent to the API
name = "Grok 4.5"                           # shown in /model picker
description = "Grok 4.5 via Sub2API (Responses)"
# base_url inherits from [endpoints].models_base_url; override only if needed:
# base_url = "${baseUrl}"
env_key = "XAI_API_KEY"                     # or: api_key = "${apiKey}"  (not recommended)
api_backend = "responses"                   # chat_completions | responses | messages
context_window = 500000                     # drives auto-compaction timing
# Optional sampling (global defaults can live under [models] instead):
# temperature = 0.7
# top_p = 0.95
# max_completion_tokens = 8192
# Server-side (backend) web_search tools — only if your gateway exposes them:
supports_backend_search = true

[model."grok-build-0.1"]
model = "grok-build-0.1"
name = "Grok Build"
description = "Coding / agent sessions (xAI recommends grok-build* for coding)"
env_key = "XAI_API_KEY"
api_backend = "responses"
context_window = 256000
supports_backend_search = true

# Text multi-agent / client web_search sub-agent (NOT Imagine image/video).
[model."grok-4.20-multi-agent-0309"]
model = "grok-4.20-multi-agent-0309"
name = "Grok 4.20 Multi Agent (text / web_search)"
description = "Text multi-agent; use for web_search sub-agent, not image/video"
env_key = "XAI_API_KEY"
api_backend = "responses"
context_window = 1000000
supports_backend_search = true

[model."grok-4.3"]
model = "grok-4.3"
name = "Grok 4.3"
env_key = "XAI_API_KEY"
api_backend = "responses"
context_window = 1000000
supports_backend_search = true

# Optional short alias for /model grok:
# [model."grok"]
# model = "grok-4.5"
# name = "Grok"
# env_key = "XAI_API_KEY"
# api_backend = "responses"
# context_window = 1000000
# supports_backend_search = true

[models]
# xAI recommends grok-build* for coding/agent sessions; use grok-4.5 for general chat.
default = "grok-4.5"
web_search = "grok-4.5"                     # client-side web_search tool model (must exist as [model.*])
image_description = "grok-4.5"              # vision/describe-image helper model
# Optional environment-wide sampling defaults (per-model values win):
# temperature = 0.7
# top_p = 0.95
# max_completion_tokens = 8192
# max_retries = 8

[session]
auto_compact_threshold_percent = 80         # auto-compact at this % of context_window (default 85)

# Imagine tools: model IDs go to Sub2API media endpoints (not the text [model.*] catalog).
# Enable only if the Grok group allows image/video generation.
[features]
image_gen = true
video_gen = true
image_gen_model_override = "grok-imagine-image-quality"   # or grok-imagine-image
image_edit_model_override = "grok-imagine-edit"
# Optional feature flags (defaults shown in docs):
# telemetry = false
# remote_fetch = true                         # set false for air-gapped / pure-gateway catalogs
# lsp_tools = false`

  return [
    { path: envPath, content: envContent },
    {
      path: joinConfigPath(configDir, 'config.toml', isWindowsPath),
      content: configContent,
      hint: t('keys.useKeyModal.grok.configTomlHint')
    }
  ]
}

function generateGrokCodexFiles(baseUrl: string, apiKey: string): FileConfig[] {
  // Codex config reference: wire_api = "responses" only; prefer env_key over experimental_bearer_token.
  // Non-OpenAI gateways should set supports_websockets = false (HTTP/SSE).
  const shell = activeTab.value
  const isWindowsPath = shell === 'windows' || shell === 'cmd' || shell === 'powershell'
  const configDir = isWindowsPath ? '%userprofile%\\.codex' : '~/.codex'
  const model = selectCodexCatalogModel('grok-4.5')

  let envPath: string
  let envContent: string
  switch (shell) {
    case 'cmd':
      envPath = 'Command Prompt'
      envContent = `set SUB2API_API_KEY=${apiKey}`
      break
    case 'powershell':
    case 'windows':
      envPath = 'PowerShell'
      envContent = `$env:SUB2API_API_KEY="${apiKey}"`
      break
    default:
      envPath = 'Terminal'
      envContent = `export SUB2API_API_KEY="${apiKey}"`
  }

  const configContent = `# Codex CLI → Sub2API Grok group
# Docs: Codex config reference (model_providers.*, wire_api = "responses")
#
# Text models only. Image/video: grok-imagine-image / grok-imagine-video on media endpoints.
# Switch model: grok-4.5 | grok-4.3 | grok-build-0.1 | grok-4.20-multi-agent-0309 (text / web_search)

model_provider = "sub2api"
model = "${model}"
${codexLocalCatalogToml.value}# Optional:
# review_model = "${model}"
# model_reasoning_effort = "medium"
# model_context_window = 500000
# disable_response_storage = true
# network_access = "enabled"
# windows_wsl_setup_acknowledged = true

[model_providers.sub2api]
name = "Sub2API Grok"
base_url = "${baseUrl}"
${codexModelCatalogMode.value === 'remote' ? `model_catalog_url = "${escapeTomlBasicString(buildCodexModelCatalogUrl(baseUrl))}"\n` : ''}# Prefer env_key (variable NAME). Do not combine with experimental_bearer_token.
env_key = "SUB2API_API_KEY"
# Fallback only if you cannot set env (discouraged — keeps secret on disk):
# experimental_bearer_token = "${apiKey}"
wire_api = "responses"
# API-key providers: do not require ChatGPT OAuth login
requires_openai_auth = false
# Grok/Sub2API path is HTTP/SSE; disable WS (Codex may otherwise try WebSocket first)
supports_websockets = false

${codexModelCatalogMode.value === 'remote' ? '[features]\napi_key_model_discovery = true\n\n# Optional:' : '# Optional:\n# [features]'}
# goals = true`

  return [
    { path: envPath, content: envContent },
    {
      path: joinConfigPath(configDir, 'config.toml', isWindowsPath),
      content: configContent,
      hint: t('keys.useKeyModal.grok.codexConfigTomlHint')
    }
  ]
}

function generateRoutedCodexFiles(
  baseUrl: string,
  apiKey: string,
  platform: GroupPlatform
): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'
  const preferredModels: Partial<Record<GroupPlatform, string>> = {
    openai: 'gpt-5.5',
    anthropic: 'claude-sonnet-4-6',
    gemini: 'gemini-2.5-pro',
    antigravity: 'claude-sonnet-4-6',
    grok: 'grok-4.5',
    kimi: 'kimi-k2.5',
    zhipu: 'glm-4.7',
    deepseek: 'deepseek-v4-pro',
    minimax: 'MiniMax-M3',
    opencode_go: 'glm-5.3',
    composite: 'gpt-5.5'
  }
  const preferredModel = preferredModels[platform] || ''
  const model = selectCodexCatalogModel(preferredModel)
  const labels: Record<GroupPlatform, string> = {
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    gemini: 'Gemini',
    antigravity: 'Antigravity',
    grok: 'Grok',
    kimi: 'Kimi',
    zhipu: 'Zhipu',
    deepseek: 'DeepSeek',
    minimax: 'MiniMax',
    opencode_go: 'OpenCode',
    typesafe: 'TypeSafe / Jev',
    command_code: 'Command Code',
    cline: 'Cline',
    composite: 'Composite',
    kiro: 'Kiro'
  }
  const label = labels[platform]
  const envContent = isWindows
    ? `$env:SUB2API_API_KEY="${apiKey}"`
    : `export SUB2API_API_KEY="${apiKey}"`

  const configContent = `# Codex CLI -> Sub2API ${label} group
model_provider = "sub2api"
model = "${model}"
review_model = "${model}"
disable_response_storage = true
${codexLocalCatalogToml.value}
[model_providers.sub2api]
name = "Sub2API ${label}"
base_url = "${baseUrl}"
${codexModelCatalogMode.value === 'remote' ? `model_catalog_url = "${escapeTomlBasicString(buildCodexModelCatalogUrl(baseUrl))}"\n` : ''}env_key = "SUB2API_API_KEY"
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false${codexModelCatalogMode.value === 'remote' ? '\n\n[features]\napi_key_model_discovery = true' : ''}`

  return [
    { path: isWindows ? 'PowerShell' : 'Terminal', content: envContent },
    {
      path: joinConfigPath(configDir, 'config.toml', isWindows),
      content: configContent,
      hint: t(
        platform === 'deepseek' || platform === 'minimax' || platform === 'composite'
          ? `keys.useKeyModal.${platform}.codexConfigTomlHint`
          : 'keys.useKeyModal.routedCodex.configTomlHint'
      )
    }
  ]
}

function generateOpenAIWsFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'
  const model = selectCodexCatalogModel('gpt-5.5')
  const reasoningEffortLine = codexReasoningEffortTomlLine(model)

  // config.toml content with WebSocket v2
  const configContent = `model_provider = "OpenAI"
model = "${model}"
review_model = "${model}"
${reasoningEffortLine}disable_response_storage = true
${codexLocalCatalogToml.value}network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
${codexModelCatalogMode.value === 'remote' ? `model_catalog_url = "${escapeTomlBasicString(buildCodexModelCatalogUrl(baseUrl))}"\n` : ''}wire_api = "responses"
supports_websockets = true
${generateCodexProviderAuthConfig(apiKey)}

[features]
${codexModelCatalogMode.value === 'remote' ? 'api_key_model_discovery = true\n' : ''}responses_websockets_v2 = true
goals = true`

  return buildOpenAICodexFileConfigs(configDir, configContent, apiKey)
}

function generateOpenCodeConfig(platform: string, baseUrl: string, apiKey: string, pathLabel?: string): FileConfig {
  const provider: Record<string, any> = {
    [platform]: {
      options: {
        baseURL: baseUrl,
        apiKey
      }
    }
  }
  const openaiModels = {
    'gpt-6': {
      name: 'GPT-6 (Astra)',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-6-astra': {
      name: 'GPT-6 Astra',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.2': {
      name: 'GPT-5.2',
      limit: {
        context: 400000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.6': {
      name: 'GPT-5.6 (Sol)',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-6.1-terra': {
      name: 'GPT-6.1 Terra',
      limit: { context: 1050000, output: 128000 },
      options: { store: false },
      variants: { none: {}, low: {}, medium: {}, high: {}, xhigh: {}, max: {} }
    },
    'gpt-6.1-sol': {
      name: 'GPT-6.1 Sol',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-6-sol': {
      name: 'GPT-6 Sol',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        none: {},
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-sol': {
      name: 'GPT-5.6 Sol',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-terra': {
      name: 'GPT-5.6 Terra',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-6-luna': {
      name: 'GPT-6 Luna',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        none: {},
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-luna': {
      name: 'GPT-5.6 Luna',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.5': {
      name: 'GPT-5.5',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.4': {
      name: 'GPT-5.4',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.4-mini': {
      name: 'GPT-5.4 Mini',
      limit: {
        context: 400000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.3-codex-spark': {
      name: 'GPT-5.3 Codex Spark',
      limit: {
        context: 128000,
        output: 32000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'codex-mini-latest': {
      name: 'Codex Mini',
      limit: {
        context: 200000,
        output: 100000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {}
      }
    }
  }
  const geminiModels = {
    'gemini-2.0-flash': {
      name: 'Gemini 2.0 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-2.5-pro': {
      name: 'Gemini 2.5 Pro',
      limit: {
        context: 2097152,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.5-flash': {
      name: 'Gemini 3.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-3-flash-preview': {
      name: 'Gemini 3 Flash Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-3-pro-preview': {
      name: 'Gemini 3 Pro Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-preview': {
      name: 'Gemini 3.1 Pro Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }

  const antigravityGeminiModels = {
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'disable'
        }
      }
    },
    'gemini-2.5-flash-lite': {
      name: 'Gemini 2.5 Flash Lite',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-2.5-flash-thinking': {
      name: 'Gemini 2.5 Flash (Thinking)',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3-flash': {
      name: 'Gemini 3 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-low': {
      name: 'Gemini 3.1 Pro Low',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-high': {
      name: 'Gemini 3.1 Pro High',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-2.5-flash-image': {
      name: 'Gemini 2.5 Flash Image',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image'],
        output: ['image']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-flash-image': {
      name: 'Gemini 3.1 Flash Image',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image'],
        output: ['image']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }
  const claudeModels = {
    'claude-fable-5-1': {
      name: 'Claude Fable 5.1',
      limit: {
        context: 1048576,
        output: 128000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          type: 'adaptive'
        }
      }
    },
    'claude-fable-5': {
      name: 'Claude Fable 5',
      limit: {
        context: 1048576,
        output: 128000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          type: 'adaptive'
        }
      }
    },
    'claude-opus-4-6-thinking': {
      name: 'Claude 4.6 Opus (Thinking)',
      limit: {
        context: 200000,
        output: 128000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'claude-sonnet-4-6': {
      name: 'Claude 4.6 Sonnet',
      limit: {
        context: 200000,
        output: 64000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }
  // Align context_window with Grok Build official sample (docs.x.ai/build/settings) where known.
  // Image/video: grok-imagine-image / grok-imagine-video on media endpoints — not this list.
  const grokModels = {
    'grok-4.5': {
      name: 'Grok 4.5',
      limit: { context: 500000, output: 64000 }
    },
    'grok-build-0.1': {
      name: 'Grok Build 0.1',
      limit: { context: 256000, output: 64000 }
    },
    'grok-4.20-multi-agent-0309': {
      name: 'Grok 4.20 Multi Agent (text / web_search)',
      limit: { context: 1000000, output: 64000 }
    },
    'grok-4.3': {
      name: 'Grok 4.3',
      limit: { context: 1000000, output: 64000 }
    },
    'grok-composer-2.5-fast': {
      name: 'Grok Composer 2.5 Fast',
      limit: { context: 500000, output: 64000 }
    }
  }

  if (platform === 'gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].models = geminiModels
  } else if (platform === 'anthropic') {
    provider[platform].npm = '@ai-sdk/anthropic'
    provider[platform].models = {
      'claude-opus-5-5': {
        name: 'Claude Opus 5.5',
        limit: { context: 1000000, output: 128000 },
        modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
        options: { thinking: { type: 'adaptive' }, effort: 'medium' },
        variants: {
          low: { effort: 'low' },
          medium: { effort: 'medium' },
          high: { effort: 'high' },
          xhigh: { effort: 'xhigh' },
          max: { effort: 'max' }
        }
      },
      'claude-sonnet-5-5': {
        name: 'Claude Sonnet 5.5',
        limit: { context: 1000000, output: 128000 },
        modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
        options: { thinking: { type: 'adaptive' }, effort: 'high' },
        variants: {
          low: { effort: 'low' },
          medium: { effort: 'medium' },
          high: { effort: 'high' },
          xhigh: { effort: 'xhigh' },
          max: { effort: 'max' }
        }
      }
    }
  } else if (platform === 'antigravity-claude') {
    provider[platform].npm = '@ai-sdk/anthropic'
    provider[platform].name = 'Antigravity (Claude)'
    provider[platform].models = claudeModels
  } else if (platform === 'antigravity-gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].name = 'Antigravity (Gemini)'
    provider[platform].models = antigravityGeminiModels
  } else if (platform === 'openai') {
    provider[platform].models = openaiModels
  } else if (platform === 'grok') {
    // Custom provider pointing at Sub2API OpenAI-compatible Responses/Chat endpoints.
    provider[platform].npm = '@ai-sdk/openai-compatible'
    provider[platform].name = 'Grok via Sub2API'
    provider[platform].models = grokModels
  }

  const agent =
    platform === 'openai'
      ? {
          build: {
            options: {
              store: false
            }
          },
          plan: {
            options: {
              store: false
            }
          }
        }
      : undefined

  const content = JSON.stringify(
    {
      provider,
      ...(agent ? { agent } : {}),
      $schema: 'https://opencode.ai/config.json'
    },
    null,
    2
  )

  return {
    path: pathLabel ?? 'opencode.json',
    content,
    hint: t('keys.useKeyModal.opencode.hint')
  }
}

const copyContent = async (content: string, index: number) => {
  const success = await clipboardCopy(content, t('keys.copied'))
  if (success) {
    copiedIndex.value = index
    setTimeout(() => {
      copiedIndex.value = null
    }, 2000)
  }
}
</script>
