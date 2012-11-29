// Public Domain (-) 2011 The Golly Authors.
// See the Golly UNLICENSE file for details.

package argo

type Error string

func (err Error) Error() string {
	return "argo error: " + string(err)
}

type TypeMismatchError string

func (err TypeMismatchError) Error() string {
	return "argo error: " + string(err)
}

var (
	OutOfRangeError = Error("out of range size value")
	PointerError    = Error("error deferencing pointers")
)

func raise(err error) {
	panic(err)
}

func typeError(expected string, got byte) error {
	return TypeMismatchError("expected " + expected + ", got " + typeNames[got])
}
