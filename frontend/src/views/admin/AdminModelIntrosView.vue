<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.modelIntros.title') }}
          </h2>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <input
              v-model="searchKeyword"
              type="text"
              class="input w-56"
              :placeholder="t('admin.modelIntros.searchPlaceholder')"
              @keyup.enter="onSearch"
            />
            <button class="btn btn-secondary" @click="onSearch">{{ t('common.search') }}</button>
            <button @click="loadList" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              {{ t('admin.modelIntros.createBtn') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading">
          <template #cell-cover_url="{ row }">
            <div class="flex items-center">
              <img
                v-if="row.cover_url"
                :src="row.cover_url"
                :alt="row.model_key"
                class="h-10 w-16 rounded object-cover ring-1 ring-gray-200 dark:ring-dark-700"
                @error="onCoverError"
              />
              <span v-else class="text-xs text-gray-400">—</span>
            </div>
          </template>

          <template #cell-model_key="{ row }">
            <button
              class="flex flex-col items-start gap-0.5 text-left hover:underline"
              @click="openDetailDialog(row)"
            >
              <span class="font-mono text-xs text-blue-600 dark:text-blue-400">{{ row.model_key }}</span>
              <span v-if="row.title" class="text-xs text-gray-500 dark:text-gray-400">{{ row.title }}</span>
            </button>
          </template>

          <template #cell-description="{ row }">
            <span class="line-clamp-2 max-w-md text-xs text-gray-600 dark:text-gray-400">
              {{ localizedDescription(row) || '—' }}
            </span>
          </template>

          <template #cell-default_params="{ row }">
            <div class="flex flex-wrap gap-1 text-xs">
              <span
                v-for="(v, k) in row.default_params"
                :key="String(k)"
                class="rounded bg-gray-100 px-1.5 py-0.5 text-gray-700 dark:bg-dark-700 dark:text-gray-200"
              >{{ k }}: {{ formatParamValue(v) }}</span>
              <span v-if="!hasAnyParam(row.default_params)" class="text-xs text-gray-400">—</span>
            </div>
          </template>

          <template #cell-output_fields="{ row }">
            <div class="flex flex-wrap gap-1 text-xs">
              <template v-if="Array.isArray(row.output_fields) && row.output_fields.length > 0">
                <span
                  v-for="(f, i) in row.output_fields"
                  :key="i"
                  :class="[
                    'rounded px-1.5 py-0.5 font-mono',
                    outputTypeBadgeClass(f.type)
                  ]"
                  :title="f.description || f.key"
                >
                  <span v-if="row.result_field && f.key === row.result_field" class="mr-0.5">★</span>{{ f.label || f.key }}
                  <span class="ml-0.5 opacity-70">[{{ f.type }}]</span>
                </span>
              </template>
              <span v-else class="text-xs text-gray-400">—</span>
            </div>
          </template>

          <template #cell-enabled="{ row }">
            <span :class="['badge', row.enabled ? 'badge-success' : 'badge-default']">
              {{ row.enabled ? t('admin.modelIntros.statusEnabled') : t('admin.modelIntros.statusDisabled') }}
            </span>
          </template>

          <template #cell-sort_order="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-400">{{ value }}</span>
          </template>

          <template #cell-updated_at="{ value }">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-xs" @click="openDetailDialog(row)">{{ t('common.view') }}</button>
              <button class="btn btn-secondary btn-xs" @click="openEditDialog(row)">{{ t('common.edit') }}</button>
              <button class="btn btn-danger btn-xs" @click="askDelete(row)">{{ t('common.delete') }}</button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="(p) => { pagination.page = p; loadList() }"
          @update:page-size="(s) => { pagination.page_size = s; pagination.page = 1; loadList() }"
        />
      </template>
    </TablePageLayout>

    <!-- Create / Edit Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="editingKey === null ? t('admin.modelIntros.createTitle') : t('admin.modelIntros.editTitle')"
      width="extra-wide"
      @close="showFormDialog = false"
    >
      <div class="space-y-4">
        <div>
          <label class="form-label">{{ t('admin.modelIntros.fields.modelKey') }}</label>
          <Select
            :model-value="form.model_key"
            :options="modelKeyOptions"
            :placeholder="t('admin.modelIntros.fields.modelKeyPlaceholder')"
            :disabled="editingKey !== null"
            searchable
            creatable
            :creatable-prefix="t('admin.modelIntros.fields.modelKeyUseCustom')"
            :search-placeholder="t('admin.modelIntros.fields.modelKeySearchPlaceholder')"
            :empty-text="t('admin.modelIntros.fields.modelKeyEmpty')"
            @update:model-value="form.model_key = String($event ?? '')"
          />
          <p class="mt-1 text-xs text-gray-500">
            {{ t('admin.modelIntros.fields.modelKeyHint') }}
            <span v-if="candidates.length > 0" class="ml-1 text-blue-500">
              · {{ t('admin.modelIntros.fields.candidatesHint', { n: candidates.length }) }}
            </span>
            <span v-else-if="candidatesLoading" class="ml-1 text-gray-400">
              · {{ t('admin.modelIntros.fields.candidatesLoading') }}
            </span>
          </p>
        </div>

        <div>
          <label class="form-label">{{ t('admin.modelIntros.fields.title') }}</label>
          <input
            v-model="form.title"
            type="text"
            class="input"
            :placeholder="t('admin.modelIntros.fields.titlePlaceholder')"
          />
        </div>

        <!-- 模型介绍：中英双文
             为管理员维护同一模型的中英两份介绍。渲染层根据当前 i18n locale
             自动挑选：中文界面用 description，英文界面用 description_en，
             缺失时相互兜底。每个 label 右侧带"翻译"按钮：读另一语言字段
             作为源，走底部工具区选定的 API Key + 目标模型 调 chat/completions。
             源为空 / API Key 未选 / 模型未填时按钮禁用。 -->
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="flex flex-col gap-1">
            <div class="flex items-center justify-between gap-2">
              <label class="form-label mb-0">
                {{ t('admin.modelIntros.fields.description') }}
                <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                  {{ t('admin.modelIntros.fields.descriptionLangZh') }}
                </span>
              </label>
              <button
                type="button"
                class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium text-primary-600 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-400 dark:hover:bg-primary-950"
                :disabled="!introTranslationCtx.ready.value || !(form.description_en || '').trim() || introTranslatingZh"
                :title="introTranslateBtnTitle('zh')"
                @click="onIntroTranslate('zh')"
              >
                <svg v-if="introTranslatingZh" class="h-3 w-3 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
                </svg>
                <svg v-else class="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M7.5 3a1 1 0 011 1v1h3a1 1 0 010 2h-.6a8.9 8.9 0 01-2.4 4.3 8.2 8.2 0 002 1.3 1 1 0 01-.8 1.8 10.2 10.2 0 01-2.7-1.8 10.2 10.2 0 01-3.6 2 1 1 0 01-.7-1.9 8.3 8.3 0 003-1.6A8.9 8.9 0 015 7H4a1 1 0 110-2h2.5V4a1 1 0 011-1zm.5 4h-.9a6.9 6.9 0 001.4 2.5A6.9 6.9 0 009.4 7H8zm5.5 4a1 1 0 01.94.66l2.5 7a1 1 0 11-1.88.68L14.15 14h-2.3l-.41 1.34a1 1 0 11-1.88-.68l2.5-7A1 1 0 0113 11zm-.55 3h1.1L13 12.55 12.45 14z"/>
                </svg>
                {{ t('admin.modelIntros.fields.translateBtn') }}
              </button>
            </div>
            <textarea
              v-model="form.description"
              rows="4"
              class="input"
              :placeholder="t('admin.modelIntros.fields.descriptionPlaceholder')"
            />
          </div>
          <div class="flex flex-col gap-1">
            <div class="flex items-center justify-between gap-2">
              <label class="form-label mb-0">
                {{ t('admin.modelIntros.fields.description') }}
                <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                  {{ t('admin.modelIntros.fields.descriptionLangEn') }}
                </span>
              </label>
              <button
                type="button"
                class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium text-primary-600 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-400 dark:hover:bg-primary-950"
                :disabled="!introTranslationCtx.ready.value || !(form.description || '').trim() || introTranslatingEn"
                :title="introTranslateBtnTitle('en')"
                @click="onIntroTranslate('en')"
              >
                <svg v-if="introTranslatingEn" class="h-3 w-3 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
                </svg>
                <svg v-else class="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M7.5 3a1 1 0 011 1v1h3a1 1 0 010 2h-.6a8.9 8.9 0 01-2.4 4.3 8.2 8.2 0 002 1.3 1 1 0 01-.8 1.8 10.2 10.2 0 01-2.7-1.8 10.2 10.2 0 01-3.6 2 1 1 0 01-.7-1.9 8.3 8.3 0 003-1.6A8.9 8.9 0 015 7H4a1 1 0 110-2h2.5V4a1 1 0 011-1zm.5 4h-.9a6.9 6.9 0 001.4 2.5A6.9 6.9 0 009.4 7H8zm5.5 4a1 1 0 01.94.66l2.5 7a1 1 0 11-1.88.68L14.15 14h-2.3l-.41 1.34a1 1 0 11-1.88-.68l2.5-7A1 1 0 0113 11zm-.55 3h1.1L13 12.55 12.45 14z"/>
                </svg>
                {{ t('admin.modelIntros.fields.translateBtn') }}
              </button>
            </div>
            <textarea
              v-model="form.description_en"
              rows="4"
              class="input"
              :placeholder="t('admin.modelIntros.fields.descriptionPlaceholderEn')"
            />
          </div>
        </div>

        <div>
          <label class="form-label">{{ t('admin.modelIntros.fields.coverUrl') }}</label>
          <input
            v-model="form.cover_url"
            type="text"
            class="input"
            :placeholder="t('admin.modelIntros.fields.coverUrlPlaceholder')"
          />
          <div v-if="form.cover_url" class="mt-2">
            <img
              :src="form.cover_url"
              alt="cover preview"
              class="h-20 rounded object-cover ring-1 ring-gray-200 dark:ring-dark-700"
              @error="onCoverError"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="form-label">{{ t('admin.modelIntros.fields.sortOrder') }}</label>
            <input v-model.number="form.sort_order" type="number" class="input" step="1" />
          </div>
          <div class="flex items-center gap-3 pt-6">
            <input id="intro-enabled" v-model="form.enabled" type="checkbox" class="h-4 w-4" />
            <label for="intro-enabled" class="form-label !mb-0">{{ t('admin.modelIntros.fields.enabled') }}</label>
          </div>
        </div>

        <!-- Schema 段：包含"输入参数"与"输出参数"两个子区块；
             输入在前，输出在后；下方附带一行 "主结果指示器" (result_field + result_type) 。 -->
        <fieldset class="rounded border border-gray-200 p-4 dark:border-dark-700">
          <legend class="px-1 text-sm font-semibold text-gray-800 dark:text-gray-200">
            {{ t('admin.modelIntros.fields.schema') }}
          </legend>
          <!-- Schema 导入导出工具栏：一键复制 / 下载文件 / 弹窗导入（同时支持粘贴与上传文件） -->
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <p class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.schemaHint') }}</p>
            <div class="flex flex-wrap items-center gap-2">
              <button class="btn btn-secondary btn-xs" @click="copySchemaToClipboard">
                {{ t('admin.modelIntros.fields.schemaCopy') }}
              </button>
              <button class="btn btn-secondary btn-xs" @click="downloadSchema">
                {{ t('admin.modelIntros.fields.schemaDownload') }}
              </button>
              <button class="btn btn-secondary btn-xs" @click="openImportDialog">
                {{ t('admin.modelIntros.fields.schemaImport') }}
              </button>
            </div>
          </div>

          <!-- 输入参数：default_params 声明；每行现在使用递归 ParamSchemaEditor，
               支持 object/array 嵌套子字段（方案 C：JSON Schema 编辑器）。 -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <!-- 标题：加大字号 + 加粗，视觉上作为一个明显的段落分组标题 -->
              <h3 class="text-lg font-semibold text-gray-800 dark:text-gray-100">
                {{ t('admin.modelIntros.fields.inputParams') }}
              </h3>
              <button class="btn btn-secondary btn-xs" @click="addParam">{{ t('admin.modelIntros.fields.addParam') }}</button>
            </div>
            <!-- 输入参数解说：说明每列作用；解说独立于 hint，便于新配置人快速上手。 -->
            <div class="rounded bg-blue-50 px-3 py-2 text-xs text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">
              {{ t('admin.modelIntros.fields.inputParamsIntro') }}
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.defaultParamsHint') }}</p>
            <!--
              顶层输入参数：用 VueDraggable 包裹整个列表。
                - handle=".drag-handle" 与 ParamSchemaEditor 内部的把手关联，
                  管理员按住行首把手即可拖拽；
                - 每个 ParamSchemaEditor 同时暴露 ↑/↓ 按钮，通过 @move-up / @move-down
                  触发相邻元素 swap，作为拖拽的等效轻交互；
                - VueDraggable 直接 v-model 到 form.params，拖动结束后数组顺序更新，
                  下一次序列化就会带上新的 x-order；
                - 空数组时不渲染 VueDraggable（否则会出现一个空 drop-zone），改用提示文案占位。
            -->
            <VueDraggable
              v-if="form.params.length > 0"
              v-model="form.params"
              :animation="200"
              handle=".drag-handle"
              class="space-y-3"
            >
              <div
                v-for="(row, idx) in form.params"
                :key="row.uid"
                class="rounded border border-gray-200 p-3 dark:border-dark-700"
              >
                <ParamSchemaEditor
                  :model-value="row"
                  :removable="true"
                  :can-move-up="idx > 0"
                  :can-move-down="idx < form.params.length - 1"
                  @update:modelValue="onParamRowUpdate(idx, $event)"
                  @remove="removeParam(idx)"
                  @move-up="moveParam(idx, -1)"
                  @move-down="moveParam(idx, 1)"
                />
              </div>
            </VueDraggable>
            <p v-else class="text-xs text-gray-500">
              {{ t('admin.modelIntros.fields.defaultParamsEmpty') }}
            </p>
          </div>

          <!-- 分隔线 -->
          <div class="my-4 border-t border-dashed border-gray-200 dark:border-dark-700"></div>

          <!-- 输出参数：output_fields 声明；使用与输入参数完全一致的递归
               ParamSchemaEditor（JSON Schema 标准 type：string/number/boolean/
               object/array）。字段本身直接就是一份 schema 声明，是否必须、
               描述、枚举、object/array 嵌套等能力天然共享。 -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <!-- 标题：与输入参数保持同一层级的视觉分量 -->
              <h3 class="text-lg font-semibold text-gray-800 dark:text-gray-100">
                {{ t('admin.modelIntros.fields.outputParams') }}
              </h3>
              <button class="btn btn-secondary btn-xs" @click="addOutputField">
                {{ t('admin.modelIntros.fields.addOutputField') }}
              </button>
            </div>
            <!-- 输出参数解说：说明 key 路径语法与 schema 类型的含义。 -->
            <div class="rounded bg-emerald-50 px-3 py-2 text-xs text-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-200">
              {{ t('admin.modelIntros.fields.outputParamsIntro') }}
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.outputFieldsHint') }}</p>
            <div class="space-y-3">
              <div
                v-for="(row, idx) in form.outputFields"
                :key="row.uid"
                class="rounded border border-gray-200 p-3 dark:border-dark-700"
              >
                <!-- 顶层每一行都是一个 ParamSchemaEditor：内部递归处理 object/array，
                     并自带 required / description / enum 等一致的填写能力。 -->
                <ParamSchemaEditor
                  :model-value="row"
                  :removable="true"
                  @update:modelValue="onOutputRowUpdate(idx, $event)"
                  @remove="removeOutputField(idx)"
                />
              </div>
              <p v-if="form.outputFields.length === 0" class="text-xs text-gray-500">
                {{ t('admin.modelIntros.fields.outputFieldsEmpty') }}
              </p>
            </div>
          </div>

          <!-- Schema 段底部：主结果指示器（result_field 下拉 + result_type 单选） -->
          <div class="mt-4 rounded bg-gray-50 p-3 dark:bg-dark-800/60">
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="form-label">{{ t('admin.modelIntros.fields.resultField') }}</label>
                <!-- 使用通用 Select 组件；第一项 value="" 代表未选（回退到默认 video/image 兜底）。 -->
                <Select
                  :model-value="form.result_field"
                  :options="resultFieldSelectOptions"
                  :disabled="resultFieldOptions.length === 0"
                  :searchable="resultFieldOptions.length > 5"
                  :placeholder="t('admin.modelIntros.fields.resultFieldNone')"
                  @update:model-value="form.result_field = String($event ?? '')"
                />
                <p class="mt-1 text-xs text-gray-500">{{ t('admin.modelIntros.fields.resultFieldHint') }}</p>
              </div>
              <div>
                <label class="form-label">{{ t('admin.modelIntros.fields.resultType') }}</label>
                <div class="flex items-center gap-4 pt-1">
                  <label class="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="form.result_type" type="radio" value="video" class="h-4 w-4" />
                    {{ t('admin.modelIntros.fields.resultTypeVideo') }}
                  </label>
                  <label class="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
                    <input v-model="form.result_type" type="radio" value="image" class="h-4 w-4" />
                    {{ t('admin.modelIntros.fields.resultTypeImage') }}
                  </label>
                </div>
                <p class="mt-1 text-xs text-gray-500">{{ t('admin.modelIntros.fields.resultTypeHint') }}</p>
              </div>
            </div>
          </div>
        </fieldset>
      </div>

      <template #footer>
        <!-- 翻译工具区（仅编辑参数 Schema 时需要用；放 footer 左侧，避免占用表单主体） -->
        <div class="mr-auto flex flex-wrap items-center gap-2">
          <span class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.modelIntros.fields.translationTools') }}
          </span>
          <!-- API Key 下拉：搜索 > 5 项时开启；无 key 时显示"无可用 Key"提示 -->
          <label class="flex items-center gap-1 text-[11px] text-gray-600 dark:text-gray-300">
            <span class="whitespace-nowrap">{{ t('admin.modelIntros.fields.translationKeyLabel') }}</span>
            <div class="w-56">
              <Select
                v-if="translationKeyOptions.length"
                v-model="translationSelectedKeyId"
                :options="translationKeyOptions"
                :searchable="translationKeyOptions.length > 5"
                :placeholder="t('admin.modelIntros.fields.translationSelectKey')"
                size="sm"
              />
              <span v-else class="text-[11px] text-gray-400">
                {{ translationKeysLoading ? t('common.loading') : t('admin.modelIntros.fields.translationNoKey') }}
              </span>
            </div>
          </label>
          <!-- 目标模型：选完 API Key 后自动拉可用模型列表；creatable=true 兼容自定义模型名。
               列表加载中/为空时依然可用 —— Select 头部会显示 "current" 保留项。 -->
          <label class="flex items-center gap-1 text-[11px] text-gray-600 dark:text-gray-300">
            <span class="whitespace-nowrap">{{ t('admin.modelIntros.fields.translationModelLabel') }}</span>
            <div class="w-56">
              <Select
                v-model="translationModel"
                :options="translationModelOptions"
                :searchable="true"
                :creatable="true"
                :creatable-prefix="t('common.search')"
                :placeholder="translationModelsLoading
                  ? t('common.loading')
                  : t('admin.modelIntros.fields.translationModelPlaceholder')"
                size="sm"
              />
            </div>
          </label>
        </div>
        <button class="btn btn-secondary" @click="showFormDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting" @click="submitForm">
          {{ submitting ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.modelIntros.deleteTitle')"
      :message="t('admin.modelIntros.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Schema 导入弹窗：支持 A（粘贴 JSON）与 B（从文件上传）两种方式；
         导出侧的复制/下载按钮在编辑弹窗内直接完成，不需要专门弹窗。 -->
    <BaseDialog
      :show="showImportDialog"
      :title="t('admin.modelIntros.fields.schemaImportTitle')"
      width="wide"
      @close="showImportDialog = false"
    >
      <div class="space-y-3">
        <p class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.schemaImportHint') }}</p>
        <div class="flex flex-wrap items-center gap-2">
          <button class="btn btn-secondary btn-xs" @click="pickImportFile">
            {{ t('admin.modelIntros.fields.schemaImportPickFile') }}
          </button>
          <input
            ref="fileInputRef"
            type="file"
            accept="application/json,.json"
            class="hidden"
            @change="onImportFileChange"
          />
        </div>
        <textarea
          v-model="importText"
          rows="14"
          class="input font-mono text-xs"
          :placeholder="t('admin.modelIntros.fields.schemaImportPlaceholder')"
        />
        <p v-if="importError" class="text-xs text-red-500">{{ importError }}</p>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showImportDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" @click="applyImport">
          {{ t('admin.modelIntros.fields.schemaImportApply') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Detail Dialog -->
    <BaseDialog
      :show="showDetailDialog"
      :title="t('admin.modelIntros.detailTitle')"
      width="wide"
      @close="showDetailDialog = false"
    >
      <div v-if="detailRow" class="space-y-5">
        <!-- Header: cover + key + title -->
        <div class="flex items-start gap-4">
          <img
            v-if="detailRow.cover_url"
            :src="detailRow.cover_url"
            :alt="detailRow.model_key"
            class="h-24 w-40 flex-shrink-0 rounded object-cover ring-1 ring-gray-200 dark:ring-dark-700"
            @error="onCoverError"
          />
          <div class="flex-1 min-w-0">
            <div class="font-mono text-sm text-gray-900 dark:text-gray-100 break-all">
              {{ detailRow.model_key }}
            </div>
            <div v-if="detailRow.title" class="mt-1 text-base font-medium text-gray-800 dark:text-gray-200">
              {{ detailRow.title }}
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <span class="badge" :class="detailRow.enabled ? 'badge-success' : 'badge-default'">
                {{ detailRow.enabled ? t('admin.modelIntros.statusEnabled') : t('admin.modelIntros.statusDisabled') }}
              </span>
              <span class="text-xs text-gray-500">
                {{ t('admin.modelIntros.columns.sortOrder') }}: {{ detailRow.sort_order }}
              </span>
            </div>
          </div>
        </div>

        <!-- 模型介绍：中英双文
             如果两语言都填了，展示为并排两栏（sm+ 断点）；只填了一份也不做额外区分，
             仅展示存在的那份文案，避免出现空白框。 -->
        <div v-if="detailRow.description || detailRow.description_en" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div v-if="detailRow.description">
            <div class="form-label">
              {{ t('admin.modelIntros.fields.description') }}
              <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ t('admin.modelIntros.fields.descriptionLangZh') }}
              </span>
            </div>
            <div class="whitespace-pre-wrap rounded bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-300">
              {{ detailRow.description }}
            </div>
          </div>
          <div v-if="detailRow.description_en">
            <div class="form-label">
              {{ t('admin.modelIntros.fields.description') }}
              <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ t('admin.modelIntros.fields.descriptionLangEn') }}
              </span>
            </div>
            <div class="whitespace-pre-wrap rounded bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-300">
              {{ detailRow.description_en }}
            </div>
          </div>
        </div>

        <!-- Default params (输入参数) -->
        <div>
          <div class="form-label">{{ t('admin.modelIntros.fields.inputParams') }}</div>
          <div v-if="hasAnyParam(detailRow.default_params)" class="space-y-1">
            <div
              v-for="(v, k) in detailRow.default_params"
              :key="String(k)"
              class="flex items-start gap-2 rounded bg-gray-50 px-2 py-1 text-xs dark:bg-dark-800"
            >
              <span class="font-mono text-gray-500">{{ k }}</span>
              <span class="text-gray-400">=</span>
              <span class="font-mono break-all text-gray-800 dark:text-gray-200">{{ formatParamValue(v) }}</span>
            </div>
          </div>
          <p v-else class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.defaultParamsEmpty') }}</p>
        </div>

        <!-- Output fields (输出参数) -->
        <div>
          <div class="form-label">{{ t('admin.modelIntros.fields.outputParams') }}</div>
          <div v-if="Array.isArray(detailRow.output_fields) && detailRow.output_fields.length > 0" class="space-y-1.5">
            <div
              v-for="(f, i) in detailRow.output_fields"
              :key="i"
              class="flex flex-wrap items-baseline gap-2 rounded bg-gray-50 px-2 py-1.5 text-xs dark:bg-dark-800"
            >
              <span
                :class="['rounded px-1.5 py-0.5 font-mono', outputTypeBadgeClass(f.type)]"
              >{{ f.type }}</span>
              <span v-if="detailRow.result_field && f.key === detailRow.result_field" class="rounded bg-yellow-100 px-1 text-[10px] text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300">
                {{ t('admin.modelIntros.fields.resultFieldBadge') }}
              </span>
              <span class="font-mono text-gray-800 dark:text-gray-200">{{ f.key }}</span>
              <span v-if="f.label" class="text-gray-600 dark:text-gray-400">{{ f.label }}</span>
              <span v-if="f.description" class="w-full break-all text-[11px] text-gray-500 dark:text-gray-400">
                {{ f.description }}
              </span>
            </div>
          </div>
          <p v-else class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.outputFieldsEmpty') }}</p>

          <!-- 主结果指示器摘要 -->
          <div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-500">
            <span>
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('admin.modelIntros.fields.resultField') }}:</span>
              <span v-if="detailRow.result_field" class="ml-1 font-mono text-gray-800 dark:text-gray-200">{{ detailRow.result_field }}</span>
              <span v-else class="ml-1 italic">{{ t('admin.modelIntros.fields.resultFieldNone') }}</span>
            </span>
            <span>
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('admin.modelIntros.fields.resultType') }}:</span>
              <span class="ml-1">{{ detailRow.result_type === 'image' ? t('admin.modelIntros.fields.resultTypeImage') : t('admin.modelIntros.fields.resultTypeVideo') }}</span>
            </span>
          </div>
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showDetailDialog = false">{{ t('common.close') }}</button>
        <button v-if="detailRow" class="btn btn-primary" @click="openEditFromDetail">
          {{ t('common.edit') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  ModelIntro,
  ModelIntroCandidate,
  OutputFieldSpec,
  ResultMediaType,
  UpsertModelIntroRequest
} from '@/api/admin/modelIntros'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
// ParamSchemaEditor：递归 JSON Schema 编辑器；输入参数与"输出参数 type=json"共用。
import ParamSchemaEditor from '@/components/common/ParamSchemaEditor.vue'
// VueDraggable：给顶层输入参数列表提供拖拽重排，与 ParamSchemaEditor 内的 ↑/↓
// 按钮双通道并存，管理员可任选一种交互。
import { VueDraggable } from 'vue-draggable-plus'
import {
  type SchemaRow,
  makeSchemaRow,
  mapToRows,
  rowsToMap,
  rowToSchema,
  schemaToRow,
} from '@/components/common/paramSchemaRow'
// keysAPI + ApiKey：底部"翻译工具"用来列出当前登录用户可用的 API Key，
// 让管理员选一把 composite key 作为翻译请求的 Bearer 凭据。
import { keysAPI } from '@/api/keys'
import type { ApiKey } from '@/types'
// provideDescriptionTranslation：在编辑 dialog 打开的这层 setup 里注入翻译上下文，
// 递归的 ParamSchemaEditor 就能无 props 透传拿到 apiKey + model + translate()。
import { provideDescriptionTranslation, fetchModelsForKey } from '@/composables/useDescriptionTranslation'

const { t, locale } = useI18n()
const appStore = useAppStore()

/**
 * localizedDescription：按当前 i18n locale 挑选"模型介绍"的显示语言。
 *   - 当前语言为 en / en-* → 优先返回 description_en；空时回落到 description。
 *   - 其它语言（默认视为中文界面）→ 优先返回 description；空时回落到 description_en。
 * 两个都为空返回空串。用于表格列的缩略展示，避免在英文界面下依旧显示中文简介。
 */
function localizedDescription(row: ModelIntro): string {
  const zh = (row.description || '').trim()
  const en = (row.description_en || '').trim()
  const isEn = String(locale.value || '').toLowerCase().startsWith('en')
  if (isEn) return en || zh
  return zh || en
}

// ============ 字段说明翻译工具区（编辑 dialog 底部左侧） ============
// 需求：管理员在编辑参数 Schema 时，希望能一键把中文说明翻译成英文（或反之）。
// 由于 Schema 是递归结构（object.children、array.items 可能嵌很多层），
// 每层的翻译按钮都要用同一把 API Key + 同一个目标模型，最好的做法是
// 在 setup 顶层 provide 一次，让所有子孙 ParamSchemaEditor inject 到同一份。
//
// 状态：
//   - translationAllKeys：当前用户全部可用 API Key（加载一次即可，无需按 group 过滤，
//     因为翻译走 /v1/chat/completions，任何有效 composite key 都行）
//   - translationSelectedKeyId：下拉里当前选中的 key.id
//   - translationModel：目标聊天模型名（可输入，如 "gpt-4o-mini"、"claude-3-5-haiku-20241022"）
//   - translationApiKey / translationModelRef：上面两者的"实际字符串值"包装，
//     用于 provide；子组件里通过 ref 感知底部下拉变化，无需 re-mount。
// localStorage 记忆：管理员每次开 dialog 都要重选很烦，我们用 localStorage 记忆
//   selectedKeyId + model；下次打开自动回填最后一次成功保存的组合。
const TRANSLATION_STORAGE_KEY = 'admin.modelIntros.translationCtx.v1'
const translationAllKeys = ref<ApiKey[]>([])
const translationKeysLoading = ref(false)
const translationSelectedKeyId = ref<number | ''>('')
const translationModel = ref<string>('gpt-4o-mini')

/**
 * translationSelectedKey：selectedKeyId 对应的 ApiKey 对象（含明文 key 字段）。
 * 只在 dialog 内用于渲染 label（掩码显示） + 拼 Bearer。id 空或找不到时返回 null。
 */
const translationSelectedKey = computed<ApiKey | null>(() => {
  if (translationSelectedKeyId.value === '') return null
  return translationAllKeys.value.find((k) => k.id === translationSelectedKeyId.value) ?? null
})

/**
 * translationApiKey：provide 给递归编辑器的"当前 API Key 明文"字符串 ref。
 * 通过 computed 派生并写回 ref，是为了让子组件拿到"响应式的最新值"而不是快照。
 */
const translationApiKey = ref<string>('')
// 每当"选中的 key"或"底层 keys 列表"变化，把明文 key 值同步到 translationApiKey；
// 子组件通过 provide 拿到的是 ref，watchEffect 一改子组件下次点击就用新值。
watchEffect(() => {
  translationApiKey.value = translationSelectedKey.value?.key ?? ''
})

// 一次性 provide：整个组件生命周期内 apiKey / model 两个 ref 引用不变，
// 子组件 inject 一次就够；下拉里改选新 key 只会触发 ref 的 .value 变化，
// 子组件在下一次点击"翻译"时自动读到最新值。
// 顶层拿到返回的 ctx，是为了让"模型介绍"这两个 textarea 也能复用同一份翻译能力
// （Vue 官方限制：同一组件里 provide 的值不能被自己 inject 回来）。
const introTranslationCtx = provideDescriptionTranslation(translationApiKey, translationModel)

// ============ "模型介绍"字段的翻译按钮状态 ============
// 与 ParamSchemaEditor 里的 translatingZh / translatingEn 语义一致；
// 因为顶层介绍字段只有一份，这里就用两个独立 ref 分别代表中/英侧的
// loading（互不阻塞：用户可先点中文侧翻译等待时再检查英文原文）。
const introTranslatingZh = ref(false)
const introTranslatingEn = ref(false)

/**
 * introTranslateBtnTitle：hover 提示。分三档（与 ParamSchemaEditor 保持一致）：
 *   - Key/Model 未选：告诉用户去底部工具区选
 *   - 源字段为空：告诉用户"先填另一语言的内容"
 *   - 就绪：显示通用"翻译"提示
 */
function introTranslateBtnTitle(target: 'zh' | 'en'): string {
  if (!introTranslationCtx.ready.value) {
    return t('admin.modelIntros.fields.translateNotReady')
  }
  const srcText = target === 'zh' ? (form.description_en ?? '') : (form.description ?? '')
  if (!srcText.trim()) {
    return t('admin.modelIntros.fields.translateSourceEmpty')
  }
  return t('admin.modelIntros.fields.translateBtnTitle')
}

/**
 * onIntroTranslate：点击"翻译"按钮时触发（顶层"模型介绍"字段专用）。
 *   - target='zh' → 用 description_en 作为源翻译成中文，写回 description；
 *   - target='en' → 用 description    作为源翻译成英文，写回 description_en。
 *   - 上下文未就绪 / 源为空时给用户提示后返回；
 *   - 翻译失败时把 err.message 抛给用户，便于快速定位（例如 402 余额不足）。
 */
async function onIntroTranslate(target: 'zh' | 'en') {
  if (!introTranslationCtx.ready.value) {
    appStore.showError(t('admin.modelIntros.fields.translateNotReady'))
    return
  }
  const source: 'zh' | 'en' = target === 'zh' ? 'en' : 'zh'
  const sourceText = source === 'zh' ? form.description : form.description_en
  if (!sourceText || !sourceText.trim()) {
    appStore.showError(t('admin.modelIntros.fields.translateSourceEmpty'))
    return
  }
  const flag = target === 'zh' ? introTranslatingZh : introTranslatingEn
  flag.value = true
  try {
    const translated = await introTranslationCtx.translate(sourceText, source, target)
    if (target === 'zh') form.description = translated
    else form.description_en = translated
    appStore.showSuccess(t('admin.modelIntros.fields.translateSuccess'))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    appStore.showError(t('admin.modelIntros.fields.translateFailed', { msg }))
  } finally {
    flag.value = false
  }
}

/**
 * translationKeyOptions：把 API Keys 转成 Select options。
 * 只保留 active 的 key；label 展示"名称 · 分组 · 掩码"，方便管理员在多个 key 里挑选。
 */
const translationKeyOptions = computed<SelectOption[]>(() =>
  translationAllKeys.value
    .filter((k) => k.status === 'active')
    .map((k) => ({
      value: k.id,
      label: `${k.name} · ${k.group?.name || '-'} · ${maskTranslationKey(k.key)}`,
    }))
)

/** maskTranslationKey：跟 VideoPlayground 里一致的脱敏规则，避免整段 key 出现在下拉里。 */
function maskTranslationKey(key: string): string {
  if (!key) return '-'
  if (key.length <= 10) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

/**
 * loadTranslationKeys：拉当前用户全部 API Keys（分页 1/100，本管理员的 key 数量足够）。
 * 拉完后：
 *   - 尝试从 localStorage 恢复上次选择（selectedKeyId + model）；
 *   - 恢复的 keyId 若已失效或不存在，则回落到第一把 active key；
 *   - 恢复失败或首次使用时保留默认模型 "gpt-4o-mini"。
 */
async function loadTranslationKeys() {
  translationKeysLoading.value = true
  try {
    const resp = await keysAPI.list(1, 100)
    translationAllKeys.value = resp.items ?? []
    restoreTranslationCtx()
  } catch {
    translationAllKeys.value = []
  } finally {
    translationKeysLoading.value = false
  }
}

/**
 * restoreTranslationCtx：从 localStorage 读取上次选择的 keyId/model 并回填。
 * 若 storage 里的 keyId 已不在当前 key 列表（已失效/删除），则默认选中第一把 active 的。
 * 只在 keys 加载完之后调用，避免拿不到候选就写回默认值把用户偏好覆盖掉。
 */
function restoreTranslationCtx() {
  try {
    const raw = localStorage.getItem(TRANSLATION_STORAGE_KEY)
    if (raw) {
      const stored = JSON.parse(raw) as { keyId?: number; model?: string }
      if (typeof stored.model === 'string' && stored.model.trim()) {
        translationModel.value = stored.model
      }
      if (typeof stored.keyId === 'number') {
        const hit = translationAllKeys.value.find(
          (k) => k.id === stored.keyId && k.status === 'active'
        )
        if (hit) {
          translationSelectedKeyId.value = hit.id
          return
        }
      }
    }
  } catch {
    // localStorage 读取或 JSON 解析失败，忽略，走默认路径。
  }
  // 回落：选第一把 active key（若有），让"打开就能用"的体验更顺滑。
  if (!translationSelectedKeyId.value) {
    const firstActive = translationAllKeys.value.find((k) => k.status === 'active')
    if (firstActive) translationSelectedKeyId.value = firstActive.id
  }
}

/**
 * 持久化：selectedKeyId / model 任一变化都写回 localStorage。
 * 用 { immediate: false } 是为了避免"刚 mount、还没恢复"就把默认值覆盖到 storage；
 * restoreTranslationCtx 里的赋值会触发这里的 watch 补写一次，达到自动落盘效果。
 */
watch(
  [translationSelectedKeyId, translationModel],
  ([keyId, model]) => {
    try {
      localStorage.setItem(
        TRANSLATION_STORAGE_KEY,
        JSON.stringify({ keyId: keyId === '' ? undefined : keyId, model })
      )
    } catch {
      // 隐私模式或配额满都会抛错；持久化失败不影响本次翻译使用。
    }
  }
)

// ============ 目标模型下拉：按 key 拉取 + 本地缓存 ============
// 需求：管理员选完 API Key 后，"目标模型"应该自动列出该 key 可用模型；不要每次
// 打开编辑弹窗都手动重敲一次。缓存策略：
//   - 内存：translationModelsCache（Record<keyId, string[]>），本次会话命中最快；
//   - 磁盘：localStorage，跨会话记忆，避免每次刷新页面都请求；
//   - 时机：每次打开 dialog 只对"当前选中的 key"刷新一次（用户明确要求），
//           watch keyId 切换时也会自动刷新新 key 的列表。
// 兼容：如果拉取失败或后端返回空，Select 仍开启 creatable，允许手写模型名。
const TRANSLATION_MODELS_CACHE_KEY = 'admin.modelIntros.translationModelsCache.v1'
const TRANSLATION_MODELS_CACHE_LIMIT = 10 // 最多缓存 10 把 key 的模型列表
const translationModelsCache = ref<Record<string, string[]>>(loadModelsCacheFromStorage())
const translationModelsLoading = ref(false)

/**
 * loadModelsCacheFromStorage：启动时从 localStorage 读初始缓存。
 * 读取失败或格式非法一律返回空对象；不做校验成本，反正下次刷新会覆盖。
 */
function loadModelsCacheFromStorage(): Record<string, string[]> {
  try {
    const raw = localStorage.getItem(TRANSLATION_MODELS_CACHE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const out: Record<string, string[]> = {}
      for (const [k, v] of Object.entries(parsed)) {
        if (Array.isArray(v) && v.every((x) => typeof x === 'string')) {
          out[k] = v as string[]
        }
      }
      return out
    }
  } catch {
    // 忽略：格式非法或 storage 不可用。
  }
  return {}
}

/**
 * saveModelsCacheToStorage：写回 localStorage；超出 LIMIT 时按 key 数量做 LRU 简化裁剪。
 * 因为我们没记 access-time，这里就直接保留字典序前 LIMIT 项（够用即可）。
 */
function saveModelsCacheToStorage(cache: Record<string, string[]>) {
  try {
    const keys = Object.keys(cache)
    let toSave: Record<string, string[]> = cache
    if (keys.length > TRANSLATION_MODELS_CACHE_LIMIT) {
      toSave = {}
      for (const k of keys.slice(0, TRANSLATION_MODELS_CACHE_LIMIT)) toSave[k] = cache[k]
    }
    localStorage.setItem(TRANSLATION_MODELS_CACHE_KEY, JSON.stringify(toSave))
  } catch {
    // 隐私模式或配额满：忽略。
  }
}

/**
 * translationModels：当前选中 key 对应的模型 ID 列表（响应式）。
 * 未选 key 或缓存里没有则返回空数组；打开 dialog 后 refreshTranslationModels 会更新它。
 */
const translationModels = computed<string[]>(() => {
  const id = translationSelectedKeyId.value
  if (id === '') return []
  return translationModelsCache.value[String(id)] ?? []
})

/**
 * translationModelOptions：转成 Select options。
 * 仅在"列表尚未拉到（拉取中或拉取失败）"时，才把当前手写值兜底展示一条，
 * 让用户不至于在切换 key 后看到"选中值消失"。一旦列表拉回来了：
 *   - 如果当前值不在列表里，会由 refreshTranslationModels 主动清空，
 *   - 所以这里就不再画蛇添足地追加 "(current)" 项，避免"实际已失效的模型
 *     还能被选中"的错觉（这是切换 key 后的常见 bug 触发路径）。
 */
const translationModelOptions = computed<SelectOption[]>(() => {
  const list = translationModels.value
  const opts: SelectOption[] = list.map((id) => ({ value: id, label: id }))
  const current = translationModel.value.trim()
  // list 为空 = 还没成功拉到；此时才保留手写值以维持可用性。
  if (list.length === 0 && current) {
    opts.unshift({ value: current, label: current })
  }
  return opts
})

/**
 * refreshTranslationModels：按当前 key 拉一次 /v1/models，并写回内存 + localStorage 缓存。
 *   - key 为空：直接返回，不发请求；
 *   - 拉取失败：静默降级——保留旧缓存（如果有），Select 依旧允许 creatable 手写；
 *   - 成功：完全覆盖该 key 对应的缓存条目，并校验当前选中值是否仍然有效，
 *          无效则清空 translationModel（防止显示"已失效模型"）。
 *
 * 关键设计：为了保证切换 key 后一定拿到"最新"列表，本函数默认会强制走网络，
 * 拉取过程中先把该 key 的缓存清空（下拉临时显示为空 + loading 文案），
 * 避免用户误以为老列表仍适用于新 key。
 */
async function refreshTranslationModels() {
  const keyId = translationSelectedKeyId.value
  const key = translationApiKey.value
  if (keyId === '' || !key) return
  // 强制清空该 key 的旧缓存条目，避免"切 key 后老模型看起来还能选"的错觉。
  if (translationModelsCache.value[String(keyId)]) {
    const cleared = { ...translationModelsCache.value }
    delete cleared[String(keyId)]
    translationModelsCache.value = cleared
  }
  translationModelsLoading.value = true
  try {
    const ids = await fetchModelsForKey(key)
    if (ids.length > 0) {
      const next = { ...translationModelsCache.value, [String(keyId)]: ids }
      translationModelsCache.value = next
      saveModelsCacheToStorage(next)
      // 校验当前选中的模型是否仍在新列表中；不在则清空，交给用户重新选。
      const current = translationModel.value.trim()
      if (current && !ids.includes(current)) {
        translationModel.value = ''
      }
    }
  } catch {
    // 拉取失败：不改动缓存，让用户走 creatable 手写路径。
  } finally {
    translationModelsLoading.value = false
  }
}

// watch keyId 切换：立即清空当前选中的模型（旧 key 的模型对新 key 未必有效），
// 并触发新 key 的模型列表刷新。dialog 关闭时不监听、不请求
// （openCreate/openEdit 打开时会主动刷）。
watch(translationSelectedKeyId, (newId, oldId) => {
  if (!showFormDialog.value) return
  if (newId === oldId) return
  // 用户切到另一把 key：立即清空模型选择，避免"旧模型对新 key 不可用却还选中"。
  translationModel.value = ''
  if (newId === '') return
  refreshTranslationModels()
})


const rows = ref<ModelIntro[]>([])
const loading = ref(false)
const submitting = ref(false)
const searchKeyword = ref('')

const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const showFormDialog = ref(false)
const showDeleteDialog = ref(false)
const showDetailDialog = ref(false)
const editingKey = ref<string | null>(null)
const deletingKey = ref<string | null>(null)
const detailRow = ref<ModelIntro | null>(null)

// 下拉候选模型（与"视频模型"菜单同源）：来自所有 fal 账号中
// 开启"支持视频模型"开关的帐号 model_mapping。允许自由输入，下拉只作提示。
const candidates = ref<ModelIntroCandidate[]>([])
const candidatesLoading = ref(false)

// 将候选映射为通用 Select 的 options（value/label 结构），保持与项目其他
// 下拉样式一致；同时在 label 后面附带支持该模型的上游账号数，方便管理员识别。
const modelKeyOptions = computed<SelectOption[]>(() => {
  return candidates.value.map((c) => ({
    value: c.model_key,
    label: c.account_count > 0 ? `${c.model_key} (${c.account_count})` : c.model_key
  }))
})

// 方案 C：输入参数与输出参数都统一到 SchemaRow（递归 JSON Schema 编辑器）。
// SchemaRow 由 './paramSchemaRow' 导出，顶层每行都是一个可能递归的 schema 节点。
//
// 序列化关系：
//   form.params        (SchemaRow[])  <-- mapToRows / rowsToMap                --  default_params (存储 shape，map)
//   form.outputFields  (SchemaRow[])  <-- outputFieldsToSchemaRows / schemaRowsToOutputFields --  output_fields (存储 shape，array)
interface FormState {
  model_key: string
  title: string
  description: string
  // description_en：模型介绍的英文版本。与 description 并存，前端编辑器
  // 把两个 textarea 横向并排，并在 label 旁侧提供“翻译”按钮。
  description_en: string
  cover_url: string
  sort_order: number
  enabled: boolean
  outputFields: SchemaRow[]
  params: SchemaRow[]
  result_field: string
  result_type: ResultMediaType
}
const form = reactive<FormState>({
  model_key: '',
  title: '',
  description: '',
  description_en: '',
  cover_url: '',
  sort_order: 0,
  enabled: true,
  outputFields: [],
  params: [],
  result_field: '',
  result_type: 'video'
})

// ============ Schema 导入导出弹窗状态 ============
// 支持三种方式：
//   A) 弹窗 + textarea 手动粘贴/复制
//   B) 文件上传/下载（.json）
//   C) 一键复制到剪贴板
// 三种方式共享同一份 JSON shape（后端存储形状）：
//   {
//     default_params: { key: { value, required?, description?, enum?, options? }, ... },
//     output_fields:  [{ key, label, type, description, default, required?, enum?, options? }, ...],
//     result_field:   string,
//     result_type:    'video' | 'image'
//   }
const showImportDialog = ref(false)
const importText = ref('')
const importError = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)

// resultFieldOptions：递归遍历 form.outputFields（SchemaRow 树），把每一个
// 可作为"主结果指向"的节点都展开成一条候选。语义如下：
//   - 顶层叶子 "seed"                            → "seed"
//   - object 节点 "data" 自身                    → "data"
//   - object 节点的子字段 "data.video"           → "data.video"
//   - object 节点 → array 元素 → 子字段 "images[*].url" → "images[*].url"
//   - array 元素为叶子 "urls[*]"                 → "urls[*]"
// 生成规则（与 paramSpec.pickByPath 的路径语法保持完全一致）：
//   - object 层：拼接为 `${prefix}.${childKey}`（顶层无 prefix 时直接用 key）
//   - array 层：先产出 `${prefix}` 作为"整个数组"候选，再对 items 递归时，
//     使用 `${prefix}[*]` 作为新 prefix；items.key 恒为空。
//   - 空 key 的节点（如未填名的顶层行）跳过，避免产出无效路径。
// 深度优先展开，展示层缩进用 · 分级前缀直观呈现层级关系。
interface ResultFieldOption {
  key: string          // 真实路径，用于 result_field
  label: string        // 展示 label（等于 key 本身，可读性最好）
  depth: number        // 层级，供 Select 缩进展示
}
function collectResultFieldPaths(rows: SchemaRow[]): ResultFieldOption[] {
  const out: ResultFieldOption[] = []
  function walkNode(node: SchemaRow, prefix: string, depth: number) {
    // 当前节点自身的路径（空 prefix 表示顶层，直接用 key）
    if (!prefix) return // 兜底：不带 prefix 的入口不会自己产出，由 walkTop 负责
    // 除了顶层入口外，每一层都先把 prefix 加入（object/array/叶子都可作为主结果）
    out.push({ key: prefix, label: prefix, depth })
    if (node.type === 'object') {
      for (const ch of node.children) {
        const ck = (ch.key || '').trim()
        if (!ck) continue
        walkNode(ch, `${prefix}.${ck}`, depth + 1)
      }
      return
    }
    if (node.type === 'array' && node.items) {
      // items.key 恒为空，所以直接把 `[*]` 附加到当前 prefix 后作为新 prefix，
      // 再进入 items 展开。若 items 是 object/array，会继续递归其子层。
      walkNode(node.items, `${prefix}[*]`, depth + 1)
      return
    }
  }
  function walkTop(node: SchemaRow, depth: number) {
    const k = (node.key || '').trim()
    if (!k) return
    // 顶层入口路径直接就是 key
    walkNode(node, k, depth)
  }
  for (const r of rows) walkTop(r, 0)
  return out
}
const resultFieldOptions = computed<ResultFieldOption[]>(() =>
  collectResultFieldPaths(form.outputFields)
)

// resultFieldSelectOptions：给通用 Select 用的 { value, label } 数组；首项 value=''
// 表示"未选择（回退到第一个 video/image）"。label 里用 · 缩进直观展示层级。
const resultFieldSelectOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.modelIntros.fields.resultFieldNone') },
  ...resultFieldOptions.value.map((o) => ({
    value: o.key,
    // 缩进符：每一层多一个 "  · "，配合 font-mono 视觉上一目了然
    label: o.depth > 0 ? `${'    '.repeat(o.depth)}└ ${o.key}` : o.key,
  })),
])

