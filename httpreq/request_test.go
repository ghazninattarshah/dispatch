// Package httpreq ...
// Copyright (c) 2020, Ghazni Nattarshah <ghazni.nattarshah@gmail.com>
// See LICENSE for licensing information
package httpreq

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHttpRequestDispatch(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		urlpath       string
		pathParams    []string
		queryParams   map[string]string
		headerParams  map[string]string
		body          io.Reader
		bodyStruct    interface{}
		bodyValues    url.Values
		verbose       bool
		username      string
		password      string
		response      interface{}
		scanResult    bool
		expectedError error
	}{
		{
			"EmptyMethod",
			"",
			"",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			false,
			"",
			"",
			nil,
			false,
			errorMethodIsEmpty,
		},
		{
			"InvalidMethod",
			"test",
			"",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			false,
			"",
			"",
			nil,
			false,
			errMethodUnknown,
		},
		{
			"NoPathParamValuePassed",
			http.MethodGet,
			"/:user",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			false,
			"",
			"",
			nil,
			false,
			errNoPathParamValueSet,
		},
		{
			"PathParamValueMissing",
			http.MethodGet,
			"/:team/users/:user",
			[]string{"bar"},
			nil,
			nil,
			nil,
			nil,
			nil,
			false,
			"",
			"",
			nil,
			false,
			errPathParamPassedIncorrect,
		},
		{
			"ValidPathParam",
			http.MethodGet,
			"/:user",
			[]string{"bar"},
			nil,
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidpathparamMultiple",
			http.MethodGet,
			"/:team/users/:user",
			[]string{"bar", "ghazni"},
			nil,
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidGetRequest",
			http.MethodPost,
			"/users/:user",
			[]string{"foo"},
			nil,
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidGetRequestWithQueryParamsAndPathParams",
			http.MethodPost,
			"/users/:user",
			[]string{"foo"},
			map[string]string{
				"name": "gamma",
				"type": "alpha",
			},
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidPostRequestWithoutBody",
			http.MethodPost,
			"/users",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidPostRequestWithBodyStruct",
			http.MethodPost,
			"/users",
			nil,
			nil,
			nil,
			nil,
			struct {
				name string
				city string
			}{"foo", "bar"},
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidPostRequestWithBodyValues",
			http.MethodPost,
			"/users",
			nil,
			nil,
			nil,
			nil,
			nil,
			url.Values{
				"username": []string{"john"},
			},
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidPostRequestWithBody",
			http.MethodPost,
			"/users",
			nil,
			nil,
			nil,
			bytes.NewBufferString(`{
				"name": "foo"
			}`),
			nil,
			nil,
			true,
			"",
			"",
			"",
			false,
			nil,
		},
		{
			"ValidPostRequestWithBodyDispatchScan",
			http.MethodPost,
			"/users",
			nil,
			map[string]string{
				"wrb": "true", // writeresponsebody
			},
			nil,
			nil,
			nil,
			nil,
			true,
			"",
			"",
			"",
			true,
			nil,
		},
		{
			"ValidPostRequestBasicAuthHeaders",
			http.MethodPost,
			"/users",
			nil,
			nil,
			map[string]string{
				"Authorization": "ksjhf23j1lkj23",
			},
			nil,
			nil,
			nil,
			false,
			"foo",
			"foopazz",
			"",
			false,
			nil,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			serv := httptest.NewServer(http.HandlerFunc(testserver))
			defer serv.Close()

			r := New(test.method, serv.URL+test.urlpath)

			if test.pathParams != nil {
				r.PathParams(test.pathParams...)
			}

			if test.queryParams != nil {
				for k, v := range test.queryParams {
					r.QueryParam(k, v)
				}
			}

			r.BasicAuth(test.username, test.password)

			if test.body != nil {
				r.Body(test.body)
			} else if test.bodyStruct != nil {
				r.BodyStruct(test.bodyStruct)
			} else if test.bodyValues != nil {
				r.BodyValues(test.bodyValues)
			}
			r.Verbose(test.verbose)

			var res *http.Response
			var err error
			if !test.scanResult {
				res, err = r.Dispatch()
			} else {
				const expectedName = "foo"
				var resp struct {
					Name string
				}
				err = r.DispatchScan(&resp)
				if err != nil {
					t.Error("didn't expect error:", err)
				}
				if resp.Name != expectedName {
					t.Errorf("expected scan result %v, got %v", expectedName, resp.Name)
				}
			}

			if test.expectedError != nil && !errors.Is(test.expectedError, err) {
				t.Errorf("expected error %v, got %v", test.expectedError, err)
			}

			if res != nil {
				resbody, err := ioutil.ReadAll(res.Body)
				if err != nil {
					t.Errorf("Error reading response body: %v", err)
				}
				if string(resbody) != test.response {
					t.Errorf("expected response %v, got %v", test.response, string(resbody))
				}
			}
		})
	}
}

func testserver(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodPost {
		if wrb := r.URL.Query().Get("wrb"); wrb == "true" {
			w.Write([]byte(`{ "name": "foo"}`))
		}
	}
}
