package money

import (
	"errors"
	"testing"
)

func TestParseUsesTheDefaultCurrency(t *testing.T) {
	for _, test := range []struct {
		text, fallback string
		want           Amount
	}{
		{"123.50", "AUD", Amount{Minor: 12350, Currency: "AUD"}},
		{"45", "AUD", Amount{Minor: 4500, Currency: "AUD"}},
		{"0.05", "AUD", Amount{Minor: 5, Currency: "AUD"}},
		{"  12.3  ", "AUD", Amount{Minor: 1230, Currency: "AUD"}},
		{"-12.50", "AUD", Amount{Minor: -1250, Currency: "AUD"}},
		{"+12.50", "AUD", Amount{Minor: 1250, Currency: "AUD"}},
		{"USD 45", "AUD", Amount{Minor: 4500, Currency: "USD"}},
		{"AU 45", "USD", Amount{Minor: 4500, Currency: "AUD"}},
		{"45 au", "USD", Amount{Minor: 4500, Currency: "AUD"}},
		{"AU45", "USD", Amount{Minor: 4500, Currency: "AUD"}},
		{"-AU45", "USD", Amount{Minor: -4500, Currency: "AUD"}},
		{"JP 1200", "", Amount{Minor: 1200, Currency: "JPY"}},
		{"DE 45", "", Amount{Minor: 4500, Currency: "EUR"}},
		{"45 USD", "AUD", Amount{Minor: 4500, Currency: "USD"}},
		{"usd 45", "AUD", Amount{Minor: 4500, Currency: "USD"}},
		{"USD45", "AUD", Amount{Minor: 4500, Currency: "USD"}},
		{"-USD45", "AUD", Amount{Minor: -4500, Currency: "USD"}},
		{"JPY 1200", "AUD", Amount{Minor: 1200, Currency: "JPY"}},
		{"KWD 1.234", "AUD", Amount{Minor: 1234, Currency: "KWD"}},
		{"CNY 100", "", Amount{Minor: 10000, Currency: "CNY"}},
	} {
		got, err := Parse(test.text, test.fallback)
		if err != nil {
			t.Errorf("Parse(%q, %q): %v", test.text, test.fallback, err)
			continue
		}
		if got != test.want {
			t.Errorf("Parse(%q, %q) = %+v, want %+v", test.text, test.fallback, got, test.want)
		}
	}
}

func TestCountryAliasesResolveToSupportedCurrencies(t *testing.T) {
	for country, currency := range countryCurrencies {
		if !Known(currency) {
			t.Errorf("%s resolves to unsupported currency %s", country, currency)
		}
	}
}

func TestParseRefusesWhatWouldBeGuesswork(t *testing.T) {
	for _, test := range []struct{ text, fallback string }{
		{"1,234", "AUD"},               // grouping means different things in different places
		{"12.345", "AUD"},              // more decimals than the currency has
		{"JPY 12.5", "AUD"},            // a currency with no decimals
		{"twelve", "AUD"},              // not a number
		{"12.50 USD AUD", ""},          // two currencies
		{"12.50", "ZZZ"},               // a currency this build does not know
		{"ZZZ 12.50", "AUD"},           // the same, named in the text
		{"ZZ 12.50", "AUD"},            // an unknown country code
		{"12.50", ""},                  // no currency anywhere
		{"9999999999999999999", "AUD"}, // beyond int64
	} {
		if got, err := Parse(test.text, test.fallback); err == nil {
			t.Errorf("Parse(%q, %q) = %+v, want an error", test.text, test.fallback, got)
		}
	}
}

func TestParseTellsEmptyApartFromWrong(t *testing.T) {
	for _, text := range []string{"", "   "} {
		if _, err := Parse(text, "AUD"); !errors.Is(err, ErrEmpty) {
			t.Errorf("Parse(%q) error = %v, want ErrEmpty", text, err)
		}
	}
	if _, err := Parse("12.50", ""); !errors.Is(err, ErrNoCurrency) {
		t.Errorf("Parse with no currency error = %v, want ErrNoCurrency", err)
	}
	var unknown UnknownCurrencyError
	if _, err := Parse("ZZZ 1", ""); !errors.As(err, &unknown) || unknown.Code != "ZZZ" {
		t.Errorf("Parse of an unknown currency error = %v, want UnknownCurrencyError{ZZZ}", err)
	}
}

