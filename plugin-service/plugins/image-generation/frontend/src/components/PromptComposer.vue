<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { CSSProperties } from 'vue'
import { imageParameterLabel } from '../parameterLabels'
import type { ImagePresetSelection, ImageReference } from '../types'

const props = withDefaults(defineProps<{
  prompt: string
  model: string
  size: string
  outputCount?: number
  maxOutputImages?: number
  sizeOptions?: string[]
  aspectRatio?: string
  aspectRatioOptions?: string[]
  resolution?: string
  resolutionOptions?: string[]
  quality?: string
  qualityOptions?: string[]
  outputFormat?: string
  outputFormatOptions?: string[]
  outputCompression?: number | null
  compressionMin?: number
  compressionMax?: number
  supportsOutputCompression?: boolean
  background?: string
  backgroundOptions?: string[]
  inputFidelity?: string
  inputFidelityOptions?: string[]
  models: string[]
  promptOptimizerModel?: string
  promptOptimizerModels?: string[]
  optimizingPrompt?: boolean
  presetSelection?: ImagePresetSelection
  busy: boolean
  references?: ImageReference[]
  maxReferenceImages?: number
  referenceLimitExceeded?: boolean
}>(), {
  references: () => [],
  outputCount: 1,
  maxOutputImages: 1,
  maxReferenceImages: 1,
  referenceLimitExceeded: false,
  promptOptimizerModel: '',
  promptOptimizerModels: () => [],
  optimizingPrompt: false,
  presetSelection: () => ({ styles: [], scenes: [], effects: [], angles: [], separateAngleImages: false, keepAngleConsistency: false }),
  sizeOptions: () => [],
  aspectRatio: '', aspectRatioOptions: () => [],
  resolution: '', resolutionOptions: () => [],
  quality: '', qualityOptions: () => [],
  outputFormat: '', outputFormatOptions: () => [],
  outputCompression: null, compressionMin: 0, compressionMax: 100, supportsOutputCompression: false,
  background: '', backgroundOptions: () => [],
  inputFidelity: '', inputFidelityOptions: () => [],
})

const emit = defineEmits<{
  'update:prompt': [value: string]
  'update:model': [value: string]
  'update:size': [value: string]
  'update:outputCount': [value: number]
  'update:aspectRatio': [value: string]
  'update:resolution': [value: string]
  'update:quality': [value: string]
  'update:outputFormat': [value: string]
  'update:outputCompression': [value: number]
  'update:background': [value: string]
  'update:inputFidelity': [value: string]
  'update:promptOptimizerModel': [value: string]
  'update:presetSelection': [value: ImagePresetSelection]
  applyPresetSelection: [value: ImagePresetSelection]
  submit: []
  stop: []
  optimizePrompt: [model: string]
  cancelPromptOptimization: []
  referenceFiles: [value: File[]]
  removeReference: [id: string]
  clearReferences: []
}>()

const fanExpanded = ref(false)
const optimizeDialogOpen = ref(false)
const presetDialogOpen = ref(false)
const selectedOptimizerModel = ref(props.promptOptimizerModel)
const effectiveSizeOptions = computed(() => props.sizeOptions.length ? props.sizeOptions : props.size ? [props.size] : [])
const canOptimizePrompt = computed(() => Boolean(props.prompt.trim() && props.promptOptimizerModels.length && !props.busy))
const presetCount = computed(() => props.presetSelection.styles.length + props.presetSelection.scenes.length + props.presetSelection.effects.length + props.presetSelection.angles.length)
let preserveFanFocus = false

const presetGroups = [
  { key: 'styles', label: '风格', options: [
    ['cinematic', '电影感'], ['photorealistic', '写实摄影'], ['anime', '动漫'], ['illustration', '插画'], ['product', '产品摄影'],
  ] },
  { key: 'scenes', label: '场景', options: [
    ['studio', '摄影棚'], ['outdoor', '户外'], ['interior', '室内'], ['night', '夜景'], ['minimal', '极简背景'],
  ] },
  { key: 'effects', label: '特效', options: [
    ['soft_light', '柔和光线'], ['dramatic_light', '戏剧光'], ['depth_of_field', '景深'], ['volumetric_light', '体积光'], ['motion', '动态效果'],
  ] },
  { key: 'angles', label: '角度', options: [
    ['front', '正面'], ['back', '背面'], ['left', '左侧'], ['right', '右侧'], ['three_quarter', '45 度'], ['top', '俯视'],
  ] },
] as const

