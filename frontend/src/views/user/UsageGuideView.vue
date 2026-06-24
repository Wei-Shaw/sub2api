<template>
  <AppLayout>
    <section class="usage-guide-shell">
      <nav
        data-test="usage-guide-topic-nav-desktop"
        class="usage-guide-side-nav"
        aria-label="使用方法分类"
      >
        <button
          v-for="topic in guideTopics"
          :key="topic.id"
          type="button"
          class="usage-guide-nav-item"
          :class="{ 'usage-guide-nav-item-active': topic.id === activeTopicId }"
          @click="activeTopicId = topic.id"
        >
          <span class="usage-guide-nav-title">{{ topic.title }}</span>
          <span class="usage-guide-nav-desc">{{ topic.description }}</span>
        </button>
      </nav>

      <div class="usage-guide-main">
        <div
          data-test="usage-guide-topic-tabs-mobile"
          class="usage-guide-mobile-tabs"
          role="tablist"
          aria-label="使用方法分类"
        >
          <button
            v-for="topic in guideTopics"
            :key="topic.id"
            type="button"
            class="usage-guide-mobile-tab"
            :class="{ 'usage-guide-mobile-tab-active': topic.id === activeTopicId }"
            role="tab"
            :aria-selected="topic.id === activeTopicId"
            @click="activeTopicId = topic.id"
          >
            {{ topic.title }}
          </button>
        </div>

        <header class="usage-guide-header">
          <span class="usage-guide-kicker">使用方法</span>
          <h2 class="usage-guide-heading">{{ activeTopic.title }}</h2>
          <p class="usage-guide-description">{{ activeTopic.description }}</p>
        </header>

        <div v-if="activeTopic.kind === 'steps'" class="usage-guide-steps">
          <article
            v-for="step in activeTopic.steps"
            :key="step.step"
            data-test="usage-guide-step"
            class="guide-step"
          >
            <div v-if="step.imagePosition === 'beforeTitle'" class="guide-images guide-images-before">
              <img
                v-for="image in step.images"
                :key="image.alt"
                :src="image.src"
                :alt="image.alt"
                class="guide-image"
                loading="lazy"
              >
            </div>

            <div class="guide-heading">
              <span class="guide-kicker">步骤 {{ step.step }}</span>
              <h3 class="guide-title">{{ step.title }}</h3>
            </div>

            <div v-if="step.imagePosition !== 'beforeTitle'" class="guide-images">
              <img
                v-for="image in step.images"
                :key="image.alt"
                :src="image.src"
                :alt="image.alt"
                class="guide-image"
                loading="lazy"
              >
            </div>
          </article>
        </div>

        <div v-else class="usage-guide-sections">
          <section
            v-for="section in activeTopic.sections"
            :key="section.title"
            class="usage-guide-section-card"
          >
            <h3 class="usage-guide-section-title">{{ section.title }}</h3>
            <p
              v-for="paragraph in section.paragraphs"
              :key="paragraph"
              class="usage-guide-section-text"
            >
              {{ paragraph }}
            </p>

            <div v-if="section.priceRows" class="usage-guide-price-grid">
              <div
                v-for="row in section.priceRows"
                :key="row.label"
                class="usage-guide-price-row"
              >
                <span>{{ row.label }}</span>
                <strong>{{ row.price }}</strong>
              </div>
            </div>

            <pre v-if="section.code" class="usage-guide-code"><code>{{ section.code }}</code></pre>
          </section>
        </div>
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import step01Image from '@/assets/usage-guide/step-01-shop-entry.png'
import step02Image from '@/assets/usage-guide/step-02-login-register.png'
import step03Image from '@/assets/usage-guide/step-03-subscription-plans.png'
import step04PaymentImage from '@/assets/usage-guide/step-04-payment-submitted.png'
import step04RedeemImage from '@/assets/usage-guide/step-04-redeem-code.png'
import step05Image from '@/assets/usage-guide/step-05-create-api-key.png'
import step06Image from '@/assets/usage-guide/step-06-key-group-advanced.png'
import step07ProviderImage from '@/assets/usage-guide/step-07-cc-switch-provider-list.png'
import step07EditImage from '@/assets/usage-guide/step-07-cc-switch-edit-provider.png'
import step08Image from '@/assets/usage-guide/step-08-cc-switch-active.png'

