package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func newTestSePay(t *testing.T) *SePay {
	t.Helper()
	p, err := NewSePay("1", map[string]string{
		"accountNumber": "0123499999",
		"bankCode":      "MB",
		"apiKey":        "webhook-secret",
	})
	if err != nil {
		t.Fatalf("NewSePay() error: %v", err)
	}
	return p
}

func TestNewSePayRejectsUnknownBank(t *testing.T) {
	t.Parallel()

	_, err := NewSePay("1", map[string]string{
		"accountNumber": "0123499999",
		"bankCode":      "NOT_A_BANK",
		"apiKey":        "k",
	})
	if err == nil {
		t.Fatal("NewSePay() should reject a bank code with no Napas BIN")
	}
}

// A wrong CRC makes a banking app refuse the code outright, so the checksum is
// checked against a payload whose bytes are fully determined by the inputs.
func TestBuildVietQRPayloadStructureAndChecksum(t *testing.T) {
	t.Parallel()

	qr := buildVietQRPayload("970422", "0123499999", "100000", "SUB220260101ABCD1234")

	for _, want := range []string{
		"000201",                   // payload format indicator
		"010212",                   // dynamic QR (carries an amount)
		"0010A000000727",           // Napas GUID
		"0006970422",               // acquirer BIN
		"01100123499999",           // beneficiary account
		"0208QRIBFTTA",             // transfer to account
		"5303704",                  // VND
		"5406100000",               // amount
		"5802VN",                   // country
		"0820SUB220260101ABCD1234", // order code as the transfer description
	} {
		if !strings.Contains(qr, want) {
			t.Fatalf("payload %q missing %q", qr, want)
		}
	}

	// Pinned: a payload built from fixed inputs is byte-for-byte deterministic,
	// so any change to field order, lengths or the checksum shows up here.
	const want = "00020101021238540010A00000072701240006970422011001234999990208QRIBFTTA" +
		"530370454061000005802VN62240820SUB220260101ABCD123463041CB1"
	if qr != want {
		t.Fatalf("payload =\n%s\nwant\n%s", qr, want)
	}
}

// The checksum is only interoperable if it is genuinely CRC-16/CCITT-FALSE;
// recomputing it with the same function would prove nothing, so this asserts
// the check value the specification publishes for that algorithm.
func TestVietQRCRC16MatchesSpecCheckValue(t *testing.T) {
	t.Parallel()

	if got := vietQRCRC16("123456789"); got != "29B1" {
		t.Fatalf("vietQRCRC16(\"123456789\") = %s, want 29B1", got)
	}
}

func TestSePayCreatePaymentEmitsScannableQR(t *testing.T) {
	t.Parallel()

	resp, err := newTestSePay(t).CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "SUB220260101ABCD1234",
		Amount:  "100000",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error: %v", err)
	}
	if resp.QRCode == "" {
		t.Fatal("CreatePayment() returned no QR payload")
	}
	if resp.Currency != payment.CurrencySePay {
		t.Fatalf("currency = %q, want %q", resp.Currency, payment.CurrencySePay)
	}
	if resp.Transfer == nil || resp.Transfer.Content != "SUB220260101ABCD1234" {
		t.Fatalf("transfer instructions = %+v, want the order code as content", resp.Transfer)
	}
	// No upstream call happens, so nothing can carry a trade number yet.
	if resp.TradeNo != "" {
		t.Fatalf("TradeNo = %q, want empty until the transfer arrives", resp.TradeNo)
	}
}

func TestSePayVerifyNotification(t *testing.T) {
	t.Parallel()

	const body = `{"id":92704,"gateway":"MBBank","transactionDate":"2026-01-01 14:02:37",` +
		`"accountNumber":"0123499999","code":null,"content":"CT DEN:123 SUB2 20260101 ABCD1234 GD",` +
		`"transferType":"in","transferAmount":100000,"referenceCode":"MBVCB.0000000000","description":""}`

	auth := map[string]string{"authorization": "Apikey webhook-secret"}

	t.Run("accepts a credit carrying an order code", func(t *testing.T) {
		t.Parallel()
		notification, err := newTestSePay(t).VerifyNotification(context.Background(), body, auth)
		if err != nil {
			t.Fatalf("VerifyNotification() error: %v", err)
		}
		if notification == nil {
			t.Fatal("VerifyNotification() returned no notification")
		}
		// Banks reformat the description freely; the code has to survive that.
		if notification.OrderID != "SUB220260101ABCD1234" {
			t.Fatalf("OrderID = %q, want SUB220260101ABCD1234", notification.OrderID)
		}
		if notification.Amount != 100000 {
			t.Fatalf("Amount = %v, want 100000", notification.Amount)
		}
		if notification.Status != payment.ProviderStatusSuccess {
			t.Fatalf("Status = %q, want success", notification.Status)
		}
	})

	t.Run("rejects a mismatched api key", func(t *testing.T) {
		t.Parallel()
		_, err := newTestSePay(t).VerifyNotification(context.Background(), body,
			map[string]string{"authorization": "Apikey wrong"})
		if err == nil {
			t.Fatal("VerifyNotification() should reject a mismatched api key")
		}
	})

	t.Run("ignores outgoing transfers", func(t *testing.T) {
		t.Parallel()
		out := `{"id":1,"accountNumber":"0123499999","content":"SUB220260101ABCD1234",` +
			`"transferType":"out","transferAmount":100000}`
		notification, err := newTestSePay(t).VerifyNotification(context.Background(), out, auth)
		if err != nil {
			t.Fatalf("VerifyNotification() error: %v", err)
		}
		if notification != nil {
			t.Fatalf("a debit must not fulfil an order, got %+v", notification)
		}
	})

	t.Run("ignores credits with no order code", func(t *testing.T) {
		t.Parallel()
		unrelated := `{"id":2,"accountNumber":"0123499999","content":"salary january",` +
			`"transferType":"in","transferAmount":5000000}`
		notification, err := newTestSePay(t).VerifyNotification(context.Background(), unrelated, auth)
		if err != nil {
			t.Fatalf("VerifyNotification() error: %v", err)
		}
		if notification != nil {
			t.Fatalf("an unrelated credit must be ignored, got %+v", notification)
		}
	})

	t.Run("rejects a credit to another account", func(t *testing.T) {
		t.Parallel()
		foreign := `{"id":3,"accountNumber":"9999999999","content":"SUB220260101ABCD1234",` +
			`"transferType":"in","transferAmount":100000}`
		if _, err := newTestSePay(t).VerifyNotification(context.Background(), foreign, auth); err == nil {
			t.Fatal("VerifyNotification() should reject a credit to a different account")
		}
	})
}