// 输入参数 type 下拉选项已迁移到 ParamSchemaEditor 内部（string / number / boolean / object / array）。
// 输出参数改造后也复用同一编辑器，因此这里不再需要独立的输出 type 下拉选项。

const columns = computed<Column[]>(() => [
  { key: 'cover_url', label: t('admin.modelIntros.columns.cover') },
  { key: 'model_key', label: t('admin.modelIntros.columns.modelKey') },
  { key: 'description', label: t('admin.modelIntros.columns.description') },
  { key: 'default_params', label: t('admin.modelIntros.columns.defaultParams') },
  { key: 'output_fields', label: t('admin.modelIntros.columns.outputFields') },
  { key: 'sort_order', label: t('admin.modelIntros.columns.sortOrder') },
  { key: 'enabled', label: t('admin.modelIntros.columns.status') },
  { key: 'updated_at', label: t('admin.modelIntros.columns.updatedAt') },
  { key: 'actions', label: t('admin.modelIntros.columns.actions') }
])

function hasAnyParam(m: Record<string, unknown>): boolean {
  if (!m) return false
  for (const _ in m) return true
  return false
}

// formatParamValue：展示层用。新格式支持三种 shape：
//   - 叶子 { value, ... }：取 value 展示
//   - object { properties: {...} }：展示为 "{...}" 摘要（含子字段数量）
//   - array { items: {...} }：展示为 "[...]" 摘要
// 其它非结构化值原样展示。
function formatParamValue(v: unknown): string {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    const r = v as Record<string, unknown>
    if ('properties' in r) {
      const props = r.properties as Record<string, unknown> | undefined
      const n = props && typeof props === 'object' ? Object.keys(props).length : 0
      return `{…${n}}`
    }
    if ('items' in r) {
      return '[…]'
    }
    if ('value' in r || 'required' in r || 'description' in r || 'enum' in r || 'options' in r) {
      const inner = r.value
      if (inner === null || inner === undefined) return ''
      if (typeof inner === 'string') return inner
      if (typeof inner === 'number' || typeof inner === 'boolean') return String(inner)
      try {
        return JSON.stringify(inner)
      } catch {
        return ''
      }
    }
  }
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return ''
  }
}

