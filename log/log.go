// Public Domain (-) 2010-2014 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package log provides an extensible logging framework.
package log

import (
	"code.google.com/p/go.crypto/ssh/terminal"
	"encoding/json"
	"fmt"
	"github.com/tav/golly/process"
	"io"
	stdlog "log"
	"os"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"
)

// You can specify the LogType field on Options to control whether to log info
// logs, error logs or both.
type LogType int

const (
	InfoLog LogType = 1 << iota
	ErrorLog
	MixedLog LogType = InfoLog | ErrorLog
)

var (
	entryPool = &sync.Pool{}
	root      = &Logger{}
	slicePool = &sync.Pool{}
)

var defaultFuncMap = template.FuncMap{
	"lower": func(s string) string {
		return strings.ToLower(s)
	},
	"title": func(s string) string {
		return strings.ToTitle(s)
	},
	"upper": func(s string) string {
		return strings.ToUpper(s)
	},
}

type Data map[string]interface{}

type Entry struct {
	Context    string    `codec:"ctx"                  json:"ctx"`
	Data       Data      `codec:"data"                 json:"data"`
	Error      bool      `codec:"error"                json:"error"`
	File       string    `codec:"file,omitempty"       json:"file,omitempty"`
	LineNumber int       `codec:"line,omitempty"       json:"line,omitempty"`
	Message    string    `codec:"msg"                  json:"msg"`
	Stacktrace []byte    `codec:"stacktrace,omitempty" json:"stacktrace,omitempty"`
	Timestamp  time.Time `codec:"timestamp"            json:"timestamp"`
}

type Formatter interface {
	Format(*Entry) ([]byte, error)
}

func JSONFormatter(entrySeparator string) Formatter {
	return jsonFmt{}
}

type jsonFmt struct {
}

func (f jsonFmt) Format(e *Entry) ([]byte, error) {
	return json.Marshal(e)
}

func TextFormatter(template string, color bool, funcs template.FuncMap) Formatter {
	return &textFmt{}
}

type textFmt struct {
	ForceColor bool
	FuncMap    template.FuncMap
	NoColor    bool
	Template   string
}

func (f *textFmt) Format(e *Entry) ([]byte, error) {
	return nil, nil
}

type Handler interface {
	Close()
	Flush()
	Log(*Entry) error
}

// fmtData

func Stream(opts *Options) Handler {
	s := &stream{}
	// process.RegisterSignalHandler(syscall.SIGHUP, file.Rotate)
	go s.flush()
	go s.rotate()
	return s
}

type stream struct {
	closed  bool
	f       *os.File
	mu      sync.Mutex
	w       io.Writer
	written int64
}

func (s *stream) Close() {
	if s.f != nil {
		s.f.Close()
	}
}

func (s *stream) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.f != nil {
		s.f.Sync()
	}
}

func (s *stream) Log(*Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		// panic?
	}
	return nil
}

func (s *stream) flush() {
	for {
		s.mu.Lock()
		if s.closed {
			return
		}
		s.mu.Unlock()
	}
}

func (s *stream) rotate() {
}

func Async(capacity int, h Handler) Handler {
	a := &async{
		ch: make(chan *Entry, capacity),
		h:  h,
	}
	go a.run()
	return a
}

type async struct {
	ch chan *Entry
	h  Handler
}

func (a async) Close() {
	close(a.ch)
}

func (a async) Flush() {
	a.h.Flush()
}

func (a async) Log(e *Entry) error {
	a.ch <- e
	return nil
}

func (a async) run() {
	var e *Entry
	for e = range a.ch {
		a.h.Log(e)
	}
	a.h.Close()
}

func FailoverHandler(handlers ...Handler) Handler {
	return failover{handlers}
}

type failover struct {
	handlers []Handler
}

func (f failover) Close() {
	for _, h := range f.handlers {
		h.Close()
	}
}

func (f failover) Flush() {
	for _, h := range f.handlers {
		h.Flush()
	}
}

func (f failover) Log(e *Entry) error {
	var err error
	for _, h := range f.handlers {
		err = h.Log(e)
		if err == nil {
			return nil
		}
		if e.Data == nil {
			e.Data = Data{}
			e.Data["golly_log_failover_1"] = err.Error()
		} else {
			id := 1
			key := fmt.Sprintf("golly_log_failover_%d", id)
			for {
				if _, exists := e.Data[key]; exists {
					id += 1
					key = fmt.Sprintf("golly_log_failover_%d", id)
				} else {
					break
				}
			}
			e.Data[key] = err.Error()
		}
	}
	return err
}

func MultiHandler(handlers ...Handler) Handler {
	return list{handlers}
}

type list struct {
	handlers []Handler
}

func (l list) Close() {
	for _, h := range l.handlers {
		h.Close()
	}
}

func (l list) Flush() {
	for _, h := range l.handlers {
		h.Flush()
	}
}

func (l list) Log(e *Entry) error {
	var prevError error
	for _, h := range l.handlers {
		err := h.Log(e)
		if prevError != nil {
			prevError = err
		}
	}
	return prevError
}

