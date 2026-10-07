package usagestats

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestOptionalTokenCount(t *testing.T) {
	for _, raw := range []string{`{}`, `{"r":null}`, `{"r":-1}`, `{"r":1.5}`, `{"r":"12"}`, `{"r":2147483648}`} {
		require.Nil(t, OptionalTokenCount(gjson.Parse(raw), "r"), raw)
	}
	for _, raw := range []string{`{"r":0}`, `{"r":12}`} {
		value := OptionalTokenCount(gjson.Parse(raw), "r")
		require.NotNil(t, value)
		require.EqualValues(t, gjson.Get(raw, "r").Int(), *value)
	}
}
