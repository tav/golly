// Public Domain (-) 2012-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package DynamoDB implements a client library for
// interfacing with Amazon's NoSQL Database Service.
//
// The heart of the package revolves around the Client. You
// instantiate it by calling Dial with an endpoint and
// authentication details, e.g.
//
//
//     import "dynamodb"
//
//     secret := dynamodb.Auth("accessKey", "secretKey")
//     client := dynamodb.Dial(dynamodb.USWest1, secret, nil)
package dynamodb

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tav/golly/tlsconf"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

const (
	iso8601 = "20060102T150405Z"
)

type endpoint struct {
	name   string
	region string
	host   string
	tls    bool
	url    string
}

func (e endpoint) String() string {
	return fmt.Sprintf("<%s: %s>", e.name, e.host)
}

// EndPoint creates an endpoint struct for use with Dial.
// It's useful when using a local mock DynamoDB server, e.g.
//
//     dev := EndPoint("dev", "eu-west-1", "localhost:9091", false)
//
// Otherwise, unless Amazon upgrade their infrastructure,
// the predefined endpoints like USEast1 should suffice.
func EndPoint(name, region, host string, tls bool) endpoint {
	var url string
	if tls {
		url = "https://" + host + "/"
	} else {
		url = "http://" + host + "/"
	}
	return endpoint{
		name:   name,
		region: region,
		host:   host,
		tls:    tls,
		url:    url,
	}
}

// Current DynamoDB endpoints within Amazon's
// infrastructure.
var (
	APNorthEast1 = EndPoint("Tokyo", "ap-northeast-1", "dynamodb.ap-northeast-1.amazonaws.com", true)
	APSouthEast1 = EndPoint("Singapore", "ap-southeast-1", "dynamodb.ap-southeast-1.amazonaws.com", true)
	APSouthEast2 = EndPoint("Sydney", "ap-southeast-2", "dynamodb.ap-southeast-2.amazonaws.com", true)
	EUWest1      = EndPoint("Ireland", "eu-west-1", "dynamodb.eu-west-1.amazonaws.com", true)
	SAEast1      = EndPoint("Sao Paulo", "sa-east-1", "dynamodb.sa-east-1.amazonaws.com", true)
	USEast1      = EndPoint("N. Virginia", "us-east-1", "dynamodb.us-east-1.amazonaws.com", true)
	USWest1      = EndPoint("Oregon", "us-west-1", "dynamodb.us-west-1.amazonaws.com", true)
	USWest2      = EndPoint("Northern California", "us-west-2", "dynamodb.us-west-2.amazonaws.com", true)
)

type auth struct {
	accessKey string
	secretKey []byte
}

func Auth(accessKey, secretKey string) auth {
	return auth{
		accessKey: accessKey,
		secretKey: []byte("AWS4" + secretKey),
	}
}

// Item specifies an interface for encoding and decoding a
// struct into the custom JSON format required by DynamoDB.
// The dynamodb-marshal tool, that accompanies this package
// in the cmd directory, is capable of auto-generating
// optimised code to satisfy this interface.
//
// To make use of it, put the structs you want to optimise
// in a file, e.g. model.go
//
//     package campaign
//
//     type Contribution struct {
//         Email string
//         On    time.Time
//         Tags  []string
//     }
//
// Then run the tool from the command line, e.g.
//
//    $ dynamodb-marshal model.go
//
// This will generate a model_marshal.go file which would
// contain implementations for the Encode() and Decode()
// methods that satisfy the Item interface, e.g.
//
//     package campaign
//
//     func (c *Contribution) Encode(buf *bytes.Buffer) {
//         // optimised implementation ...
//     }
//
//     func (c *Contribution) Decode(data map[string]map[string]interface{}) {
//         // optimised implementation ...
//     }
//
// You can expect the performance of the optimised version
// to be somewhere between 1.5x to 10x the reflection-based
// default implementation.
type Item interface {
	Encode(buf *bytes.Buffer)
	Decode(data map[string]map[string]interface{})
}

type Key struct {
}

type Options map[string]interface{}

type Query struct {
	table      *Table
	cursor     Key
	descending bool
	eventually bool
	index      string
	limit      int
	selector   string
}

func (q *Query) Ascending() *Query {
	q.descending = false
	return q
}

func (q *Query) Descending() *Query {
	q.descending = true
	return q
}

