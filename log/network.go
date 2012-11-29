// Public Domain (-) 2010-2011 The Golly Authors.
// See the Golly UNLICENSE file for details.

package log

import (
	"io"
)

type NetworkLogger struct {
	fallback *FileLogger
	stream   *io.Writer
	receiver chan *Record
}
