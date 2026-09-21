//go:build unit

package adobe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageModelCatalogRegistersFamilies(t *testing.T) {
	_, ok := ResolveImageModel("firefly-gpt-image-2-2k-16x9")
	require.False(t, ok, "gpt-image 2 不再展开全量 id")

	_, ok = ResolveImageModel("firefly-gpt-image-1.5-2k-16x9")
	require.False(t, ok, "gpt-image 1.5 不再展开全量 id")

	nano, ok := ResolveImageModel("firefly-nano-banana-pro-4k-1x1")
	require.True(t, ok)
	require.Equal(t, "gemini-flash", nano.UpstreamModelID)
	require.Equal(t, "nano-banana-2", nano.UpstreamModelVersion)
	require.Equal(t, Resolution4K, nano.OutputResolution)
	require.Equal(t, "1:1", nano.AspectRatio)
	require.Equal(t, Size{4096, 4096}, nano.SizePixels)

	fiveFour, ok := ResolveImageModel("firefly-nano-banana-pro-2k-5x4")
	require.True(t, ok, "Pro 必须注册 5:4")
	require.Equal(t, "5:4", fiveFour.AspectRatio)
	require.Equal(t, Size{2048, 2048}, fiveFour.SizePixels)

	nano2, ok := ResolveImageModel("firefly-nano-banana2-2k-1x1")
	require.True(t, ok)
	require.Equal(t, "nano-banana-3", nano2.UpstreamModelVersion)
}

// nano-banana2 独有的超长横幅/竖幅比例，其它 nano 家族不注册。
func TestNanoBanana2ExtendedRatios(t *testing.T) {
	for id, wantRatio := range map[string]string{
		"firefly-nano-banana2-2k-1x8": "1:8",
		"firefly-nano-banana2-2k-4x1": "4:1",
		"firefly-nano-banana2-2k-1x4": "1:4",
		"firefly-nano-banana2-2k-8x1": "8:1",
	} {
		conf, ok := ResolveImageModel(id)
		require.True(t, ok, "%s 应已注册", id)
		require.Equal(t, wantRatio, conf.AspectRatio)
	}

	_, ok := ResolveImageModel("firefly-nano-banana-pro-2k-1x8")
	require.False(t, ok, "nano-banana-pro 不支持 1:8")
	_, ok = ResolveImageModel("firefly-nano-banana-2k-1x8")
	require.False(t, ok, "nano-banana 不支持 1:8")
}

func TestResolveImageModel(t *testing.T) {
	t.Run("空 id 回退默认模型", func(t *testing.T) {
		conf, ok := ResolveImageModel("")
		require.True(t, ok)
		require.Equal(t, DefaultImageModelID, conf.ModelID)
	})

	t.Run("未知 id 返回 false", func(t *testing.T) {
		_, ok := ResolveImageModel("firefly-unknown-9k-1x1")
		require.False(t, ok)
	})

	t.Run("族级 id 不被当作全量 id", func(t *testing.T) {
		_, ok := ResolveImageModel("firefly-gpt-image-2")
		require.False(t, ok)
	})
}