watch(() => props.promptOptimizerModel, (model) => {
  selectedOptimizerModel.value = model
})

watch(() => props.promptOptimizerModels, (models) => {
  if (!models.includes(selectedOptimizerModel.value)) selectedOptimizerModel.value = models[0] ?? ''
})

watch(() => props.references.length, (count) => {
  if (count < 2) fanExpanded.value = false
})

function toggleFan() {
  if (props.references.length >= 2) fanExpanded.value = !fanExpanded.value
}

function collapseFan() {
  fanExpanded.value = false
}

function keydownComposer(event: KeyboardEvent) {
  if (event.key === 'Escape' && fanExpanded.value) {
    event.preventDefault()
    collapseFan()
    return
  }
  keydown(event)
}

function focusoutReference(event: FocusEvent) {
  if (preserveFanFocus) return
  const current = event.currentTarget as HTMLElement
  if (!current.contains(event.relatedTarget as Node | null)) collapseFan()
}

function removeReference(id: string) {
  preserveFanFocus = true
  emit('removeReference', id)
  requestAnimationFrame(() => { preserveFanFocus = false })
}

function fanItemStyle(index: number, count: number): CSSProperties {
  const progress = count <= 1 ? 0 : index / (count - 1)
  const distance = Math.min(240, 92 + (count - 1) * 15)
  return {
    '--fan-x': `${Math.round((progress - 0.5) * 84)}px`,
    '--fan-y': `${Math.round(-72 - progress * distance)}px`,
    '--fan-rotation': `${Math.round((progress - 0.5) * 24)}deg`,
    '--fan-layer': index + 1,
  } as CSSProperties
}

function compactItemStyle(index: number): CSSProperties {
  return {
    zIndex: index + 1,
    transform: `translate(${index * 4}px, ${index * -4}px) rotate(${index * 4 - 2}deg)`,
  }
}

function stackLayerStyle(index: number): CSSProperties {
  return {
    transform: `translate(${(2 - index) * 3}px, ${(index - 2) * 3}px) rotate(${(index - 1) * 3}deg)`,
  }
}

function keydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.ctrlKey || event.metaKey || event.shiftKey) return
  event.preventDefault()
  emit('submit')
}

function readReference(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  if (!files.length) return
  emit('referenceFiles', files)
  input.value = ''
}

function openOptimizeDialog() {
  if (!canOptimizePrompt.value) return
  selectedOptimizerModel.value = props.promptOptimizerModel || props.promptOptimizerModels[0] || ''
  optimizeDialogOpen.value = true
}

function handleMagicButton() {
  if (props.optimizingPrompt) {
    emit('cancelPromptOptimization')
    return
  }
  openOptimizeDialog()
}

function confirmOptimizePrompt() {
  if (!selectedOptimizerModel.value) return
  emit('update:promptOptimizerModel', selectedOptimizerModel.value)
  emit('optimizePrompt', selectedOptimizerModel.value)
  optimizeDialogOpen.value = false
}

function togglePreset(group: 'styles' | 'scenes' | 'effects' | 'angles', value: string) {
  const current = props.presetSelection[group]
  const next = current.includes(value) ? current.filter(item => item !== value) : [...current, value]
  emit('update:presetSelection', { ...props.presetSelection, [group]: next })
}

function clearPresets() {
  emit('update:presetSelection', { styles: [], scenes: [], effects: [], angles: [], separateAngleImages: false, keepAngleConsistency: false })
}

function toggleSeparateAngleImages(value: boolean) {
  emit('update:presetSelection', { ...props.presetSelection, separateAngleImages: value })
}

function toggleAngleConsistency(value: boolean) {
  emit('update:presetSelection', { ...props.presetSelection, keepAngleConsistency: value })
}