func (q *Query) EventuallyConsistent() *Query {
	q.eventually = true
	return q
}

func (q *Query) Index(name string) *Query {
	q.index = name
	return q
}

func (q *Query) Only(attrs ...string) *Query {
	return q
}

func (q *Query) Limit(n int) *Query {
	q.limit = n
	return q
}

func (q *Query) Run() error {
	return q.table.client.makeRequest("Query", payload)
}

func (q *Query) Select(mechanism string) *Query {
	q.selector = mechanism
	return q
}

func (q *Query) WithCursor(key Key) *Query {
	q.cursor = key
	return q
}

type Table struct {
	client     *Client
	eventually bool
	mutex      sync.RWMutex
	name       string
}

func (t *Table) CheckAndSet(key Key) error {
	return c.makeRequest("PutItem", payload)
}

func (t *Table) EventuallyConsistent() *Table {
	t.mutex.Lock()
	t.eventually = true
	t.mutex.Unlock()
	return t
}

func (t *Table) Get(key Key) error {
	return c.makeRequest("GetItem", payload)
}

func (t *Table) Delete(key Key) error {
	return c.makeRequest("DeleteItem", payload)
}

func (t *Table) Put(key Key) error {
	return c.makeRequest("PutItem", payload)
}

func (t *Table) Query() *Query {
	return &Query{}
}

func (t *Table) Update(key Key) error {
	return c.makeRequest("UpdateItem", payload)
}

type Client struct {
	auth     auth
	endpoint endpoint
	web      *http.Client
}

// Call does the heavy-lifting of initiating a DynamoDB API
// call and returns the response data.
//
//  - CreateTable
//  - DescribeTable
//  - DeleteTable
//  - ListTables
//  - UpdateTable
//
// The above API methods are best initiated via the Call
// method.
func (c *Client) Call(method string, opts Options) (resp Options, err error) {
	payload, err := json.Marshal(opts)
	fmt.Println("PAYLOAD: ", string(payload))
	if err != nil {
		return err
	}
	body, err := c.makeRequest(method, payload)
	fmt.Println("RESP PAYLOAD: ", string(body))
	if err != nil {
		return err
	}
	resp = Options{}
	return json.Unmarshal(body, &resp)
}

func (c *Client) Table(name string) *Table {
	return &Table{
		client:      c,
		consistency: true,
		name:        name,
	}
}

// TODO(tav): Minimise string allocation by writing to a
// buffer of some kind.
func (c *Client) makeRequest(method string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", c.endpoint.url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	hasher.Write(payload)
	datetime := time.Now().UTC().Format(iso8601)
	date := datetime[:8]
	method = "DynamoDB_20120810." + method
	canonicalReq := "POST\n/\n\ncontent-type:application/x-amz-json-1.0\nhost:" + c.endpoint.host + "\nx-amz-date:" + datetime + "\nx-amz-target:" + method + "\n\ncontent-type;host;x-amz-date;x-amz-target\n" + hex.EncodeToString(hasher.Sum(nil))
	hasher.Reset()
	hasher.Write([]byte(canonicalReq))
	post := "AWS4-HMAC-SHA256\n" + datetime + "\n" + date + "/" + c.endpoint.region + "/dynamodb/aws4_request\n" + hex.EncodeToString(hasher.Sum(nil))
	sig := hex.EncodeToString(doHMAC(doHMAC(doHMAC(doHMAC(doHMAC(c.auth.secretKey, date), c.endpoint.region), "dynamodb"), "aws4_request"), post))
	credential := "AWS4-HMAC-SHA256 Credential=" + c.auth.accessKey + "/" + date + "/" + c.endpoint.region + "/dynamodb/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=" + sig
	req.Header.Set("Authorization", credential)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("Host", c.endpoint.host)
	req.Header.Set("X-Amz-Date", datetime)
	req.Header.Set("X-Amz-Target", method)
	resp, err := c.web.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		fmt.Println("NOT 200")
		fmt.Println(string(body))
	}
	return body, nil
}

func doHMAC(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func Dial(region endpoint, creds auth, transport http.RoundTripper) *Client {
	if transport == nil {
		transport = &http.Transport{TLSClientConfig: tlsconf.Config}
	}
	return &Client{
		auth:     creds,
		endpoint: region,
		web:      &http.Client{Transport: transport},
	}
}
