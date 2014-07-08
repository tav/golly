// Public Domain (-) 2010-2014 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package log provides an extensible logging framework.
package log

import (
	"bufio"
	"code.google.com/p/go.crypto/ssh/terminal"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mgutz/ansi"
	"github.com/tav/golly/process"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
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

var Must must

type must struct{}

func (m must) StreamHandler(o *Options) Handler {
	h, err := StreamHandler(o)
	if err != nil {
		panic(err)
	}
	return h
}

func (m must) TemplateFormatter(tmpl string, color bool, funcs template.FuncMap) Formatter {
	f, err := TemplateFormatter(tmpl, color, funcs)
	if err != nil {
		panic(err)
	}
	return f
}

var funcMap = template.FuncMap{
	"basepath": func(p string) string {
		return filepath.Base(p)
	},
	"color": func(style string) string {
		return ""
	},
	"json": func(v interface{}) string {
		out, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(out)
	},
	"jsonindent": func(v interface{}, indent string) string {
		out, err := json.MarshalIndent(v, "", indent)
		if err != nil {
			return ""
		}
		return string(out)
	},
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

var colorFuncMap = template.FuncMap{}

var (
	errAlreadyClosed    = errors.New("log: stream already closed")
	errMissingFormatter = errors.New("log: stream options need to have the Formatter field set")
	errMissingStream    = errors.New("log: stream options need to have either the Filename or the Stream fields set")
)

type Data map[string]interface{}

type Entry struct {
	Context    string    `codec:"ctx"                  json:"ctx"`
	Data       Data      `codec:"data"                 json:"data"`
	Error      bool      `codec:"error"                json:"error"`
	File       string    `codec:"file,omitempty"       json:"file,omitempty"`
	Line       int       `codec:"line,omitempty"       json:"line,omitempty"`
	Message    string    `codec:"msg"                  json:"msg"`
	Stacktrace string    `codec:"stacktrace,omitempty" json:"stacktrace,omitempty"`
	Timestamp  time.Time `codec:"timestamp"            json:"timestamp"`
}

type Formatter interface {
	Write(*Entry, io.Writer) error
}

func JSONFormatter(pretty bool) Formatter {
	return &jsonFormatter{pretty}
}

type jsonFormatter struct {
	pretty bool
}

func (f *jsonFormatter) Write(e *Entry, w io.Writer) error {
	var (
		err error
		out []byte
	)
	if f.pretty {
		out, err = json.MarshalIndent(e, "", "  ")
	} else {
		out, err = json.Marshal(e)
	}
	if err != nil {
		return err
	}
	_, err = w.Write(append(out, '\xff', '\n'))
	return err
}

// SupportsColor inspects the file descriptor to figure out if it's attached to
// a terminal, and if so, returns true for color support. Example usage:
//
//     useColor := log.SupportsColor(os.Stdout)
func SupportsColor(f *os.File) bool {
	return terminal.IsTerminal(int(f.Fd()))
}

func TemplateFormatter(tmpl string, color bool, funcs template.FuncMap) (Formatter, error) {
	if funcs == nil || len(funcs) == 0 {
		if color {
			funcs = colorFuncMap
		} else {
			funcs = funcMap
		}
	} else {
		var base template.FuncMap
		if color {
			base = colorFuncMap
		} else {
			base = funcMap
		}
		for k, v := range base {
			if funcs[k] == nil {
				funcs[k] = v
			}
		}
	}
	indented := strings.Join(strings.Split(strings.TrimSpace(tmpl), "\n"), "\n    ")
	description := "log.formatter\n\n    " + indented + "\n\n"
	t, err := template.New(description).Funcs(funcs).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	return &templateFormatter{
		template: t,
	}, nil
}

type templateFormatter struct {
	template *template.Template
}

func (f *templateFormatter) Write(e *Entry, w io.Writer) error {
	return f.template.Execute(w, e)
}

type Handler interface {
	Async() bool
	Close() error
	Flush() error
	Log(*Entry) error
}

type Options struct {
	BufferSize    int
	Filename      string
	Filter        func(*Entry) bool
	Formatter     Formatter
	FlushInterval time.Duration
	LogType       LogType
	Rotate        *RotateOptions
	Stream        io.WriteCloser
}

type RotateOptions struct {
	Filename   string
	LocalTime  bool
	MaxAge     time.Duration
	MaxBackups int
	MaxSize    int
	Signal     os.Signal
}

func StreamHandler(o *Options) (Handler, error) {
	s := &stream{}
	var (
		err error
	)
	if o.Filename != "" {
		// Rotate existing..
		s.file, err = os.OpenFile(o.Filename, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return nil, err
		}
		s.wc = s.file
	} else if o.Stream != nil {
		s.wc = o.Stream
	} else {
		return nil, errMissingStream
	}
	if o.Formatter == nil {
		return nil, errMissingFormatter
	}
	s.f = o.Formatter
	if o.BufferSize > 0 {
		s.buf = bufio.NewWriterSize(s.wc, o.BufferSize)
		if o.FlushInterval > 0 {
			go s.flush(o.FlushInterval)
		} else {
			go s.flush(time.Second)
		}
	}
	if o.LogType&ErrorLog != 0 {
		s.logErr = true
	}
	if o.LogType&InfoLog != 0 {
		s.logInfo = true
	}
	// process.RegisterSignalHandler(syscall.SIGHUP, file.Rotate)
	if s.file != nil && o.Rotate != nil {
		go s.rotate(o)
	}
	return s, nil
}

type stream struct {
	buf     *bufio.Writer
	closed  bool
	f       Formatter
	file    *os.File
	filter  func(*Entry) bool
	logInfo bool
	logErr  bool
	maxSize int64
	mu      sync.Mutex
	wc      io.WriteCloser
	written int64
}

func (s *stream) Async() bool {
	return false
}

func (s *stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		return s.wc.Close()
	}
	return errAlreadyClosed
}

func (s *stream) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errAlreadyClosed
	}
	if s.buf != nil {
		err := s.buf.Flush()
		if err != nil {
			return err
		}
	}
	if s.file != nil {
		err := s.file.Sync()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *stream) Log(e *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errAlreadyClosed
	}
	if e.Error {
		if !s.logErr {
			return nil
		}
	} else if !s.logInfo {
		return nil
	}
	if s.filter != nil && !s.filter(e) {
		return nil
	}
	return s.f.Write(e, s.wc)
}

