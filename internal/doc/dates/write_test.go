package dates

import "testing"

func TestCompactAndFinancialYear(t *testing.T) {
	for in, want := range map[string]string{"2010-01-01": "20100101", "2010-01": "201001", "soon": "soon"} {
		if got := Compact(in); got != want {
			t.Errorf("Compact(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"2023-06-30": "FY2023", "2023-07-01": "FY2024", "2024-02": "FY2024", "soon": "soon"} {
		if got := FinancialYear(in, DefaultYearStart); got != want {
			t.Errorf("FinancialYear(%q) = %q, want %q", in, got, want)
		}
	}
	if got := FinancialYear("2023-07-01", 1); got != "FY2023" {
		t.Errorf("a year starting in January is the calendar year, got %q", got)
	}
}
