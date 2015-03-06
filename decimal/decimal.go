// Public Domain (-) 2010-2015 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package decimal implements support for arbitrary precision decimals.
//
// This package used to be a fork of the math/big package from the standard
// library, but that became too much of a hassle to maintain. So it's now just a
// simple wrapper around big.Rat. As a result, it's a bit slower and doesn't
// support methods like Div(), but that should be fine for most use cases.
package decimal

import (
	"math/big"
	"strings"
)

type Decimal big.Rat

func (z *Decimal) Add(x, y *Decimal) *Decimal {
	return (*Decimal)((*big.Rat)(z).Add((*big.Rat)(x), (*big.Rat)(y)))
}

func (x *Decimal) Cmp(y *Decimal) int {
	return (*big.Rat)(x).Cmp((*big.Rat)(y))
}

func (d *Decimal) Float32() (f float32, exact bool) {
	return (*big.Rat)(d).Float32()
}

func (d *Decimal) Float64() (f float64, exact bool) {
	return (*big.Rat)(d).Float64()
}

func (d *Decimal) Format(prec int) string {
	s := strings.TrimRight((*big.Rat)(d).FloatString(prec), "0.")
	if s == "" {
		return "0"
	}
	return s
}

func (d *Decimal) IsInt() bool {
	return (*big.Rat)(d).IsInt()
}

func (d *Decimal) Int() *big.Int {
	i, _ := (&big.Int{}).SetString(d.Format(0), 10)
	return i
}

func (z *Decimal) Mul(x, y *Decimal) *Decimal {
	return (*Decimal)((*big.Rat)(z).Mul((*big.Rat)(x), (*big.Rat)(y)))
}

func (d *Decimal) Sign() int {
	return (*big.Rat)(d).Sign()
}

func (d *Decimal) String() string {
	return d.Format(100)
}

func (z *Decimal) Sub(x, y *Decimal) *Decimal {
	return (*Decimal)((*big.Rat)(z).Sub((*big.Rat)(x), (*big.Rat)(y)))
}

// New returns a Decimal for the given string value.
func New(v string) (*Decimal, bool) {
	d, ok := &big.Rat{}, false
	d, ok = d.SetString(v)
	if !ok {
		return nil, false
	}
	return (*Decimal)(d), true
}
