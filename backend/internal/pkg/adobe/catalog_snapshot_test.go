//go:build unit

package adobe

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// discoverySnapshot 是 testdata/discovery_snapshot.json 的最小反序列化形状。
// 只解析我们目录里真正引用的字段——保留原始文件更容易做人工核对（比如版本 displayName），
// 但结构体越小、测试越不容易被上游 schema 抖动打破。
type discoverySnapshot struct {
	Models []discoveryModel `json:"models"`
}
type discoveryModel struct {
	ModelID       string                      `json:"modelId"`
	ModelVersions map[string]discoveryVersion `json:"modelVersions"`
}
type discoveryVersion struct {
	Enabled          bool     `json:"enabled"`
	ReleaseReadiness string   `json:"releaseReadiness"`
	OutputModality   []string `json:"outputModality"`
	SizeEnum         []Size   `json:"size_enum"`
}

func loadDiscoverySnapshot(t *testing.T) *discoverySnapshot {
	t.Helper()
	raw, err := os.ReadFile("testdata/discovery_snapshot.json")
	require.NoError(t, err, "读不到 discovery snapshot——是不是 testdata 目录丢了？")
	var snap discoverySnapshot
	require.NoError(t, json.Unmarshal(raw, &snap))
	return &snap
}

func (s *discoverySnapshot) hasVersion(modelID, modelVersion string) bool {
	for _, m := range s.Models {
		if m.ModelID != modelID {
			continue
		}
		_, ok := m.ModelVersions[modelVersion]
		return ok
	}
	return false
}

func (s *discoverySnapshot) sizeEnum(modelID, modelVersion string) []Size {
	for _, m := range s.Models {
		if m.ModelID != modelID {
			continue
		}
		return m.ModelVersions[modelVersion].SizeEnum
	}
	return nil
}

// TestCatalogAgainstDiscoverySnapshot 是「上游还认我们的 modelId+modelVersion 吗」的守卫。
//
// 上游哪天下架 gpt-image-2.5-prism（比如换成 -prism-2）时，这条测试会立刻报红——
// 而不是等到端到端请求收到 400 才发现。刷新 snapshot 的做法：跑一遍 adobe_firefly.py
// 抓包，把 discovery 响应更新到 testdata/discovery_snapshot.json（脚本已经在 scratchpad 里）。
func TestCatalogAgainstDiscoverySnapshot(t *testing.T) {
	snap := loadDiscoverySnapshot(t)
	for _, spec := range imageFamilySpecs {
		require.True(t,
			snap.hasVersion(spec.upstreamModelID, spec.upstreamModelVersion),
			"family %s 声明的上游 (%s, %s) 已不在 discovery snapshot 中——上游可能下架了",
			spec.familyID, spec.upstreamModelID, spec.upstreamModelVersion)
	}
}

// TestSupportedSizesMatchDiscoverySnapshot 守 enum-size 家族的允许尺寸表。
// 上游哪天悄悄改了 flux 的 5 个 size（换成别的、加了新的），我们的 NearestSize 会挑到
// 上游拒收的尺寸，请求全 400。这条把 catalog.go 里的允许集与 snapshot 逐点对齐。
func TestSupportedSizesMatchDiscoverySnapshot(t *testing.T) {
	snap := loadDiscoverySnapshot(t)
	for _, spec := range imageFamilySpecs {
		if len(spec.supportedSizes) == 0 {
			continue
		}
		want := snap.sizeEnum(spec.upstreamModelID, spec.upstreamModelVersion)
		require.NotEmpty(t, want,
			"family %s 的 (%s, %s) 在 snapshot 里没有 size_enum——是不是 snapshot 精简时误删了？",
			spec.familyID, spec.upstreamModelID, spec.upstreamModelVersion)
		require.ElementsMatch(t, want, spec.supportedSizes,
			"family %s 的允许尺寸表与上游 discovery 不一致", spec.familyID)
	}
}

// TestDiscoverySnapshotEnabledGACoverage 是**提示性**的（不 fail 只 t.Log）：
// snapshot 里凡是 image / enabled=true / readiness=ga 的 modelVersion，
// 都应该在 sub2api 有对应 family。用来提示「上游又出新版本了，考虑跟进」。
//
// 不 fail 是刻意的——我们不追每个上游新版本，追不上就一直红没意义。
func TestDiscoverySnapshotEnabledGACoverage(t *testing.T) {
	snap := loadDiscoverySnapshot(t)
	covered := make(map[string]bool)
	for _, spec := range imageFamilySpecs {
		covered[spec.upstreamModelID+"/"+spec.upstreamModelVersion] = true
	}
	var uncovered []string
	for _, m := range snap.Models {
		for vname, v := range m.ModelVersions {
			if !v.Enabled || v.ReleaseReadiness != "ga" {
				continue
			}
			isImage := false
			for _, o := range v.OutputModality {
				if o == "image" {
					isImage = true
					break
				}
			}
			if !isImage {
				continue
			}
			if !covered[m.ModelID+"/"+vname] {
				uncovered = append(uncovered, m.ModelID+"/"+vname)
			}
		}
	}
	if len(uncovered) > 0 {
		t.Logf("snapshot 里有 %d 个 enabled+ga 的图像版本 sub2api 未覆盖: %v",
			len(uncovered), uncovered)
	}
}
