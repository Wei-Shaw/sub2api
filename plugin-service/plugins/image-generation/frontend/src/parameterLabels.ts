export type ImageParameterLabelKind = 'quality' | 'background' | 'input_fidelity'

const imageParameterLabels: Record<ImageParameterLabelKind, Record<string, string>> = {
  quality: {
    auto: '自动画质',
    low: '低画质',
    medium: '标准画质',
    high: '高画质',
  },
  background: {
    auto: '自动背景',
    transparent: '透明背景',
    opaque: '不透明背景',
  },
  input_fidelity: {
    low: '低保真',
    high: '高保真',
  },
}

export function imageParameterLabel(kind: ImageParameterLabelKind, value: string): string {
  return imageParameterLabels[kind][value] ?? value
}