function resetForm() {
  form.model_key = ''
  form.title = ''
  form.description = ''
  form.description_en = ''
  form.cover_url = ''
  form.sort_order = 0
  form.enabled = true
  form.outputFields = []
  form.params = []
  form.result_field = ''
  form.result_type = 'video'
}

// paramsFromMap / paramsToMap 现由 './paramSchemaRow' 中的 mapToRows / rowsToMap
// 递归实现。为最小化本文件的语义漂移，这里封装两个薄适配函数，命名保持原样。
function paramsFromMap(m: Record<string, unknown>): SchemaRow[] {
  return mapToRows(m)
}
function paramsToMap(paramRows: SchemaRow[]): Record<string, unknown> {
  return rowsToMap(paramRows)
}

// 按字段的类型把枚举 option 字符串还原为合适的 JS 值。
// 现在输入参数枚举由 SchemaRow (paramSchemaRow) 内部处理；此函数仅保留给
// 输出参数编辑器的 schemaRowsToOutputFields 使用。
function coerceEnumOption(s: string, type: 'string' | 'number' | 'boolean'): unknown {
  switch (type) {
    case 'number': {
      const n = Number(s)
      return Number.isFinite(n) ? n : s
    }
    case 'boolean': {
      const low = s.toLowerCase()
      if (low === 'true') return true
      if (low === 'false') return false
      return s
    }
    case 'string':
    default:
      return s
  }
}

