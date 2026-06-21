<template>
  <AppLayout>
    <section class="mx-auto flex max-w-5xl flex-col gap-5">
      <article
        v-for="step in usageGuideSteps"
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
          <h2 class="guide-title">{{ step.title }}</h2>
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
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
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

interface UsageGuideImage {
  src: string
  alt: string
}

interface UsageGuideStep {
  step: number
  title: string
  images: UsageGuideImage[]
  imagePosition?: 'beforeTitle'
}

const usageGuideSteps: UsageGuideStep[] = [
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
</script>

<style scoped>
.guide-step {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(255 255 255);
  padding: 1rem;
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.05);
}

.dark .guide-step {
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

.guide-title {
  color: rgb(17 24 39);
  font-size: 1rem;
  font-weight: 700;
  line-height: 1.6;
}

.dark .guide-title {
  color: rgb(243 244 246);
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
  .guide-step {
    padding: 1.25rem;
  }

  .guide-title {
    font-size: 1.125rem;
  }
}
</style>
