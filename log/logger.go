// Public Domain (-) 2010-2014 The Golly Authors.
// See the Golly UNLICENSE file for details.

package log

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

var (
	entryPool = &sync.Pool{}
	root      = &Logger{}
	slicePool = &sync.Pool{}
)

type Logger struct {
	context    string
	data       Data
	debug      bool
	handler    Handler
	lazy       []string
	parent     *Logger
	stacktrace bool
	stop       bool
}

// Create a new logger.
func (l *Logger) New(ctx ...interface{}) *Logger {
	switch len(ctx) {
	case 0:
		return &Logger{
			context: l.context,
			parent:  l,
		}
	case 1:
		if context, ok := ctx[0].(string); ok {
			if l.context != "" {
				context = l.context + "." + context
			}
			return &Logger{
				context: context,
				parent:  l,
			}
		} else if data, ok := ctx[0].(Data); ok {
			return &Logger{
				context: l.context,
				data:    data,
				parent:  l,
			}
		}
	case 2:
		if context, ok := ctx[0].(string); ok {
			if l.context != "" {
				context = l.context + "." + context
			}
			if data, ok := ctx[1].(Data); ok {
				return &Logger{
					context: context,
					data:    data,
					parent:  l,
				}
			}
		}
	}
	panic("log.New must be called with a either a single argument (which must be a string or log.Data) or with two arguments (first the string context, followed by a log.Data object)")
}

// Close the underlying handler for this logger.
//
// Please note that if you set a handler on the root logger, then it is your
// responsibility to manually close any underlying resources.
func (l *Logger) Close() {
	if l.handler != nil {
		l.handler.Close()
	}
}

func (l *Logger) ToggleDebug(enable bool, stacktrace bool) {
	l.debug = enable
	l.stacktrace = stacktrace
}

// Flush the underlying handler for this logger.
func (l *Logger) Flush() {
	if l.handler != nil {
		l.handler.Flush()
	}
}

func (l *Logger) Error(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		if vdata, ok := v.(Data); ok {
			l.log("", vdata, true)
			return
		}
		msg = fmt.Sprint(msg)
	}
	if len(data) > 0 {
		l.log(msg, data[0], true)
	} else {
		l.log(msg, nil, true)
	}
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(fmt.Sprintf(format, args...), nil, true)
}

func (l *Logger) Info(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		if vdata, ok := v.(Data); ok {
			l.log("", vdata, false)
			return
		}
		msg = fmt.Sprint(msg)
	}
	if len(data) > 0 {
		l.log(msg, data[0], false)
	} else {
		l.log(msg, nil, false)
	}
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(fmt.Sprintf(format, args...), nil, false)
}

func (l *Logger) log(msg string, data Data, isError bool) {
	var e *Entry
	entry := entryPool.Get()
	if entry == nil {
		e = &Entry{}
	} else {
		e = entry.(*Entry)
		e.File = ""
		e.Line = 0
	}
	e.Context = l.context
	e.Data = data
	e.Error = isError
	e.Message = msg
	e.Timestamp = time.Now()
	l.logEntry(e, 3)
}

func (l *Logger) logEntry(e *Entry, depth int) {
	var buf []byte
	var debugSet int
	var async bool
	for {
		if l.debug && debugSet == 0 {
			_, e.File, e.Line, _ = runtime.Caller(3)
			if l.stacktrace {
				slice := slicePool.Get()
				if slice == nil {
					buf = make([]byte, 4096)
				} else {
					buf = slice.([]byte)
				}
				e.Stacktrace = string(buf[:runtime.Stack(buf, false)])
				debugSet = 2
			} else {
				debugSet = 1
			}
		}
		if l.handler != nil {
			l.handler.Log(e)
			if l.handler.Async() {
				async = true
			}
		}
		if l.parent == nil || l.stop {
			break
		}
		l = l.parent
	}
	if !async {
		entryPool.Put(e)
		if debugSet == 2 {
			slicePool.Put(buf)
		}
	}
}

func (l *Logger) LogEntry(e *Entry) {
	l.logEntry(e, 2)
}

// SetHandler and ToggleDebug are not intended to be threadsafe. Make sure
// to set them before using the logger from multiple goroutines.
func (l *Logger) SetHandler(h Handler) {
	l.handler = h
}

func (l *Logger) StopPropagation() {
	l.stop = true
}
