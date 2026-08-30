package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestX5M5XPerRequestHTMLHelpers(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<table><tr class="request-table-row">
		<th class="m-name" title="MiniMax-M3">MiniMax-M3</th>
		<td class="request-cost">¥0.0010</td><td class="request-cost">¥0.0015</td><td class="request-cost">¥0.0020</td>
	</tr></table>`))
	require.NoError(t, err)
	require.Equal(t, "MiniMax-M3", firstDescendantText(doc, "th", "m-name"))
	require.Equal(t, []float64{0.001, 0.0015, 0.002}, descendantPriceTexts(doc, "request-cost"))
}
