package dates

import (
	"regexp"
	"strconv"
)

var kept = regexp.MustCompile(`^(\d{4})-(\d{2})(?:-(\d{2}))?$`)

// Compact writes a kept date, 2010-01-01, as 20100101, and a month, 2010-01,
// as 201001. Anything else is written as it is.
func Compact(value string) string {
	m := kept.FindStringSubmatch(value)
	if m == nil {
		return value
	}
	return m[1] + m[2] + m[3]
}

// DefaultYearStart is the month a financial year starts in: July, as in
// Australia, where FY2024 runs from July 2023 to June 2024.
const DefaultYearStart = 7

// FinancialYear writes the financial year a kept date or month falls in,
// named by the calendar year it ends in: FY2024. start is the month the
// year begins, 1 to 12. Anything else is written as it is.
func FinancialYear(value string, start int) string {
	m := kept.FindStringSubmatch(value)
	if m == nil {
		return value
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	if start > 1 && month >= start {
		year++
	}
	return "FY" + strconv.Itoa(year)
}
