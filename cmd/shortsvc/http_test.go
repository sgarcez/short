package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/discard"

	"github.com/sgarcez/short/pkg/shortendpoint"
	"github.com/sgarcez/short/pkg/shortservice"
	"github.com/sgarcez/short/pkg/shorttransport"
)

func TestHTTP(t *testing.T) {
	svc := shortservice.NewInMemService(log.NewNopLogger(), discard.NewCounter(), discard.NewCounter())
	eps := shortendpoint.New(svc, log.NewNopLogger(), discard.NewHistogram())
	mux := shorttransport.NewHTTPHandler(eps, log.NewNopLogger())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, testcase := range []struct {
		method, url, body, want string
	}{
		{"POST", srv.URL + "/api", `{"v":"12345"}`, `{"k":"gnzLDu"}`},
		{"GET", srv.URL + "/api/gnzLDu", ``, `{"v":"12345"}`},
	} {
		req, _ := http.NewRequest(testcase.method, testcase.url, strings.NewReader(testcase.body))
		resp, _ := http.DefaultClient.Do(req)
		body, _ := ioutil.ReadAll(resp.Body)
		if want, have := testcase.want, strings.TrimSpace(string(body)); want != have {
			t.Errorf("%s %s %s: want %q, have %q", testcase.method, testcase.url, testcase.body, want, have)
		}
	}
}
