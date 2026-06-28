<template>
  <section class="rounded-xl border border-gray-200 bg-gray-50/80 p-4 dark:border-dark-700 dark:bg-dark-800/60">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <label class="text-sm font-semibold text-gray-900 dark:text-white">
            Platform profile registry / guide metadata
          </label>
          <span
            class="rounded-full px-2.5 py-1 text-xs font-medium"
            :class="statusBadgeClass"
            data-test="registry-status"
          >
            {{ statusLabel }}
          </span>
        </div>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          Visual editor này vẫn lưu về đúng JSON string trong settings DB. Operator chỉnh theo platform/client/copy block;
          Raw JSON chỉ dùng cho import/export nâng cao.
        </p>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled || !canFormat"
          @click="formatJSON"
        >
          <Icon name="document" size="xs" />
          Format JSON
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled || !canEditVisual"
          data-test="add-platform-button"
          @click="insertPlatformTemplate"
        >
          <Icon name="plus" size="xs" />
          Add platform profile
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled"
          @click="resetToDefaultOnSave"
        >
          <Icon name="refresh" size="xs" />
          Reset mặc định 3 platform khi lưu
        </button>
      </div>
    </div>

    <div
      v-if="validation.errors.length"
      class="mt-4 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300"
      data-test="registry-errors"
    >
      <div class="font-semibold">Không thể lưu registry hiện tại:</div>
      <ul class="mt-1 list-disc space-y-1 pl-5">
        <li v-for="error in validation.errors" :key="error">{{ error }}</li>
      </ul>
    </div>

    <div
      v-if="validation.warnings.length"
      class="mt-4 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-300"
      data-test="registry-warnings"
    >
      <div class="font-semibold">Lưu ý:</div>
      <ul class="mt-1 list-disc space-y-1 pl-5">
        <li v-for="warning in validation.warnings" :key="warning">{{ warning }}</li>
      </ul>
    </div>

    <div
      v-if="validation.isEmpty"
      class="mt-4 rounded-lg border border-dashed border-amber-300 bg-white p-4 text-sm text-amber-800 dark:border-amber-800 dark:bg-dark-900/60 dark:text-amber-200"
    >
      Registry đang để trống. Nếu bấm Lưu ngay, backend sẽ insert default registry OpenAI, Anthropic, Gemini.
      Nếu muốn override trước khi lưu, bấm <span class="font-semibold">Add platform profile</span> để tạo JSON có cấu trúc.
    </div>

    <div
      v-if="canEditVisual"
      class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-[18rem_minmax(0,1fr)]"
      data-test="registry-visual-editor"
    >
      <aside class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/60">
        <div class="border-b border-gray-100 p-3 dark:border-dark-700">
          <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Platforms
          </div>
          <dl class="mt-3 grid grid-cols-3 gap-2 text-center">
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Platforms</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ profileCount }}</dd>
            </div>
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Clients</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ clientCount }}</dd>
            </div>
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Blocks</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ copyBlockCount }}</dd>
            </div>
          </dl>
        </div>

        <div v-if="profiles.length" class="max-h-[46rem] space-y-2 overflow-y-auto p-3">
          <button
            v-for="profile in profiles"
            :key="profileKey(profile)"
            type="button"
            class="w-full rounded-lg border p-3 text-left transition-colors"
            :class="
              selectedPlatform === normalizedProfilePlatform(profile)
                ? 'border-primary-300 bg-primary-50 text-primary-900 dark:border-primary-700 dark:bg-primary-950/30 dark:text-primary-100'
                : 'border-gray-200 bg-white text-gray-700 hover:border-primary-200 hover:bg-primary-50/50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-primary-700 dark:hover:bg-primary-950/20'
            "
            data-test="platform-card"
            @click="selectPlatform(normalizedProfilePlatform(profile))"
          >
            <div class="flex items-start justify-between gap-2">
              <div>
                <div class="font-mono text-sm font-semibold">
                  {{ normalizedProfilePlatform(profile) || 'missing-platform' }}
                </div>
                <div class="mt-1 text-xs opacity-75">
                  {{ profile.provider_name || profile.provider_id || 'Provider chưa đặt tên' }}
                </div>
              </div>
              <span class="rounded-full bg-white/80 px-2 py-0.5 text-[11px] font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-300">
                {{ profile.guide?.profile_id || profile.platform || 'profile' }}
              </span>
            </div>
            <div class="mt-2 flex flex-wrap gap-1 text-[11px]">
              <span class="rounded-full bg-gray-100 px-2 py-0.5 dark:bg-dark-800">
                {{ profileClients(profile).length }} clients
              </span>
              <span class="rounded-full bg-gray-100 px-2 py-0.5 dark:bg-dark-800">
                {{ profileBlocks(profile).length }} blocks
              </span>
            </div>
          </button>
        </div>

        <div v-else class="p-4 text-sm text-gray-500 dark:text-gray-400">
          Chưa có platform profile trong JSON hiện tại.
        </div>
      </aside>

      <div v-if="selectedProfile" class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="flex flex-col gap-3 border-b border-gray-100 pb-3 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <div class="text-sm font-semibold text-gray-900 dark:text-white">
                Platform details
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                Các field này map trực tiếp vào profile metadata mà Bot-Sales/User modal consume.
              </p>
            </div>
            <button
              type="button"
              class="btn btn-danger btn-sm"
              :disabled="disabled"
              data-test="remove-platform-button"
              @click="removeSelectedPlatform"
            >
              Remove platform
            </button>
          </div>

          <div class="mt-4 rounded-lg border border-primary-100 bg-primary-50/60 p-3 text-sm dark:border-primary-900/40 dark:bg-primary-950/20">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0">
                <div class="font-semibold text-gray-900 dark:text-white">
                  {{ selectedProfile.guide?.title || 'Guide chưa có title' }}
                </div>
                <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-300">
                  {{ selectedProfile.guide?.description || 'Guide chưa có description' }}
                </p>
                <p
                  v-if="selectedProfile.guide?.note"
                  class="mt-2 text-xs font-medium text-primary-800 dark:text-primary-200"
                >
                  {{ selectedProfile.guide.note }}
                </p>
              </div>
              <a
                v-if="selectedProfile.guide?.docs_url"
                class="shrink-0 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                :href="selectedProfile.guide.docs_url"
                target="_blank"
                rel="noopener noreferrer"
              >
                Docs
              </a>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label class="input-label" for="registry-platform-id">Platform ID</label>
              <input
                id="registry-platform-id"
                :value="selectedProfile.platform || ''"
                type="text"
                class="input font-mono text-sm"
                :disabled="disabled"
                data-test="platform-id-input"
                @input="updateSelectedProfileField('platform', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-provider-id">Provider ID</label>
              <input
                id="registry-provider-id"
                :value="selectedProfile.provider_id || ''"
                type="text"
                class="input font-mono text-sm"
                :disabled="disabled"
                @input="updateSelectedProfileField('provider_id', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-provider-name">Provider name</label>
              <input
                id="registry-provider-name"
                :value="selectedProfile.provider_name || ''"
                type="text"
                class="input"
                :disabled="disabled"
                @input="updateSelectedProfileField('provider_name', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-api-style">API style</label>
              <input
                id="registry-api-style"
                :value="selectedProfile.api_style || ''"
                type="text"
                class="input font-mono text-sm"
                :disabled="disabled"
                @input="updateSelectedProfileField('api_style', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-guide-profile-id">Guide profile ID</label>
              <input
                id="registry-guide-profile-id"
                :value="selectedProfile.guide?.profile_id || ''"
                type="text"
                class="input font-mono text-sm"
                :disabled="disabled"
                @input="updateSelectedGuideField('profile_id', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-default-client">Default client</label>
              <select
                id="registry-default-client"
                :value="selectedProfile.guide?.default_client || ''"
                class="input"
                :disabled="disabled"
                data-test="default-client-select"
                @change="updateSelectedGuideField('default_client', inputValue($event))"
              >
                <option value="">Chưa chọn</option>
                <option
                  v-for="client in selectedProfileClients"
                  :key="client.id"
                  :value="client.id"
                >
                  {{ client.label || client.id }}
                </option>
              </select>
            </div>
            <div class="md:col-span-2">
              <label class="input-label" for="registry-guide-title">Guide title</label>
              <input
                id="registry-guide-title"
                :value="selectedProfile.guide?.title || ''"
                type="text"
                class="input"
                :disabled="disabled"
                data-test="guide-title-input"
                @input="updateSelectedGuideField('title', inputValue($event))"
              />
            </div>
            <div class="md:col-span-2">
              <label class="input-label" for="registry-guide-description">Description</label>
              <textarea
                id="registry-guide-description"
                :value="selectedProfile.guide?.description || ''"
                rows="3"
                class="input resize-y text-sm leading-6"
                :disabled="disabled"
                @input="updateSelectedGuideField('description', inputValue($event))"
              ></textarea>
            </div>
            <div>
              <label class="input-label" for="registry-docs-url">Docs URL</label>
              <input
                id="registry-docs-url"
                :value="selectedProfile.guide?.docs_url || ''"
                type="url"
                class="input"
                :disabled="disabled"
                @input="updateSelectedGuideField('docs_url', inputValue($event))"
              />
            </div>
            <div>
              <label class="input-label" for="registry-guide-note">Operator/customer note</label>
              <input
                id="registry-guide-note"
                :value="selectedProfile.guide?.note || ''"
                type="text"
                class="input"
                :disabled="disabled"
                @input="updateSelectedGuideField('note', inputValue($event))"
              />
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 pb-3 dark:border-dark-700">
            <div>
              <div class="text-sm font-semibold text-gray-900 dark:text-white">Clients</div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                Mỗi client là một tab/target ở user modal. OS chips giúp operator hiểu block nào dành cho môi trường nào.
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="disabled"
              data-test="add-client-button"
              @click="addClientToSelectedProfile"
            >
              <Icon name="plus" size="xs" />
              Add client
            </button>
          </div>

          <div class="mt-4 space-y-3">
            <article
              v-for="(client, clientIndex) in selectedProfileClients"
              :key="client.id || clientIndex"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70"
              data-test="client-row"
            >
              <div class="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,0.75fr)_minmax(0,1fr)_auto]">
                <div>
                  <label class="input-label" :for="`registry-client-id-${clientIndex}`">Client ID</label>
                  <input
                    :id="`registry-client-id-${clientIndex}`"
                    :value="client.id || ''"
                    type="text"
                    class="input font-mono text-sm"
                    :disabled="disabled"
                    data-test="client-id-input"
                    @input="updateClientField(clientIndex, 'id', inputValue($event))"
                  />
                </div>
                <div>
                  <label class="input-label" :for="`registry-client-label-${clientIndex}`">Label</label>
                  <input
                    :id="`registry-client-label-${clientIndex}`"
                    :value="client.label || ''"
                    type="text"
                    class="input"
                    :disabled="disabled"
                    data-test="client-label-input"
                    @input="updateClientField(clientIndex, 'label', inputValue($event))"
                  />
                </div>
                <div class="flex items-end">
                  <button
                    type="button"
                    class="btn btn-danger btn-sm"
                    :disabled="disabled"
                    @click="removeClientFromSelectedProfile(clientIndex)"
                  >
                    Remove
                  </button>
                </div>
              </div>
              <div class="mt-3 flex flex-wrap gap-2">
                <label
                  v-for="osOption in osOptions"
                  :key="osOption"
                  class="inline-flex items-center gap-2 rounded-full border border-gray-200 bg-white px-3 py-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200"
                >
                  <input
                    type="checkbox"
                    class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    :checked="client.os?.includes(osOption)"
                    :disabled="disabled"
                    @change="toggleClientOS(clientIndex, osOption)"
                  />
                  {{ osOption }}
                </label>
              </div>
            </article>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 pb-3 dark:border-dark-700">
            <div>
              <div class="text-sm font-semibold text-gray-900 dark:text-white">Copy blocks</div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                Nội dung template là text thật, không cần escape JSON newline/quote như raw JSON.
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="disabled || !selectedProfileClients.length"
              data-test="add-copy-block-button"
              @click="addCopyBlockToSelectedProfile"
            >
              <Icon name="plus" size="xs" />
              Add copy block
            </button>
          </div>

          <div
            v-if="selectedProfileBlocks.length"
            class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-[minmax(14rem,0.38fr)_minmax(0,1fr)]"
            data-test="copy-block-workspace"
          >
            <div class="rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800/50">
              <div class="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                Blocks trong platform
              </div>
              <div class="max-h-[32rem] space-y-2 overflow-y-auto pr-1">
                <button
                  v-for="block in selectedProfileBlocks"
                  :key="block.id || `${block.client_id}-${block.path}`"
                  type="button"
                  class="w-full rounded-lg border p-3 text-left transition-colors"
                  :class="
                    selectedBlockId === block.id
                      ? 'border-primary-300 bg-white text-primary-900 shadow-sm ring-1 ring-primary-100 dark:border-primary-700 dark:bg-primary-950/30 dark:text-primary-100 dark:ring-primary-900/40'
                      : 'border-gray-200 bg-white text-gray-700 hover:border-primary-200 hover:bg-primary-50/50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-primary-700 dark:hover:bg-primary-950/20'
                  "
                  data-test="copy-block-tab"
                  @click="selectedBlockId = block.id || ''"
                >
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0">
                      <div class="truncate font-mono text-xs font-semibold">{{ block.id || 'missing-id' }}</div>
                      <div class="mt-1 text-xs opacity-75">{{ block.client_id || 'no-client' }} · {{ block.os || 'all OS' }}</div>
                    </div>
                    <span
                      v-if="block.language"
                      class="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-[10px] uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-300"
                    >
                      {{ block.language }}
                    </span>
                  </div>
                  <div class="mt-2 truncate text-[11px] opacity-70">{{ block.path || 'Chưa có target' }}</div>
                </button>
              </div>
            </div>

            <div
              v-if="selectedBlock"
              class="space-y-4 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/70"
              data-test="copy-block-editor"
            >
              <div class="flex flex-col gap-3 border-b border-gray-100 pb-3 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0">
                  <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Editing copy block
                  </div>
                  <div class="mt-1 truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">
                    {{ selectedBlock.id || 'missing-id' }}
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                    Chỉnh target và template dạng text thật. Preview phía dưới dùng placeholder mẫu, không dùng API key thật.
                  </p>
                </div>
                <button
                  type="button"
                  class="btn btn-danger btn-sm shrink-0"
                  :disabled="disabled"
                  @click="removeSelectedCopyBlock"
                >
                  Remove block
                </button>
              </div>

              <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div class="md:col-span-2 xl:col-span-2">
                  <label class="input-label" for="registry-block-id">Block ID</label>
                  <input
                    id="registry-block-id"
                    :value="selectedBlock.id || ''"
                    type="text"
                    class="input font-mono text-sm"
                    :disabled="disabled"
                    data-test="block-id-input"
                    @input="updateSelectedBlockField('id', inputValue($event))"
                  />
                </div>
                <div>
                  <label class="input-label" for="registry-block-client">Client</label>
                  <select
                    id="registry-block-client"
                    :value="selectedBlock.client_id || ''"
                    class="input"
                    :disabled="disabled"
                    data-test="block-client-select"
                    @change="updateSelectedBlockField('client_id', inputValue($event))"
                  >
                    <option
                      v-for="client in selectedProfileClients"
                      :key="client.id"
                      :value="client.id"
                    >
                      {{ client.label || client.id }}
                    </option>
                  </select>
                </div>
                <div>
                  <label class="input-label" for="registry-block-os">OS</label>
                  <select
                    id="registry-block-os"
                    :value="selectedBlock.os || ''"
                    class="input"
                    :disabled="disabled"
                    @change="updateSelectedBlockField('os', inputValue($event))"
                  >
                    <option value="">All / unspecified</option>
                    <option v-for="osOption in osOptions" :key="osOption" :value="osOption">
                      {{ osOption }}
                    </option>
                  </select>
                </div>
                <div>
                  <label class="input-label" for="registry-block-language">Language</label>
                  <select
                    id="registry-block-language"
                    :value="selectedBlock.language || ''"
                    class="input"
                    :disabled="disabled"
                    @change="updateSelectedBlockField('language', inputValue($event))"
                  >
                    <option value="">Unspecified</option>
                    <option
                      v-for="languageOption in languageOptions"
                      :key="languageOption"
                      :value="languageOption"
                    >
                      {{ languageOption }}
                    </option>
                  </select>
                </div>
                <div class="md:col-span-2 xl:col-span-3">
                  <label class="input-label" for="registry-block-path">Target path / nơi paste</label>
                  <input
                    id="registry-block-path"
                    :value="selectedBlock.path || ''"
                    type="text"
                    class="input font-mono text-sm"
                    :disabled="disabled"
                    @input="updateSelectedBlockField('path', inputValue($event))"
                  />
                </div>
                <div class="md:col-span-2 xl:col-span-4">
                  <label class="input-label" for="registry-block-hint">Hint cho operator/customer</label>
                  <input
                    id="registry-block-hint"
                    :value="selectedBlock.hint || ''"
                    type="text"
                    class="input"
                    :disabled="disabled"
                    @input="updateSelectedBlockField('hint', inputValue($event))"
                  />
                </div>
              </div>

              <div>
                <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <label class="input-label mb-0" for="registry-block-template">Content template</label>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      Editor chiếm full width để đọc/sửa command nhiều dòng, không còn phải edit JSON escape.
                    </p>
                  </div>
                  <span class="rounded-full bg-gray-100 px-2.5 py-1 text-[11px] font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-300">
                    {{ selectedBlock.language || 'text' }}
                  </span>
                </div>
                <textarea
                  id="registry-block-template"
                  :value="selectedBlock.content_template || ''"
                  rows="14"
                  class="input min-h-[18rem] w-full resize-y whitespace-pre font-mono text-xs leading-5"
                  :disabled="disabled"
                  data-test="block-template-textarea"
                  @input="updateSelectedBlockField('content_template', rawInputValue($event))"
                ></textarea>
              </div>

              <div class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(0,0.95fr)_minmax(20rem,1.05fr)]">
                <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/60">
                  <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Placeholder picker
                  </div>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    Bấm để append vào cuối template đang chọn.
                  </p>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <button
                      v-for="placeholder in placeholders"
                      :key="placeholder"
                      type="button"
                      class="rounded-full border border-gray-200 bg-white px-3 py-1 font-mono text-xs text-gray-700 transition-colors hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-primary-500 dark:hover:text-primary-300"
                      :disabled="disabled"
                      data-test="placeholder-button"
                      @click="appendPlaceholderToSelectedBlock(placeholder)"
                    >
                      {{ placeholder }}
                    </button>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/60">
                  <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Render preview
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
                    <code class="font-semibold text-gray-900 dark:text-white">{{ selectedBlock.path || 'Target chưa đặt' }}</code>
                    <span class="text-gray-400">{{ selectedBlock.client_id || 'no-client' }}</span>
                    <span v-if="selectedBlock.os" class="text-gray-400">{{ selectedBlock.os }}</span>
                  </div>
                  <pre class="mt-3 max-h-80 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-950 p-3 text-[11px] leading-5 text-gray-100">{{ renderedSelectedBlock }}</pre>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="mt-4 rounded-lg border border-dashed border-gray-300 p-4 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            Chưa có copy block cho platform này.
          </div>
        </div>
      </div>

    </div>

    <div class="mt-4 rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/60">
      <button
        type="button"
        class="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold text-gray-900 dark:text-white"
        data-test="advanced-json-toggle"
        @click="rawJsonOpen = !rawJsonOpen"
      >
        <span>Advanced JSON import/export</span>
        <span class="text-xs text-gray-500 dark:text-gray-400">
          {{ rawJsonOpen || !validation.ok ? 'Ẩn' : 'Mở' }} raw JSON
        </span>
      </button>

      <div v-if="rawJsonOpen || !validation.ok" class="border-t border-gray-100 p-4 dark:border-dark-700">
        <textarea
          id="platform-profile-registry"
          :value="modelValue"
          rows="14"
          class="input w-full resize-y font-mono text-xs leading-5"
          :class="validation.ok ? '' : 'input-error ring-2 ring-red-500/20'"
          spellcheck="false"
          :disabled="disabled"
          placeholder='{"version":1,"profiles":[...]}'
          data-test="raw-json-textarea"
          @input="handleInput"
        />
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          Dùng tab này khi cần paste/export toàn bộ registry. Visual editor phía trên sẽ tự cập nhật sau khi JSON hợp lệ.
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, shallowRef, watch } from "vue";
import Icon from "@/components/icons/Icon.vue";
import {
  appendPlatformProfileTemplate,
  clonePlatformProfileRegistry,
  createEmptyPlatformProfileRegistry,
  formatPlatformProfileRegistryInput,
  platformProfileRegistryLanguageOptions,
  platformProfileRegistryOSOptions,
  platformProfileRegistryPlaceholderSampleValues,
  platformProfileRegistryPlaceholders,
  renderPlatformProfileGuideTemplate,
  validatePlatformProfileRegistryInput,
} from "./platformProfileRegistry";
import type {
  PlatformGuideClient,
  PlatformGuideCopyBlock,
  PlatformGuideMetadata,
  PlatformProfile,
  PlatformProfileRegistry,
} from "./platformProfileRegistry";

