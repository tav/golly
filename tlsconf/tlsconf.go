// Public Domain (-) 2010-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package tlsconf provides utility functions to support
// secure TLS configurations.
//
// It deals with the commonly overlooked issue of having the
// up-to-date root certificates data for trusted Certificate
// Authorities.
//
// When the package is initialised, it generates a global
// tlsconf.Config from the file specified in the $CACERT
// environment variable. This file should be PEM-encoded and
// contain the list of trusted TLS root certificates.
//
// The best way to generate such a file is to use the
// excellent extract-nss-root-certs tool written by Adam
// Langley, e.g.
//
//     $ go get github.com/agl/extract-nss-root-certs
//
// Grab the latest certificate data from Mozilla:
//
//     $ curl -O https://hg.mozilla.org/mozilla-central/raw-file/tip/security/nss/lib/ckfw/builtins/certdata.txt
//
// And generate the root certificate file:
//
//     $ extract-nss-root-certs certdata.txt > ca.certs
//
// And, then, finally, export it as the $CACERT environment
// variable to make it accessible to all programs which use
// this tlsconf package, i.e.
//
//     $ export CACERT=`pwd`/ca.certs
//
// Developers can then just use tlsconf.Config wherever they
// need to use a properly configured *tls.Config.
package tlsconf

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

var Config *tls.Config

// Load provides a utility function to create a tls.Config
// from a PEM file containing trusted root certificates.
func Load(certpath string) (*tls.Config, error) {
	data, err := ioutil.ReadFile(certpath)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(data)
	config := &tls.Config{
		Rand:    rand.Reader,
		Time:    time.Now,
		RootCAs: roots,
	}
	return config, nil
}

// init loads the data within the $CACERT file and initialises the
// tlsconf.Config variable.
func init() {
	path := os.Getenv("CACERT")
	if path == "" {
		fmt.Println("ERROR: The $CACERT environment variable hasn't been set!")
		os.Exit(1)
	}
	var err error
	Config, err = Load(path)
	if err != nil {
		fmt.Printf("ERROR: Couldn't load $CACERT file %s: %s\n", path, err)
		os.Exit(1)
	}
}
