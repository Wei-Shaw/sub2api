let globalStableObjectKeySeed = 0

/**
 * 为对象实例生成稳定 key（基于 WeakMap，不污染业务对象）
 */
export function createStableObjectKeyResolver<T extends object>(prefix = 'item') {
  const keyMap = new WeakMap<T, string>()

  return (item: T): string => {
    const cached = keyMap.get(item)
    if (cached) {
      return cached
    }

    const key = `${prefix}-${++globalStableObjectKeySeed}`
    keyMap.set(item, key)
    return key
  }
}