interface Props {
  modelValue: string;
  disabled?: boolean;
}

type PlatformProfileStringField = "platform" | "provider_id" | "provider_name" | "api_style";
type PlatformGuideStringField = keyof Pick<
  PlatformGuideMetadata,
  "profile_id" | "title" | "description" | "note" | "docs_url" | "default_client"
>;
type PlatformGuideClientStringField = keyof Pick<PlatformGuideClient, "id" | "label">;
type PlatformGuideCopyBlockStringField = keyof Pick<
  PlatformGuideCopyBlock,
  "id" | "client_id" | "os" | "path" | "hint" | "language" | "content_template"
>;

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
});

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const selectedPlatform = shallowRef("");
const selectedBlockId = shallowRef("");
const rawJsonOpen = shallowRef(false);

const validation = computed(() =>
  validatePlatformProfileRegistryInput(props.modelValue || ""),
);

const editableRegistry = computed<PlatformProfileRegistry | null>(() => {
  if (validation.value.registry) return validation.value.registry;
  if (validation.value.isEmpty) return createEmptyPlatformProfileRegistry();
  return null;
});

const profiles = computed<PlatformProfile[]>(() => editableRegistry.value?.profiles || []);
const placeholders = platformProfileRegistryPlaceholders;
const osOptions = platformProfileRegistryOSOptions;
const languageOptions = platformProfileRegistryLanguageOptions;

