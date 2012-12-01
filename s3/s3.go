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

type InitiatorInfo struct {
	DisplayName string
	ID          string
}

type MultipartUploadsInfo struct {
	Bucket             string
	CommonPrefixes     []string `xml:">Prefix"`
	Delimiter          string
	IsTruncated        bool
	KeyMarker          string
	MaxUploads         int
	NextKeyMarker      string
	NextUploadIdMarker string
	Prefix             string
	Upload             []UploadInfo
	UploadIdMarker     string
}

type OwnerInfo struct {
	DisplayName string
	ID          string
}

type UploadInfo struct {
	Key          string
	Initiated    string
	Initiator    InitiatorInfo
	Owner        OwnerInfo
	StorageClass string
	UploadId     string
}

type RequestData struct {
	AuthBase      string
	Client        *http.Client
	Endpoint      string
	RaiseResponse bool
	SecretKey     []byte
}

func (r *RequestData) Call(method, bucket, path, canonicalPath, contentType string, recv interface{}) (*http.Response, error) {
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
		pad = append(pad, canonicalPath...)
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
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		max := len(body)
		if len(body) > 1000 {
			max = 1000
		}
		return nil, fmt.Errorf("s3 error: got %s on %s\n%s", resp.Status, path, body[:max])
	}
	if recv != nil {
		err := xml.NewDecoder(resp.Body).Decode(recv)
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
		Region:             region.ID,
		RequestData: &RequestData{
			AuthBase:  s.RequestData.AuthBase,
			Client:    s.RequestData.Client,
			Endpoint:  region.S3Endpoint,
			SecretKey: s.RequestData.SecretKey[:],
		}}
}

func (s *Service) ListBuckets() (*BucketsInfo, error) {
	info := &BucketsInfo{}
	_, err := s.RequestData.Call("GET", "", "/", "/", "", info)
	return info, err
}

type Bucket struct {
	Name               string
	LocationConstraint string
	Region             string
	RequestData        *RequestData
}

func (b *Bucket) do(method, path string, recv interface{}) (*http.Response, error) {
	return b.RequestData.Call(method, b.Name, path, path, "", recv)
}

func (b *Bucket) CanAccess() (bool, error) {
	resp, err := b.do("HEAD", "/", nil)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != 200 {
		return false, fmt.Errorf(
			"s3 error: cannot access bucket %q in %s, got %s",
			b.Name, b.Region, resp.Status)
	}
	return true, nil
}

func (b *Bucket) ListMultipartUploads(opts *url.Values) (*MultipartUploadsInfo, error) {
	info := &MultipartUploadsInfo{}
	path := "/?uploads"
	if opts != nil {
		path = "/?uploads&" + opts.Encode()
	}
	_, err := b.RequestData.Call("GET", b.Name, path, "/?uploads", "", info)
	return info, err
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
