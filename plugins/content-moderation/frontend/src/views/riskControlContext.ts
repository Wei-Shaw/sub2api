import type { InjectionKey } from 'vue'
import type { RiskControlComposable } from './useRiskControl'

export const RiskControlKey: InjectionKey<RiskControlComposable> = Symbol('RiskControl')
