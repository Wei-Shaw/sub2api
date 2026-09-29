/**
 * ChatGPT（OpenAI）订阅档位 plan_type 的解析与展示映射。
 *
 * plan_type 由上游原样透传，同一档位会出现 `chatgpt_pro`、`self_serve_business_prolite`
 * 这类下划线/别名写法，因此匹配前一律先归一化。
 *
 * 这里的档位命名只对 OpenAI 平台成立：Antigravity 的 `Pro`、Grok 的 `pro` 是各自产品线的
 * 档位，套用 ChatGPT 的倍率命名（Pro 100 / Pro 200 / Pro 500）会显示错误。
 */

/**
 * plan_type 归一化：去首尾空白、转小写，并去掉空格/下划线/连字符。
 * 例：`self_serve_business_prolite` → `selfservebusinessprolite`。
 */
export function normalizePlanType(value?: string | null): string {
  return (value || '').trim().toLowerCase().replace(/[\s_-]+/g, '')
}

/**
 * ChatGPT 档位 → 展示标签；未知档位返回空串，由调用方决定是否回退为原始值。
 *
 * Pro 的档位命名：`prolite` 为 Pro 100，`pro`/`chatgptpro` 为 Pro 200，`promax` 为 Pro 500；
 * Team/Business：`team` 为 Business Standard，`self_serve_business_prolite` 为 Business Premium。
 */
export function openAIPlanTypeLabel(value?: string | null): string {
  switch (normalizePlanType(value)) {
    case 'plus':
      return 'Plus'
    case 'chatgptpro':
    case 'pro':
      return 'Pro 200'
    case 'prolite':
      return 'Pro 100'
    case 'promax':
      return 'Pro 500'
    case 'selfservebusinessprolite':
      return 'Business Premium'
    case 'team':
      return 'Business Standard'
    case 'free':
      return 'Free'
    default:
      return ''
  }
}