const profileCount = computed(() => profiles.value.length);
const clientCount = computed(() =>
  profiles.value.reduce((total, profile) => total + profileClients(profile).length, 0),
);
const copyBlockCount = computed(() =>
  profiles.value.reduce((total, profile) => total + profileBlocks(profile).length, 0),
);

const canFormat = computed(() => validation.value.ok && !validation.value.isEmpty);
const canEditVisual = computed(() => Boolean(editableRegistry.value));

const selectedProfileIndex = computed(() =>
  profiles.value.findIndex(
    (profile) => normalizedProfilePlatform(profile) === selectedPlatform.value,
  ),
);
const selectedProfile = computed<PlatformProfile | null>(() => {
  const index = selectedProfileIndex.value;
  return index >= 0 ? profiles.value[index] : profiles.value[0] || null;
});
const selectedProfileClients = computed(() =>
  selectedProfile.value ? profileClients(selectedProfile.value) : [],
);
const selectedProfileBlocks = computed(() =>
  selectedProfile.value ? profileBlocks(selectedProfile.value) : [],
);
const selectedBlockIndex = computed(() =>
  selectedProfileBlocks.value.findIndex((block) => block.id === selectedBlockId.value),
);
const selectedBlock = computed<PlatformGuideCopyBlock | null>(() => {
  const index = selectedBlockIndex.value;
  return index >= 0 ? selectedProfileBlocks.value[index] : selectedProfileBlocks.value[0] || null;
});
const renderedSelectedBlock = computed(() =>
  selectedBlock.value
    ? renderPlatformProfileGuideTemplate(
        selectedBlock.value.content_template || "",
        platformProfileRegistryPlaceholderSampleValues,
      )
    : "",
);

