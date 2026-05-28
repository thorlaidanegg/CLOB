// Package types provides the numeric and domain foundation for the CLOB engine.
package types

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// pow10 is a lookup table for powers of 10, index 0–18.
var pow10 [19]int64

func init() {
	pow10[0] = 1
	for i := 1; i < 19; i++ {
		pow10[i] = pow10[i-1] * 10
	}
}

// Decimal is a fixed-point integer. value is the integer mantissa;
// precision is the number of decimal places (0–18).
//
// Example: $101.25 with precision 2 → Decimal{value: 10125, precision: 2}
// Example: 0.00000001 BTC with precision 8 → Decimal{value: 1, precision: 8}
//
// float64 is NEVER used for prices, quantities, or fees.
type Decimal struct {
	value     int64
	precision uint8
}

// NewDecimal constructs a Decimal from a raw integer mantissa and precision.
func NewDecimal(value int64, precision uint8) Decimal {
	return Decimal{value: value, precision: precision}
}

// Zero returns a Decimal with value 0 at the given precision.
func Zero(precision uint8) Decimal {
	return Decimal{precision: precision}
}

// ParseDecimal parses a decimal string like "101.25" into a Decimal at the
// given precision. Returns an error if the string has more decimal places than
// precision allows or is otherwise malformed.
func ParseDecimal(s string, precision uint8) (Decimal, error) {
	if s == "" {
		return Decimal{}, fmt.Errorf("clob/types: empty decimal string")
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
		if s == "" {
			return Decimal{}, fmt.Errorf("clob/types: invalid decimal: bare minus")
		}
	}

	dotIdx := strings.Index(s, ".")
	var intPart, fracPart string
	if dotIdx == -1 {
		intPart = s
		fracPart = ""
	} else {
		intPart = s[:dotIdx]
		fracPart = s[dotIdx+1:]
		if len(fracPart) > int(precision) {
			return Decimal{}, fmt.Errorf("clob/types: decimal %q has %d fractional digits, max is %d", s, len(fracPart), precision)
		}
	}

	if intPart == "" {
		intPart = "0"
	}

	iv, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return Decimal{}, fmt.Errorf("clob/types: invalid decimal %q: %w", s, err)
	}

	// Pad fracPart to exactly precision digits.
	for len(fracPart) < int(precision) {
		fracPart += "0"
	}

	var fv int64
	if fracPart != "" {
		fv, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return Decimal{}, fmt.Errorf("clob/types: invalid decimal %q: %w", s, err)
		}
	}

	result := iv*pow10[precision] + fv
	if negative {
		result = -result
	}
	return Decimal{value: result, precision: precision}, nil
}

// MustDecimal parses a decimal string and panics on error.
func MustDecimal(s string, precision uint8) Decimal {
	d, err := ParseDecimal(s, precision)
	if err != nil {
		panic(err)
	}
	return d
}

func assertSamePrecision(a, b Decimal) {
	if a.precision != b.precision {
		panic(fmt.Sprintf("clob/types: precision mismatch: %d vs %d", a.precision, b.precision))
	}
}

// Add returns d + other. Panics if precisions differ.
func (d Decimal) Add(other Decimal) Decimal {
	assertSamePrecision(d, other)
	return Decimal{value: d.value + other.value, precision: d.precision}
}

// Sub returns d - other. Panics if precisions differ.
func (d Decimal) Sub(other Decimal) Decimal {
	assertSamePrecision(d, other)
	return Decimal{value: d.value - other.value, precision: d.precision}
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	return Decimal{value: -d.value, precision: d.precision}
}

// Abs returns the absolute value of d.
func (d Decimal) Abs() Decimal {
	if d.value < 0 {
		return Decimal{value: -d.value, precision: d.precision}
	}
	return d
}

// MulInt returns d × n, used for integer scaling (e.g. quantity × integer).
func (d Decimal) MulInt(n int64) Decimal {
	return Decimal{value: d.value * n, precision: d.precision}
}

// mul128 multiplies two int64 values returning a 128-bit result as (hi, lo uint64).
func mul128(a, b int64) (hi int64, lo uint64) {
	hi64, lo64 := bits.Mul64(uint64(a), uint64(b))
	// Handle sign: if a and b have different signs, negate.
	if (a < 0) != (b < 0) {
		// negate (hi64, lo64)
		lo64 = ^lo64
		hi64 = ^hi64
		lo64++
		if lo64 == 0 {
			hi64++
		}
	}
	return int64(hi64), lo64
}

// div128by64 divides a 128-bit unsigned value by a 64-bit unsigned divisor,
// returning a 64-bit quotient. Used internally for Mul normalization.
func div128by64(hi, lo uint64, divisor uint64) int64 {
	// Use Go's bits package for 128/64 division.
	q, _ := bits.Div64(hi, lo, divisor)
	return int64(q)
}

// Mul returns d × other, normalized back to d.precision.
// Uses 128-bit intermediate to avoid overflow for large values.
// Panics if precisions differ.
func (d Decimal) Mul(other Decimal) Decimal {
	assertSamePrecision(d, other)

	// Use int128 arithmetic for overflow safety.
	// raw = d.value * other.value  (may overflow int64)
	// result = raw / pow10(other.precision)
	hiSigned, loUnsigned := mul128(d.value, other.value)

	negative := false
	hi := uint64(hiSigned)
	lo := loUnsigned
	if hiSigned < 0 || (hiSigned == 0 && false) {
		// Determine sign separately.
		negative = (d.value < 0) != (other.value < 0)
	} else {
		negative = (d.value < 0) != (other.value < 0)
	}

	// Work with absolute values for division.
	absA := d.value
	if absA < 0 {
		absA = -absA
	}
	absB := other.value
	if absB < 0 {
		absB = -absB
	}
	hiU, loU := bits.Mul64(uint64(absA), uint64(absB))
	_ = hi
	_ = lo

	divisor := uint64(pow10[other.precision])
	q := div128by64(hiU, loU, divisor)

	if negative {
		q = -q
	}
	return Decimal{value: q, precision: d.precision}
}

