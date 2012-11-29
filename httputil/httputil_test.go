// Public Domain (-) 2012 The Golly Authors.
// See the Golly UNLICENSE file for details.

package httputil

import (
	"net/http"
	"testing"
)

func setup(key, value string) *Acceptable {
	hdr := http.Header{}
	hdr.Set(key, value)
	return Parse(&http.Request{Header: hdr}, key)
}

func TestAcceptParsing(t *testing.T) {
	s := setup("Accept-Encoding", "deflate,gzip;q=0,gzip;q=0;q=0.2")
	t.Logf("MATCH: %v", s.Accepts("foo"))
	t.Logf("MATCH: %v", s.Accepts("gzip"))
	s = setup("Accept-Encoding", "deflate, no-gzip")
	t.Logf("MATCH: %v", s.Accepts("foo"))
	t.Logf("MATCH: %v", s.Accepts("gzip"))
	s = setup("Accept-Encoding", "gzip;q=1.0, identity; q=0.5, *;q=0")
	t.Logf("MATCH: %v", s.Accepts("foo"))
	t.Logf("MATCH: %v", s.Accepts("gzip"))
	s = setup("Accept-Encoding", "identity;q=0")
	t.Logf("MATCH: %v", s.Accepts("identity"))
	s = setup("Accept", "audio/*; q=0.2, audio/basic")
	t.Logf("PREF: %v", s.FindPreferred("audio/mp3", "audio/basic"))
	for _, opt := range s.opts {
		t.Logf("meta: %q %v", opt.metaPrefix, opt.metaWildcard)
	}
	s = setup("Accept", "*/*;q=0.2, audio/*; q=0.2, audio/basic")
	t.Logf("OPTS: %v", s.Options())
	for _, opt := range s.opts {
		t.Logf("meta: %q %v", opt.metaPrefix, opt.metaWildcard)
	}
	s = setup("Accept", "text/plain; q=0.5, text/html,text/x-dvi; q=0.8, text/x-c, */*")
	t.Logf("PREF: %#v", s.FindPreferred("image/png", "text/plain", "text/x-c", "text/html"))
	s = setup("Accept-Language", "da, en;q=0.7, en-gb;q=0.8")
	t.Logf("PREF: %#v", s.FindPreferred("fr", "gb", "en-us", "en-gb", "de", "da", "en-in"))
	s = setup("Accept-Encoding", "deflate,gzip;q=0,gzip;q=0.2")
	t.Logf("PREF: %#v", s.FindPreferred("gzip", "compress"))
}
