package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/danenherdi/faas-provider/types"
	"github.com/gorilla/mux"
)

const NameExpression = "-a-zA-Z_0-9."

func varHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("name: %s params: %s", vars["name"], vars["params"])))
}

func testResolver(functionName string) (url.URL, error) {
	return url.URL{
		Scheme: "http",
		Host:   functionName,
	}, nil
}

type mockResolver struct {
	u   *url.URL
	err error
}

func (m mockResolver) Resolve(name string) (url.URL, error) {
	if m.u != nil {
		return *m.u, m.err
	}
	return url.URL{}, m.err
}

func Test_ProxyHandler_StatusCode(t *testing.T) {

	testcases := []int{200, 204, 400, 409, 422, 500, 503}
	config := types.FaaSConfig{ReadTimeout: time.Second}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("returns %d when upstream returns %d", tc, tc), func(t *testing.T) {

			// upstream represents the will be resolved and the function call then sent to
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc > 399 {
					http.Error(w, "upstream error", tc)
					return
				}
				w.WriteHeader(tc)
			}))

			u, err := url.Parse(upstream.URL)
			proxyHandler := NewHandlerFunc(config, mockResolver{u, err}, false)

			rr := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "", nil)
			if err != nil {
				t.Fatal(err)
			}

			// we must set the function name URL variable to pass the validation in proxyRequest
			req = mux.SetURLVars(req, map[string]string{"name": "foo"})
			proxyHandler.ServeHTTP(rr, req)

			if rr.Code != tc {
				t.Fatalf("unexpected status code; got: %d, expected: %d", rr.Code, tc)
			}

			if tc > 399 && strings.TrimSpace(rr.Body.String()) != "upstream error" {
				t.Fatalf("unexpected response body, got: %s", rr.Body.String())
			}
		})
	}
}

func Test_pathParsing(t *testing.T) {
	tt := []struct {
		name         string
		functionPath string
		functionName string
		extraPath    string
		statusCode   int
	}{
		{
			"simple_name_match",
			"/function/echo",
			"echo",
			"",
			200,
		},
		{
			"simple_name_match",
			"/function/echo.openfaas-fn",
			"echo.openfaas-fn",
			"",
			200,
		},
		{
			"simple_name_match_with_trailing_slash",
			"/function/echo/",
			"echo",
			"",
			200,
		},
		{
			"name_match_with_additional_path_values",
			"/function/echo/subPath/extras",
			"echo",
			"subPath/extras",
			200,
		},
		{
			"name_match_with_additional_path_values_and_querystring",
			"/function/echo/subPath/extras?query=true",
			"echo",
			"subPath/extras",
			200,
		},
		{
			"not_found_if_no_name",
			"/function/",
			"",
			"",
			404,
		},
	}

	// Need to create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc("/function/{name:["+NameExpression+"]+}", varHandler)
	router.HandleFunc("/function/{name:["+NameExpression+"]+}/", varHandler)
	router.HandleFunc("/function/{name:["+NameExpression+"]+}/{params:.*}", varHandler)

	for _, s := range tt {
		t.Run(s.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req, err := http.NewRequest("GET", s.functionPath, nil)
			if err != nil {
				t.Fatal(err)
			}

			router.ServeHTTP(rr, req)
			if rr.Code != s.statusCode {
				t.Fatalf("unexpected status code; got: %d, expected: %d", rr.Code, s.statusCode)
			}

			body := rr.Body.String()
			expectedBody := fmt.Sprintf("name: %s params: %s", s.functionName, s.extraPath)
			if s.statusCode == http.StatusOK && body != expectedBody {
				t.Fatalf("incorrect function name and path params; got: %s, expected: %s", body, expectedBody)
			}
		})
	}
}

