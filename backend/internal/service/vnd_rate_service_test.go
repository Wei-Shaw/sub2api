package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Trimmed copy of a real Vietcombank pXML.aspx response, including the leading
// comment and the thousands separators that make the amounts unparseable by
// strconv on their own.
const vcbSampleXML = `<!--For reference only. Only one request every 5 minutes!-->
<ExrateList>
  <DateTime>8/14/2026 10:20:00 AM</DateTime>
  <Exrate CurrencyCode="AUD" CurrencyName="AUSTRALIAN DOLLAR   " Buy="17,936.08" Transfer="18,117.25" Sell="18,697.70" />
  <Exrate CurrencyCode="CNY" CurrencyName="YUAN RENMINBI       " Buy="3,769.67" Transfer="3,807.75" Sell="3,930.25" />
  <Exrate CurrencyCode="THB" CurrencyName="THAILAND BAHT       " Buy="692.39" Transfer="769.32" Sell="801.95" />
  <Exrate CurrencyCode="USD" CurrencyName="US DOLLAR           " Buy="25,860.00" Transfer="25,890.00" Sell="26,270.00" />
  <Source>Joint Stock Commercial Bank for Foreign Trade of Vietnam - Vietcombank</Source>
</ExrateList>`

func TestParseVCBRate_Columns(t *testing.T) {
	for _, tc := range []struct {
		column string
		want   float64
	}{
		{VNDRateColumnBuy, 25860},
		{VNDRateColumnTransfer, 25890},
		{VNDRateColumnSell, 26270},
	} {
		got, err := parseVCBRate([]byte(vcbSampleXML), "USD", tc.column)
		require.NoError(t, err, tc.column)
		require.InDelta(t, tc.want, got, 0.001, tc.column)
	}
}

// An unrecognised column must not silently read a different one; sell is the
// documented default and the conservative side of the board for a seller.
func TestParseVCBRate_UnknownColumnFallsBackToSell(t *testing.T) {
	got, err := parseVCBRate([]byte(vcbSampleXML), "USD", "midpoint")
	require.NoError(t, err)
	require.InDelta(t, 26270, got, 0.001)
}

func TestParseVCBRate_CurrencyLookupIsCaseInsensitive(t *testing.T) {
	got, err := parseVCBRate([]byte(vcbSampleXML), "usd", VNDRateColumnSell)
	require.NoError(t, err)
	require.InDelta(t, 26270, got, 0.001)
}

func TestParseVCBRate_MissingCurrency(t *testing.T) {
	_, err := parseVCBRate([]byte(vcbSampleXML), "KRW", VNDRateColumnSell)
	require.Error(t, err)
	require.Contains(t, err.Error(), "KRW")
}

func TestParseVCBRate_MalformedDocument(t *testing.T) {
	_, err := parseVCBRate([]byte("not xml at all"), "USD", VNDRateColumnSell)
	require.Error(t, err)
}

// A currency present but with a blank column must fail rather than resolve to
// 0, which downstream means "conversion disabled, charge the plan price as-is".
func TestParseVCBRate_BlankColumnIsAnError(t *testing.T) {
	doc := `<ExrateList><Exrate CurrencyCode="USD" Buy="" Transfer="" Sell="" /></ExrateList>`
	_, err := parseVCBRate([]byte(doc), "USD", VNDRateColumnSell)
	require.Error(t, err)
}

func TestParseVCBAmount(t *testing.T) {
	got, err := parseVCBAmount(" 26,270.00 ")
	require.NoError(t, err)
	require.InDelta(t, 26270, got, 0.001)

	_, err = parseVCBAmount("")
	require.Error(t, err)

	_, err = parseVCBAmount("abc")
	require.Error(t, err)
}

func TestApplyVNDRateMargin(t *testing.T) {
	require.InDelta(t, 26270, applyVNDRateMargin(26270, 0), 0.001)
	require.InDelta(t, 26795.4, applyVNDRateMargin(26270, 2), 0.001)
	// Negative margins are ignored so a typo cannot undercut the board rate.
	require.InDelta(t, 26270, applyVNDRateMargin(26270, -5), 0.001)
}