const statusLabel = computed(() => {
  if (validation.value.isEmpty) return "Default sẽ ghi vào DB";
  return validation.value.ok ? "Schema hợp lệ" : "Schema lỗi";
});

const statusBadgeClass = computed(() => {
  if (!validation.value.ok) {
    return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
  }
  if (validation.value.isEmpty) {
    return "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300";
  }
  return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
});

watch(
  profiles,
  (nextProfiles) => {
    if (!nextProfiles.length) {
      selectedPlatform.value = "";
      selectedBlockId.value = "";
      return;
    }
    const selectedStillExists = nextProfiles.some(
      (profile) => normalizedProfilePlatform(profile) === selectedPlatform.value,
    );
    if (!selectedStillExists) {
      selectedPlatform.value = normalizedProfilePlatform(nextProfiles[0]);
    }
  },
  { immediate: true },
);

watch(
  selectedProfileBlocks,
  (blocks) => {
    if (!blocks.length) {
      selectedBlockId.value = "";
      return;
    }
    if (!blocks.some((block) => block.id === selectedBlockId.value)) {
      selectedBlockId.value = blocks[0].id || "";
    }
  },
  { immediate: true },
);

function update(value: string): void {
  emit("update:modelValue", value);
}

function handleInput(event: Event): void {
  update(inputValue(event));
}