async function loadList() {
  loading.value = true
  try {
    const resp = await adminAPI.modelIntros.list(
      pagination.page,
      pagination.page_size,
      searchKeyword.value.trim()
    )
    rows.value = resp.items ?? []
    pagination.total = resp.total ?? 0
  } catch (_e) {
    appStore.showError(t('admin.modelIntros.loadFailed'))
  } finally {
    loading.value = false
  }
}

function onSearch() {
  pagination.page = 1
  loadList()
}

function openCreateDialog() {
  resetForm()
  editingKey.value = null
  showFormDialog.value = true
  ensureCandidatesLoaded()
  // 每次打开编辑弹窗都对"当前选中的翻译 key"刷新一次可用模型列表；
  // 有本地缓存时用户不会看到闪烁，请求返回后无声更新。
  refreshTranslationModels()
}

function openEditDialog(row: ModelIntro) {
  editingKey.value = row.model_key
  form.model_key = row.model_key
  form.title = row.title || ''
  form.description = row.description || ''
  form.description_en = row.description_en || ''
  form.cover_url = row.cover_url || ''
  form.sort_order = row.sort_order || 0
  form.enabled = !!row.enabled
        form.outputFields = outputFieldsToSchemaRows(row.output_fields)
  form.result_field = row.result_field || ''
  form.result_type = (row.result_type as ResultMediaType) || 'video'
  form.params = paramsFromMap(row.default_params || {})
  showFormDialog.value = true
  ensureCandidatesLoaded()
  // 同 openCreateDialog：进入编辑页面刷新一次可用模型列表。
  refreshTranslationModels()
}