const codexSetupSteps = [
  {
    step: 1,
    title: '访问 aaccx.pw/shop 页面，点击图中的进入按钮',
    images: [{ src: step01Image, alt: '步骤 1 截图 1' }],
  },
  {
    step: 2,
    title: '新用户注册，老用户登录',
    images: [{ src: step02Image, alt: '步骤 2 截图 1' }],
  },
  {
    step: 3,
    title: '选择订阅的页面，选择合适的套餐',
    images: [{ src: step03Image, alt: '步骤 3 截图 1' }],
  },
  {
    step: 4,
    title: '完成支付后，悠一会给你一个兑换码',
    images: [
      { src: step04PaymentImage, alt: '步骤 4 截图 1' },
      { src: step04RedeemImage, alt: '步骤 4 截图 2' },
    ],
  },
  {
    step: 5,
    title: '兑换成功后，去 API Key 页面生成密钥',
    images: [{ src: step05Image, alt: '步骤 5 截图 1' }],
  },
  {
    step: 6,
    title: '选择分组，并且可以设置高级功能',
    images: [{ src: step06Image, alt: '步骤 6 截图 1' }],
  },
  {
    step: 7,
    title: '启动 cc-switch，粘贴 API Key 和请求端口',
    images: [
      { src: step07ProviderImage, alt: '步骤 7 截图 1' },
      { src: step07EditImage, alt: '步骤 7 截图 2' },
    ],
  },
  {
    step: 8,
    imagePosition: 'beforeTitle',
    images: [{ src: step08Image, alt: '步骤 8 截图 1' }],
    title: '保存配置后，重启 Codex，即可使用！',
  },
]

const imageEditRequestExample = `# JSON 方式：适合已经有公网图片链接
curl https://api.aaccx.pw/v1/images/edits \\
  -H "Authorization: Bearer sk-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-image-2",
    "prompt": "把这张图改成黑白极简海报风格，保留主体轮廓",
    "images": [
      {
        "image_url": "https://example.com/input.png"
      }
    ],
    "size": "1024x1024",
    "response_format": "b64_json"
  }'

# multipart 方式：适合直接上传本地图片
curl https://api.aaccx.pw/v1/images/edits \\
  -H "Authorization: Bearer sk-xxxx" \\
  -F "model=gpt-image-2" \\
  -F "prompt=把这张图改成黑白极简海报风格，保留主体轮廓" \\
  -F "image=@/absolute/path/input.png" \\
  -F "size=1024x1024" \\
  -F "response_format=b64_json"`

const guideTopics = [
  {
    id: 'codex',
    title: 'Codex 接入',
    description: '从购买订阅、兑换、创建 API Key 到配置 cc-switch 的完整步骤。',
    kind: 'steps',
    steps: codexSetupSteps,
  },
  {
    id: 'image-generation',
    title: '生图方法',
    description: '使用现有 API Key 调用 OpenAI 兼容图生图接口，并了解图片额度扣费方式。',
    kind: 'sections',
    sections: [
      {
        title: '可用范围',
        paragraphs: [
          '29/39/59/99 元套餐已支持生图和图生图，使用你已经生成的 API Key 即可直接请求图片接口。',
          '请求地址保持为 https://api.aaccx.pw/v1，不需要更换新的服务端点。',
        ],
      },
      {
        title: '接口与扣费',
        paragraphs: [
          '图生图编辑接口使用 POST /v1/images/edits，模型填写 gpt-image-2。',
          'JSON 请求可传 images[].image_url，上传本地文件时改用 multipart 的 image=@...；需要局部修改时可以再加 mask。',
          '图片按实际分辨率消耗订阅日额度，余额和用量记录会按图片价格统计。',
        ],
        priceRows: [
          { label: '1K 图片', price: '$0.10 / 张' },
          { label: '2K 图片', price: '$0.20 / 张' },
          { label: '4K 图片', price: '$0.40 / 张' },
        ],
      },
      {
        title: '请求示例',
        paragraphs: [
          '把示例中的 sk-xxxx 换成你自己的 API Key。不要在公开文档、聊天或截图里展示完整密钥。',
          '如果你只是想从文字直接生成图片，可以把请求路径换成 /v1/images/generations；图生图优先使用下面这组 /v1/images/edits 示例。',
        ],
        code: imageEditRequestExample,
      },
    ],
  },
]

const activeTopicId = ref(guideTopics[0].id)

const activeTopic = computed(() => (
  guideTopics.find((topic) => topic.id === activeTopicId.value) ?? guideTopics[0]
))
</script>

<style scoped>
.usage-guide-shell {
  display: grid;
  gap: 1rem;
  max-width: 80rem;
  margin: 0 auto;
}

.usage-guide-side-nav {
  display: none;
}

.usage-guide-main {
  min-width: 0;
}

.usage-guide-mobile-tabs {
  display: flex;
  gap: 0.5rem;
  overflow-x: auto;
  padding-bottom: 0.75rem;
}

