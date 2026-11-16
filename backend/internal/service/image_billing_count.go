package service

// resolveBillableImageCount 决定一次图片请求按几张计费。
//
// 官方多图响应没有 n 字段，只以响应体 data/output 中实际返回的图片为准。
// 解析不到张数时：
//   - 分组开启按张计费：按 1 张兜底，避免已成功出图的请求完全漏计；
//   - 未开启：保持原口径（流式 0 张、非流式回退请求 n），不能因为开关之外的原因多扣。
func resolveBillableImageCount(reported, requested int, fallbackToOne bool) int {
	if reported > 0 {
		return reported
	}
	if fallbackToOne {
		return 1
	}
	if requested > 0 {
		return requested
	}
	return 0
}