func Test_buildProxyRequest_Body_Method_Query(t *testing.T) {
	srcBytes := []byte("hello world")

	reader := bytes.NewReader(srcBytes)
	request, _ := http.NewRequest(http.MethodPost, "/?code=1", reader)
	request.Header.Set("X-Source", "unit-test")

	if request.URL.RawQuery != "code=1" {
		t.Errorf("Query - want: %s, got: %s", "code=1", request.URL.RawQuery)
		t.Fail()
	}

	funcURL, _ := testResolver("funcName")
	upstream, err := buildProxyRequest(request, funcURL, "")
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Method != upstream.Method {
		t.Errorf("Method - want: %s, got: %s", request.Method, upstream.Method)
		t.Fail()
	}

	upstreamBytes, _ := io.ReadAll(upstream.Body)

	if string(upstreamBytes) != string(srcBytes) {
		t.Errorf("Body - want: %s, got: %s", string(upstreamBytes), string(srcBytes))
		t.Fail()
	}

	if request.Header.Get("X-Source") != upstream.Header.Get("X-Source") {
		t.Errorf("Header X-Source - want: %s, got: %s", request.Header.Get("X-Source"), upstream.Header.Get("X-Source"))
		t.Fail()
	}

	if request.URL.RawQuery != upstream.URL.RawQuery {
		t.Errorf("URL.RawQuery - want: %s, got: %s", request.URL.RawQuery, upstream.URL.RawQuery)
		t.Fail()
	}

}

func Test_buildProxyRequest_NoBody_GetMethod_NoQuery(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "/", nil)

	funcURL, _ := testResolver("funcName")
	upstream, err := buildProxyRequest(request, funcURL, "")
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Method != upstream.Method {
		t.Errorf("Method - want: %s, got: %s", request.Method, upstream.Method)
		t.Fail()
	}

	if upstream.Body != nil {
		t.Errorf("Body - expected nil")
		t.Fail()
	}

	if request.URL.RawQuery != upstream.URL.RawQuery {
		t.Errorf("URL.RawQuery - want: %s, got: %s", request.URL.RawQuery, upstream.URL.RawQuery)
		t.Fail()
	}

}

func Test_buildProxyRequest_HasXForwardedHostHeaderWhenSet(t *testing.T) {
	srcBytes := []byte("hello world")

	reader := bytes.NewReader(srcBytes)
	request, err := http.NewRequest(http.MethodPost, "http://gateway/function?code=1", reader)

	if err != nil {
		t.Fatal(err)
	}

	funcURL, _ := testResolver("funcName")
	upstream, err := buildProxyRequest(request, funcURL, "/")
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Host != upstream.Header.Get("X-Forwarded-Host") {
		t.Errorf("Host - want: %s, got: %s", request.Host, upstream.Header.Get("X-Forwarded-Host"))
	}
}

func Test_buildProxyRequest_XForwardedHostHeader_Empty_WhenNotSet(t *testing.T) {
	srcBytes := []byte("hello world")

	reader := bytes.NewReader(srcBytes)
	request, err := http.NewRequest(http.MethodPost, "/function", reader)

	if err != nil {
		t.Fatal(err)
	}

	funcURL, _ := testResolver("funcName")
	upstream, err := buildProxyRequest(request, funcURL, "/")
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Host != upstream.Header.Get("X-Forwarded-Host") {
		t.Errorf("Host - want: %s, got: %s", request.Host, upstream.Header.Get("X-Forwarded-Host"))
	}
}

func Test_buildProxyRequest_XForwardedHostHeader_WhenAlreadyPresent(t *testing.T) {
	srcBytes := []byte("hello world")
	headerValue := "test.openfaas.com"
	reader := bytes.NewReader(srcBytes)
	request, err := http.NewRequest(http.MethodPost, "/function/test", reader)

	if err != nil {
		t.Fatal(err)
	}

	request.Header.Set("X-Forwarded-Host", headerValue)
	funcURL, _ := testResolver("funcName")
	upstream, err := buildProxyRequest(request, funcURL, "/")
	if err != nil {
		t.Fatal(err.Error())
	}

	if upstream.Header.Get("X-Forwarded-Host") != headerValue {
		t.Errorf("X-Forwarded-Host - want: %s, got: %s", headerValue, upstream.Header.Get("X-Forwarded-Host"))
	}
}

