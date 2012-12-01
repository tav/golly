// Public Domain (-) 2012 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package s3 implements support for AWS S3.
package s3

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"github.com/tav/golly/aws"
	"github.com/tav/golly/tlsconf"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type BucketInfo struct {
	CreationDate string
	Name         string
}

type BucketsInfo struct {
	Owner   OwnerInfo
	Buckets []BucketInfo `xml:"Buckets>Bucket"`
}

type OwnerInfo struct {
	DisplayName string
	ID          string
}

type RequestData struct {
	AuthBase      string
	Client        *http.Client
	Endpoint      string
	RaiseResponse bool
	SecretKey     []byte
}

func (r *RequestData) Call(method, bucket, path, contentType string, recv interface{}) (*http.Response, error) {
	pad := []byte(method)
	pad = append(pad, '\n')
	pad = append(pad, '\n')
	if contentType != "" {
		pad = append(pad, contentType...)
	}
	pad = append(pad, '\n')
	date := time.Now().UTC().Format(http.TimeFormat)
	pad = append(pad, date...)
	pad = append(pad, '\n')
	if bucket == "" {
		pad = append(pad, path...)
		path = "https://" + r.Endpoint + path
	} else {
		pad = append(pad, '/')
		pad = append(pad, bucket...)
		pad = append(pad, path...)
		path = "https://" + bucket + "." + r.Endpoint + path
	}
	url, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	req := &http.Request{
		Header:     http.Header{},
		Host:       url.Host,
		Method:     method,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		URL:        url,
	}
	req.Header["Date"] = []string{date}
	req.Header["Authorization"] = []string{r.AuthBase + r.Sign(pad)}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if r.RaiseResponse {
		panic(resp)
	}
	if recv != nil {
		err := xml.NewDecoder(resp.Body).Decode(recv)
		resp.Body.Close()
		return nil, err
	}
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(string(body))
	return resp, nil
}

func (r *RequestData) Sign(data []byte) string {
	h := hmac.New(sha1.New, r.SecretKey)
	h.Write(data)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

type Service struct {
	RequestData *RequestData
}

func (s *Service) Bucket(bucket string, region *aws.Region) *Bucket {
	return &Bucket{
		Name:               bucket,
		LocationConstraint: region.S3LocationConstraint,
		RequestData: &RequestData{
			AuthBase:  s.RequestData.AuthBase,
			Client:    s.RequestData.Client,
			Endpoint:  region.S3Endpoint,
			SecretKey: s.RequestData.SecretKey[:],
		}}
}

func (s *Service) ListBuckets() (*BucketsInfo, error) {
	buckets := &BucketsInfo{}
	_, err := s.RequestData.Call("GET", "", "/", "", buckets)
	return buckets, err
}

type Bucket struct {
	Name               string
	LocationConstraint string
	RequestData        *RequestData
}

func (b *Bucket) do(method, path string, recv interface{}) (*http.Response, error) {
	return b.RequestData.Call(method, b.Name, path, "", recv)
}

func (b *Bucket) doWithCT(method, path, contentType string, recv interface{}) (*http.Response, error) {
	return b.RequestData.Call(method, b.Name, path, contentType, recv)
}

func (b *Bucket) CanAccess() bool {
	resp, err := b.do("HEAD", "/", nil)
	if err != nil {
		return false
	}
	if resp.StatusCode != 200 {
		return false
	}
	return true
}

func New(accessKey, secretKey string, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig: tlsconf.Config,
		}}
	}
	return &Service{&RequestData{
		AuthBase:  "AWS " + accessKey + ":",
		Client:    client,
		Endpoint:  "s3.amazonaws.com",
		SecretKey: []byte(secretKey),
	}}
}
