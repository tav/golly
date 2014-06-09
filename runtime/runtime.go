// Public Domain (-) 2010-2014 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package runtime package provides utilities to manage the runtime environment
// for processes.
package runtime

import (
	"fmt"
	"github.com/tav/golly/log"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const Platform = runtime.GOOS

var SignalHandlers = make(map[os.Signal][]func())

func handleSignals() {
	notifier := make(chan os.Signal, 100)
	signal.Notify(notifier)
	var sig os.Signal
	for {
		sig = <-notifier
		handlers, found := SignalHandlers[sig]
		if found {
			for _, handler := range handlers {
				handler()
			}
		} else if sig == syscall.SIGTERM || sig == os.Interrupt {
			os.Exit(1)
		}
	}
}

func Exit(code int) {
	log.Wait()
	for _, handler := range SignalHandlers[os.Interrupt] {
		handler()
	}
	os.Exit(code)
}

func RegisterExitHandler(handler func()) {
	SignalHandlers[os.Interrupt] = append(SignalHandlers[os.Interrupt], handler)
}

func RegisterSignalHandler(signal os.Signal, handler func()) {
	SignalHandlers[signal] = append(SignalHandlers[signal], handler)
}

func Error(format string, v ...interface{}) {
	log.Error(format, v...)
	Exit(1)
}

func StandardError(err error) {
	log.StandardError(err)
	Exit(1)
}

func CreatePidFile(path string) {
	pidFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		StandardError(err)
	}
	fmt.Fprintf(pidFile, "%d", os.Getpid())
	err = pidFile.Close()
	if err != nil {
		StandardError(err)
	}
}

type Lock struct {
	link     string
	file     string
	acquired bool
}

func GetLock(directory, name string) (lock *Lock, err error) {
	file := path.Join(directory, fmt.Sprintf("%s-%d.lock", name, os.Getpid()))
	lockFile, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	lockFile.Close()
	link := path.Join(directory, name+".lock")
	err = os.Link(file, link)
	if err == nil {
		lock = &Lock{
			link: link,
			file: file,
		}
		RegisterExitHandler(func() { lock.ReleaseLock() })
	} else {
		os.Remove(file)
	}
	return
}

func (lock *Lock) ReleaseLock() {
	os.Remove(lock.file)
	os.Remove(lock.link)
}

// JoinPath joins the given path with the directory unless it happens to be an
// absolute path, in which case it returns the path exactly as it was given.
func JoinPath(directory, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(directory, filepath.Clean(path))
}

// SetMaxProcs will set Go's internal GOMAXPROCS to double the number of CPUs
// detected.
func SetMaxProcs() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 2)
}

// InitProcess acquires a process lock and writes the PID file for the current
// process.
func InitProcess(runPath, name string) {

	// Get the runtime lock to ensure we only have one process of any given name
	// running within the same run path at any time.
	_, err := GetLock(runPath, name)
	if err != nil {
		Error("Couldn't successfully acquire a process lock:\n\n\t%s\n", err)
	}

	// Write the process ID into a file for use by external scripts.
	go CreatePidFile(filepath.Join(runPath, name+".pid"))

}

// GetIP tries to determine the IP address of the current machine.
func GetIP() string {
	hostname, err := os.Hostname()
	if err != nil {
		StandardError(err)
	}
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		StandardError(err)
	}
	var ip string
	for _, addr := range addrs {
		if strings.Contains(addr, ":") || strings.HasPrefix(addr, "127.") {
			continue
		}
		ip = addr
		break
	}
	if ip == "" {
		Error("Couldn't determine local IP address")
	}
	return ip
}

// GetAddr returns host:port and fills in empty host parameter with the current
// machine's IP address if need be.
func GetAddr(host string, port int) string {
	if host == "" {
		host = GetIP()
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// GetAddrListener tries to determine the IP address of the machine when the
// host variable is empty and binds a TCP listener to the given host:port.
func GetAddrListener(host string, port int) (string, net.Listener) {
	addr := GetAddr(host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		Error("Cannot listen on %s: %v", addr, err)
	}
	return addr, listener
}

func init() {
	go handleSignals()
}
