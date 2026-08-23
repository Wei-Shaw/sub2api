<!--
  ParamSchemaEditor：递归"参数 Schema"编辑器。

  一个节点（SchemaRow）描述一个字段声明：
    - key         字段名（顶层来自 default_params 的 map key；object 子字段用 properties 的 key；array 元素为空串）
    - type        叶子: 'string' | 'number' | 'boolean' ；复合: 'object' | 'array'
    - required / description / enum / options  与外层保持同一套语义
    - 叶子：value (string) / boolValue (boolean)
    - object：children 递归（每个 child 又是一个 SchemaRow）
    - array：items 单个 SchemaRow（数组同构；items.key 恒为空）

  组件通过 defineOptions({ name: 'ParamSchemaEditor' }) 自引用，实现对 object / array
  子字段递归渲染。数据由父组件通过 v-model 传入；这里所有 UI 直接改动 SchemaRow 内部
  字段，然后 emit('update:modelValue', node) 让父层感知（通过 deep watch 即可）。

  ============================================================
  字段排序（v-drag + 上下箭头，两套并存）：
    - 本组件不负责"当前节点自己在父层数组里的位置"，只 emit 'move-up' / 'move-down'
      让父组件对相邻两个 SchemaRow 做 swap。同时暴露 canMoveUp / canMoveDown 两个
      prop，让父组件控制首行禁用 ↑、末行禁用 ↓。
    - 对 object.children：本组件内直接 splice，同时用 VueDraggable 包裹整个子行
      容器，管理员既可点按钮微调，也可拖拽一次到位。
    - array.items 只有一份，天然无法排序，不显示排序控件。
    - 顶层排序由父层 AdminModelIntrosView 用同一套 VueDraggable + 按钮驱动。
