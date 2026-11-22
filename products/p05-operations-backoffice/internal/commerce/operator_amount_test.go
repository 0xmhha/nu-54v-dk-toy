package commerce

import "testing"

func TestOperatorDecimalUnits(t *testing.T) {
	for input, expected := range map[string]string{
		"7": "7000000", "7.10": "7100000", "0.000001": "1",
		"9007199254740993.000001": "9007199254740993000001",
		"0":                       "", "0.0000001": "", "01.2": "", "1e6": "", "+2": "",
	} {
		result, err := operatorDecimalUnits(input, 6)
		if result != expected || (err != nil) != (expected == "") {
			t.Errorf("%s: %s (%v), want %s", input, result, err, expected)
		}
	}
}

func TestOperatorPriceAdjustments(t *testing.T) {
	for _, test := range []struct{ base, tax, discount, want string }{
		{"7.10", "0.10", "0.20", "7000000"},
		{"0.000001", "", "", "1"},
		{"1", "", "1", ""},
		{"1", "-1", "", ""},
		{"1", "0.0000001", "", ""},
	} {
		row := operatorRow{DisplayPrice: test.base, Tax: test.tax, Discount: test.discount}
		err := row.resolvePrice(6)
		if row.Price != test.want || (err != nil) != (test.want == "") {
			t.Errorf("%+v -> %q %v", test, row.Price, err)
		}
	}
}