func New(ctx ...interface{}) *Logger {
	return root.New(ctx...)
}

func Close() {
	root.Close()
}

func Flush() {
	root.Flush()
}

func Error(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		msg = fmt.Sprint(msg)
	}
	if len(data) > 0 {
		root.log(msg, data[0], true)
	} else {
		root.log(msg, nil, true)
	}
}

func Errorf(format string, args ...interface{}) {
	root.log(fmt.Sprintf(format, args...), nil, true)
}

func Fatal(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		msg = fmt.Sprint(msg)
	}
	if len(data) > 0 {
		root.log(msg, data[0], true)
	} else {
		root.log(msg, nil, true)
	}
	process.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	root.log(fmt.Sprintf(format, args...), nil, true)
	process.Exit(1)
}

func Info(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		msg = fmt.Sprint(msg)
	}
	if len(data) > 0 {
		root.log(msg, data[0], false)
	} else {
		root.log(msg, nil, false)
	}
}

func Infof(format string, args ...interface{}) {
	root.log(fmt.Sprintf(format, args...), nil, false)
}

func SetHandler(h Handler) {
	root.SetHandler(h)
}

func ToggleDebug(lineinfo bool, stacktrace bool) {
	root.ToggleDebug(lineinfo, stacktrace)
}

type Logger struct {
	context    string
	data       Data
	debug      bool
	handler    Handler
	lazy       []string
	parent     *Logger
	stacktrace bool
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

func (l *Logger) Close() {
	if l.handler != nil {
		l.handler.Close()
	}
}

func (l *Logger) ToggleDebug(enable bool, stacktrace bool) {
	l.debug = enable
	l.stacktrace = stacktrace
}

// Manage the underlying handlers for this logger.
func (l *Logger) Flush() {
	if l.handler != nil {
		l.handler.Flush()
	}
}

func (l *Logger) Error(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
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
		e.LineNumber = 0
	}
	e.Context = l.context
	e.Error = isError
	e.Message = msg
	e.Timestamp = time.Now()
	var buf []byte
	var debugSet int
	for l.handler != nil {
		if l.debug && debugSet == 0 {
			_, e.File, e.LineNumber, _ = runtime.Caller(2)
			if l.stacktrace {
				slice := slicePool.Get()
				if slice == nil {
					buf = make([]byte, 4096)
				} else {
					buf = slice.([]byte)
				}
				buf = buf[:runtime.Stack(buf, false)]
				debugSet = 2
			} else {
				debugSet = 1
			}
		}
		l.handler.Log(e)
		if l.parent == nil {
			break
		}
		l = l.parent
	}
	entryPool.Put(e)
	if debugSet == 2 {
		slicePool.Put(buf)
	}
}

// SetHandler and ToggleDebug are not intended to be threadsafe. Make sure
// to set them before using the logger from multiple goroutines.
func (l *Logger) SetHandler(h Handler) {
	root.SetHandler(h)
}

type Options struct {
	BufferSize     int
	Filename       string
	Filter         func(*Entry) bool
	Formatter      Formatter
	FlushInterval  time.Time
	LocalTime      bool
	LogType        LogType
	MaxAge         time.Duration
	MaxBackups     int
	MaxSize        int
	RotateOnSignal os.Signal
	Stream         io.WriteCloser
}

// var (
// 	colors           = map[string]string{"info": "32", "error": "31"}
// 	checker          = make(chan int, 1)
// 	consoleTimestamp = true
// )

// func (Logger *ConsoleLogger) log() {

// 	var record *Record
// 	var file *os.File
// 	var items []interface{}
// 	var prefix, status string
// 	var prefixErr, prefixInfo string
// 	var suffix []byte
// 	var write bool

// 	if colorify {
// 		prefixErr = fmt.Sprintf("\x1b[%sm", colors["error"])
// 		prefixInfo = fmt.Sprintf("\x1b[%sm", colors["info"])
// 		suffix = []byte("\x1b[0m\n")
// 	} else {
// 		suffix = []byte{'\n'}
// 	}

// 	for {
// 		select {
// 		case record = <-Logger.receiver:
// 			items = record.Items
// 			write = true
// 			if filter, present := ConsoleFilters[record.Type]; present {
// 				write, items = filter(items)
// 				if !write || items == nil {
// 					continue
// 				}
// 			}
// 			if record.Error {
// 				file = os.Stderr
// 			} else {
// 				file = os.Stdout
// 			}
// 			if record.Error {
// 				prefix = prefixErr
// 				status = "ERROR: "
// 			} else {
// 				prefix = prefixInfo
// 				status = ""
// 			}
// 			if consoleTimestamp {
// 				year, month, day := now.Date()
// 				hour, minute, second := now.Clock()
// 				fmt.Fprintf(file, "%s[%s-%s-%s %s:%s:%s] %s", prefix,
// 					encoding.PadInt(year, 4), encoding.PadInt(int(month), 2),
// 					encoding.PadInt(day, 2), encoding.PadInt(hour, 2),
// 					encoding.PadInt(minute, 2), encoding.PadInt(second, 2),
// 					status)
// 			} else {
// 				fmt.Fprintf(file, "%s%s", prefix, status)
// 			}
// 			for idx, item := range items {
// 				if idx == 0 {
// 					fmt.Fprintf(file, "%v", item)
// 				} else {
// 					fmt.Fprintf(file, " %v", item)
// 				}
// 			}
// 			file.Write(suffix)
// 		case <-checker:
// 			if len(Logger.receiver) > 0 {
// 				checker <- 1
// 				continue
// 			}
// 			waiter <- 1
// 		}
// 	}

