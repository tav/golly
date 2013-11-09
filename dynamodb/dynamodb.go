// Public Domain (-) 2012-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package dynamodb implements a client library for
// interfacing with DynamoDB, Amazon's NoSQL Database
// Service.
//
// To start with, make sure that you have the appropriate
// AWS keys to instantiate an auth object:
//
//     auth := dynamodb.Auth("your-access-key", "your-secret-key")
//
// Next, assuming you are connecting directly to  Amazon's
// servers, choose one of the predefined endpoints like
// USEast1, EUWest1, etc.
//
//     endpoint := dynamodb.USWest2
//
// If you happen to be connecting to a region which hasn't
// been defined yet or want to connect to a DynamoDB Local
// instance for development, define your own custom
// endpoint, e.g.
//
//     endpoint := dynamodb.EndPoint("DynamoDB Local", "home", "localhost:8000", false)
//
// You are now ready to Dial the endpoint and instantiate a client:
//
//     client := dynamodb.Dial(endpoint, auth, nil)
//
// The third parameter is normally nil to Dial lets you specify a custom
// http.Transport should you need one. This is particularly
// useful in PaaS environments like Google App Engine where
// you might not be able use the standard transport. If you
// specify nil
//
// For example, on a restricted environment like Google App
// Engine, where the standard transport isn't available, you
// can use the transport they expose via the
// appengine/urlfetch package:
//
//     transport := &urlfetch.Transport{
//         Context:  appengine.NewContext(req),
//         Deadline: 10 * time.Second,
//     }
//
//     client := dynamodb.Dial(endpoint, auth, transport)
//
// The heart of the package revolves around the Client. You
// instantiate it by calling Dial with an endpoint and
// authentication details, e.g.
//
//
//     import "dynamodb"
//
//     auth := dynamodb.Auth("your-access-key", "your-secret-key")
//     client := dynamodb.Dial(dynamodb.USWest1, secret, nil)
//
//     query := table.Query()
//     query.Sort('-').Limit(20)
//
//     resp, err := client.Call("CreateTable", dynamodb.Map{
//         "TableName": "mytable",
//         "ProvisionedThroughput": dynamodb.Map{
//             "ReadCapacityUnits": 5,
//             "WriteCapacityUnits": 5,
//         },
//     })
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
	"strings"
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

// Error represents all responses to DynamoDB API calls with
// an HTTP status code other than 200.
type Error struct {
	Body       []byte
	StatusCode int
}

// Error satisfies the default error interface and
// automatically tries to parse any JSON response that
// DynamoDB may have sent in order to provide a useful error
// message.
func (e Error) Error() string {
	errtype, message := e.Info()
	if errtype == "" || message == "" {
		return fmt.Sprintf("dynamodb: error with http status code %d", e.StatusCode)
	}
	return fmt.Sprintf("dynamodb: %s: %s", errtype, message)
}

// Info tries to parse the error type and message from the
// JSON body that DynamoDB may have responded with.
func (e Error) Info() (errtype string, message string) {
	if e.Body == nil {
		return
	}
	info := map[string]string{}
	if json.Unmarshal(e.Body, &info) != nil {
		return
	}
	errtype = info["__type"]
	idx := strings.Index(errtype, "#")
	if idx > 0 {
		errtype = errtype[idx+1:]
	}
	return errtype, info["message"]
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

// Map provides a shortcut for the abstract data type used
// in all DynamoDB API calls.
type Map map[string]interface{}

type Query struct {
	table      *Table
	cursor     Key
	descending bool
	eventually bool
	index      string
	limit      int
	selector   string
}

func (q *Query) Sort(order byte) *Query {
	if order == '+' {
		q.descending = false
	} else if order == '-' {
		q.descending = true
	}
	return q
}

// func (q *Query) EventuallyConsistent() *Query {
// 	q.eventually = true
// 	return q
// }

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

func (q *Query) Run(consistent bool) error {
	// q.table.client.makeRequest("Query", payload)
	return nil
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
	client *Client
	name   string
}

func (t *Table) Get(key Key) error {
	// return c.makeRequest("GetItem", payload)
	return nil
}

func (t *Table) Delete(key Key) error {
	// return c.makeRequest("DeleteItem", payload)
	return nil
}

func (t *Table) Put(key Key) error {
	// return c.makeRequest("PutItem", payload)
	return nil
}

func (t *Table) PutIf(key Key) error {
	// return c.makeRequest("PutItem", payload)
	return nil
}

func (t *Table) Query() *Query {
	return &Query{}
}

func (t *Table) Update(key Key) error {
	// return c.makeRequest("UpdateItem", payload)
	return nil
}

type Client struct {
	auth     auth
	endpoint endpoint
	web      *http.Client
}

// Call does the heavy-lifting of initiating a DynamoDB API
// call and parsing the JSON response into a map.
//
// It's best to call certain API methods directly using this
// method:
//
//  - CreateTable
//  - DescribeTable
//  - DeleteTable
//  - ListTables
//  - UpdateTable
//
func (c *Client) Call(method string, params Map) (resp Map, err error) {
	var payload []byte
	if params == nil {
		payload = []byte{'{', '}'}
	} else {
		payload, err = json.Marshal(params)
		if err != nil {
			return
		}
	}
	// fmt.Println("PAYLOAD: ", string(payload))
	payload, err = c.makeRequest(method, payload)
	// fmt.Println("RESP PAYLOAD: ", string(payload))
	if err != nil {
		return
	}
	resp = Map{}
	err = json.Unmarshal(payload, &resp)
	return
}

func (c *Client) Table(name string) *Table {
	return &Table{
		client: c,
		name:   name,
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
		return nil, Error{
			Body:       body,
			StatusCode: resp.StatusCode,
		}
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
