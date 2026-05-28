package types

import (
	"encoding/json"
	"testing"
)

func TestDecimal_FloatTrap(t *testing.T) {
	// The classic float64 trap: 0.1 + 0.2 != 0.3 in float64.
	// With Decimal it must be exactly equal.
	a := MustDecimal("0.1", 1)
	b := MustDecimal("0.2", 1)
	got := a.Add(b)
	want := MustDecimal("0.3", 1)
	if !got.Equal(want) {
		t.Fatalf("0.1 + 0.2 = %s, want 0.3", got)
	}
}

func TestDecimal_Add(t *testing.T) {
	cases := []struct {
		a, b string
		prec uint8
		want string
	}{
		{"101.25", "0.75", 2, "102.00"},
		{"0.00", "0.00", 2, "0.00"},
		{"-5.00", "3.00", 2, "-2.00"},
		{"100", "200", 0, "300"},
	}
	for _, tc := range cases {
		a := MustDecimal(tc.a, tc.prec)
		b := MustDecimal(tc.b, tc.prec)
		got := a.Add(b)
		want := MustDecimal(tc.want, tc.prec)
		if !got.Equal(want) {
			t.Errorf("Add(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDecimal_Sub(t *testing.T) {
	a := MustDecimal("5.00", 2)
	b := MustDecimal("3.25", 2)
	got := a.Sub(b)
	want := MustDecimal("1.75", 2)
	if !got.Equal(want) {
		t.Errorf("5.00 - 3.25 = %s, want 1.75", got)
	}
}

func TestDecimal_Neg(t *testing.T) {
	d := MustDecimal("5.00", 2)
	got := d.Neg()
	want := MustDecimal("-5.00", 2)
	if !got.Equal(want) {
		t.Errorf("Neg(5.00) = %s, want -5.00", got)
	}
}

func TestDecimal_Abs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"-5.00", "5.00"},
		{"5.00", "5.00"},
		{"0.00", "0.00"},
	}
	for _, tc := range cases {
		got := MustDecimal(tc.in, 2).Abs()
		want := MustDecimal(tc.want, 2)
		if !got.Equal(want) {
			t.Errorf("Abs(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestDecimal_Mul(t *testing.T) {
	cases := []struct {
		a, b string
		prec uint8
		want string
	}{
		{"10.00", "3.00", 2, "30.00"},
		{"0.01", "0.01", 2, "0.00"}, // truncates
		{"100.00", "0.50", 2, "50.00"},
		{"-2.00", "3.00", 2, "-6.00"},
		{"-2.00", "-3.00", 2, "6.00"},
	}
	for _, tc := range cases {
		a := MustDecimal(tc.a, tc.prec)
		b := MustDecimal(tc.b, tc.prec)
		got := a.Mul(b)
		want := MustDecimal(tc.want, tc.prec)
		if !got.Equal(want) {
			t.Errorf("Mul(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDecimal_Div(t *testing.T) {
	cases := []struct {
		a        string
		aPrec    uint8
		b        string
		bPrec    uint8
		outPrec  uint8
		want     string
	}{
		{"10.00", 2, "2.00", 2, 2, "5.00"},
		{"1.00", 2, "3.00", 2, 4, "0.3333"},
		{"0.10", 2, "2.00", 2, 2, "0.05"},
	}
	for _, tc := range cases {
		a := MustDecimal(tc.a, tc.aPrec)
		b := MustDecimal(tc.b, tc.bPrec)
		got := a.Div(b, tc.outPrec)
		want := MustDecimal(tc.want, tc.outPrec)
		if !got.Equal(want) {
			t.Errorf("Div(%s, %s, %d) = %s, want %s", tc.a, tc.b, tc.outPrec, got, tc.want)
		}
	}
}

func TestDecimal_DivByZeroPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on division by zero")
		}
	}()
	a := MustDecimal("1.00", 2)
	b := Zero(2)
	_ = a.Div(b, 2)
}

func TestDecimal_MulInt(t *testing.T) {
	d := MustDecimal("3.00", 2)
	got := d.MulInt(4)
	want := MustDecimal("12.00", 2)
	if !got.Equal(want) {
		t.Errorf("MulInt(3.00, 4) = %s, want 12.00", got)
	}
}

func TestDecimal_IsValidTick(t *testing.T) {
	cases := []struct {
		price    string
		tick     string
		wantOK   bool
	}{
		{"100.00", "0.01", true},
		{"100.05", "0.05", true},
		{"100.03", "0.05", false},
		{"100.00", "0.25", true},
		{"100.10", "0.25", false},
		{"0.00", "0.01", false},  // non-positive price fails
	}
	for _, tc := range cases {
		price := MustDecimal(tc.price, 2)
		tick := MustDecimal(tc.tick, 2)
		got := price.IsValidTick(tick)
		if got != tc.wantOK {
			t.Errorf("IsValidTick(%s, %s) = %v, want %v", tc.price, tc.tick, got, tc.wantOK)
		}
	}
}

func TestDecimal_IsValidLot(t *testing.T) {
	cases := []struct {
		qty    string
		lot    string
		wantOK bool
	}{
		{"100", "1", true},
		{"100", "100", true},
		{"101", "100", false},
		{"0", "1", false},
	}
	for _, tc := range cases {
		qty := MustDecimal(tc.qty, 0)
		lot := MustDecimal(tc.lot, 0)
		got := qty.IsValidLot(lot)
		if got != tc.wantOK {
			t.Errorf("IsValidLot(%s, %s) = %v, want %v", tc.qty, tc.lot, got, tc.wantOK)
		}
	}
}

func TestDecimal_String(t *testing.T) {
	cases := []struct {
		value    int64
		prec     uint8
		want     string
	}{
		{10125, 2, "101.25"},
		{100, 2, "1.00"},
		{-500, 2, "-5.00"},
		{0, 2, "0.00"},
		{100, 0, "100"},
		{1, 8, "0.00000001"},
	}
	for _, tc := range cases {
		d := NewDecimal(tc.value, tc.prec)
		got := d.String()
		if got != tc.want {
			t.Errorf("String(%d, %d) = %q, want %q", tc.value, tc.prec, got, tc.want)
		}
	}
}

func TestDecimal_JSON(t *testing.T) {
	d := MustDecimal("101.25", 2)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"101.25"` {
		t.Errorf("MarshalJSON = %s, want %q", b, "101.25")
	}

	var d2 Decimal
	d2.precision = 2
	if err := json.Unmarshal(b, &d2); err != nil {
		t.Fatal(err)
	}
	if !d.Equal(d2) {
		t.Errorf("round-trip: got %s, want %s", d2, d)
	}
}

func TestDecimal_ParseErrors(t *testing.T) {
	cases := []struct{ s string }{
		{""},
		{"abc"},
		{"1.234"}, // 3 decimals, precision 2
		{"-"},
	}
	for _, tc := range cases {
		_, err := ParseDecimal(tc.s, 2)
		if err == nil {
			t.Errorf("ParseDecimal(%q, 2) expected error, got nil", tc.s)
		}
	}
}

func TestDecimal_PrecisionMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on precision mismatch")
		}
	}()
	a := MustDecimal("1.00", 2)
	b := MustDecimal("1.000", 3)
	_ = a.Add(b)
}