func TestZeroAmountIsNotAZeroAmount(t *testing.T) {
	if (Amount{}).Valid() {
		t.Error("the zero Amount reports itself valid; an invitation has no total, not a total of nothing")
	}
	if !(Amount{Currency: "AUD"}).Valid() {
		t.Error("an amount of zero AUD should be valid")
	}
}

func TestStringAndDisplay(t *testing.T) {
	for _, test := range []struct {
		amount        Amount
		text, display string
	}{
		{Amount{Minor: 12350, Currency: "AUD"}, "AUD 123.50", "AUD 123.50"},
		{Amount{Minor: 124050, Currency: "AUD"}, "AUD 1240.50", "AUD 1,240.50"},
		{Amount{Minor: 123456789, Currency: "AUD"}, "AUD 1234567.89", "AUD 1,234,567.89"},
		{Amount{Minor: 5, Currency: "AUD"}, "AUD 0.05", "AUD 0.05"},
		{Amount{Minor: 0, Currency: "AUD"}, "AUD 0.00", "AUD 0.00"},
		{Amount{Minor: -1250, Currency: "AUD"}, "AUD -12.50", "AUD -12.50"},
		{Amount{Minor: 1200, Currency: "JPY"}, "JPY 1200", "JPY 1,200"},
		{Amount{Minor: 1234, Currency: "KWD"}, "KWD 1.234", "KWD 1.234"},
	} {
		if got := test.amount.String(); got != test.text {
			t.Errorf("%+v.String() = %q, want %q", test.amount, got, test.text)
		}
		if got := test.amount.Display(); got != test.display {
			t.Errorf("%+v.Display() = %q, want %q", test.amount, got, test.display)
		}
	}
}

func TestParseAndStringRoundTrip(t *testing.T) {
	for _, text := range []string{"AUD 123.50", "JPY 1200", "KWD 1.234", "AUD -12.50"} {
		amount, err := Parse(text, "")
		if err != nil {
			t.Fatalf("Parse(%q): %v", text, err)
		}
		again, err := Parse(amount.String(), "")
		if err != nil {
			t.Fatalf("Parse(%q): %v", amount.String(), err)
		}
		if again != amount {
			t.Errorf("%q parsed, printed and parsed again = %+v, want %+v", text, again, amount)
		}
	}
}

func TestSumNeverCrossesCurrencies(t *testing.T) {
	totals := Sum([]Amount{
		{Minor: 100000, Currency: "AUD"},
		{Minor: 24050, Currency: "AUD"},
		{Minor: 38000, Currency: "USD"},
		{Minor: 1200, Currency: "JPY"},
		{},
	})
	want := []Total{
		{Amount: Amount{Minor: 124050, Currency: "AUD"}, Count: 2},
		{Amount: Amount{Minor: 1200, Currency: "JPY"}, Count: 1},
		{Amount: Amount{Minor: 38000, Currency: "USD"}, Count: 1},
	}
	if len(totals) != len(want) {
		t.Fatalf("Sum returned %d totals, want %d: %+v", len(totals), len(want), totals)
	}
	for index := range want {
		if totals[index] != want[index] {
			t.Errorf("total %d = %+v, want %+v", index, totals[index], want[index])
		}
	}
}

func TestSumOfManyCentsStaysExact(t *testing.T) {
	amounts := make([]Amount, 0, 1000)
	for range 1000 {
		amounts = append(amounts, Amount{Minor: 1, Currency: "AUD"})
	}
	totals := Sum(amounts)
	if len(totals) != 1 || totals[0].Amount.Minor != 1000 {
		t.Fatalf("Sum of a thousand cents = %+v, want 1000 minor units", totals)
	}
	if got := totals[0].Amount.Display(); got != "AUD 10.00" {
		t.Errorf("Sum of a thousand cents prints %q, want %q", got, "AUD 10.00")
	}
}

func TestSumOfNothing(t *testing.T) {
	if totals := Sum(nil); len(totals) != 0 {
		t.Errorf("Sum(nil) = %+v, want none", totals)
	}
	if totals := Sum([]Amount{{}, {}}); len(totals) != 0 {
		t.Errorf("Sum of amounts that are not amounts = %+v, want none", totals)
	}
}