func (s *stream) flush(duration time.Duration) {
	time.Sleep(duration)
	for s.Flush() != nil {
		time.Sleep(duration)
	}
}

func (s *stream) rotate(o *Options) {
}

// AsyncHandler wraps a handler with a channel and runs it in a separate
// goroutine so that logging doesn't block the current thread.
func AsyncHandler(bufsize int, h Handler) Handler {
	a := &async{
		flush:     make(chan struct{}),
		flushWait: make(chan struct{}),
		handler:   h,
		queue:     make(chan *Entry, bufsize),
		stop:      make(chan struct{}),
	}
	go a.run()
	return a
}

type async struct {
	closed    bool
	handler   Handler
	flush     chan struct{}
	flushWait chan struct{}
	mu        sync.Mutex
	queue     chan *Entry
	stop      chan struct{}
}

func (a async) Async() bool {
	return true
}

func (a async) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()
	a.stop <- struct{}{}
	<-a.stop
	close(a.stop)
	return a.handler.Close()
}

func (a async) Flush() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	a.flush <- struct{}{}
	<-a.flushWait
	return nil
}

func (a async) Log(e *Entry) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.queue <- e
	a.mu.Unlock()
	return nil
}

func (a async) run() {
	var e *Entry
	flush := false
	stop := false
	for {
		select {
		case e = <-a.queue:
			a.handler.Log(e)
		case <-a.stop:
			stop = true
		case <-a.flush:
			flush = true
		}
		if flush {
			a.mu.Lock()
			if len(a.queue) == 0 {
				a.mu.Unlock()
				a.handler.Flush()
				flush = false
				a.flushWait <- struct{}{}
			} else {
				a.mu.Unlock()
			}
		} else if stop && len(a.queue) == 0 { // TODO(tav): check if len is thread safe
			close(a.flush)
			close(a.flushWait)
			close(a.queue)
			a.stop <- struct{}{}
			break
		}
	}
}

func FailoverHandler(handlers ...Handler) Handler {
	return failover{handlers}
}

type failover struct {
	handlers []Handler
}

func (f failover) Async() bool {
	for _, h := range f.handlers {
		if h.Async() {
			return true
		}
	}
	return false
}

func (f failover) Close() error {
	var firstError error
	for _, h := range f.handlers {
		err := h.Close()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

func (f failover) Flush() error {
	var firstError error
	for _, h := range f.handlers {
		err := h.Flush()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
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

func (l list) Async() bool {
	for _, h := range l.handlers {
		if h.Async() {
			return true
		}
	}
	return false
}

func (l list) Close() error {
	var firstError error
	for _, h := range l.handlers {
		err := h.Close()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

func (l list) Flush() error {
	var firstError error
	for _, h := range l.handlers {
		err := h.Close()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
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

func Flush() {
	root.Flush()
}

func Error(v interface{}, data ...Data) {
	msg, ok := v.(string)
	if !ok {
		if vdata, ok := v.(Data); ok {
			root.log("", vdata, true)
			return
		}
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
		if vdata, ok := v.(Data); ok {
			root.log("", vdata, true)
			return
		}
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
		if vdata, ok := v.(Data); ok {
			root.log("", vdata, false)
			return
		}
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

func LogEntry(e *Entry) {
	root.logEntry(e, 2)
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

type hijacker struct{}

func (h hijacker) Write(p []byte) (int, error) {
	Info(string(p))
	return len(p), nil
}

func init() {
	for k, v := range funcMap {
		colorFuncMap[k] = v
	}
	colorFuncMap["color"] = ansi.ColorCode
}

func init() {

	// Hijack the standard library's log functionality.
	stdlog.SetFlags(0)
	stdlog.SetOutput(hijacker{})

	// Flush logs on exit.
	process.SetExitHandler(Flush)

	// Initialise the default log handlers.
	if os.Getenv("DISABLE_DEFAULT_LOG_HANDLERS") == "" {
		stdoutHandler := Must.StreamHandler(&Options{
			BufferSize: 4096,
			Formatter: Must.TemplateFormatter(
				`{{color "green"}}{{.Timestamp.Format "[2006-01-02 15:04:05]"}} {{printf "%-60s" .Message}}{{if .Data}}{{json .Data}}{{end}}{{color "reset"}}
`, SupportsColor(os.Stdout), nil),
			LogType: InfoLog,
			Stream:  os.Stdout,
		})
		stderrHandler := Must.StreamHandler(&Options{
			BufferSize: 4096,
			Formatter: Must.TemplateFormatter(
				`{{color "red"}}{{.Timestamp.Format "[2006-01-02 15:04:05]"}} ERROR: {{printf "%-60s" .Message}}{{if .Data}}{{json .Data}}{{end}}{{color "reset"}}
`, SupportsColor(os.Stderr), nil),
			LogType: ErrorLog,
			Stream:  os.Stderr,
		})
		SetHandler(MultiHandler(stdoutHandler, stderrHandler))
	}

}
