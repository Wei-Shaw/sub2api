package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

const ledgerDecimalPlaces int32 = 10

type Currency string

const CurrencyUSD Currency = "USD"

type Money struct {
	amount   decimal.Decimal
	currency Currency
}

func NewMoney(rawAmount string, currency Currency) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}

	amount, err := decimal.NewFromString(strings.TrimSpace(rawAmount))
	if err != nil {
		return Money{}, fmt.Errorf("invalid money amount %q: %w", rawAmount, err)
	}
	if amount.IsNegative() {
		return Money{}, fmt.Errorf("money amount must not be negative")
	}
	if err := validateMoneyDecimalInvariant(amount); err != nil {
		return Money{}, err
	}

	return Money{amount: amount, currency: currency}, nil
}

// MustUSD is intended for tests and constants whose decimal literal is known
// at compile time. Production input must use NewMoney and handle its error.
func MustUSD(rawAmount string) Money {
	money, err := NewMoney(rawAmount, CurrencyUSD)
	if err != nil {
		panic(err)
	}
	return money
}

func (m Money) Currency() Currency {
	return m.currency
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.requireSameCurrency(other); err != nil {
		return Money{}, err
	}
	amount := m.amount.Add(other.amount)
	if err := validateMoneyDecimalInvariant(amount); err != nil {
		return Money{}, err
	}
	return Money{amount: amount, currency: m.currency}, nil
}

func (m Money) Subtract(other Money) (Money, error) {
	if err := m.requireSameCurrency(other); err != nil {
		return Money{}, err
	}
	amount := m.amount.Sub(other.amount)
	if err := validateMoneyDecimalInvariant(amount); err != nil {
		return Money{}, err
	}
	return Money{amount: amount, currency: m.currency}, nil
}

func (m Money) Compare(other Money) (int, error) {
	if err := m.requireSameCurrency(other); err != nil {
		return 0, err
	}
	return m.amount.Cmp(other.amount), nil
}

func (m Money) IsNegative() bool {
	return m.amount.IsNegative()
}

func (m Money) Decimal() decimal.Decimal {
	return m.amount
}

// String returns the exact NUMERIC(20,10) representation used at the SQL
// boundary. Callers must pass this string (or Decimal), never a float64.
func (m Money) String() string {
	return m.amount.StringFixed(ledgerDecimalPlaces)
}

func (m Money) MarshalJSON() ([]byte, error) {
	if err := validateCurrency(m.currency); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Amount   string   `json:"amount"`
		Currency Currency `json:"currency"`
	}{
		Amount:   m.String(),
		Currency: m.currency,
	})
}

func (m *Money) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("cannot unmarshal money into nil receiver")
	}

	var payload struct {
		Amount   string   `json:"amount"`
		Currency Currency `json:"currency"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode money: %w", err)
	}

	parsed, err := NewMoney(payload.Amount, payload.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

func (m Money) requireSameCurrency(other Money) error {
	if err := validateCurrency(m.currency); err != nil {
		return err
	}
	if err := validateCurrency(other.currency); err != nil {
		return err
	}
	if m.currency != other.currency {
		return fmt.Errorf("currency mismatch: %s != %s", m.currency, other.currency)
	}
	return nil
}

func validateCurrency(currency Currency) error {
	value := string(currency)
	if len(value) != 3 {
		return fmt.Errorf("currency must be a 3-letter uppercase ISO code")
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return fmt.Errorf("currency must be a 3-letter uppercase ISO code")
		}
	}
	return nil
}

func validateMoneyDecimalInvariant(amount decimal.Decimal) error {
	if amount.Exponent() < -ledgerDecimalPlaces {
		return fmt.Errorf("money amount must not have more than %d decimal places", ledgerDecimalPlaces)
	}
	if len(amount.Abs().Truncate(0).String()) > 10 {
		return fmt.Errorf("money amount exceeds NUMERIC(20,10) integer precision")
	}
	return nil
}
