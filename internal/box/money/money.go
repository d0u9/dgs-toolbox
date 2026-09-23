// Package money is the one definition of an amount written on a scanned
// document: how it is read from what someone typed, how it is stored, how it
// is shown, and how several of them are added up. It reads no files and draws
// nothing, so the intake page, the browser and a report all agree on what
// "123.50" meant.
//
// An amount is an integer count of a currency's minor unit and the currency it
// is counted in. Nothing here is a float: summing a few dozen float64 receipts
// produces answers like 1234.9999999998, which is not acceptable for the only
// reason a receipt is kept.
package money

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Amount is a signed count of Currency's minor unit — cents for AUD, whole yen
// for JPY. Negative is a refund or a cancelled purchase, and is normal.
//
// The zero Amount is not a zero amount. It has no currency, and Valid reports
// false: a document with no total, such as an invitation, has no Amount at all
// rather than an Amount of zero.
type Amount struct {
	Minor    int64
	Currency string
}

// Valid reports whether a is an amount rather than the zero value.
func (a Amount) Valid() bool { _, known := Exponent(a.Currency); return known }

var (
	// ErrEmpty is returned by Parse for text holding nothing. It is how a
	// skipped amount field is told apart from a malformed one.
	ErrEmpty = errors.New("no amount")
	// ErrNoCurrency is returned when text names no currency and no default was
	// given, which is what an empty box.currency means.
	ErrNoCurrency = errors.New("no currency: write one, such as USD 45")
)

// UnknownCurrencyError is returned for a currency this build has no minor-unit
// exponent for. Guessing two decimals would be wrong by a factor of a hundred
// for JPY and by a thousand for KWD, so an unknown code is refused instead.
type UnknownCurrencyError struct{ Code string }

func (e UnknownCurrencyError) Error() string {
	return fmt.Sprintf("unknown currency %q", e.Code)
}

// Parse reads an amount the way it is typed on the intake page: a number on
// its own — 123.50 — in defaultCurrency, or a number with a currency code
// before or after it — USD 45, 45 USD. The code is matched without regard to
// case. An empty defaultCurrency requires the text to name one.
//
// Grouping separators are refused rather than guessed at. "1,234" is a
// thousand two hundred in one half of the world and one point two in the
// other, and a scanned receipt is exactly the material that comes from both.
//
// More decimals than the currency has are refused; fewer are scaled up, so
// "45" in AUD is 4500 cents.
func Parse(text, defaultCurrency string) (Amount, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Amount{}, ErrEmpty
	}
	code := strings.ToUpper(strings.TrimSpace(defaultCurrency))
	var number string
	switch len(fields) {
	case 1:
		number = fields[0]
		if head, rest, found := splitLeadingCode(fields[0]); found {
			code, number = head, rest
		}
	case 2:
		switch {
		case isCode(fields[0]):
			code, number = strings.ToUpper(fields[0]), fields[1]
		case isCode(fields[1]):
			code, number = strings.ToUpper(fields[1]), fields[0]
		default:
			return Amount{}, fmt.Errorf("read amount %q: expected a number and a currency", text)
		}
	default:
		return Amount{}, fmt.Errorf("read amount %q: expected a number and a currency", text)
	}
	if code == "" {
		return Amount{}, ErrNoCurrency
	}
	exponent, known := Exponent(code)
	if !known {
		return Amount{}, UnknownCurrencyError{Code: code}
	}
	minor, err := parseMinor(number, exponent)
	if err != nil {
		return Amount{}, fmt.Errorf("read amount %q: %w", text, err)
	}
	return Amount{Minor: minor, Currency: code}, nil
}

// splitLeadingCode separates a currency code written against the number, as in
// USD45 or -USD45.
func splitLeadingCode(field string) (code, rest string, found bool) {
	sign := ""
	if strings.HasPrefix(field, "-") || strings.HasPrefix(field, "+") {
		sign, field = field[:1], field[1:]
	}
	if len(field) < 4 || !isCode(field[:3]) {
		return "", "", false
	}
	return strings.ToUpper(field[:3]), sign + field[3:], true
}

func isCode(field string) bool {
	if len(field) != 3 {
		return false
	}
	for _, r := range field {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func parseMinor(number string, exponent int) (int64, error) {
	negative := false
	switch {
	case strings.HasPrefix(number, "-"):
		negative, number = true, number[1:]
	case strings.HasPrefix(number, "+"):
		number = number[1:]
	}
	whole, fraction, hasPoint := strings.Cut(number, ".")
	if hasPoint && exponent == 0 {
		return 0, errors.New("this currency has no decimals")
	}
	if len(fraction) > exponent {
		return 0, fmt.Errorf("at most %d decimals", exponent)
	}
	digits := whole + fraction + strings.Repeat("0", exponent-len(fraction))
	if whole == "" && fraction == "" {
		return 0, errors.New("expected a number")
	}
	var minor int64
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, errors.New("expected a number")
		}
		next := minor*10 + int64(r-'0')
		if next < minor {
			return 0, errors.New("number is too large")
		}
		minor = next
	}
	if negative {
		minor = -minor
	}
	return minor, nil
}

// String writes the code and the number with its currency's decimals and
// nothing else — "AUD 1240.50" — so that what it prints is what Parse reads
// back. It is what goes into a message, a log line or a test.
//
// Display is the same amount for a person to look at.
func (a Amount) String() string { return a.text(false) }

// Display writes an amount for the screen: like String, but with a comma every
// three digits — "AUD 1,240.50". Parse refuses that comma on the way in, on
// purpose, so this is the one direction grouping belongs in.
func (a Amount) Display() string { return a.text(true) }

func (a Amount) text(grouped bool) string {
	exponent, known := Exponent(a.Currency)
	if !known {
		return fmt.Sprintf("%s %d", a.Currency, a.Minor)
	}
	minor, sign := a.Minor, ""
	if minor < 0 {
		minor, sign = -minor, "-"
	}
	digits := fmt.Sprintf("%0*d", exponent+1, minor)
	whole, fraction := digits[:len(digits)-exponent], digits[len(digits)-exponent:]
	if grouped {
		whole = group(whole)
	}
	if exponent == 0 {
		return fmt.Sprintf("%s %s%s", a.Currency, sign, whole)
	}
	return fmt.Sprintf("%s %s%s.%s", a.Currency, sign, whole, fraction)
}

func group(whole string) string {
	var out strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	return out.String()
}

// Total is one currency's share of a set of amounts: what they add up to, and
// how many of them there were.
type Total struct {
	Amount Amount
	Count  int
}

// Sum adds amounts up per currency, in alphabetical order of code, and never
// across currencies. A single combined figure would need an exchange rate,
// which is a fact about an instant, is not available offline, and would never
// be updated — so it would be a fabrication rather than a rounding.
//
// Amounts that are not Valid are left out: an absent total is not a zero one.
func Sum(amounts []Amount) []Total {
	byCode := map[string]*Total{}
	for _, amount := range amounts {
		if !amount.Valid() {
			continue
		}
		total, seen := byCode[amount.Currency]
		if !seen {
			total = &Total{Amount: Amount{Currency: amount.Currency}}
			byCode[amount.Currency] = total
		}
		total.Amount.Minor += amount.Minor
		total.Count++
	}
	codes := make([]string, 0, len(byCode))
	for code := range byCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	totals := make([]Total, 0, len(codes))
	for _, code := range codes {
		totals = append(totals, *byCode[code])
	}
	return totals
}
