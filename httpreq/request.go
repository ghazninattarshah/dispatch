// Package httpreq ...
// Copyright (c) 2020, Ghazni Nattarshah <ghazni.nattarshah@gmail.com>
// See LICENSE for licensing information
package httpreq

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// ContentTypeJSON is a constant that denotes "application/json"
	ContentTypeJSON = "application/json"

	// ContentTypeURLENCODED is a constant that denotes "application/x-www-form-urlencoded"
	ContentTypeURLENCODED = "application/x-www-form-urlencoded"

	pathParamIndicator    = ":"
	urlSeparator          = "/"
	urlPathParamSeparator = "/:"
)

var (
	// defaultTimeout for any http request
	defaultTimeout              = 30 * time.Second
	errPathParamConflift        = errors.New("path parameter placeholder and actual vaues count doesn't match")
	errorMethodIsEmpty          = errors.New("request method is not set")
	errMethodUnknown            = errors.New("unknown request METHOD")
	errorResponseStructIsNil    = errors.New("response struct is nil")
	errNoPathParamValueSet      = errors.New("no path param value passed")
	errPathParamPassedIncorrect = errors.New("not enough path parameter values passed")
)

// Request is client that constructs the http request
// which then shall be dispatched to the passed URL
type Request struct {
	url                 string
	method              string
	headers             map[string]string
	pathParams          []string
	queryParams         map[string]string
	unescapeQueryParams bool

	httpcli     *http.Client
	proxyURL    string
	transport   http.RoundTripper
	timeout     time.Duration
	contentType string
	verbose     bool

	username string
	password string

	body       io.Reader
	bodyStruct interface{}
	bodyValues url.Values
}

// New constructs a httpreq.Request with passed method and url
func New(method, url string) *Request {
	return &Request{
		method:      method,
		url:         url,
		headers:     make(map[string]string),
		queryParams: make(map[string]string),
		pathParams:  make([]string, 0),
	}
}

// HTTPClient to override the default client
func (r *Request) HTTPClient(client *http.Client) *Request {
	r.httpcli = client
	return r
}

// ProxyURL to which the request has to pass through
// when dispatching the actual request
// if the HTTPClient is passed the proxyURL will be set
// as the caller has to set the proxy with the *http.Client
// configuration+
func (r *Request) ProxyURL(url string) *Request {
	r.proxyURL = url
	return r
}

// PathParams Inject params to the url path described with :
// Ex: http://foo.com/api/users/:name
func (r *Request) PathParams(params ...string) *Request {
	r.pathParams = params
	return r
}

// QueryParam Set a query parameter to a request
// Ex: http://foo.com/api/users?name=username
func (r *Request) QueryParam(key, value string) *Request {
	r.queryParams[key] = value
	return r
}

// Header Set a header value to a request
func (r *Request) Header(key, value string) *Request {
	r.headers[key] = value
	return r
}

// BodyStruct set the request struct which will be converted to json
// if the content type is application/json
func (r *Request) BodyStruct(request interface{}) *Request {
	r.bodyStruct = request
	return r
}

// BodyValues takes a key/value params and pass it as form values when the content type
// is set to application/x-www-form-urlencoded
func (r *Request) BodyValues(values url.Values) *Request {
	r.bodyValues = values
	return r
}

// Body set the request struct which will be converted to json during request
func (r *Request) Body(reader io.Reader) *Request {
	r.body = reader
	return r
}

// Timeout is the duration for a http request
func (r *Request) Timeout(timeout time.Duration) *Request {
	r.timeout = timeout
	return r
}

// UnescapeQueryParams perform the unescaping the query params before
// dispatching the request
func (r *Request) UnescapeQueryParams(unescape bool) *Request {
	r.unescapeQueryParams = unescape
	return r
}

// Verbose displays the detailed logs about the request progress and log
// the response body as string (this is recommended to know if you never know
// what's the response body looks like)
func (r *Request) Verbose(verbose bool) *Request {
	r.verbose = verbose
	return r
}

// BasicAuth set the base64 auth token in header
func (r *Request) BasicAuth(username, password string) *Request {
	r.username = username
	r.password = password
	return r
}

// DispatchScan performs sending the actual http request
// and scan the response (unmarshall to an struct)
func (r *Request) DispatchScan(response interface{}) error {
	if response == nil {
		return errorResponseStructIsNil
	}

	res, err := r.Dispatch()
	if err != nil {
		if res != nil {
			res.Body.Close()
		}
		return err
	}
	defer res.Body.Close()

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode %d response: %w", res.StatusCode, err)
	}
	return nil
}