function openDetailDialog(row: ModelIntro) {
  detailRow.value = row
  showDetailDialog.value = true
}

function openEditFromDetail() {
  if (!detailRow.value) return
  const row = detailRow.value
  showDetailDialog.value = false
  openEditDialog(row)
}

async function ensureCandidatesLoaded() {
  if (candidates.value.length > 0 || candidatesLoading.value) return
  candidatesLoading.value = true
  try {
    const resp = await adminAPI.modelIntros.listCandidates()
    candidates.value = resp.items ?? []
  } catch {
    // 候选获取失败不阻断手写，静默降级。
    candidates.value = []
  } finally {
    candidatesLoading.value = false
  }
}

// addParam：追加一条空的顶层输入参数行（默认 type=string）。
function addParam() {
  form.params.push(makeSchemaRow({ key: '', type: 'string' }))
}

// onParamRowUpdate：ParamSchemaEditor emit('update:modelValue') 的接收器。
// 由于 SchemaRow 通过引用传入子组件，子组件的所有修改已直接反映在数组元素上，
// 这里只需要触发 Vue 的响应式感知（触发引用相等的重新赋值即可）。
function onParamRowUpdate(idx: number, next: SchemaRow) {
  form.params[idx] = next
}

// ============ output_fields 辅助 ============
// 输出参数改造后与输入参数完全同型（SchemaRow[] ↔ OutputFieldSpec[]），
// 编辑器 UI 也直接复用 ParamSchemaEditor。填写体验、字段能力、嵌套支持全对齐。

