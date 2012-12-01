// Public Domain (-) 2010-2011 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package tls provides utility functions to support TLS connections.
//
// When the package is initialised via tlsconf.Init(), it generates a default
// configuration from the TLS Certificate data found in the $CACERT file.
package tlsconf

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"github.com/tav/golly/runtime"
	"io/ioutil"
	"os"
	"time"
)

var Config *tls.Config

func GenConfig(file string) (config *tls.Config, err error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(data)
	config = &tls.Config{
		Rand:    rand.Reader,
		Time:    time.Now,
		RootCAs: roots,
	}
	return config, nil
}

// Init loads the data within the $CACERT file and initialises the
// tlsconf.Config variable.
func Init() {
	path := os.Getenv("CACERT")
	if path == "" {
		runtime.Error("The $CACERT environment variable hasn't been set!")
		return
	}
	var err error
	Config, err = GenConfig(path)
	if err != nil {
		runtime.Error("Couldn't load %s: %s", path, err)
	}
}
