package money

// The minor-unit exponent of every currency this build accepts, from ISO 4217.
// Most currencies divide by a hundred; the exceptions are what this table is
// for, because assuming two decimals stores a hundred times too little for a
// yen and a thousand times too little for a Kuwaiti dinar.
//
// The list is deliberately finite rather than a guess of two decimals for
// anything three letters long: a code that is not here is refused, which is a
// wrong currency noticed at once instead of a wrong number kept forever.
// Extending it is one line.
var exponents = map[string]int{
	// No minor unit.
	"BIF": 0, "CLP": 0, "DJF": 0, "GNF": 0, "ISK": 0, "JPY": 0, "KMF": 0,
	"KRW": 0, "PYG": 0, "RWF": 0, "UGX": 0, "VND": 0, "VUV": 0, "XAF": 0,
	"XOF": 0, "XPF": 0,
	// Three decimals.
	"BHD": 3, "IQD": 3, "JOD": 3, "KWD": 3, "LYD": 3, "OMR": 3, "TND": 3,
	// Two decimals.
	"AED": 2, "ARS": 2, "AUD": 2, "AZN": 2, "BGN": 2, "BND": 2, "BRL": 2,
	"CAD": 2, "CHF": 2, "CNY": 2, "COP": 2, "CZK": 2, "DKK": 2, "EGP": 2,
	"EUR": 2, "FJD": 2, "GBP": 2, "GEL": 2, "HKD": 2, "HUF": 2, "IDR": 2,
	"ILS": 2, "INR": 2, "KES": 2, "KHR": 2, "KZT": 2, "LAK": 2, "LKR": 2,
	"MAD": 2, "MNT": 2, "MOP": 2, "MXN": 2, "MYR": 2, "NOK": 2, "NPR": 2,
	"NZD": 2, "PEN": 2, "PHP": 2, "PKR": 2, "PLN": 2, "QAR": 2, "RON": 2,
	"RSD": 2, "RUB": 2, "SAR": 2, "SEK": 2, "SGD": 2, "THB": 2, "TRY": 2,
	"TWD": 2, "TZS": 2, "UAH": 2, "USD": 2, "UZS": 2, "ZAR": 2,
}

// Exponent is how many decimal places a currency is written with, and whether
// this build knows the currency at all.
func Exponent(code string) (int, bool) {
	exponent, known := exponents[code]
	return exponent, known
}

// Known reports whether a currency code can be used. Configuration checks this
// so a mistyped box.currency is refused when the file is read rather than when
// the first amount is typed.
func Known(code string) bool { _, known := exponents[code]; return known }