-->
<template>
  <div class="param-schema-editor space-y-2">
    <!-- ============================================================
         第一行：拖拽把手 / 上下箭头 / 字段名 / 类型 /（叶子）默认值 / 删除按钮
    ============================================================ -->
    <div class="flex flex-wrap items-end gap-2">
      <!--
        排序把手 + ↑ / ↓ 按钮。
        - 当 canMoveUp / canMoveDown 都为 false（顶层某轴上就一行 / array items）时，
          整块儿隐藏，避免占位造成第一行"名称"跳位不齐。
        - drag-handle class 与 VueDraggable 的 handle=".drag-handle" 关联，
          鼠标按住这里即可拖动该行；父层负责套 VueDraggable 容器。
      -->
      <div
        v-if="canMoveUp || canMoveDown"
        class="flex flex-col items-center gap-0.5 self-end pb-1"
      >
        <span
          class="drag-handle cursor-grab text-gray-300 hover:text-gray-500 active:cursor-grabbing dark:text-dark-600 dark:hover:text-dark-400"
          :title="t('admin.modelIntros.fields.dragToReorder')"
        >
          <svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4z"/>
          </svg>
        </span>
        <div class="flex flex-col">
          <button
            type="button"
            class="px-0.5 text-gray-400 hover:text-gray-700 disabled:opacity-30 disabled:hover:text-gray-400 dark:hover:text-gray-200"
            :disabled="!canMoveUp"
            :title="t('admin.modelIntros.fields.moveUp')"
            @click="$emit('move-up')"
          >
            <svg class="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M10 5a1 1 0 0 1 .78.375l4 5a1 1 0 1 1-1.56 1.25L10 7.6l-3.22 4.025a1 1 0 1 1-1.56-1.25l4-5A1 1 0 0 1 10 5z" clip-rule="evenodd"/>
            </svg>
          </button>
          <button
            type="button"
            class="px-0.5 text-gray-400 hover:text-gray-700 disabled:opacity-30 disabled:hover:text-gray-400 dark:hover:text-gray-200"
            :disabled="!canMoveDown"
            :title="t('admin.modelIntros.fields.moveDown')"
            @click="$emit('move-down')"
          >
            <svg class="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M10 15a1 1 0 0 1-.78-.375l-4-5a1 1 0 1 1 1.56-1.25L10 12.4l3.22-4.025a1 1 0 1 1 1.56 1.25l-4 5A1 1 0 0 1 10 15z" clip-rule="evenodd"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 名称：数组元素时禁用 key 编辑（显示 index），其他情况正常输入。
           当外部通过 hideKey 声明"根节点无对外字段名"时，整列不渲染。 -->
      <div v-if="!hideKey" class="flex w-40 flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ isArrayItem ? t('common.index') : t('admin.modelIntros.fields.labelKey') }}
        </label>
        <div
          v-if="isArrayItem"
          class="flex h-8 items-center rounded-xl border border-dashed border-gray-300 px-3 font-mono text-xs text-gray-500 dark:border-dark-600"
        >
          [{{ arrayIndex ?? 0 }}]
        </div>
        <input
          v-else
          v-model="node.key"
          type="text"
          class="input h-8 text-xs"
          :placeholder="t('admin.modelIntros.fields.paramKey')"
          @input="emitChange"
        />
      </div>
      <!-- 类型下拉 -->
      <div class="flex w-32 flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelType') }}
        </label>
        <!--
          用一个 wrapper div 包住 Select，配合 <style scoped> 里的 :deep(.select-trigger-sm)
          把 sm 尺寸的实际高度强制到 h-8（32px），与同一行的 <input class="input h-8"> 视觉对齐。
          直接改 Select.vue 会影响全站，所以只在本组件内 override。
        -->
        <div class="param-select-wrapper">
          <Select
            :model-value="node.type"
            :options="typeOptions"
            :searchable="false"
            size="sm"
            @update:modelValue="(v: string | number | boolean | null) => onTypeChange(v)"
          />
        </div>
      </div>
      <!-- 叶子：默认值
           这里的输入框只用于管理端**输入默认值**，与"演练页最终展示为 input 还是
           textarea"是两回事：widget 只影响演练页的最终渲染控件，不影响编辑器里
           这个默认值输入框，故此处恒为单行 input（多行内容也支持粘贴保存）。 -->
      <div
        v-if="(node.type === 'string' || node.type === 'number') && !isArrayItem"
        class="flex flex-1 min-w-[10rem] flex-col gap-1"
      >
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelDefault') }}
        </label>
        <input
          v-model="node.value"
          type="text"
          class="input h-8 text-xs"
          :placeholder="t('admin.modelIntros.fields.paramValue')"
          @input="emitChange"
        />
      </div>
      <!-- 控件类型（widget）
           - type=string：input / textarea / image
           - type=array ：input / ImageUrls / VideoUrls / AudioUrls
           其它类型（number / boolean / object）没有可选控件形态，这一列被隐藏。 -->
      <div v-if="canPickWidget" class="flex w-36 flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelWidget') }}
        </label>
        <div class="param-select-wrapper">
          <Select
            :model-value="node.widget"
            :options="widgetOptions"
            :searchable="false"
            size="sm"
            @update:modelValue="(v: string | number | boolean | null) => onWidgetChange(v)"
          />
        </div>
      </div>
      <!-- string + textarea 专属：行数（仅 textarea 时显示） -->
      <div
        v-if="node.type === 'string' && (node.widget === 'textarea' || node.widget === 'PromptTextArea')"
        class="flex w-24 flex-col gap-1"
      >
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelTextareaRows') }}
        </label>
        <input
          v-model.number="node.textareaRows"
          type="number"
          min="1"
          max="100"
          class="input h-8 text-xs"
          @input="emitChange"
        />
      </div>
      <!-- string 专属：输出值最大字符数；留空/0 表示不限制。 -->
      <div v-if="allowMaxChars && node.type === 'string'" class="flex w-28 flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelMaxChars') }}
        </label>
        <input
          v-model.number="node.maxChars"
          type="number"
          min="0"
          class="input h-8 text-xs"
          :placeholder="t('admin.modelIntros.fields.maxCharsUnlimited')"
          :title="t('admin.modelIntros.fields.maxCharsHint')"
          @input="onMaxCharsChange"
        />
      </div>
      <div
        v-if="node.type === 'string' && node.widget === 'PromptTextArea'"
        class="flex min-w-64 flex-1 flex-col gap-1"
      >
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelReferenceFields') }}
        </label>
        <div class="flex min-h-8 flex-wrap gap-2 rounded-xl border border-gray-300 bg-white px-2 py-1.5 dark:border-dark-600 dark:bg-dark-800">
          <label v-for="field in referenceFieldOptions" :key="field" class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-700 dark:text-gray-200">
            <input type="checkbox" :checked="node.referenceFields.includes(field)" @change="toggleReferenceField(field)" />
            <span class="font-mono">{{ field }}</span>
          </label>
          <span v-if="referenceFieldOptions.length === 0" class="text-xs text-gray-400">{{ t('admin.modelIntros.fields.noReferenceFields') }}</span>
        </div>
      </div>
      <!-- array 专属：元素个数上限（0 / 留空 = 不限制） -->
      <div v-if="node.type === 'array'" class="flex w-28 flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelMaxItems') }}
        </label>
        <input
          v-model.number="node.maxItems"
          type="number"
          min="0"
          max="100"
          class="input h-8 text-xs"
          :placeholder="t('admin.modelIntros.fields.maxItemsUnlimited')"
          :title="t('admin.modelIntros.fields.maxItemsHint')"
          @input="onMaxItemsChange"
        />
      </div>
      <div v-else-if="node.type === 'boolean' && !isArrayItem" class="flex flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelDefault') }}
        </label>
        <label
          class="flex h-8 items-center gap-2 rounded-xl border border-gray-200 px-3 dark:border-dark-600"
        >
          <input
            v-model="node.boolValue"
            type="checkbox"
            class="h-4 w-4"
            @change="emitChange"
          />
          <span class="text-xs text-gray-500">{{ node.boolValue ? 'true' : 'false' }}</span>
        </label>
      </div>
      <!-- 删除按钮（顶层 / 子行都可删；数组同构下唯一一份 items 不能删） -->
      <button
        v-if="removable"
        type="button"
        class="btn btn-ghost btn-xs ml-auto self-end text-red-500"
        :title="t('common.remove')"
        @click="$emit('remove')"
      >
        {{ t('common.remove') }}
      </button>
    </div>

    <!-- ============================================================
         第二行：required + description
         required 使用项目通用 Toggle 控件（与全站其它开关风格一致）。
         array 顶层 items 只定义同构元素类型，不是一个具名请求字段，因此不显示
         required / advanced / description；对应的多个默认值在父 array 区域内添加。

         **顺序说明**：description / required 放在"嵌套展开区之前"，
         紧贴第一行 key/type；否则 object/array 展开的子字段列表会把
         description 挤到很下面，导致用户误以为"复合类型无法填描述"。
    ============================================================ -->
    <div v-if="!isArrayItem" class="space-y-1.5 pl-1">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div class="flex items-center gap-2">
          <Toggle v-model="node.required" @update:modelValue="emitChange" />
          <span class="text-xs text-gray-600 dark:text-gray-400">
            {{ t('admin.modelIntros.fields.paramRequired') }}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <Toggle v-model="node.advanced" @update:modelValue="emitChange" />
          <span class="text-xs text-gray-600 dark:text-gray-400">
            {{ t('admin.modelIntros.fields.paramAdvanced') }}
          </span>
        </div>
      </div>
      <!-- 字段说明：中英双文
           两个 textarea 并排，方便管理员维护中英两份文案；渲染层根据
           当前 i18n locale 选择合适语种，缺失时相互兜底不为空即用。
           每个 label 右侧带一个"翻译"按钮：读另一语言字段作为源，通过
           useDescriptionTranslation 上下文调用大模型翻译并写回本字段。
           翻译上下文由页面顶层（AdminModelIntrosView）provide；上下文缺失
           或未选 apiKey/model 时按钮禁用，避免误点。 -->
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <div class="flex flex-col gap-1">
          <div class="flex items-center justify-between gap-2">
            <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.modelIntros.fields.labelDescription') }}
              <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ t('admin.modelIntros.fields.labelDescriptionZh') }}
              </span>
            </label>
            <button
              v-if="translation"
              type="button"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-primary-600 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-400 dark:hover:bg-primary-950"
              :disabled="!translation.ready.value || !(node.descriptionEn ?? '').trim() || translatingZh"
              :title="translateBtnTitle('zh')"
              @click="onTranslate('zh')"
            >
              <svg v-if="translatingZh" class="h-3 w-3 animate-spin" fill="none" viewBox="0 0 24 24">
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
            v-model="node.description"
            rows="2"
            class="input text-xs"
            :placeholder="t('admin.modelIntros.fields.paramDescription')"
            @input="emitChange"
          />
        </div>
        <div class="flex flex-col gap-1">
          <div class="flex items-center justify-between gap-2">
            <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.modelIntros.fields.labelDescription') }}
              <span class="ml-1 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ t('admin.modelIntros.fields.labelDescriptionEn') }}
              </span>
            </label>
            <button
              v-if="translation"
              type="button"
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-primary-600 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-400 dark:hover:bg-primary-950"
              :disabled="!translation.ready.value || !(node.description ?? '').trim() || translatingEn"
              :title="translateBtnTitle('en')"
              @click="onTranslate('en')"
            >
              <svg v-if="translatingEn" class="h-3 w-3 animate-spin" fill="none" viewBox="0 0 24 24">
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
            v-model="node.descriptionEn"
            rows="2"
            class="input text-xs"
            :placeholder="t('admin.modelIntros.fields.paramDescriptionEn')"
            @input="emitChange"
          />
        </div>
      </div>
    </div>

    <!-- ============================================================
         嵌套 schema 展开区：**放在 description/required 之后**渲染。
         左侧一条彩色竖条 + 左内边距，使层级越深越向右缩进、色条累积，
         直观表达"这是一个嵌套结构"。
           - object：蓝色（primary）竖条
           - array ：紫色（violet）竖条
    ============================================================ -->
    <!-- object 分支：递归展开 children（可拖拽 + 上下箭头） -->
    <div
      v-if="node.type === 'object'"
      class="nested-block nested-block--object mt-2"
    >
      <div class="mb-2 flex items-center justify-between">
        <span class="font-mono text-[11px] text-gray-500">
          {{ '{' }} object · {{ node.children.length }} {{ '}' }}
        </span>
        <button type="button" class="btn btn-secondary btn-xs" @click="addChild">
          {{ t('admin.modelIntros.fields.addParam') }}
        </button>
      </div>
      <div v-if="node.children.length === 0" class="pl-1 text-[11px] text-gray-400">
        {{ t('admin.modelIntros.fields.defaultParamsEmpty') }}
      </div>
      <VueDraggable
        v-else
        v-model="node.children"
        :animation="200"
        handle=".drag-handle"
        class="space-y-3"
        @end="emitChange"
      >
        <div
          v-for="(child, i) in node.children"
          :key="child.uid"
          class="rounded border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800"
        >
          <ParamSchemaEditor
            :model-value="child"
            :removable="true"
            :allow-array-defaults="allowArrayDefaults"
            :allow-max-chars="allowMaxChars"
            :reference-field-options="referenceFieldOptions"
            :can-move-up="i > 0"
            :can-move-down="i < node.children.length - 1"
            @update:modelValue="onChildUpdate(i, $event)"
            @remove="removeChild(i)"
            @move-up="moveChild(i, -1)"
            @move-down="moveChild(i, 1)"
          />
        </div>
      </VueDraggable>
    </div>

    <!-- array 分支：单个 items schema（数组同构，无法排序）
         媒体 URL 组控件的元素形态被固定为"完整 URL 字符串"，
         再让管理员编辑 items schema 只会带来误配置，因此换成一行说明。 -->
    <div
      v-if="node.type === 'array'"
      class="nested-block nested-block--array mt-2"
    >
      <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-[11px] text-gray-500">
          [ array items<template v-if="allowArrayDefaults"> · {{ node.arrayDefaults.length }}</template> ]
        </span>
        <div class="flex items-center gap-2">
          <span v-if="node.maxItems > 0" class="font-mono text-[11px] text-gray-400">
            max {{ node.maxItems }}
          </span>
          <button
            v-if="allowArrayDefaults"
            type="button"
            class="btn btn-secondary btn-xs"
            :disabled="!canAddArrayDefault"
            @click="addArrayDefault"
          >
            <Icon name="plus" size="xs" />
            {{ t('admin.modelIntros.fields.addArrayItem') }}
          </button>
        </div>
      </div>
      <div
        v-if="mediaUrlWidget"
        class="rounded border border-dashed border-violet-300 bg-violet-50/50 p-2 text-[11px] leading-relaxed text-violet-800 dark:border-violet-800 dark:bg-violet-900/10 dark:text-violet-200"
      >
        {{ t('admin.modelIntros.fields.mediaUrlsItemsFixed', { widget: mediaUrlWidget }) }}
      </div>
      <div v-else class="rounded border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800">
        <ParamSchemaEditor
          v-if="node.items"
          :model-value="node.items"
          :removable="false"
          :is-array-item="true"
          :array-index="0"
          :allow-array-defaults="allowArrayDefaults"
          :allow-max-chars="allowMaxChars"
          :reference-field-options="referenceFieldOptions"
          @update:modelValue="onItemsUpdate"
        />
      </div>
      <div v-if="allowArrayDefaults" class="mt-2 space-y-2">
        <p v-if="node.arrayDefaults.length === 0" class="text-[11px] text-gray-400">
          {{ t('admin.modelIntros.fields.arrayItemsEmpty') }}
        </p>
        <VueDraggable
          v-else
          v-model="node.arrayDefaults"
          :animation="200"
          handle=".array-default-drag-handle"
          class="space-y-2"
          @end="emitChange"
        >
          <div
            v-for="(value, index) in node.arrayDefaults"
            :key="index"
            class="flex items-start gap-2 rounded border border-violet-200 bg-white p-2 dark:border-violet-900 dark:bg-dark-800"
          >
            <div class="mt-1 flex shrink-0 items-center gap-0.5">
              <button
                type="button"
                class="array-default-drag-handle cursor-grab p-1 text-gray-400 hover:text-gray-700 active:cursor-grabbing dark:hover:text-gray-200"
                :title="t('admin.modelIntros.fields.dragToReorder')"
              >
                <Icon name="arrowsUpDown" size="xs" />
              </button>
              <div class="flex flex-col">
                <button
                  type="button"
                  class="p-0.5 text-gray-400 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:text-gray-200"
                  :disabled="index === 0"
                  :title="t('admin.modelIntros.fields.moveUp')"
                  @click="moveArrayDefault(index, -1)"
                >
                  <Icon name="chevronUp" size="xs" />
                </button>
                <button
                  type="button"
                  class="p-0.5 text-gray-400 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:text-gray-200"
                  :disabled="index === node.arrayDefaults.length - 1"
                  :title="t('admin.modelIntros.fields.moveDown')"
                  @click="moveArrayDefault(index, 1)"
                >
                  <Icon name="chevronDown" size="xs" />
                </button>
              </div>
            </div>
            <span class="mt-2 w-8 shrink-0 font-mono text-[11px] text-gray-400">[{{ index }}]</span>
            <SchemaValueEditor
              v-if="node.items"
              class="min-w-0 flex-1"
              :schema="node.items"
              :model-value="value"
              @update:model-value="updateArrayDefault(index, $event)"
            />
            <button
              type="button"
              class="btn btn-ghost btn-xs shrink-0 text-red-500"
              :title="t('common.remove')"
              @click="removeArrayDefault(index)"
            >
              <Icon name="trash" size="xs" />
            </button>
          </div>
        </VueDraggable>
      </div>
    </div>

    <!-- ============================================================
         第三行：enum + options
         只有 string / number 才允许配置枚举（canEnum computed）：
           - boolean 只有 true/false 两个取值，本身即枚举，无需再配
           - object / array 是结构化数据，谈不上"可枚举值"
         enum 也换成 Toggle 保持整体风格统一。
    ============================================================ -->
    <div v-if="canEnum" class="space-y-2 pl-1">
      <div class="flex items-center gap-2">
        <Toggle v-model="node.isEnum" @update:modelValue="emitChange" />
        <span class="text-xs text-gray-600 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.paramEnum') }}
        </span>
      </div>
      <div v-if="node.isEnum" class="flex flex-col gap-1">
        <label class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.modelIntros.fields.labelEnumOptions') }}
        </label>
        <textarea
          v-model="node.optionsText"
          rows="2"
          class="input font-mono text-xs"
          :placeholder="t('admin.modelIntros.fields.paramEnumPlaceholder')"
          @input="emitChange"
        />
        <p class="text-xs text-gray-500">{{ t('admin.modelIntros.fields.paramEnumHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * ParamSchemaEditor 组件实现说明：
 *  - 通过 defineOptions({ name: 'ParamSchemaEditor' }) 支持模板内递归调用。
 *  - 与父组件通过 v-model 交互；子组件里所有更改都会同步到 node 上，
 *    并 emit('update:modelValue', node)，父组件的 SchemaRow 是同一引用。
 *  - 数据形状（SchemaRow）由 './paramSchemaRow.ts' 统一维护（工具函数 & 类型），
 *    这里只做 UI；`makeChild` / `resetForType` 等辅助放在 helper 文件中，避免
 *    多处重复实现，同时便于 AdminModelIntrosView 直接调用其序列化函数。
 *  - 排序：本节点自身位置由父层控制，故只 emit 'move-up' / 'move-down'；
 *    object.children 的排序（拖拽 + 上下按钮）在本组件内直接完成。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import SchemaValueEditor from './SchemaValueEditor.vue'
import {
  defaultValueForSchemaRow,
  makeSchemaRow,
  normalizeValueForSchemaRow,
  resetRowForType,
  type SchemaRow,
  type SchemaRowType,
  type SchemaWidget,
} from './paramSchemaRow'
import {
  useDescriptionTranslation,
  type TranslationLang,
} from '@/composables/useDescriptionTranslation'
import { useAppStore } from '@/stores/app'
import { normalizeMediaUrlWidget } from '@/utils/mediaUrlWidget'

defineOptions({ name: 'ParamSchemaEditor' })

const { t } = useI18n()
const appStore = useAppStore()
// translation：字段说明翻译上下文（由顶层页面 provide）。
//   - null    表示"当前页面未启用翻译能力"，模板里 v-if 隐藏两个翻译按钮；
//   - 非 null 时按钮渲染，disabled 取决于 ready（apiKey/model 是否已选）
//     以及"源语言字段是否有内容"。
const translation = useDescriptionTranslation()

const props = defineProps<{
  /** 当前编辑的 SchemaRow 节点（父传入引用；子层修改同步反映） */
  modelValue: SchemaRow
  /** 是否显示"删除"按钮（顶层或 object 子字段为 true；array items 唯一一份为 false） */
  removable?: boolean
  /** 是否是 array 的同构 items schema：隐藏字段级元数据和默认值输入。 */
  isArrayItem?: boolean
  /** array 元素时展示的 index（默认 0） */
  arrayIndex?: number
  /**
   * 是否隐藏"字段名（key）"这一整列。
   * 用于顶层"整棵 schema 的根节点"没有对外字段名的场景，例如
   * "输出参数 type=json"时，整个 default 值就是一份匿名 schema。
   */
  hideKey?: boolean
  /**
   * canMoveUp / canMoveDown：由父层根据当前节点在数组里的位置传入。
   * 首行 canMoveUp=false，末行 canMoveDown=false；两者都 false 时排序控件整块隐藏
   * （通常是 array items 独苗，或父层根本不需要重排）。
   */
  canMoveUp?: boolean
  canMoveDown?: boolean
  /** 是否允许为 array 配置请求默认值；输出 schema 关闭此能力。 */
  allowArrayDefaults?: boolean
  /** 是否允许为 string 输出字段配置最大字符数。 */
  allowMaxChars?: boolean
  /** 根参数 schema 自动计算出的可引用媒体字段路径。 */
  referenceFieldOptions?: string[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: SchemaRow): void
  (e: 'remove'): void
  (e: 'move-up'): void
  (e: 'move-down'): void
}>()

// 直接在模板里 v-model 到 node.xxx；node 就是父传入的引用。
const node = props.modelValue
if (!Array.isArray(node.arrayDefaults)) node.arrayDefaults = []

// 类型下拉候选：叶子 3 种 + 2 种复合类型。
const typeOptions: SelectOption[] = [
  { value: 'string', label: 'string' },
  { value: 'number', label: 'number' },
  { value: 'boolean', label: 'boolean' },
  { value: 'object', label: 'object' },
  { value: 'array', label: 'array' },
]

const canEnum = computed(() => node.type === 'string' || node.type === 'number')

/**
 * canPickWidget：哪些类型有"控件形态"可选。
 *   - string：input / textarea / image
 *   - array ：input / ImageUrls / VideoUrls / AudioUrls
 * number / boolean / object 没有可选形态，整列隐藏。
 */
const canPickWidget = computed(() => node.type === 'string' || node.type === 'array')
const mediaUrlWidget = computed(() => normalizeMediaUrlWidget(node.widget))
const canAddArrayDefault = computed(() =>
  node.maxItems <= 0 || node.arrayDefaults.length < node.maxItems
)

/**
 * widgetOptions：按当前类型给出可选控件形态。
 *
 * string：
 *   - input    → 单行 <input>
 *   - textarea → 多行 <textarea>（可配 rows）
 *   - image    → 单张图片输入（演练台渲染为 URL / 本地上传 / 素材库三合一）
 * array：
 *   - input     → 默认，逐元素按 items schema 递归渲染
 *   - *Urls → 整组媒体输入，值为对应媒体完整 URL 的字符串数组
 */
const widgetOptions = computed<SelectOption[]>(() => {
  if (node.type === 'array') {
    return [
      { value: 'input', label: 'input' },
      { value: 'ImageUrls', label: 'ImageUrls' },
      { value: 'VideoUrls', label: 'VideoUrls' },
      { value: 'AudioUrls', label: 'AudioUrls' },
    ]
  }
  return [
    { value: 'input', label: 'input' },
    { value: 'textarea', label: 'textarea' },
    { value: 'PromptTextArea', label: 'PromptTextArea' },
    { value: 'image', label: 'image' },
  ]
})

/**
 * emitChange：任何叶子字段变化都要通知父层重新序列化。
 * 由于 node 是父传入的引用，这里 emit 只是"戳一下"父层的 watch 触发。
 */
function emitChange() {
  emit('update:modelValue', node)
}

// ============ 字段说明翻译 ============
// 每个 SchemaRow 单独维护自己的两侧翻译 loading 状态；递归子行互不影响。
// 只有在 translation 上下文非空、目标模型可用、且源语言字段非空时按钮才能点。
const translatingZh = ref(false)
const translatingEn = ref(false)

/**
 * translateBtnTitle：按钮 hover 提示文本；分三档：
 *   - 未 provide 或 apiKey/model 未选：告诉用户去底部先选（i18n 消息）
 *   - 源字段为空：告诉用户"先填另一语言的内容"
 *   - 就绪：显示"翻译"通用提示
 * 这样即便按钮 disabled，用户 hover 也能明白为什么点不了。
 */
function translateBtnTitle(target: TranslationLang): string {
  if (!translation) return ''
  if (!translation.ready.value) {
    return t('admin.modelIntros.fields.translateNotReady')
  }
  const srcText = target === 'zh' ? (node.descriptionEn ?? '') : (node.description ?? '')
  if (!srcText.trim()) {
    return t('admin.modelIntros.fields.translateSourceEmpty')
  }
  return t('admin.modelIntros.fields.translateBtnTitle')
}

/**
 * onTranslate：点击"翻译"按钮时触发。
 *   - 从"另一语言字段"取源文本，源为空直接告警返回；
 *   - 调 translation.translate() 拿译文，成功后写入本语言字段并 emit 一次让父层落盘；
 *   - 失败时通过 appStore 弹错误提示，message 直接拼上原始错误方便管理员排障。
 * 期间对应侧的 loading flag 置 true，按钮变 spinner。
 */
async function onTranslate(target: TranslationLang) {
  if (!translation) return
  const source: TranslationLang = target === 'zh' ? 'en' : 'zh'
  const sourceText = source === 'zh' ? (node.description ?? '') : (node.descriptionEn ?? '')
  if (!sourceText.trim()) {
    appStore.showError(t('admin.modelIntros.fields.translateSourceEmpty'))
    return
  }
  const flag = target === 'zh' ? translatingZh : translatingEn
  flag.value = true
  try {
    const translated = await translation.translate(sourceText, source, target)
    if (target === 'zh') node.description = translated
    else node.descriptionEn = translated
    emitChange()
    appStore.showSuccess(t('admin.modelIntros.fields.translateSuccess'))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    appStore.showError(t('admin.modelIntros.fields.translateFailed', { msg }))
  } finally {
    flag.value = false
  }
}

/**
 * onTypeChange：Select @update:modelValue 的接收器。
 * 切换类型时清空/初始化对应字段（复用 helper 里的 resetRowForType）。
 * 当切换到不可枚举类型（boolean / object / array）时，主动关闭 isEnum
 * 并清空 optionsText，避免"上一次填过的枚举残留"在序列化时被写出。
 */
function onTypeChange(v: string | number | boolean | null) {
  const next = (v == null ? 'string' : String(v)) as SchemaRowType
  node.type = next
  resetRowForType(node)
  if (next !== 'string' && next !== 'number') {
    node.isEnum = false
    node.optionsText = ''
  }
  // widget 归位：string 与 array 的候选集互不相通，
  // 切换类型后若残留上一个类型的取值，下拉会显示一个非法选项。统一收回 'input'。
  node.widget = 'input'
  // maxItems 只对 array 有意义；切走时清零，避免序列化出无意义的上限。
  if (next !== 'array') node.maxItems = 0
  if (next !== 'array') node.arrayDefaults = []
  emitChange()
}

/**
 * onWidgetChange：控件形态切换。
 * - string / textarea：如果之前 textareaRows 未设或非法，补默认 3
 * - string / image：不需要 rows；也不清空已有默认值（可能是一个图片 URL）
 * - array / 媒体 URL 组：把 items 固定为一个 string 元素（值即媒体完整 URL）。
 *   元素 schema 不再暴露给管理员编辑，这里帮他写好，保证存储 shape 合法。
 * - 切回 input：不清空 textareaRows（下次切回 textarea 可复用），只是不再序列化。
 */
function onWidgetChange(v: string | number | boolean | null) {
  const raw = v == null ? 'input' : String(v)
  if (node.type === 'array') {
    node.widget = normalizeMediaUrlWidget(raw) ?? 'input'
    if (normalizeMediaUrlWidget(node.widget)) {
      // 元素恒为 URL 字符串。ImageUrls 继续复用单图控件声明；视频和音频
      // 暂无单值控件，切回 input 时按普通字符串编辑。
      const itemWidget: SchemaWidget = node.widget === 'ImageUrls' ? 'image' : 'input'
      if (!node.items || node.items.type !== 'string') {
        node.items = makeSchemaRow({ key: '', type: 'string', widget: itemWidget })
      } else {
        node.items.widget = itemWidget
      }
      node.arrayDefaults = node.arrayDefaults.map((value) =>
        normalizeValueForSchemaRow(node.items as SchemaRow, value)
      )
    }
    emitChange()
    return
  }
  const next: SchemaWidget = raw === 'textarea' ? 'textarea' : raw === 'PromptTextArea' ? 'PromptTextArea' : raw === 'image' ? 'image' : 'input'
  node.widget = next
  if (next === 'textarea' || next === 'PromptTextArea') {
    const r = Number(node.textareaRows)
    if (!Number.isFinite(r) || r <= 0) node.textareaRows = 3
  }
  emitChange()
}

const referenceFieldOptions = computed(() => props.referenceFieldOptions ?? [])
watch(referenceFieldOptions, (options) => {
  if (node.widget !== 'PromptTextArea') return
  const allowed = new Set(options)
  const normalized = node.referenceFields.filter((field) => allowed.has(field))
  if (normalized.length !== node.referenceFields.length) {
    node.referenceFields = normalized
    emitChange()
  }
}, { immediate: true })

function toggleReferenceField(field: string) {
  node.referenceFields = node.referenceFields.includes(field)
    ? node.referenceFields.filter((item) => item !== field)
    : [...node.referenceFields, field]
  emitChange()
}

/** object：新增一个子字段（默认 string 类型）。 */
function addChild() {
  node.children.push(makeSchemaRow({ key: '', type: 'string' }))
  emitChange()
}
/** object：删除第 i 个子字段。 */
function removeChild(i: number) {
  node.children.splice(i, 1)
  emitChange()
}
/**
 * moveChild：把 children[i] 与 children[i+dir] 交换。
 * dir = -1 表示向上，dir = +1 表示向下。越界时不动作。
 * 交换后 emitChange 触发父层 watch 重新序列化 default_params，从而
 * 把新顺序写回后端（通过 x-order 字段承载）。
 */
function moveChild(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= node.children.length) return
  const tmp = node.children[i]
  node.children[i] = node.children[j]
  node.children[j] = tmp
  emitChange()
}
/** object：子字段 emit update 时不需要重新赋值（同引用），只需触发父层 emit。 */
function onChildUpdate(_i: number, _v: SchemaRow) {
  emitChange()
}
/** array items 更新时同上，只 forward 一次 change 事件。 */
function onItemsUpdate(_v: SchemaRow) {
  if (node.items) {
    node.arrayDefaults = node.arrayDefaults.map((value) =>
      normalizeValueForSchemaRow(node.items as SchemaRow, value)
    )
  }
  emitChange()
}

function onMaxItemsChange() {
  const max = Number(node.maxItems)
  if (Number.isFinite(max) && max > 0 && node.arrayDefaults.length > Math.trunc(max)) {
    node.arrayDefaults.splice(Math.trunc(max))
  }
  emitChange()
}

function onMaxCharsChange() {
  const value = Number(node.maxChars)
  node.maxChars = Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0
  emitChange()
}

function addArrayDefault() {
  if (!node.items || !canAddArrayDefault.value) return
  node.arrayDefaults.push(defaultValueForSchemaRow(node.items))
  emitChange()
}

function updateArrayDefault(index: number, value: unknown) {
  node.arrayDefaults[index] = value
  emitChange()
}

function moveArrayDefault(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= node.arrayDefaults.length) return
  const [value] = node.arrayDefaults.splice(index, 1)
  node.arrayDefaults.splice(targetIndex, 0, value)
  emitChange()
}

function removeArrayDefault(index: number) {
  node.arrayDefaults.splice(index, 1)
  emitChange()
}
</script>

<style scoped>
.param-schema-editor {
  width: 100%;
}

/*
  紧凑下拉框的高度对齐：Select sm 默认 `py-1 text-xs` 自然高度约 26–28px，
  与同一行 `input.h-8`（32px）不一致，视觉上"名称/类型高度不一样"。
  这里通过 :deep() 局部把 sm 触发器高度锁定到 32px，圆角略微收紧以贴近 input 样式。
*/
.param-select-wrapper :deep(.select-trigger-sm) {
  height: 2rem; /* == h-8 */
  padding-top: 0;
  padding-bottom: 0;
  display: flex;
  align-items: center;
}

/*
  嵌套 schema 展开区通用样式：
  - 左侧一条彩色竖条（border-left 4px）+ 左内边距，形成"层级色标 + 缩进"效果；
  - 保留原先的浅灰底 / 圆角，视觉上依旧是一个明确的分组卡片；
  - 递归嵌套时，每一层都套一层同类容器，色条与缩进天然累积，直观呈现深度。
*/
.nested-block {
  padding: 0.75rem 0.75rem 0.75rem 0.875rem;
  border-radius: 0.5rem;
  border: 1px solid rgb(229 231 235); /* gray-200 */
  border-left-width: 4px;
  background-color: rgb(249 250 251 / 0.6); /* gray-50/60 */
}
.dark .nested-block {
  border-color: rgb(55 65 81); /* dark-700 */
  background-color: rgb(31 41 55 / 0.4); /* dark-800/40 */
}
/* object：蓝色（primary）色标 */
.nested-block--object {
  border-left-color: rgb(59 130 246); /* blue-500，与项目 primary 相近 */
}
/* array：紫色色标，与 object 区分 */
.nested-block--array {
  border-left-color: rgb(139 92 246); /* violet-500 */
}
</style>
