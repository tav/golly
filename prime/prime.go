// Public Domain (-) 2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

package prime

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

var Max = Numbers[Count-1]

func getRandomPrime(init, length int) (int64, error) {
	buf := make([]byte, 4)
	n, err := rand.Read(buf)
	if err != nil {
		return 0, err
	}
	if n != 4 {
		return 0, fmt.Errorf("prime: only read %d of 4 random bytes", n)
	}
	return Numbers[init+int(binary.BigEndian.Uint32(buf)%uint32(length))], nil
}

func Between(start, end int64) (int64, error) {
	if start >= end || start < 0 || end < 0 || end > Max || start > Max {
		return 0, fmt.Errorf("prime: none available in the range (%d, %d)", start, end)
	}
	i := 0
	for Numbers[i] < start {
		i++
	}
	init := i
	for i < Count && Numbers[i] <= end {
		i++
	}
	length := i - init
	if length == 0 {
		return 0, fmt.Errorf("prime: none available in the range (%d, %d)", start, end)
	}
	return getRandomPrime(init, length)
}

func Select() (int64, error) {
	return getRandomPrime(0, Count)
}