.usage-guide-mobile-tab,
.usage-guide-nav-item {
  border: 1px solid rgb(229 231 235);
  background: rgb(255 255 255);
  color: rgb(55 65 81);
  text-align: left;
  transition:
    background 0.18s ease,
    border-color 0.18s ease,
    color 0.18s ease;
}

.dark .usage-guide-mobile-tab,
.dark .usage-guide-nav-item {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39);
  color: rgb(209 213 219);
}

.usage-guide-mobile-tab {
  flex: 0 0 auto;
  border-radius: 0.5rem;
  padding: 0.55rem 0.8rem;
  font-size: 0.875rem;
  font-weight: 700;
}

.usage-guide-mobile-tab-active,
.usage-guide-nav-item-active {
  border-color: rgb(17 24 39);
  background: rgb(17 24 39);
  color: rgb(255 255 255);
}

.dark .usage-guide-mobile-tab-active,
.dark .usage-guide-nav-item-active {
  border-color: rgb(243 244 246);
  background: rgb(243 244 246);
  color: rgb(17 24 39);
}

.usage-guide-header {
  margin-bottom: 1rem;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(255 255 255);
  padding: 1rem;
}

.dark .usage-guide-header {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39);
}

.usage-guide-kicker {
  color: rgb(107 114 128);
  font-size: 0.75rem;
  font-weight: 800;
}

.dark .usage-guide-kicker {
  color: rgb(156 163 175);
}

.usage-guide-heading {
  margin-top: 0.25rem;
  color: rgb(17 24 39);
  font-size: 1.35rem;
  font-weight: 800;
  line-height: 1.25;
}

.dark .usage-guide-heading {
  color: rgb(243 244 246);
}

.usage-guide-description {
  margin-top: 0.5rem;
  color: rgb(75 85 99);
  font-size: 0.9rem;
  line-height: 1.65;
}

.dark .usage-guide-description {
  color: rgb(209 213 219);
}

.usage-guide-steps,
.usage-guide-sections {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.guide-step,
.usage-guide-section-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(255 255 255);
  padding: 1rem;
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.05);
}

.dark .guide-step,
.dark .usage-guide-section-card {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39);
}

.guide-heading {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.guide-kicker {
  color: rgb(107 114 128);
  font-size: 0.75rem;
  font-weight: 700;
  line-height: 1rem;
}

.dark .guide-kicker {
  color: rgb(156 163 175);
}

.guide-title,
.usage-guide-section-title {
  color: rgb(17 24 39);
  font-size: 1rem;
  font-weight: 700;
  line-height: 1.6;
}

.dark .guide-title,
.dark .usage-guide-section-title {
  color: rgb(243 244 246);
}

.usage-guide-section-text {
  color: rgb(75 85 99);
  font-size: 0.92rem;
  line-height: 1.75;
}

.dark .usage-guide-section-text {
  color: rgb(209 213 219);
}

.usage-guide-price-grid {
  display: grid;
  gap: 0.5rem;
}

.usage-guide-price-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  padding: 0.75rem;
  color: rgb(55 65 81);
  font-size: 0.9rem;
}

.dark .usage-guide-price-row {
  border-color: rgb(55 65 81);
  color: rgb(209 213 219);
}

.usage-guide-price-row strong {
  color: rgb(17 24 39);
  white-space: nowrap;
}

.dark .usage-guide-price-row strong {
  color: rgb(243 244 246);
}

.usage-guide-code {
  overflow-x: auto;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(17 24 39);
  color: rgb(243 244 246);
  padding: 1rem;
  font-size: 0.82rem;
  line-height: 1.7;
}

.guide-images {
  display: grid;
  gap: 1rem;
}

.guide-images-before {
  margin-bottom: 0.25rem;
}

.guide-image {
  display: block;
  width: 100%;
  max-width: 100%;
  height: auto;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(249 250 251);
}

.dark .guide-image {
  border-color: rgb(55 65 81);
  background: rgb(3 7 18);
}

@media (min-width: 768px) {
  .guide-step,
  .usage-guide-section-card,
  .usage-guide-header {
    padding: 1.25rem;
  }

  .guide-title,
  .usage-guide-section-title {
    font-size: 1.125rem;
  }
}

@media (min-width: 1024px) {
  .usage-guide-shell {
    grid-template-columns: 16rem minmax(0, 1fr);
    align-items: start;
  }

  .usage-guide-side-nav {
    position: sticky;
    top: 5.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .usage-guide-mobile-tabs {
    display: none;
  }

  .usage-guide-nav-item {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    border-radius: 0.5rem;
    padding: 0.9rem;
  }

  .usage-guide-nav-title {
    font-size: 0.95rem;
    font-weight: 800;
  }

  .usage-guide-nav-desc {
    font-size: 0.78rem;
    line-height: 1.5;
    opacity: 0.78;
  }
}
</style>
