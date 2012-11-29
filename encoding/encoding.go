// Public Domain (-) 2010-2011 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package encoding provides utility functions for padded integer
// representations.
package encoding

// PadInt provides cheap integer to fixed-width decimal ASCII conversion. Use a
// negative width to avoid zero-padding.
func PadInt(i int, width int) string {
	u := uint(i)
	if u == 0 && width <= 1 {
		return "0"
	}
	// Assemble the decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; u > 0 || width > 0; u /= 10 {
		bp--
		width--
		b[bp] = byte(u%10) + '0'
	}
	return string(b[bp:])
}

func PadInt64(i int64, width int) string {
	u := uint(i)
	if u == 0 && width <= 1 {
		return "0"
	}
	// Assemble the decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; u > 0 || width > 0; u /= 10 {
		bp--
		width--
		b[bp] = byte(u%10) + '0'
	}
	return string(b[bp:])
}