function resetToDefaultOnSave(): void {
  update("");
}

function formatJSON(): void {
  const formatted = formatPlatformProfileRegistryInput(props.modelValue || "");
  if (formatted) update(formatted);
}

async function insertPlatformTemplate(): Promise<void> {
  const result = appendPlatformProfileTemplate(props.modelValue || "");
  if (!result) return;
  update(result.value);
  await nextTick();
  selectedPlatform.value = result.platform;
}

function selectPlatform(platform: string): void {
  selectedPlatform.value = platform;
}

function profileClients(profile: PlatformProfile): PlatformGuideClient[] {
  return Array.isArray(profile.guide?.clients) ? profile.guide.clients : [];
}

function profileBlocks(profile: PlatformProfile): PlatformGuideCopyBlock[] {
  return Array.isArray(profile.guide?.copy_blocks) ? profile.guide.copy_blocks : [];
}

function normalizedProfilePlatform(profile: PlatformProfile): string {
  return (profile.platform || profile.guide?.profile_id || "").trim().toLowerCase();
}

function profileKey(profile: PlatformProfile): string {
  return normalizedProfilePlatform(profile) || profile.guide?.title || "missing-platform";
}

function inputValue(event: Event): string {
  const target = event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null;
  return target?.value.trim() ?? "";
}