// onOutputRowUpdate：ParamSchemaEditor emit('update:modelValue') 的接收器。
// 与 onParamRowUpdate 语义一致：SchemaRow 是引用传递，子组件的修改已直接反映
// 在数组元素上，这里显式赋值以触发 Vue 响应式感知。
function onOutputRowUpdate(idx: number, next: SchemaRow) {
  form.outputFields[idx] = next
}

// removeParam：删除一条输入参数行。输入参数编辑器 ParamSchemaEditor 通过
// @remove 事件回调此函数，直接从 form.params 中移除该行。
function removeParam(idx: number) {
  form.params.splice(idx, 1)
}

/**
 * moveParam：把 form.params[i] 与 form.params[i+dir] 交换，用于顶层输入参数
 * 上下箭头 (@move-up / @move-down) 触发的相邻 swap。dir = -1 向上 / +1 向下；
 * 越界时静默不动作。拖拽重排走 VueDraggable v-model 自动更新，这里不负责。
 */
function moveParam(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= form.params.length) return
  const tmp = form.params[i]
  form.params[i] = form.params[j]
  form.params[j] = tmp
}

// 添加/删除一条输出字段声明；默认类型为 string，与输入参数保持一致。
function addOutputField() {
  form.outputFields.push(makeSchemaRow({ key: '', type: 'string' }))
}

