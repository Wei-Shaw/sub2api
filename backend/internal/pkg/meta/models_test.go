//go:build unit

package meta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsMatchCatalogIDs(t *testing.T) {
	models := DefaultModels()
	ids := DefaultModelIDs()
	require.Len(t, models, len(ids))
	for i, model := range models {
		require.Equal(t, ids[i], model.ID)
		require.Equal(t, "meta", model.OwnedBy)
		require.Equal(t, ids[i], model.DisplayName)
	}
}

func TestImageModelIsServedAndClassified(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "muse-image-1.0")
	require.True(t, IsMuseModelID("muse-image-1.0"))
	require.True(t, IsImageModelID("Muse-Image-1.0"))
	require.False(t, IsImageModelID("muse-spark-1.3"))
	require.False(t, IsMuseModelID("muse-voice-transcribe-1.0"))
	require.False(t, IsMuseModelID("sam-3.1"))
}

func TestParseModelListFiltersAndDeduplicates(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"muse-spark-1.3","object":"model","owned_by":"meta","created":123},
		{"id":"gpt-5","object":"model"},
		{"id":"muse-wonder-1.0"},
		{"id":"muse-spark-1.3"},
		{"id":"sam-3.1"},
		{"id":"muse-image-1.0"},
		{"id":"muse-voice-transcribe-1.0"},
		{"id":""}
	]}`)

	models, err := ParseModelList(body)
	require.NoError(t, err)
	require.Len(t, models, 3)
	require.Equal(t, "muse-spark-1.3", models[0].ID)
	require.Equal(t, "meta", models[0].OwnedBy)
	require.Equal(t, "muse-wonder-1.0", models[1].ID)
	require.Equal(t, "meta", models[1].OwnedBy)
	require.Equal(t, "muse-image-1.0", models[2].ID)

	_, err = ParseModelList(nil)
	require.Error(t, err)
	_, err = ParseModelList([]byte(`{"data":[{"id":"gpt-5"}]}`))
	require.Error(t, err)
}