// Dispatch send the http request to the passed URL
func (r *Request) Dispatch() (*http.Response, error) {
	// validate the metho type
	if err := r.validateMethod(); err != nil {
		return nil, err
	}

	// validate the path params count if exist
	if err := r.validatePathParams(); err != nil {
		return nil, err
	}

	// prepare the body
	body := r.body
	contentType := r.contentType
	if body == nil {
		if r.bodyStruct != nil {
			bits, err := json.Marshal(r.bodyStruct)
			if err != nil {
				return nil, fmt.Errorf("failed to marshall json request: %w", err)
			}
			body = bytes.NewBuffer(bits)
			contentType = ContentTypeJSON
		} else if r.bodyValues != nil {
			body = bytes.NewBufferString(r.bodyValues.Encode())
			contentType = ContentTypeURLENCODED
		}
	}

	// create request
	req, err := http.NewRequest(r.method, r.url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request :%w", err)
	}

	// Sets the basic auth header
	if r.username != "" || r.password != "" {
		req.Header.Add("Authorization", "Basic "+basicAuth(r.username, r.password))
	}

	// Sets the content type
	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}

	// Sets the header
	if len(r.headers) > 0 {
		for k, v := range r.headers {
			req.Header.Add(k, v)
		}
	}

	// Set the query params
	if len(r.queryParams) > 0 {
		q := req.URL.Query()
		for k, v := range r.queryParams {
			q.Add(k, v)
		}

		params := q.Encode()
		if r.unescapeQueryParams {
			var err error
			params, err = url.QueryUnescape(params)
			if err != nil {
				return nil, fmt.Errorf("query unescape failed: %w", err)
			}
		}
		req.URL.RawQuery = params
	}

	r.transport = http.DefaultTransport
	if r.httpcli == nil && r.proxyURL != "" {
		proxy, err := url.Parse(r.proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy url %w", err)
		}
		r.transport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
	}

	// create the http client if there isn't any passed
	if r.httpcli == nil {
		httpcli := &http.Client{
			Transport: r.transport,
			Timeout:   defaultTimeout,
		}

		if r.timeout > 0 {
			httpcli.Timeout = r.timeout
		}
		r.httpcli = httpcli
	}

	if r.verbose {
		log.Println("dispatching request to ", req.URL.String())
	}

	// perform the actual request
	res, err := r.httpcli.Do(req)
	if err != nil {
		if res != nil {
			res.Body.Close()
		}
		return nil, fmt.Errorf("dispatching request failed :%w", err)
	}

	// log the response body to console if verbose
	// this would be helpful while troubleshooting
	if r.verbose {
		resbody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading body: %v", err)
		}
		log.Printf("dispatch response: [%d][%s][%s]", res.StatusCode, res.Status, string(resbody))

		// set a new body
		res.Body = ioutil.NopCloser(bytes.NewBuffer(resbody))
	}
	return res, err
}

// validatePathParams checks whether the passed path params count
// and matches with the actual parameter passed or if it's isn't passed
func (r *Request) validatePathParams() error {
	if strings.Contains(r.url, urlPathParamSeparator) {
		actualPathParamCount := len(r.pathParams)
		if actualPathParamCount == 0 {
			return errNoPathParamValueSet
		}

		if strings.Count(r.url, urlPathParamSeparator) != actualPathParamCount {
			return errPathParamPassedIncorrect
		}

		// Set the path params
		splits := strings.Split(r.url, urlSeparator)
		var idx int
		for i, s := range splits {
			if strings.HasPrefix(s, pathParamIndicator) {
				splits[i] = r.pathParams[idx]
				idx++
			}
		}

		// Verify all the path params are set
		if idx != len(r.pathParams) {
			return errPathParamConflift
		}

		// sets the final url after injecting path params
		r.url = strings.Join(splits, urlSeparator)
	}
	return nil
}

// validateMethod checks whether the http method
// is valid, if not throw an error
func (r *Request) validateMethod() error {
	switch r.method {
	case "":
		return errorMethodIsEmpty
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace, http.MethodPatch:
	default:
		return errMethodUnknown
	}
	return nil
}

// basicAuth takes the username and password and
// form a base64 encoded string separated with colon
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