function rawInputValue(event: Event): string {
  const target = event.target as HTMLTextAreaElement | null;
  return target?.value ?? "";
}

function ensureGuide(profile: PlatformProfile): PlatformGuideMetadata {
  if (!profile.guide) {
    profile.guide = {
      profile_id: profile.platform || "new-platform",
      title: "New platform delivery guide",
      description: "Operator authored guide metadata.",
      clients: [],
      copy_blocks: [],
    };
  }
  profile.guide.clients = Array.isArray(profile.guide.clients) ? profile.guide.clients : [];
  profile.guide.copy_blocks = Array.isArray(profile.guide.copy_blocks) ? profile.guide.copy_blocks : [];
  return profile.guide;
}

function emitRegistryMutation(mutator: (registry: PlatformProfileRegistry) => void): void {
  const source = editableRegistry.value;
  if (!source) return;
  const registry = clonePlatformProfileRegistry(source);
  registry.version = Number(registry.version || 1);
  registry.profiles = Array.isArray(registry.profiles) ? registry.profiles : [];
  mutator(registry);
  update(JSON.stringify(registry, null, 2));
}

function mutateSelectedProfile(mutator: (profile: PlatformProfile) => void): void {
  const platform = selectedPlatform.value;
  emitRegistryMutation((registry) => {
    const profilesList = registry.profiles || [];
    const index = profilesList.findIndex(
      (profile) => normalizedProfilePlatform(profile) === platform,
    );
    if (index < 0) return;
    mutator(profilesList[index]);
  });
}

