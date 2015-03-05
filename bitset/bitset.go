// Public Domain (-) 2015 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package bitset implements a structure for manipulating bitsets.
package bitset

import (
	"encoding/binary"
	"strconv"
)

type word uintptr

const (
	wMax   = ^word(0)
	wLog   = wMax>>8&1 + wMax>>16&1 + wMax>>32&1
	wBytes = 1 << wLog
	wBits  = wBytes << 3
	nLog   = wLog + 3
	nBits  = wBits - 1
)

// Set represents a bitset of size defined by the New() constructor.
type Set struct {
	bits []word
	out  []byte
	size uint
}

// Clear the bit at the given bit index.
func (s *Set) Clear(idx uint) {
	s.bits[idx/wBits] &^= word(1) << (idx % wBits)
}

// Reset all the bits to zero.
func (s *Set) Reset() {
	l := len(s.bits)
	for i := 0; i < l; i++ {
		s.bits[i] = 0
	}
}

// Set the bit at the given index to 1.
func (s *Set) Set(idx uint) {
	s.bits[idx/wBits] |= word(1) << (idx % wBits)
}

// Size returns the length of the bitset.
func (s *Set) Size() uint {
	return s.size
}

// String returns a string representation of the underlying bits.
func (s *Set) String() string {
	l := len(s.bits)
	switch wBits {
	case 64:
		for i := 0; i < l; i++ {
			binary.LittleEndian.PutUint64(s.out[i*8:(i+1)*8], uint64(s.bits[i]))
		}
	case 32:
		for i := 0; i < l; i++ {
			binary.LittleEndian.PutUint32(s.out[i*4:(i+1)*4], uint32(s.bits[i]))
		}
	default:
		sBits := strconv.FormatInt(int64(wBits), 10)
		panic("bitset: string conversion not implemented for " + sBits + "-bit platforms yet")
	}
	return string(s.out)
}

// New creates a bitset of the given size.
func New(size uint) *Set {
	l := (size + nBits) >> nLog
	return &Set{
		bits: make([]word, l),
		out:  make([]byte, l*wBytes),
		size: size,
	}
}
