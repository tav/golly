// Public Domain (-) 2012-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

package dynamodb

import (
	"net/http"
)

type Field struct {
	Name  string
	Value string
	Type  string
}

func (f *Field) MarshalJSON() ([]byte, error) {
}

type Item []*Field

func (f *Field) Binary() ([]byte, bool) {

}

func (f *Field) BinarySet() *Field {
}

func (f *Field) Float() (float64, bool) {
}

func (f *Field) FloatSet() ([]float64, bool) {
}

func (f *Field) Integer() (int64, bool) {
}

func (f *Field) IntegerSet() ([]int64, bool) {
}

func (f *Field) String() (string, bool) {
}

func (f *Field) StringSet() ([]string, bool) {
}

type Client struct {
	endpoint string
	db       map[string]string
}

func (c *Client) Get() {

}

func Dial(endpoint string) *Client {

}