// Div returns d / other with the result at outPrecision.
// Panics if other.IsZero().
func (d Decimal) Div(other Decimal, outPrecision uint8) Decimal {
	if other.value == 0 {
		panic("clob/types: division by zero")
	}
	// Scale d.value up to outPrecision, then divide.
	// result = (d.value * pow10[outPrecision]) / other.value
	// We need to handle the precision difference.
	// Normalize: work in the space of outPrecision.
	// scaled = d.value * pow10[outPrecision] / pow10[d.precision] — but keep integer.
	// Simpler: result_value = (d.value * pow10[outPrecision + other.precision - d.precision]) / other.value
	// When outPrecision + other.precision >= d.precision:
	//   scaled = d.value * pow10[outPrecision + other.precision - d.precision]
	//   result_value = scaled / other.value
	// This gives a result at outPrecision.

	// For circuit breaker usage: d and other are both at pricePrecision,
	// outPrecision=4. So we compute (d.value * 10^(4 + pricePrecision - pricePrecision)) / other.value
	// = (d.value * 10^4) / other.value, which is correct.

	scale := int(outPrecision) + int(other.precision) - int(d.precision)
	var numerator int64
	if scale >= 0 {
		numerator = d.value * pow10[scale]
	} else {
		numerator = d.value / pow10[-scale]
	}

	result := numerator / other.value
	return Decimal{value: result, precision: outPrecision}
}

// Min returns the smaller of a and b. Panics if precisions differ.
func Min(a, b Decimal) Decimal {
	assertSamePrecision(a, b)
	if a.value <= b.value {
		return a
	}
	return b
}

// Max returns the larger of a and b. Panics if precisions differ.
func Max(a, b Decimal) Decimal {
	assertSamePrecision(a, b)
	if a.value >= b.value {
		return a
	}
	return b
}

// Equal returns true if d == other. Panics if precisions differ.
func (d Decimal) Equal(other Decimal) bool {
	assertSamePrecision(d, other)
	return d.value == other.value
}

// LessThan returns true if d < other. Panics if precisions differ.
func (d Decimal) LessThan(other Decimal) bool {
	assertSamePrecision(d, other)
	return d.value < other.value
}

// LessThanOrEqual returns true if d <= other. Panics if precisions differ.
func (d Decimal) LessThanOrEqual(other Decimal) bool {
	assertSamePrecision(d, other)
	return d.value <= other.value
}

// GreaterThan returns true if d > other. Panics if precisions differ.
func (d Decimal) GreaterThan(other Decimal) bool {
	assertSamePrecision(d, other)
	return d.value > other.value
}

// GreaterThanOrEqual returns true if d >= other. Panics if precisions differ.
func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	assertSamePrecision(d, other)
	return d.value >= other.value
}

// IsZero returns true if d == 0.
func (d Decimal) IsZero() bool { return d.value == 0 }

// IsPositive returns true if d > 0.
func (d Decimal) IsPositive() bool { return d.value > 0 }

// IsNegative returns true if d < 0.
func (d Decimal) IsNegative() bool { return d.value < 0 }

// IsValidTick returns true if d is a positive multiple of tickSize.
// Panics if precisions differ.
func (d Decimal) IsValidTick(tickSize Decimal) bool {
	assertSamePrecision(d, tickSize)
	if d.value <= 0 || tickSize.value <= 0 {
		return false
	}
	return d.value%tickSize.value == 0
}

// IsValidLot returns true if d is a positive multiple of lotSize.
// Panics if precisions differ.
func (d Decimal) IsValidLot(lotSize Decimal) bool {
	assertSamePrecision(d, lotSize)
	if d.value <= 0 || lotSize.value <= 0 {
		return false
	}
	return d.value%lotSize.value == 0
}

// String returns the decimal string representation.
// Trailing zeros after the decimal point are always included.
// Example: Decimal{10125, 2} → "101.25", Decimal{100, 2} → "1.00"
func (d Decimal) String() string {
	if d.precision == 0 {
		return strconv.FormatInt(d.value, 10)
	}

	negative := d.value < 0
	abs := d.value
	if negative {
		abs = -abs
	}

	p := pow10[d.precision]
	intPart := abs / p
	fracPart := abs % p

	fracStr := fmt.Sprintf("%0*d", d.precision, fracPart)

	var sb strings.Builder
	if negative {
		sb.WriteByte('-')
	}
	sb.WriteString(strconv.FormatInt(intPart, 10))
	sb.WriteByte('.')
	sb.WriteString(fracStr)
	return sb.String()
}

// MarshalJSON implements json.Marshaler. Prices and quantities serialize as
// quoted decimal strings — never as float64.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseDecimal(s, d.precision)
	if err != nil {
		return err
	}
	d.value = parsed.value
	return nil
}

// Value returns the raw integer mantissa. Use only for serialization to storage.
func (d Decimal) Value() int64 { return d.value }

// Precision returns the decimal precision.
func (d Decimal) Precision() uint8 { return d.precision }
