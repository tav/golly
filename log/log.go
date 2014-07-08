// Public Domain (-) 2010-2014 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package log provides an extensible logging framework.
package log

import (
	"fmt"
	"github.com/mgutz/ansi"
	"github.com/tav/golly/process"
	stdlog "log"
	"os"
	"text/template"
	"time"
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

// You can specify the LogType field on Options to control whether to log info
// logs, error logs or both.
type LogType int

const (
	InfoLog LogType = 1 << iota
	ErrorLog
	MixedLog LogType = InfoLog | ErrorLog
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
	process.SetExitHandler(Flush)

	// Clone the default template.FuncMap.
	for k, v := range funcMap {
		colorFuncMap[k] = v
	}

	// Override the color function.
	colorFuncMap["color"] = ansi.ColorCode

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