function removeOutputField(idx: number) {
  form.outputFields.splice(idx, 1)
  // 删除后若 result_field 指向已不存在的路径，自动清空，避免提交时后端报错。
  // 现在 result_field 可能是深路径（如 data.video.url / images[*].url），
  // 因此需要在展开后的多级候选列表中查找，而不是只对比顶层 key。
  if (
    form.result_field &&
    !resultFieldOptions.value.some((o) => o.key === form.result_field)
  ) {
    form.result_field = ''
  }
}

// outputFieldsToSchemaRows：把后端 OutputFieldSpec[] 反解为编辑器 SchemaRow[]。
//
// 每个 OutputFieldSpec 直接对应一个 SchemaRow：
//   - object 类型：读取 spec.properties（键=子字段名，值=递归 schema），
//     对每个键调用 schemaToRow 得到子 SchemaRow，装进 children。若 properties
//     缺失（老数据），退化为空 children。
//   - array 类型：读取 spec.items（递归 schema），调用 schemaToRow 得到 items
//     SchemaRow；若缺失则退化为一份默认 string items。
//   - 叶子类型：value 使用 spec.default 当作示例默认；boolean 特殊处理。
//
// 兼容旧数据（Q5=B 不做过多兼容，但为避免管理员打开旧记录直接看到空白，做一次
// 类型归一：非新白名单的一律回落为 'string'，管理员保存一次即完成升级）。
function outputFieldsToSchemaRows(fields: OutputFieldSpec[] | null | undefined): SchemaRow[] {
  if (!Array.isArray(fields)) return []
  return fields.map((f) => outputFieldToSchemaRow(f))
}

function outputFieldToSchemaRow(f: OutputFieldSpec): SchemaRow {
  const type = coerceOutputSchemaType(f.type)
  // 枚举选项统一序列化为可展示的字符串行（object/array 用 JSON.stringify）。
  const opts = Array.isArray(f.options) ? f.options : []
  const optionsText = opts
    .map((o) =>
      typeof o === 'string'
        ? o
        : (() => {
            try {
              return JSON.stringify(o)
            } catch {
              return String(o)
            }
          })()
    )
    .join('\n')

  if (type === 'object') {
    // properties 递归还原：与 default_params 里 object schema 的 shape 完全一致，
    // 直接借用 schemaToRow(childKey, childSpec) 得到子 SchemaRow。
    const children: SchemaRow[] = []
    const props =
      f.properties && typeof f.properties === 'object' && !Array.isArray(f.properties)
        ? (f.properties as Record<string, unknown>)
        : null
    if (props) {
      for (const ck of Object.keys(props)) {
        children.push(schemaToRow(ck, props[ck]))
      }
    }
    return makeSchemaRow({
      key: f.key || '',
      type: 'object',
      required: f.required === true,
      description: f.description || '',
      children,
    })
  }
  if (type === 'array') {
    // items 递归还原；若后端未提供 items（老数据或首次保存前），给一份默认 string 元素。
    const rawItems = (f as OutputFieldSpec).items
    const items =
      rawItems !== undefined && rawItems !== null
        ? schemaToRow('', rawItems)
        : makeSchemaRow({ key: '', type: 'string' })
    // items.key 强制为空（数组元素无名）。
    items.key = ''
    return makeSchemaRow({
      key: f.key || '',
      type: 'array',
      required: f.required === true,
      description: f.description || '',
      items,
    })
  }
  // 叶子（string / number / boolean）：value 来自 spec.default（若有）。
  const rawDefault = typeof f.default === 'string' ? f.default : ''
  let value = ''
  let boolValue = false
  if (type === 'boolean') {
    const low = rawDefault.trim().toLowerCase()
    boolValue = low === 'true' || low === '1' || low === 'yes'
  } else {
    value = rawDefault
  }
  return makeSchemaRow({
    key: f.key || '',
    type,
    value,
    boolValue,
    required: f.required === true,
    description: f.description || '',
    isEnum: f.enum === true,
    optionsText,
  })
}

// coerceOutputSchemaType：把任意 type 字符串归一到新的 JSON Schema 白名单。
// 非合法值一律回落为 'string'。
function coerceOutputSchemaType(t: string | undefined): SchemaRow['type'] {
  const low = String(t || '').toLowerCase()
  switch (low) {
    case 'string':
    case 'number':
    case 'boolean':
    case 'object':
    case 'array':
      return low
    default:
      return 'string'
  }
}

