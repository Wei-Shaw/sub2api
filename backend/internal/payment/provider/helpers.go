package provider

// cloneStringMap 复制一份配置表，避免服务商实例持有调用方仍在改写的 map。
func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
