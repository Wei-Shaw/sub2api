import type { MiniMaxMediaPrices } from '@/types'

export const miniMaxDefaults: MiniMaxMediaPrices = {
  'speech-2.8-hd': { characters_per_million: 100 },
  'speech-2.8-turbo': { characters_per_million: 60 },
  'speech-2.6-hd': { characters_per_million: 100 },
  'speech-2.6-turbo': { characters_per_million: 60 },
  'speech-02-hd': { characters_per_million: 100 },
  'speech-02-turbo': { characters_per_million: 60 },
  voice_clone: { voice: 1.5 },
  voice_design: { voice: 3, characters_per_million: 60 },
  'image-01': { image: 0.0035 },
  'MiniMax-H3/768P': { output_second: 0.08, input_second: 0.08, extra_image: 0.04 },
  'MiniMax-H3/2K': { output_second: 0.13, input_second: 0.13, extra_image: 0.04 },
  'MiniMax-H3-Highspeed/768P': { output_second: 0.06, input_second: 0.06, extra_image: 0.03 },
  'MiniMax-H3-Lite/768P': { output_second: 0.04, input_second: 0.04, extra_image: 0.02 },
  'MiniMax-H3-Lite/480P': { output_second: 0.025, input_second: 0.025, extra_image: 0.02 },
  'MiniMax-H3-Max/768P': { output_second: 0.08, input_second: 0.143, extra_image: 0.074 },
  'MiniMax-H3-Max/480P': { output_second: 0.05, input_second: 0.0553, extra_image: 0.074 },
}

export const cloneMiniMaxPrices = (prices?: MiniMaxMediaPrices): MiniMaxMediaPrices =>
  Object.fromEntries(Object.entries(prices ?? {}).map(([k, v]) => [k, { ...v }]))