func Test_proxyRequest_ContentType_Header(t *testing.T) {
	const requestContentType = "x-www-form-urlencoded"
	const wantContentType = "application/json"
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/function/figlet", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = mux.SetURLVars(req, map[string]string{"name": "figlet"})
	req.Header.Set("Content-Type", requestContentType)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Content-Type set in the function response
		w.Header().Set("Content-Type", wantContentType)
	}))
	defer upstream.Close()

	config := types.FaaSConfig{ReadTimeout: 1 * time.Second}

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	proxyHandler := NewHandlerFunc(config, mockResolver{u, err}, false)

	proxyHandler.ServeHTTP(rr, req)

	got := rr.Header().Get("Content-Type")
	if got != wantContentType {
		t.Errorf("Want Content-Type %q, got: %q", wantContentType, got)
	}
}

func Test_buildProxyRequest_WithPathNoQuery(t *testing.T) {
	srcBytes := []byte("hello world")
	functionPath := "/employee/info/300"

	requestPath := fmt.Sprintf("/function/xyz%s", functionPath)

	reader := bytes.NewReader(srcBytes)
	request, _ := http.NewRequest(http.MethodPost, requestPath, reader)
	request.Header.Set("X-Source", "unit-test")

	queryWant := ""
	if request.URL.RawQuery != queryWant {

		t.Errorf("Query - want: %s, got: %s", queryWant, request.URL.RawQuery)
		t.Fail()
	}

	funcURL, _ := testResolver("xyz")
	upstream, err := buildProxyRequest(request, funcURL, functionPath)
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Method != upstream.Method {
		t.Errorf("Method - want: %s, got: %s", request.Method, upstream.Method)
		t.Fail()
	}

	upstreamBytes, _ := io.ReadAll(upstream.Body)

	if string(upstreamBytes) != string(srcBytes) {
		t.Errorf("Body - want: %s, got: %s", string(upstreamBytes), string(srcBytes))
		t.Fail()
	}

	if request.Header.Get("X-Source") != upstream.Header.Get("X-Source") {
		t.Errorf("Header X-Source - want: %s, got: %s", request.Header.Get("X-Source"), upstream.Header.Get("X-Source"))
		t.Fail()
	}

	if request.URL.RawQuery != upstream.URL.RawQuery {
		t.Errorf("URL.RawQuery - want: %s, got: %s", request.URL.RawQuery, upstream.URL.RawQuery)
		t.Fail()
	}

	if functionPath != upstream.URL.Path {
		t.Errorf("URL.Path - want: %s, got: %s", functionPath, upstream.URL.Path)
		t.Fail()
	}

}

func Test_buildProxyRequest_WithNoPathNoQuery(t *testing.T) {
	srcBytes := []byte("hello world")
	functionPath := "/"

	requestPath := fmt.Sprintf("/function/xyz%s", functionPath)

	reader := bytes.NewReader(srcBytes)
	request, _ := http.NewRequest(http.MethodPost, requestPath, reader)
	request.Header.Set("X-Source", "unit-test")

	queryWant := ""
	if request.URL.RawQuery != queryWant {

		t.Errorf("Query - want: %s, got: %s", queryWant, request.URL.RawQuery)
		t.Fail()
	}

	funcURL, _ := testResolver("xyz")
	upstream, err := buildProxyRequest(request, funcURL, functionPath)
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Method != upstream.Method {
		t.Errorf("Method - want: %s, got: %s", request.Method, upstream.Method)
		t.Fail()
	}

	upstreamBytes, _ := io.ReadAll(upstream.Body)

	if string(upstreamBytes) != string(srcBytes) {
		t.Errorf("Body - want: %s, got: %s", string(upstreamBytes), string(srcBytes))
		t.Fail()
	}

	if request.Header.Get("X-Source") != upstream.Header.Get("X-Source") {
		t.Errorf("Header X-Source - want: %s, got: %s", request.Header.Get("X-Source"), upstream.Header.Get("X-Source"))
		t.Fail()
	}

	if request.URL.RawQuery != upstream.URL.RawQuery {
		t.Errorf("URL.RawQuery - want: %s, got: %s", request.URL.RawQuery, upstream.URL.RawQuery)
		t.Fail()
	}

	if functionPath != upstream.URL.Path {
		t.Errorf("URL.Path - want: %s, got: %s", functionPath, upstream.URL.Path)
		t.Fail()
	}

}