// }

// const (
// 	endOfRecord  = '\n'
// 	terminalByte = '\xff'
// )

// var endOfLogRecord = []byte{'\xff', '\n'}

// type FileLogger struct {
// 	name      string
// 	directory string
// 	rotate    int
// 	file      *os.File
// 	filename  string
// 	receiver  chan *Record
// }

// func (Logger *FileLogger) log() {

// 	rotateSignal := make(chan string)
// 	if logger.rotate > 0 {
// 		go signalRotation(logger, rotateSignal)
// 	}

// 	var record *Record
// 	var filename string

// 	for {
// 		select {
// 		case filename = <-rotateSignal:
// 			if filename != logger.filename {
// 				file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
// 				if err == nil {
// 					logger.file.Close()
// 					logger.file = file
// 					logger.filename = filename
// 				} else {
// 					fmt.Fprintf(os.Stderr, "ERROR: Couldn't rotate log: %s\n", err)
// 				}
// 			}
// 		case record = <-logger.receiver:
// 			argLength := len(record.Items)
// 			if record.Error {
// 				logger.file.Write([]byte{'E'})
// 			} else {
// 				logger.file.Write([]byte{'I'})
// 			}
// 			fmt.Fprintf(logger.file, "%v", now)
// 			for i := 0; i < argLength; i++ {
// 				message := strconv.Quote(fmt.Sprint(record.Items[i]))
// 				fmt.Fprintf(logger.file, "\xfe%s", message[0:len(message)-1])
// 			}
// 			logger.file.Write(endOfLogRecord)
// 		}
// 	}

// }

// func (logger *FileLogger) GetFilename(timestamp time.Time) string {
// 	var suffix string
// 	switch logger.rotate {
// 	case RotateNever:
// 		suffix = ""
// 	case RotateDaily:
// 		suffix = timestamp.Format(".2006-01-02")
// 	case RotateHourly:
// 		suffix = timestamp.Format(".2006-01-02.03")
// 	case RotateTest:
// 		suffix = timestamp.Format(".2006-01-02.03-04-05")
// 	}
// 	filename := logger.name + suffix + ".log"
// 	return path.Join(logger.directory, filename)
// }

// func signalRotation(logger *FileLogger, signalChannel chan string) {
// 	var interval time.Duration
// 	var filename string
// 	switch logger.rotate {
// 	case RotateDaily:
// 		interval = 86400000000000
// 	case RotateHourly:
// 		interval = 3600000000000
// 	case RotateTest:
// 		interval = 3000000000
// 	}
// 	for {
// 		filename = logger.GetFilename(now)
// 		if filename != logger.filename {
// 			signalChannel <- filename
// 		}
// 		<-time.After(interval)
// 	}
// }

// func AddFileLogger(name string, directory string, rotate int, logType int) (logger *FileLogger, err error) {
// 	logger = &FileLogger{
// 		name:      name,
// 		directory: directory,
// 		rotate:    rotate,
// 		receiver:  make(chan *Record, 100),
// 	}
// 	filename := logger.GetFilename(now)
// 	pointer := FixUpLog(filename)
// 	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
// 	if err != nil {
// 		return logger, err
// 	}
// 	if pointer > 0 {
// 		file.Seek(int64(pointer), 0)
// 	}
// 	logger.file = file
// 	logger.filename = filename
// 	go logger.log()
// 	AddReceiver(logger.receiver, logType)
// 	return logger, nil
// }

type hijacker struct{}

func (h hijacker) Write(p []byte) (int, error) {
	Info(string(p))
	return len(p), nil
}

func init() {

	// Hijack the standard library's log functionality.
	stdlog.SetFlags(0)
	stdlog.SetOutput(hijacker{})

	// Flush logs on exit.
	process.SetExitHandler(Close)

	if os.Getenv("DISABLE_DEFAULT_LOG_HANDLERS") != "" {
		format := JSONFormatter("")
		terminal.IsTerminal(int(os.Stderr.Fd()))
		stdoutOpts := &Options{
			Formatter: format,
			LogType:   InfoLog,
			Stream:    os.Stdout,
		}
		stderrOpts := &Options{
			Formatter: format,
			LogType:   ErrorLog,
			Stream:    os.Stderr,
		}
		SetHandler(MultiHandler(Stream(stdoutOpts), Stream(stderrOpts)))
	}

}