// schemaRowsToOutputFields：编辑器 SchemaRow[] 写侧归一为 OutputFieldSpec[]。
//
// 每个顶层 SchemaRow 直接映射为一条 OutputFieldSpec：
//   - key 为空的行跳过（不进入 payload）；
//   - object：递归 rowToSchema 得到 { properties, description?, required? }，
//     把 properties 挂到 OutputFieldSpec.properties 上；子字段的所有元数据
//     （包括嵌套 description / required / enum / options 等）由 rowToSchema
//     完整序列化，不会丢失。
//   - array：递归 rowToSchema(items) 得到嵌套 schema，挂到 OutputFieldSpec.items 上。
//   - 叶子行的 value 存到 default 字段作为示例（boolean 存 'true'/'false'）。
//
// required / enum / options 采用 omitempty 语义（false / 空数组不写入）。
function schemaRowsToOutputFields(rows: SchemaRow[]): OutputFieldSpec[] {
  const out: OutputFieldSpec[] = []
  for (const row of rows) {
    const key = (row.key || '').trim()
    if (!key) continue
    const type = coerceOutputSchemaType(row.type)

    const spec: OutputFieldSpec = {
      key,
      type,
      description: (row.description || '').trim(),
    }
    if (row.required) spec.required = true

    if (type === 'object') {
      // rowToSchema 会返回 { properties, required?, description? }；
      // 我们只需其中的 properties 挂到 spec 上（顶层 required/description 已单独处理）。
      const schema = rowToSchema(row)
      const props =
        schema.properties && typeof schema.properties === 'object'
          ? (schema.properties as Record<string, unknown>)
          : {}
      spec.properties = props
      out.push(spec)
      continue
    }
    if (type === 'array') {
      // rowToSchema 会返回 { items, required?, description? }；取 items。
      const schema = rowToSchema(row)
      spec.items = schema.items
      out.push(spec)
      continue
    }

    // 叶子：计算 default（仅 string/number/boolean 有意义）。
    let defaultStr = ''
    if (type === 'boolean') {
      defaultStr = row.boolValue ? 'true' : 'false'
    } else if (type === 'string' || type === 'number') {
      defaultStr = (row.value || '').trim()
    }
    if (defaultStr) spec.default = defaultStr
    if (row.isEnum && (type === 'string' || type === 'number' || type === 'boolean')) {
      spec.enum = true
      const opts = row.optionsText
        .split(/[\n,，、;；|\t]+/)
        .map((s) => s.trim())
        .filter((s) => s.length > 0)
        .map((s) => coerceEnumOption(s, type === 'boolean' ? 'string' : type))
      if (opts.length > 0) spec.options = opts
    }
    out.push(spec)
  }
  return out
}

// ============ Schema 导入导出 ============
// buildSchemaExport：把当前编辑区（输入参数 / 输出参数 / 主结果指示器）序列化
// 为一份可分享的 JSON。shape 与后端存储一致（Q3=A）。
function buildSchemaExport(): Record<string, unknown> {
  return {
    default_params: paramsToMap(form.params),
    output_fields: schemaRowsToOutputFields(form.outputFields),
    result_field: (form.result_field || '').trim(),
    result_type: form.result_type === 'image' ? 'image' : 'video'
  }
}

// downloadSchema：下载导出内容为 .json 文件（导出方式 B）。
function downloadSchema() {
  const data = buildSchemaExport()
  const json = JSON.stringify(data, null, 2)
  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  const keyPart = (form.model_key || 'model-intro').replace(/[^a-zA-Z0-9._-]+/g, '_')
  a.download = `${keyPart}.schema.json`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  appStore.showSuccess(t('admin.modelIntros.fields.schemaExportedFile'))
}

// copySchemaToClipboard：复制到剪贴板（导出方式 C）。失败时降级到 execCommand。
async function copySchemaToClipboard() {
  const json = JSON.stringify(buildSchemaExport(), null, 2)
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(json)
    } else {
      const ta = document.createElement('textarea')
      ta.value = json
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    appStore.showSuccess(t('admin.modelIntros.fields.schemaCopied'))
  } catch {
    appStore.showError(t('admin.modelIntros.fields.schemaCopyFailed'))
  }
}

// openImportDialog / closeImportDialog：管理导入弹窗；打开时清空旧内容。
function openImportDialog() {
  importText.value = JSON.stringify(buildSchemaExport(), null, 2)
  importError.value = ''
  showImportDialog.value = true
}

// pickImportFile：点击"从文件导入"按钮时打开原生 file picker。文件读到内容后
// 塞进 textarea，让用户再次确认后再点"应用"，避免误覆盖。
function pickImportFile() {
  fileInputRef.value?.click()
}

function onImportFileChange(evt: Event) {
  const input = evt.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    importText.value = String(reader.result ?? '')
    importError.value = ''
  }
  reader.onerror = () => {
    importError.value = t('admin.modelIntros.fields.schemaImportReadFailed')
  }
  reader.readAsText(file)
  // 允许连续选同一个文件：清掉 value。
  if (input) input.value = ''
}

// applyImport：解析并覆盖当前 form 的 Schema 段。宽容处理：
//   - 允许 default_params 为空对象（清空输入参数）
//   - 允许 output_fields 为空数组（清空输出参数）
//   - result_type 未指定时默认 video
//   - result_field 未匹配到 output_fields 时静默清空（不阻断导入）
function applyImport() {
  const raw = importText.value.trim()
  if (!raw) {
    importError.value = t('admin.modelIntros.fields.schemaImportEmpty')
    return
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message || 'invalid JSON'
    importError.value = t('admin.modelIntros.fields.schemaImportParseFailed', { msg })
    return
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    importError.value = t('admin.modelIntros.fields.schemaImportShape')
    return
  }
  const obj = parsed as Record<string, unknown>
  // default_params
  const dp = obj.default_params
  if (dp !== undefined && (dp === null || typeof dp !== 'object' || Array.isArray(dp))) {
    importError.value = t('admin.modelIntros.fields.schemaImportShape')
    return
  }
  form.params = paramsFromMap((dp as Record<string, unknown>) || {})
  // output_fields
  const of = obj.output_fields
  if (of !== undefined && !Array.isArray(of)) {
    importError.value = t('admin.modelIntros.fields.schemaImportShape')
    return
  }
        form.outputFields = outputFieldsToSchemaRows((of as OutputFieldSpec[]) || [])
  // result_field / result_type
  form.result_field = typeof obj.result_field === 'string' ? obj.result_field.trim() : ''
  const rt = typeof obj.result_type === 'string' ? obj.result_type.trim().toLowerCase() : ''
  form.result_type = rt === 'image' ? 'image' : 'video'
  // 校验 result_field 存在性：现在支持多级路径，需在完整展开的候选列表中查找。
  if (
    form.result_field &&
    !resultFieldOptions.value.some((o) => o.key === form.result_field)
  ) {
    form.result_field = ''
  }
  showImportDialog.value = false
  appStore.showSuccess(t('admin.modelIntros.fields.schemaImportedSuccess'))
}

// outputTypeBadgeClass 返回列表/详情中 type 徽章的 tailwind class，
// 按 JSON Schema 类型分组着色；同时兼容旧数据里可能残留的 video/image/url/text/json/number。
function outputTypeBadgeClass(t: string): string {
  switch (t) {
    case 'object':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'array':
      return 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
    case 'number':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'boolean':
      return 'bg-pink-100 text-pink-700 dark:bg-pink-900/40 dark:text-pink-300'
    case 'string':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    // ↓ 兼容旧数据（不再是新白名单，但列表仍可能读到）
    case 'video':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'image':
      return 'bg-pink-100 text-pink-700 dark:bg-pink-900/40 dark:text-pink-300'
    case 'url':
      return 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/40 dark:text-cyan-300'
    case 'json':
      return 'bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-200'
    case 'text':
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-200'
  }
}

function onCoverError(evt: Event) {
  const img = evt.target as HTMLImageElement | null
  if (img) img.style.display = 'none'
}

async function submitForm() {
  const key = form.model_key.trim()
  if (!key) {
    appStore.showError(t('admin.modelIntros.errors.modelKeyRequired'))
    return
  }
  // 方案 C 下输入参数使用 ParamSchemaEditor 递归编辑，天然保证合法 JSON，
  // 不再需要额外的 JSON 校验。
  const payload: UpsertModelIntroRequest = {
    model_key: key,
    title: form.title.trim(),
    description: form.description,
    description_en: form.description_en,
    cover_url: form.cover_url.trim(),
    default_params: paramsToMap(form.params),
    sort_order: Number.isFinite(form.sort_order) ? Number(form.sort_order) : 0,
    enabled: !!form.enabled,
    output_fields: schemaRowsToOutputFields(form.outputFields),
    result_field: (form.result_field || '').trim(),
    result_type: form.result_type === 'image' ? 'image' : 'video'
  }
  submitting.value = true
  try {
    if (editingKey.value === null) {
      await adminAPI.modelIntros.create(payload)
      appStore.showSuccess(t('admin.modelIntros.created'))
    } else {
      await adminAPI.modelIntros.update(editingKey.value, payload)
      appStore.showSuccess(t('admin.modelIntros.updated'))
    }
    showFormDialog.value = false
    await loadList()
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message ?? t('admin.modelIntros.saveFailed')
    appStore.showError(msg)
  } finally {
    submitting.value = false
  }
}

function askDelete(row: ModelIntro) {
  deletingKey.value = row.model_key
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (deletingKey.value === null) return
  try {
    await adminAPI.modelIntros.delete(deletingKey.value)
    appStore.showSuccess(t('admin.modelIntros.deleted'))
    showDeleteDialog.value = false
    deletingKey.value = null
    await loadList()
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message ?? t('admin.modelIntros.deleteFailed')
    appStore.showError(msg)
  }
}

onMounted(() => {
  loadList()
  // 预加载候选，避免用户点开弹窗时才开始拉取。
  ensureCandidatesLoaded()
  // 预加载"翻译工具"用到的 API Keys；同时恢复 localStorage 里的上次选择。
  // 页面级预拉，避免管理员打开编辑 dialog 那一刻还在 loading，翻译按钮先禁用几秒钟。
  loadTranslationKeys()
})
</script>