function updateSelectedProfileField(field: PlatformProfileStringField, value: string): void {
  const previousPlatform = selectedPlatform.value;
  mutateSelectedProfile((profile) => {
    profile[field] = value;
    if (field === "platform") {
      ensureGuide(profile).profile_id ||= value;
    }
  });
  if (field === "platform") {
    selectedPlatform.value = value.toLowerCase() || previousPlatform;
  }
}

function updateSelectedGuideField(field: PlatformGuideStringField, value: string): void {
  mutateSelectedProfile((profile) => {
    ensureGuide(profile)[field] = value;
  });
}

function addClientToSelectedProfile(): void {
  let nextClientID = "new-client";
  mutateSelectedProfile((profile) => {
    const guide = ensureGuide(profile);
    nextClientID = uniqueID(
      `${normalizedProfilePlatform(profile) || "platform"}-client`,
      guide.clients?.map((client) => client.id || "") || [],
    );
    guide.clients?.push({
      id: nextClientID,
      label: `${nextClientID} client`,
      os: ["unix"],
    });
    guide.default_client ||= nextClientID;
  });
}

function updateClientField(
  clientIndex: number,
  field: PlatformGuideClientStringField,
  value: string,
): void {
  mutateSelectedProfile((profile) => {
    const guide = ensureGuide(profile);
    const client = guide.clients?.[clientIndex];
    if (!client) return;
    const previousID = client.id;
    client[field] = value;
    if (field === "id" && previousID && previousID !== value) {
      if (guide.default_client === previousID) guide.default_client = value;
      guide.copy_blocks?.forEach((block) => {
        if (block.client_id === previousID) block.client_id = value;
      });
    }
  });
}

