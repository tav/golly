// Public Domain (-) 2012-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

package dynamodb

import (
	"bytes"
)

type Item interface {
	Encode(buf *bytes.Buffer)
	Decode(data map[string]map[string]interface{})
}

type Client struct {
	endpoint string
}

func (c *Client) Get() {
}

func Dial(endpoint string) *Client {
	return &Client{}
}
