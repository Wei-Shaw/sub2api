import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SupportedModelChip from '../SupportedModelChip.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('SupportedModelChip', () => {
  it('仅配置区间倍率时按基础价展示 token 档位', async () => {
    const wrapper = mount(SupportedModelChip, {
      attachTo: document.body,
      props: {
        model: {
          name: 'gpt-test',
          platform: '',
          pricing: {
            billing_mode: 'token',
            input_price: 10e-6,
            output_price: 50e-6,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: null,
            intervals: [{
              min_tokens: 272000,
              max_tokens: null,
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              input_multiplier: 2,
              output_multiplier: 1.5,
              per_request_price: null
            }]
          }
        },
        showPlatform: false
      }
    })

    await wrapper.find('[tabindex="0"]').trigger('mouseenter')
    await nextTick()

    expect(document.body.textContent).toContain('$20 / $75')
    wrapper.unmount()
  })

  it('按视频计费模式展示计费模式标签、按秒单价与分辨率档位定价', async () => {
    const wrapper = mount(SupportedModelChip, {
      attachTo: document.body,
      props: {
        model: {
          name: 'grok-video',
          platform: '',
          pricing: {
            billing_mode: 'video',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: 0.1,
            intervals: [
              {
                min_tokens: 0,
                max_tokens: null,
                tier_label: '480p',
                input_price: null,
                output_price: null,
                cache_write_price: null,
                cache_read_price: null,
                per_request_price: 0.1
              },
              {
                min_tokens: 0,
                max_tokens: null,
                tier_label: '720p',
                input_price: null,
                output_price: null,
                cache_write_price: null,
                cache_read_price: null,
                per_request_price: 0.2
              },
              {
                min_tokens: 0,
                max_tokens: null,
                tier_label: '1080p',
                input_price: null,
                output_price: null,
                cache_write_price: null,
                cache_read_price: null,
                per_request_price: 0.3
              }
            ]
          }
        },
        showPlatform: false
      }
    })

    await wrapper.find('[tabindex="0"]').trigger('mouseenter')
    await nextTick()

    const text = document.body.textContent ?? ''
    // 计费模式标签走 VIDEO 分支,不再落到空的 default 分支
    expect(text).toContain('availableChannels.pricing.billingModeVideo')
    // 分辨率档位标签与各自单价(旧 bug 下 formatInterval 不识别 video,全部显示为 -)
    expect(text).toContain('480p')
    expect(text).toContain('$0.1')
    expect(text).toContain('720p')
    expect(text).toContain('$0.2')
    expect(text).toContain('1080p')
    expect(text).toContain('$0.3')
    wrapper.unmount()
  })

  it('按视频计费模式无阶梯定价时展示默认每秒单价', async () => {
    const wrapper = mount(SupportedModelChip, {
      attachTo: document.body,
      props: {
        model: {
          name: 'grok-video-flat',
          platform: '',
          pricing: {
            billing_mode: 'video',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: 0.15,
            intervals: []
          }
        },
        showPlatform: false
      }
    })

    await wrapper.find('[tabindex="0"]').trigger('mouseenter')
    await nextTick()

    const text = document.body.textContent ?? ''
    expect(text).toContain('availableChannels.pricing.videoPrice')
    expect(text).toContain('$0.15')
    expect(text).toContain('availableChannels.pricing.unitPerSecond')
    wrapper.unmount()
  })
})