function toggleClientOS(clientIndex: number, os: string): void {
  mutateSelectedProfile((profile) => {
    const client = ensureGuide(profile).clients?.[clientIndex];
    if (!client) return;
    const osList = Array.isArray(client.os) ? [...client.os] : [];
    client.os = osList.includes(os)
      ? osList.filter((item) => item !== os)
      : [...osList, os];
  });
}

function removeClientFromSelectedProfile(clientIndex: number): void {
  mutateSelectedProfile((profile) => {
    const guide = ensureGuide(profile);
    const removedClient = guide.clients?.[clientIndex];
    if (!removedClient) return;
    guide.clients = guide.clients?.filter((_client, index) => index !== clientIndex) || [];
    guide.copy_blocks = guide.copy_blocks?.filter(
      (block) => block.client_id !== removedClient.id,
    ) || [];
    if (guide.default_client === removedClient.id) {
      guide.default_client = guide.clients[0]?.id || "";
    }
  });
}

function addCopyBlockToSelectedProfile(): void {
  let nextBlockID = "new-copy-block";
  mutateSelectedProfile((profile) => {
    const guide = ensureGuide(profile);
    const firstClient = guide.default_client || guide.clients?.[0]?.id || "client";
    nextBlockID = uniqueID(
      `${normalizedProfilePlatform(profile) || "platform"}-${firstClient}-block`,
      guide.copy_blocks?.map((block) => block.id || "") || [],
    );
    guide.copy_blocks?.push({
      id: nextBlockID,
      client_id: firstClient,
      os: "unix",
      path: "Terminal",
      hint: "Operator inserted block",
      language: "shell",
      content_template: `export API_BASE_URL="{{api_base_url}}"\nexport API_KEY="{{api_key}}"`,
    });
  });
  selectedBlockId.value = nextBlockID;
}

function updateSelectedBlockField(
  field: PlatformGuideCopyBlockStringField,
  value: string,
): void {
  const previousBlockID = selectedBlock.value?.id || "";
  mutateSelectedProfile((profile) => {
    const block = ensureGuide(profile).copy_blocks?.find(
      (candidate) => candidate.id === previousBlockID,
    );
    if (!block) return;
    block[field] = value;
  });
  if (field === "id") selectedBlockId.value = value;
}

function appendPlaceholderToSelectedBlock(placeholder: string): void {
  const currentTemplate = selectedBlock.value?.content_template || "";
  const separator = currentTemplate.endsWith("\n") || !currentTemplate ? "" : "\n";
  updateSelectedBlockField("content_template", `${currentTemplate}${separator}${placeholder}`);
}

function removeSelectedCopyBlock(): void {
  const blockID = selectedBlock.value?.id;
  if (!blockID) return;
  mutateSelectedProfile((profile) => {
    const guide = ensureGuide(profile);
    guide.copy_blocks = guide.copy_blocks?.filter((block) => block.id !== blockID) || [];
  });
}

function removeSelectedPlatform(): void {
  const platform = selectedPlatform.value;
  emitRegistryMutation((registry) => {
    registry.profiles = (registry.profiles || []).filter(
      (profile) => normalizedProfilePlatform(profile) !== platform,
    );
  });
}

function uniqueID(baseID: string, existingIDs: string[]): string {
  const normalizedBase = baseID.trim().toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "item";
  const existing = new Set(existingIDs.filter(Boolean));
  if (!existing.has(normalizedBase)) return normalizedBase;
  let suffix = 2;
  let candidate = `${normalizedBase}-${suffix}`;
  while (existing.has(candidate)) {
    suffix += 1;
    candidate = `${normalizedBase}-${suffix}`;
  }
  return candidate;
}
</script>