func Test_buildProxyRequest_WithPathAndQuery(t *testing.T) {
	srcBytes := []byte("hello world")
	functionPath := "/employee/info/300"

	requestPath := fmt.Sprintf("/function/xyz%s?code=1", functionPath)

	reader := bytes.NewReader(srcBytes)
	request, _ := http.NewRequest(http.MethodPost, requestPath, reader)
	request.Header.Set("X-Source", "unit-test")

	if request.URL.RawQuery != "code=1" {
		t.Errorf("Query - want: %s, got: %s", "code=1", request.URL.RawQuery)
		t.Fail()
	}

	funcURL, _ := testResolver("xyz")
	upstream, err := buildProxyRequest(request, funcURL, functionPath)
	if err != nil {
		t.Fatal(err.Error())
	}

	if request.Method != upstream.Method {
		t.Errorf("Method - want: %s, got: %s", request.Method, upstream.Method)
		t.Fail()
	}

	upstreamBytes, _ := io.ReadAll(upstream.Body)

	if string(upstreamBytes) != string(srcBytes) {
		t.Errorf("Body - want: %s, got: %s", string(upstreamBytes), string(srcBytes))
		t.Fail()
	}

	if request.Header.Get("X-Source") != upstream.Header.Get("X-Source") {
		t.Errorf("Header X-Source - want: %s, got: %s", request.Header.Get("X-Source"), upstream.Header.Get("X-Source"))
		t.Fail()
	}

	if request.URL.RawQuery != upstream.URL.RawQuery {
		t.Errorf("URL.RawQuery - want: %s, got: %s", request.URL.RawQuery, upstream.URL.RawQuery)
		t.Fail()
	}

	if functionPath != upstream.URL.Path {
		t.Errorf("URL.Path - want: %s, got: %s", functionPath, upstream.URL.Path)
		t.Fail()
	}

}

func Test_NewProxyClientConfig(t *testing.T) {
	cases := []struct {
		name                string
		config              types.FaaSConfig
		timeout             time.Duration
		maxIdleConns        int
		maxIdleConnsPerHost int
	}{
		{
			name:                "empty config sets default values",
			config:              types.FaaSConfig{},
			timeout:             10 * time.Second,
			maxIdleConns:        1024,
			maxIdleConnsPerHost: 1024,
		},
		{
			name: "custom values are set correctly",
			config: types.FaaSConfig{
				ReadTimeout:         1 * time.Microsecond,
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
			},
			timeout:             1 * time.Microsecond,
			maxIdleConns:        20,
			maxIdleConnsPerHost: 10,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewProxyClientFromConfig(tc.config)
			if client.Timeout != tc.timeout {
				t.Fatalf("expected timeout %s, got %s", tc.timeout.String(), client.Timeout.String())
			}
			transport := (client.Transport).(*http.Transport)
			if transport.MaxIdleConns != tc.maxIdleConns {
				t.Fatalf("expected MaxIdleConns %d, got %d", tc.maxIdleConns, transport.MaxIdleConns)
			}

			if transport.MaxIdleConnsPerHost != tc.maxIdleConnsPerHost {
				t.Fatalf("expected MaxIdleConnsPerHost %d, got %d", tc.maxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
			}
		})
	}
}