// 最长前缀匹配：nano-banana 不能吞掉 nano-banana-pro / nano-banana2。
func TestPickImageFamily(t *testing.T) {
	tests := []struct {
		modelID string
		want    string
		ok      bool
	}{
		{"firefly-nano-banana", "firefly-nano-banana", true},
		{"firefly-nano-banana-2k-16x9", "firefly-nano-banana", true},
		{"firefly-nano-banana-pro", "firefly-nano-banana-pro", true},
		{"firefly-nano-banana-pro-2k-16x9", "firefly-nano-banana-pro", true},
		{"firefly-nano-banana2", "firefly-nano-banana2", true},
		{"firefly-nano-banana2-2k-1x8", "firefly-nano-banana2", true},
		{"firefly-gpt-image-2", "firefly-gpt-image-2", true},
		{"firefly-gpt-image-1.5-4k-1x1", "firefly-gpt-image-1.5", true},
		{"FIREFLY-GPT-IMAGE-2", "firefly-gpt-image-2", true},
		{"gpt-image-2", "", false},
		{"firefly-unknown", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			got, ok := PickImageFamily(tt.modelID)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsImageModelID(t *testing.T) {
	require.True(t, IsImageModelID("firefly-gpt-image-2"))
	require.True(t, IsImageModelID("  firefly-anything  "))
	require.False(t, IsImageModelID("gpt-image-2"))
	require.False(t, IsImageModelID(""))
}

func TestResolveImage(t *testing.T) {
	t.Run("banana 全量 id 直接命中，忽略 size", func(t *testing.T) {
		conf, err := ResolveImage(ImageRequest{
			ModelID: "firefly-nano-banana-pro-4k-1x1",
			Size:    "1792x1024",
		})
		require.NoError(t, err)
		require.Equal(t, Resolution4K, conf.OutputResolution)
		require.Equal(t, "1:1", conf.AspectRatio)
		require.Equal(t, Size{4096, 4096}, conf.SizePixels)
	})

	t.Run("gpt-image 2 族级 id 原样转发 WxH", func(t *testing.T) {
		conf, err := ResolveImage(ImageRequest{
			ModelID: "firefly-gpt-image-2",
			Size:    "1152x928",
		})
		require.NoError(t, err)
		require.Equal(t, "firefly-gpt-image-2", conf.ModelID)
		require.Equal(t, PayloadKindGPTImage25, conf.PayloadKind)
		require.Equal(t, Size{1152, 928}, conf.SizePixels)
		require.Equal(t, Resolution2K, conf.OutputResolution)
	})

	t.Run("显式 Ratio 优先于 Size（banana）", func(t *testing.T) {
		conf, err := ResolveImage(ImageRequest{
			ModelID: "firefly-nano-banana-pro",
			Size:    "1792x1024",
			Ratio:   "3:2",
		})
		require.NoError(t, err)
		require.Equal(t, "3:2", conf.AspectRatio)
		require.Equal(t, Size{2048, 2048}, conf.SizePixels)
	})

	t.Run("Ratio 为 auto 时回落 Size 推导（banana）", func(t *testing.T) {
		conf, err := ResolveImage(ImageRequest{
			ModelID: "firefly-nano-banana-pro",
			Size:    "1024x1792",
			Ratio:   "auto",
		})
		require.NoError(t, err)
		require.Equal(t, "9:16", conf.AspectRatio)
	})

	t.Run("banana 省略 size 回落 1K 方图", func(t *testing.T) {
		for _, size := range []string{"", "auto"} {
			conf, err := ResolveImage(ImageRequest{ModelID: "firefly-nano-banana-pro", Size: size})
			require.NoError(t, err, size)
			require.Equal(t, Resolution1K, conf.OutputResolution, size)
			require.Equal(t, "1:1", conf.AspectRatio, size)
			require.Equal(t, Size{1024, 1024}, conf.SizePixels, size)
		}
	})

	t.Run("显式分辨率", func(t *testing.T) {
		conf, err := ResolveImage(ImageRequest{
			ModelID:    "firefly-nano-banana-pro",
			Ratio:      "1:1",
			Resolution: Resolution4K,
		})
		require.NoError(t, err)
		require.Equal(t, "firefly-nano-banana-pro-4k-1x1", conf.ModelID)
	})

	t.Run("家族不支持的比例报错", func(t *testing.T) {
		_, err := ResolveImage(ImageRequest{
			ModelID: "firefly-nano-banana-pro",
			Ratio:   "1:8", // 仅 nano-banana2 支持
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported aspect ratio")
		require.False(t, IsRotatable(err), "参数错误换号也无用")
	})

	t.Run("未知模型报错", func(t *testing.T) {
		// Step 10 起 "gpt-image-2" 是合法的对外名（见 TestResolveImageAcceptsExternalModelNames），
		// 所以这里换一个既不在目录也不在别名表里的名字。
		_, err := ResolveImage(ImageRequest{ModelID: "totally-unknown-model"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown firefly image model")
	})
}

// TestResolveImageCommonSizes 是「size 未命中就静默出方图」那个 bug 的回归锚点。
//
// 表里的尺寸取自参考实现 GPT2Image-Pro 的尺寸预设与 OpenAI 官方尺寸。旧实现用 9 条
// 精确查表，表外一律回落 1:1——下面标了 ★ 的行当时全是 1:1。
func TestResolveImageCommonSizes(t *testing.T) {
	cases := []struct {
		size          string
		gptRatio      string
		nanoRatio     string
		wantResoluton OutputResolution
	}{
		{"1024x1024", "1:1", "1:1", Resolution1K},
		{"1248x1248", "1:1", "1:1", Resolution2K},
		{"2048x2048", "1:1", "1:1", Resolution2K},
		{"1792x1024", "16:9", "16:9", Resolution2K},
		{"1024x1792", "9:16", "9:16", Resolution2K},
		// gpt-image 2 转发像素；banana 用比例字符串 + 方图档位。
		{"1536x1024", "3:2", "3:2", Resolution2K},
		{"1024x1536", "2:3", "2:3", Resolution2K},
		// ★ 参考项目的 "2K Wide" / "4K Wide" / "4K Tall"。
		{"2048x1152", "16:9", "16:9", Resolution2K},
		{"3840x2160", "16:9", "16:9", Resolution4K},
		{"2160x3840", "9:16", "9:16", Resolution4K},
		// 小图落 1K。
		{"512x512", "1:1", "1:1", Resolution1K},
	}

	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			gpt, err := ResolveImage(ImageRequest{ModelID: "firefly-gpt-image-2", Size: tc.size})
			require.NoError(t, err)
			w, h, ok := parseSizeWxH(tc.size)
			require.True(t, ok)
			require.Equal(t, Size{Width: w, Height: h}, gpt.SizePixels)
			require.Equal(t, tc.wantResoluton, gpt.OutputResolution)

			nano, err := ResolveImage(ImageRequest{ModelID: "firefly-nano-banana-pro", Size: tc.size})
			require.NoError(t, err)
			require.Equal(t, tc.nanoRatio, nano.AspectRatio)
			require.Equal(t, tc.wantResoluton, nano.OutputResolution)
			require.Equal(t, SquareFromResolution(tc.wantResoluton), nano.SizePixels)
		})
	}
}

func TestNearestRatio(t *testing.T) {
	nano := mustFamilyRatios(t, "firefly-nano-banana-pro")
	nano2 := mustFamilyRatios(t, "firefly-nano-banana2")

	require.Equal(t, "21:9", NearestRatio("2520x1080", nano))
	require.Equal(t, "5:4", NearestRatio("2000x1600", nano))

	require.Equal(t, "4:1", NearestRatio("1024x256", nano2))
	require.Equal(t, "21:9", NearestRatio("1024x256", nano))

	require.Equal(t, FallbackRatio, NearestRatio("", nano))
	require.Equal(t, FallbackRatio, NearestRatio("auto", nano))
	require.Equal(t, FallbackRatio, NearestRatio("abc", nano))
	require.Equal(t, FallbackRatio, NearestRatio("0x100", nano))
	require.Equal(t, FallbackRatio, NearestRatio("1024x1024", nil))
}

// TestNearestRatioIsDeterministic 守的是 map 迭代顺序泄漏进结果：候选集若直接来自
// ratioSuffixes 的 map keys，平局时同一个 size 会在不同进程里解析出不同比例。
func TestNearestRatioIsDeterministic(t *testing.T) {
	for _, familyID := range ImageFamilyModelIDs {
		ratios := mustFamilyRatios(t, familyID)
		if len(ratios) == 0 {
			continue
		}
		for _, size := range []string{"1000x999", "1234x567", "800x800", "1920x1080"} {
			want := NearestRatio(size, ratios)
			for i := 0; i < 100; i++ {
				require.Equal(t, want, NearestRatio(size, ratios), "%s / %s", familyID, size)
			}
		}
	}
}

func TestResolutionFromSize(t *testing.T) {
	require.Equal(t, Resolution1K, ResolutionFromSize("1024x1024"))
	require.Equal(t, Resolution1K, ResolutionFromSize("256x1024"))
	require.Equal(t, Resolution2K, ResolutionFromSize("1025x1024"))
	require.Equal(t, Resolution2K, ResolutionFromSize("2048x2048"))
	require.Equal(t, Resolution4K, ResolutionFromSize("2049x100"))
	require.Equal(t, Resolution4K, ResolutionFromSize("3840x2160"))

	// 非法/缺省回落默认档，与「省略 size 的请求保持 2K」一致。
	require.Equal(t, DefaultOutputResolution, ResolutionFromSize(""))
	require.Equal(t, DefaultOutputResolution, ResolutionFromSize("auto"))
	require.Equal(t, DefaultOutputResolution, ResolutionFromSize("1024"))
	require.Equal(t, DefaultOutputResolution, ResolutionFromSize("-1x-1"))
}

func TestSizeFromRatio(t *testing.T) {
	tests := []struct {
		resolution OutputResolution
		ratio      string
		want       Size
	}{
		{Resolution2K, "16:9", Size{Width: 2048, Height: 1152}},
		{Resolution1K, "9:16", Size{Width: 576, Height: 1024}},
		{Resolution4K, "21:9", Size{Width: 4096, Height: 1760}},
		{Resolution2K, "3:2", Size{Width: 2048, Height: 1360}},
		{Resolution1K, "1:1", Size{Width: 1024, Height: 1024}},
		{Resolution1K, "1:1000", Size{Width: 16, Height: 1024}},
	}
	for _, tt := range tests {
		t.Run(string(tt.resolution)+"_"+tt.ratio, func(t *testing.T) {
			got, ok := SizeFromRatio(tt.resolution, tt.ratio)
			require.True(t, ok)
			require.Equal(t, tt.want, got)
			// 长边不变，档位与计费档位一致。
			require.Equal(t, tt.resolution, ResolutionFromSize(got.String()))
		})
	}

	for _, ratio := range []string{"", "wide", "16:0", "0:9", "16x9", "-16:9"} {
		_, ok := SizeFromRatio(Resolution2K, ratio)
		require.False(t, ok, ratio)
	}
}

// TestEveryCatalogEntryReachableByFamilyAndSize 证明目录里 117 个组合全部能通过
// 「族级 id + size」触达——这是「不必把全量 id 塞进 model_mapping 白名单」的依据。
//
// size 由「比例 × 落在目标档位区间内的整数倍数」构造，而不是拿 conf 自己的像素表反推：
// Adobe 的像素表里，档位是标称值而非长边上界（gpt-image 的 5:4@1K 实为 1120x896，
// 长边已超 1024），用它反推会把 1K 判成 2K。计费认的是标称档位 conf.OutputResolution，
// 与实际返回的像素尺寸本就不是同一回事。
func TestEveryCatalogEntryReachableByFamilyAndSize(t *testing.T) {
	require.Len(t, imageModelCatalog, 102)

	for modelID, conf := range imageModelCatalog {
		t.Run(modelID, func(t *testing.T) {
			size := sizeInTier(t, conf.AspectRatio, conf.OutputResolution)

			resolved, err := ResolveImage(ImageRequest{ModelID: conf.Family, Size: size})
			require.NoError(t, err)
			require.Equal(t, modelID, resolved.ModelID,
				"族级 id %s + size %s 应解析回 %s", conf.Family, size, modelID)
		})
	}
}

// sizeInTier 造一个宽高比恰为 ratio、且长边落在 resolution 档位区间内的尺寸。
// 按整数倍放大保证 GCD 约分后仍是原比例。
func sizeInTier(t *testing.T, ratio string, resolution OutputResolution) string {
	t.Helper()
	var w, h int
	_, err := fmt.Sscanf(ratio, "%d:%d", &w, &h)
	require.NoError(t, err, "无法解析比例 %s", ratio)

	longSide := w
	if h > longSide {
		longSide = h
	}

	var factor int
	switch resolution {
	case Resolution1K:
		factor = 1024 / longSide
	case Resolution2K:
		factor = 2048 / longSide
	default:
		factor = 4096 / longSide
	}
	require.Positive(t, factor, "比例 %s 在档位 %s 下放不下", ratio, resolution)

	size := Size{Width: w * factor, Height: h * factor}
	require.Equal(t, resolution, ResolutionFromSize(size.String()),
		"构造的 size %s 应落在档位 %s", size, resolution)
	return size.String()
}

func mustFamilyRatios(t *testing.T, familyID string) []string {
	t.Helper()
	spec, ok := imageFamilySpecByID(familyID)
	require.True(t, ok, "未知族 %s", familyID)
	return spec.ratiosSorted
}

// Step 7 回归：firefly-nano-banana 的 upstreamModelVersion 曾错写成 nano-banana-2
// （与 Pro 撞车），导致所有普通版请求打在 Pro 上。这条守它不再回退。
func TestNanoBananaFamilyMapsToUnpluralizedUpstream(t *testing.T) {
	spec, ok := imageFamilySpecByID("firefly-nano-banana")
	require.True(t, ok)
	require.Equal(t, "nano-banana", spec.upstreamModelVersion,
		"firefly-nano-banana 必须指向上游 nano-banana，而不是 nano-banana-2（那是 Pro）")

	// Pro 与 base 不能撞车。
	pro, ok := imageFamilySpecByID("firefly-nano-banana-pro")
	require.True(t, ok)
	require.NotEqual(t, spec.upstreamModelVersion, pro.upstreamModelVersion,
		"nano-banana 与 nano-banana-pro 必须打到不同上游版本")
}

// gpt-image 2.5 家族的族级 id 必须真的能解析到 2.5 的上游 modelVersion。
// 改前所有 2.5 请求被 DefaultAdobeModelMapping 静默降级成 modelVersion=2。
func TestGPTImage25FamiliesResolveToRealUpstream(t *testing.T) {
	for family, wantVersion := range map[string]string{
		"firefly-gpt-image-2-5-flare": "gpt-image-2.5-flare",
		"firefly-gpt-image-2-5-prism": "gpt-image-2.5-prism",
	} {
		conf, err := ResolveImage(ImageRequest{ModelID: family, Size: "1024x1024"})
		require.NoError(t, err, family)
		require.Equal(t, wantVersion, conf.UpstreamModelVersion, family)
		require.Equal(t, PayloadKindGPTImage25, conf.PayloadKind, family)
		require.Equal(t, Size{Width: 1024, Height: 1024}, conf.SizePixels, family)
		require.Equal(t, Resolution1K, conf.OutputResolution, family)
	}
}

// enum-size 家族解析：族级 id + size → SizePixels 从允许集里挑最接近。
func TestEnumSizeFamiliesPickNearestSize(t *testing.T) {
	cases := []struct {
		family    string
		size      string
		wantPixel Size
	}{
		// flux 精确命中 1024x768（GCD 4:3）
		{"firefly-flux-pro", "1024x768", Size{1024, 768}},
		// gpt-4o-image 只有 3 个 size，请求 1400x1024 就近取 1536x1024（3:2 最接近 1400/1024）
		{"firefly-gpt-4o-image", "1400x1024", Size{1536, 1024}},
		// imagen 请求 1920x1080 就近取 1408x768（16:9 最接近）
		{"firefly-imagen-4", "1920x1080", Size{1408, 768}},
		// runway 精确命中 1920x1080
		{"firefly-runway-gen4-image", "1920x1080", Size{1920, 1080}},
	}
	for _, c := range cases {
		t.Run(c.family+"/"+c.size, func(t *testing.T) {
			conf, err := ResolveImage(ImageRequest{ModelID: c.family, Size: c.size})
			require.NoError(t, err)
			require.Equal(t, c.wantPixel, conf.SizePixels)
			require.Equal(t, PayloadKindSizeEnum, conf.PayloadKind)
			// ModelID 只到族级——不该像老族那样合成 family-res-ratio。
			require.Equal(t, c.family, conf.ModelID)
		})
	}
}

// 计费档位按实际发给上游的像素算：客户端写的 size 只决定比例，不能用来挑档。
func TestEnumSizeFamiliesBillByActualPixels(t *testing.T) {
	cases := []struct {
		family    string
		size      string
		wantPixel Size
		wantRes   OutputResolution
	}{
		// 232x100 比例最接近 2112x912：实际出 2K 以上长边，不能按请求的小尺寸记 1K。
		{"firefly-runway-gen4-image", "232x100", Size{2112, 912}, Resolution4K},
		// 请求 4096x4096 实际只出 1440x1440，不能按 4K 多收。
		{"firefly-flux-pro", "4096x4096", Size{1440, 1440}, Resolution2K},
		{"firefly-flux-ultra", "1x1", Size{1440, 1440}, Resolution2K},
		{"firefly-imagen-4", "8000x4500", Size{1408, 768}, Resolution2K},
		{"firefly-gpt-4o-image", "100x100", Size{1024, 1024}, Resolution1K},
		{"firefly-gpt-image-1.5", "4096x2730", Size{1536, 1024}, Resolution2K},
		// 空 size 取允许集第一个，档位跟着它走，不落到 2K 默认值。
		{"firefly-flux-pro", "", Size{1024, 768}, Resolution1K},
		{"firefly-runway-gen4-image", "", Size{1920, 1080}, Resolution2K},
	}
	for _, c := range cases {
		t.Run(c.family+"/"+c.size, func(t *testing.T) {
			conf, err := ResolveImage(ImageRequest{ModelID: c.family, Size: c.size})
			require.NoError(t, err)
			require.Equal(t, c.wantPixel, conf.SizePixels)
			require.Equal(t, c.wantRes, conf.OutputResolution)
		})
	}
}

// NearestSize 的兜底：允许集为空 / size 非法 应可预测地回落，不 panic。
func TestNearestSizeFallbacks(t *testing.T) {
	require.Equal(t, Size{}, NearestSize("1024x1024", nil))
	// 允许集非空但 size 缺 → 挑第一个（上游会当默认尺寸）。
	first := Size{1024, 768}
	require.Equal(t, first, NearestSize("", []Size{first, {1440, 1440}}))
	require.Equal(t, first, NearestSize("auto", []Size{first, {1440, 1440}}))
	require.Equal(t, first, NearestSize("bad", []Size{first, {1440, 1440}}))
}

// Step 10：白名单模式配出来的 model_mapping 是恒等对（"imagen-4" -> "imagen-4"），
// 到 ResolveImage 时还没有 firefly- 前缀。别名解析必须住在 adobe 包里，
// 否则账号一旦配了自定义 model_mapping，13 个对外名一个都解析不了。
func TestResolveImageAcceptsExternalModelNames(t *testing.T) {
	wantFamily := map[string]string{
		"gpt-image-2":            "firefly-gpt-image-2",
		"gpt-image-1.5":          "firefly-gpt-image-1.5",
		"gpt-image-2.5-flare":    "firefly-gpt-image-2-5-flare",
		"gpt-image-2.5-sunburst": "firefly-gpt-image-2-5-prism",
		"gemini-3-pro-image":     "firefly-nano-banana-pro",
		"gemini-2.5-flash-image": "firefly-nano-banana",
		"gemini-3.1-flash-image": "firefly-nano-banana2",
		"flux-pro":               "firefly-flux-pro",
		"flux-ultra":             "firefly-flux-ultra",
		"imagen-4":               "firefly-imagen-4",
		"imagen-4-fast":          "firefly-imagen-4-fast",
		"gpt-4o-image":           "firefly-gpt-4o-image",
		"runway-gen4-image":      "firefly-runway-gen4-image",
	}

	// 对外清单里的每一个名字都必须可解析——这就是用户在白名单选择器里能勾到的全集。
	for _, externalID := range ImageModelIDs() {
		family, ok := wantFamily[externalID]
		require.True(t, ok, "ImageModelIDs() 新增了 %s 但本用例没覆盖", externalID)

		conf, err := ResolveImage(ImageRequest{ModelID: externalID, Size: "1024x1024"})
		require.NoError(t, err, "对外名 %s 必须可解析", externalID)
		require.Equal(t, family, conf.Family, "对外名 %s 应落到族 %s", externalID, family)
	}
}

func TestResolveImageAcceptsLegacyNanoBananaAliases(t *testing.T) {
	for requested, wantFamily := range map[string]string{
		"nano-banana":                    "firefly-nano-banana",
		"nano-banana-pro":                "firefly-nano-banana-pro",
		"nano-banana2":                   "firefly-nano-banana2",
		"gemini-2.5-flash-image-preview": "firefly-nano-banana",
		"gemini-3-pro-image-preview":     "firefly-nano-banana-pro",
		"gemini-3.1-flash-image-preview": "firefly-nano-banana2",
	} {
		conf, err := ResolveImage(ImageRequest{ModelID: requested, Size: "1024x1024"})
		require.NoError(t, err, requested)
		require.Equal(t, wantFamily, conf.Family, requested)
	}
}

// sunburst 是 UI 展示名，上游 modelVersion 叫 prism。两个名字必须落到同一族——
// 这正是「别名翻译是协议知识、不是部署配置」的那半个理由。
func TestResolveImageSunburstAndPrismShareFamily(t *testing.T) {
	sunburst, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-sunburst", Size: "1024x1024"})
	require.NoError(t, err)
	prism, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-prism", Size: "1024x1024"})
	require.NoError(t, err)
	require.Equal(t, "firefly-gpt-image-2-5-prism", sunburst.Family)
	require.Equal(t, prism, sunburst)
}

// v2.5 不夹像素：4K 原样进 SizePixels；空/auto 不填像素、计费默认 2K。
func TestResolveImageGPTImage25PassesThroughRequestedPixels(t *testing.T) {
	flare4K, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-flare", Size: "3840x2160"})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 3840, Height: 2160}, flare4K.SizePixels)
	require.Equal(t, Resolution4K, flare4K.OutputResolution)

	prism1K, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-prism", Size: "1024x1024"})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 1024, Height: 1024}, prism1K.SizePixels)
	require.Equal(t, Resolution1K, prism1K.OutputResolution)

	flare23, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-flare", Size: "1024x1536"})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 1024, Height: 1536}, flare23.SizePixels)
	require.Equal(t, Resolution2K, flare23.OutputResolution)

	for _, size := range []string{"", "auto"} {
		conf, err := ResolveImage(ImageRequest{ModelID: "gpt-image-2.5-flare", Size: size})
		require.NoError(t, err, size)
		require.Equal(t, Size{}, conf.SizePixels, size)
		require.Equal(t, DefaultOutputResolution, conf.OutputResolution, size)
	}
}

func TestResolveImageGPTImage15SnapsToEnum(t *testing.T) {
	conf, err := ResolveImage(ImageRequest{ModelID: "firefly-gpt-image-1.5", Size: "1920x1080"})
	require.NoError(t, err)
	require.Equal(t, Size{1536, 1024}, conf.SizePixels)
	require.Equal(t, PayloadKindGPTImage25, conf.PayloadKind)
	require.Equal(t, "firefly-gpt-image-1.5", conf.ModelID)
	require.Equal(t, Resolution2K, conf.OutputResolution)

	auto, err := ResolveImage(ImageRequest{ModelID: "gpt-image-1.5", Size: ""})
	require.NoError(t, err)
	require.Equal(t, Size{1024, 1024}, auto.SizePixels)
	require.Equal(t, Resolution1K, auto.OutputResolution)
}

// 历史 gpt-image 名字在 Adobe 侧没有对应版本，一律落 2（与默认映射表一致）。
func TestResolveImageLegacyGPTImageAliases(t *testing.T) {
	for _, legacy := range []string{"gpt-image", "gpt-image-1", "gpt-image-1-mini"} {
		conf, err := ResolveImage(ImageRequest{ModelID: legacy, Size: "1024x1024"})
		require.NoError(t, err, legacy)
		require.Equal(t, "firefly-gpt-image-2", conf.Family, legacy)
	}
}

// 别名表不能把未知名字吞掉：错误文案要保持原样，且必须回显用户写的原串。
func TestResolveImageUnknownNameStillErrors(t *testing.T) {
	_, err := ResolveImage(ImageRequest{ModelID: "not-a-model", Size: "1024x1024"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown firefly image model")
	require.Contains(t, err.Error(), "not-a-model")
}

// 别名解析只到族级：对外名不带尺寸后缀，比例/分辨率一律从 size 推导。
// 放行 "imagen-4-2k-16x9" 会让用户以为可以在模型名里编码尺寸，而那条路不存在。
func TestResolveImageExternalNamesAreFamilyLevelOnly(t *testing.T) {
	_, err := ResolveImage(ImageRequest{ModelID: "imagen-4-2k-16x9", Size: "1024x1024"})
	require.Error(t, err)
}

// IsImageModelID 只认前缀，IsExternalImageModelID 只认别名表。两者不能互相放宽。
func TestImageModelIDPredicatesStayDisjoint(t *testing.T) {
	require.True(t, IsImageModelID("firefly-imagen-4"))
	require.False(t, IsExternalImageModelID("firefly-imagen-4"))

	require.True(t, IsExternalImageModelID("imagen-4"))
	require.False(t, IsImageModelID("imagen-4"))

	require.True(t, IsExternalImageModelID("  IMAGEN-4  "), "应大小写不敏感并忽略首尾空白")

	require.False(t, IsExternalImageModelID("gpt-4o"))
	require.False(t, IsExternalImageModelID(""))
}

// 管理端模型选择器的展示名：Adobe 此前直接显示裸 id（gpt-image-2），
// 挨着 OpenAI 的「GPT Image 2」很扎眼。
func TestDisplayLabel(t *testing.T) {
	for _, externalID := range ImageModelIDs() {
		label, ok := DisplayLabel(externalID)
		require.True(t, ok, "对外名 %s 必须有展示名", externalID)
		require.NotEmpty(t, label, externalID)
		// 内部路由用的族前缀不该出现在用户面（Step 8 的既定原则）。
		require.NotContains(t, label, "Firefly", "展示名 %q 不应带产品前缀", label)
		require.NotContains(t, label, "firefly-", externalID)
	}

	label, ok := DisplayLabel("gpt-image-2")
	require.True(t, ok)
	require.Equal(t, "GPT Image 2", label)

	// sunburst 是 UI 名，族是 prism——展示名应跟着 UI 名走。
	label, ok = DisplayLabel("gpt-image-2.5-sunburst")
	require.True(t, ok)
	require.Equal(t, "GPT Image 2.5 Sunburst", label)

	label, ok = DisplayLabel("gemini-2.5-flash-image")
	require.True(t, ok)
	require.Equal(t, "Gemini 2.5 Flash Image", label)

	label, ok = DisplayLabel("gemini-3-pro-image")
	require.True(t, ok)
	require.Equal(t, "Gemini 3 Pro Image", label)

	label, ok = DisplayLabel("gemini-3.1-flash-image")
	require.True(t, ok)
	require.Equal(t, "Gemini 3.1 Flash Image", label)

	label, ok = DisplayLabel("  IMAGEN-4  ")
	require.True(t, ok, "应大小写不敏感并忽略首尾空白")
	require.Equal(t, "Imagen 4", label)

	// 未知名返回 false，调用方回落到原始 id。
	_, ok = DisplayLabel("not-a-model")
	require.False(t, ok)
	_, ok = DisplayLabel("firefly-imagen-4")
	require.False(t, ok, "内部族 id 不是对外名")
}