function confirmPresets() {
  emit('applyPresetSelection', props.presetSelection)
  presetDialogOpen.value = false
}

</script>

<template>
  <form class="composer" data-testid="image-chat-composer" @submit.prevent="emit('submit')" @keydown="keydownComposer">
    <div class="composer-main">
      <div
        v-if="references.length"
        class="reference-stack-anchor"
        :class="{ expanded: fanExpanded }"
        @focusout="focusoutReference"
      >
        <template v-if="references.length === 1">
          <div class="compact-reference-stack" aria-label="已选择的参考图">
            <div v-for="(reference, index) in references" :key="reference.id" class="compact-reference-item" :style="compactItemStyle(index)">
              <img :src="reference.dataUrl" :alt="reference.fileName" data-testid="reference-image-preview">
              <button
                type="button"
                class="remove-reference"
                data-testid="remove-reference-image"
                :aria-label="`移除参考图 ${reference.fileName}`"
                @click="removeReference(reference.id)"
              >×</button>
            </div>
          </div>
          <label v-if="references.length < maxReferenceImages" class="reference-picker compact-add-picker" data-testid="reference-upload-label" title="继续上传参考图">
            <span class="sr-only">继续上传参考图</span>
            <input data-testid="reference-image-input" type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif,image/bmp,image/tiff" @change="readReference">
            <span class="reference-add-core" aria-hidden="true">+</span>
          </label>
        </template>
        <template v-else>
          <div class="reference-fan" :aria-hidden="!fanExpanded">
            <div
              v-for="(reference, index) in references"
              :key="reference.id"
              class="reference-fan-item"
              data-testid="reference-fan-item"
              :style="fanItemStyle(index, references.length)"
            >
              <img :src="reference.dataUrl" :alt="reference.fileName" data-testid="reference-image-preview">
              <button
                type="button"
                class="remove-reference"
                data-testid="remove-reference-image"
                :aria-label="`移除参考图 ${reference.fileName}`"
                :tabindex="fanExpanded ? 0 : -1"
                @click="removeReference(reference.id)"
              >×</button>
            </div>
          </div>
          <button
            v-if="fanExpanded"
            type="button"
            class="clear-references"
            data-testid="clear-reference-images"
            aria-label="清空全部参考图"
            @click="emit('clearReferences')"
          >清空</button>
          <div class="reference-stack-preview" aria-hidden="true">
            <span v-for="(reference, index) in references.slice(0, 3)" :key="reference.id" class="reference-stack-layer" :style="stackLayerStyle(index)">
              <img :src="reference.dataUrl" alt="">
            </span>
          </div>
          <label v-if="references.length < maxReferenceImages" class="reference-picker fan-add-picker" data-testid="reference-upload-label" title="继续上传参考图">
            <span class="sr-only">继续上传参考图</span>
            <input data-testid="reference-image-input" type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif,image/bmp,image/tiff" @change="readReference">
            <span class="reference-add-core" aria-hidden="true">+</span>
          </label>
          <button
            type="button"
            class="reference-stack-count"
            data-testid="reference-count-toggle"
            :aria-expanded="fanExpanded"
            :aria-label="fanExpanded ? `收起 ${references.length} 张参考图` : `展开 ${references.length} 张参考图`"
            @click="toggleFan"
          >{{ references.length }}/{{ maxReferenceImages }}</button>
        </template>
      </div>
      <label v-else class="reference-picker" data-testid="reference-upload-label" title="上传参考图">
        <span class="sr-only">上传参考图</span>
        <input data-testid="reference-image-input" type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif,image/bmp,image/tiff" @change="readReference">
        <span aria-hidden="true">+</span>
      </label>
      <textarea
        :value="prompt"
        rows="3"
        data-testid="image-prompt-input"
        placeholder="描述你想生成的图片内容"
        aria-label="图片生成提示词"
        :aria-describedby="referenceLimitExceeded ? 'reference-limit-error' : undefined"
        :aria-invalid="referenceLimitExceeded || undefined"
        @input="emit('update:prompt', ($event.target as HTMLTextAreaElement).value)"
      />
    </div>
    <p v-if="referenceLimitExceeded" id="reference-limit-error" class="reference-limit-error" data-testid="reference-limit-error" role="alert">
      当前模型最多支持 {{ maxReferenceImages }} 张参考图，请删除多余图片或切换模型。
    </p>
    <div class="composer-tools">
      <label class="composer-select">
        <span class="sr-only">模型</span>
        <select :value="model" data-testid="image-model-select" @change="emit('update:model', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in models" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <label v-if="effectiveSizeOptions.length" class="composer-select">
        <span class="sr-only">尺寸</span>
        <select :value="size" data-testid="image-size-select" @change="emit('update:size', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in effectiveSizeOptions" :key="item" :value="item">{{ item.replace('x', ' × ') }}</option>
        </select>
      </label>
      <label v-if="aspectRatioOptions.length" class="composer-select">
        <span class="sr-only">图片比例</span>
        <select :value="aspectRatio" data-testid="image-aspect-ratio-select" @change="emit('update:aspectRatio', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in aspectRatioOptions" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <label v-if="resolutionOptions.length" class="composer-select">
        <span class="sr-only">分辨率</span>
        <select :value="resolution" data-testid="image-resolution-select" @change="emit('update:resolution', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in resolutionOptions" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <label v-if="qualityOptions.length" class="composer-select">
        <span class="sr-only">画质</span>
        <select :value="quality" data-testid="image-quality-select" @change="emit('update:quality', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in qualityOptions" :key="item" :value="item">{{ imageParameterLabel('quality', item) }}</option>
        </select>
      </label>
      <label v-if="outputFormatOptions.length" class="composer-select">
        <span class="sr-only">输出格式</span>
        <select :value="outputFormat" data-testid="image-output-format-select" @change="emit('update:outputFormat', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in outputFormatOptions" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <label v-if="supportsOutputCompression && outputCompression !== null" class="compression-control">
        <span>压缩 {{ outputCompression }}%</span>
        <input :value="outputCompression" :min="compressionMin" :max="compressionMax" type="range" data-testid="image-output-compression" @input="emit('update:outputCompression', Number(($event.target as HTMLInputElement).value))">
      </label>
      <label v-if="backgroundOptions.length" class="composer-select">
        <span class="sr-only">背景</span>
        <select :value="background" data-testid="image-background-select" @change="emit('update:background', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in backgroundOptions" :key="item" :value="item">{{ imageParameterLabel('background', item) }}</option>
        </select>
      </label>
      <label v-if="references.length && inputFidelityOptions.length" class="composer-select">
        <span class="sr-only">参考图保真度</span>
        <select :value="inputFidelity" data-testid="image-input-fidelity-select" @change="emit('update:inputFidelity', ($event.target as HTMLSelectElement).value)">
          <option v-for="item in inputFidelityOptions" :key="item" :value="item">{{ imageParameterLabel('input_fidelity', item) }}</option>
        </select>
      </label>
      <label class="composer-select">
        <span class="sr-only">生成数量</span>
        <select :value="outputCount" data-testid="image-output-count" @change="emit('update:outputCount', Number(($event.target as HTMLSelectElement).value))">
          <option v-for="count in maxOutputImages" :key="count" :value="count">{{ count }} 张</option>
        </select>
      </label>
      <button
        v-if="!busy"
        type="button"
        class="preset-button"
        data-testid="image-preset-button"
        :aria-label="`图片预设，已选择 ${presetCount} 项`"
        @click="presetDialogOpen = true"
      >
        <span aria-hidden="true">☷</span>
        <span>预设<template v-if="presetCount"> {{ presetCount }}</template></span>
      </button>
      <button v-if="busy" type="button" class="send-button stop-button" aria-label="停止生成" @click="emit('stop')">
        <span aria-hidden="true" class="stop-icon" />
      </button>
      <button
        v-if="!busy"
        type="button"
        class="magic-button"
        :class="{ optimizing: optimizingPrompt }"
        data-testid="prompt-optimize-button"
        :aria-label="optimizingPrompt ? '停止优化提示词' : '优化提示词'"
        :disabled="!optimizingPrompt && !canOptimizePrompt"
        @click="handleMagicButton"
      >
        <span v-if="optimizingPrompt" aria-hidden="true" class="magic-spinner" />
        <span v-else aria-hidden="true">✦</span>
      </button>
      <button v-if="!busy" type="submit" class="send-button" data-testid="image-send-button" :aria-label="referenceLimitExceeded ? `无法发送：当前模型最多支持 ${maxReferenceImages} 张参考图` : '发送图片提示词'" :disabled="!prompt.trim() || referenceLimitExceeded">
        <span aria-hidden="true">↑</span>
      </button>
    </div>
    <div v-if="presetDialogOpen" class="dialog-overlay" @keydown.esc="presetDialogOpen = false">
      <section class="preset-dialog" role="dialog" aria-modal="true" aria-labelledby="preset-dialog-title">
        <div class="preset-dialog-heading">
          <h2 id="preset-dialog-title">图片预设</h2>
          <button type="button" class="preset-close-button" aria-label="关闭图片预设" @click="presetDialogOpen = false">
            <span aria-hidden="true">×</span>
          </button>
        </div>
        <fieldset v-for="group in presetGroups" :key="group.key" class="preset-group">
          <legend>{{ group.label }}</legend>
          <div class="preset-options">
            <label v-for="[value, label] in group.options" :key="value" class="preset-option">
              <input
                type="checkbox"
                :checked="presetSelection[group.key].includes(value)"
                :disabled="group.key === 'angles' && !presetSelection.angles.includes(value) && presetSelection.angles.length >= maxOutputImages"
                @change="togglePreset(group.key, value)"
              >
              <span>{{ label }}</span>
            </label>
          </div>
        </fieldset>
        <label v-if="presetSelection.angles.length > 1" class="preset-angle-output-option">
          <input
            type="checkbox"
            data-testid="separate-angle-images"
            :checked="presetSelection.separateAngleImages"
            @change="toggleSeparateAngleImages(($event.target as HTMLInputElement).checked)"
          >
          <span>每个角度单独生成一张</span>
        </label>
        <label v-if="presetSelection.angles.length > 1" class="preset-angle-output-option">
          <input
            type="checkbox"
            data-testid="keep-angle-consistency"
            :checked="presetSelection.keepAngleConsistency"
            @change="toggleAngleConsistency(($event.target as HTMLInputElement).checked)"
          >
          <span>保持各角度主体与风格一致</span>
        </label>
        <p v-if="presetSelection.angles.length > 1" class="preset-angle-note">
          {{ presetSelection.separateAngleImages
            ? `将为 ${presetSelection.angles.length} 个角度分别生成一张图片，结果显示在同一条消息中。`
            : '将生成一张包含所选角度的多视角拼图。' }}
        </p>
        <div class="dialog-actions">
          <button type="button" :disabled="!presetCount" @click="clearPresets">清空</button>
          <button type="button" class="primary-button" @click="confirmPresets">应用到提示词</button>
        </div>
      </section>
    </div>
    <div v-if="optimizeDialogOpen" class="dialog-overlay" @keydown.esc="optimizeDialogOpen = false">
      <section class="prompt-optimizer-dialog" role="dialog" aria-modal="true" aria-labelledby="prompt-optimizer-title">
        <h2 id="prompt-optimizer-title">优化提示词</h2>
        <label class="optimizer-model-field">
          <span>思考模型</span>
          <select v-model="selectedOptimizerModel" data-testid="prompt-optimizer-model">
            <option v-for="item in promptOptimizerModels" :key="item" :value="item">{{ item }}</option>
          </select>
        </label>
        <div class="dialog-actions">
          <button type="button" @click="optimizeDialogOpen = false">取消</button>
          <button type="button" class="primary-button" :disabled="!selectedOptimizerModel || optimizingPrompt" @click="confirmOptimizePrompt">
            {{ optimizingPrompt ? '优化中' : '优化' }}
          </button>
        </div>
      </section>
    </div>
  </form>
</template>
