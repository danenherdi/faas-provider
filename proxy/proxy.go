// Package proxy provides a default function invocation proxy method for OpenFaaS providers.
//
// The function proxy logic is used by the Gateway when `direct_functions` is set to false.
// This means that the provider will direct call the function and return the results.  This
// involves resolving the function by name and then copying the result into the original HTTP
// request.
//
// openfaas-provider has implemented a standard HTTP HandlerFunc that will handle setting
// timeout values, parsing the request path, and copying the request/response correctly.
//
//	bootstrapHandlers := bootTypes.FaaSHandlers{
//		FunctionProxy:  proxy.NewHandlerFunc(timeout, resolver),
//		DeleteHandler:  handlers.MakeDeleteHandler(clientset),
//		DeployHandler:  handlers.MakeDeployHandler(clientset),
//		FunctionLister: handlers.MakeFunctionLister(clientset),
//		ReplicaReader:  handlers.MakeReplicaReader(clientset),
//		ReplicaUpdater: handlers.MakeReplicaUpdater(clientset),
//		InfoHandler:    handlers.MakeInfoHandler(),
//	}
//
// proxy.NewHandlerFunc is optional, but does simplify the logic of your provider.
package proxy

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	fhttputil "github.com/danenherdi/faas-provider/httputil"
	"github.com/danenherdi/faas-provider/types"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
)

const (
	watchdogPort           = "8080"
	defaultContentType     = "text/plain"
	openFaaSInternalHeader = "X-OpenFaaS-Internal"
)

// BaseURLResolver URL resolver for proxy requests
//
// The FaaS provider implementation is responsible for providing the resolver function implementation.
// BaseURLResolver.Resolve will receive the function name and should return the URL of the
// function service.
type BaseURLResolver interface {
	Resolve(functionName string) (url.URL, error)
}

// NewHandlerFunc creates a standard http.HandlerFunc to proxy function requests.
// When verbose is set to true, the timing of each invocation will be printed out to
// stderr.
// The returned http.HandlerFunc will ensure:
//
//   - proper proxy request timeouts
//   - proxy requests for GET, POST, PATCH, PUT, and DELETE
//   - path parsing including support for extracing the function name, sub-paths, and query paremeters
//   - passing and setting the `X-Forwarded-Host` and `X-Forwarded-For` headers
//   - logging errors and proxy request timing to stdout
//
// Note that this will panic if `resolver` is nil.
func NewHandlerFunc(config types.FaaSConfig, resolver BaseURLResolver, verbose bool) http.HandlerFunc {
	if resolver == nil {
		panic("NewHandlerFunc: empty proxy handler resolver, cannot be nil")
	}

	proxyClient := NewProxyClientFromConfig(config)

	reverseProxy := httputil.ReverseProxy{}
	reverseProxy.Director = func(req *http.Request) {
		// At least an empty director is required to prevent runtime errors.
		req.URL.Scheme = "http"
	}
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
	}

	// Errors are common during disconnect of client, no need to log them.
	reverseProxy.ErrorLog = log.New(io.Discard, "", 0)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		switch r.Method {
		case http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodGet,
			http.MethodOptions,
			http.MethodHead:
			proxyRequest(w, r, proxyClient, resolver, &reverseProxy, verbose, false)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// NewFlowHandler creates a new http.HandlerFunc for handling flow requests.
func NewFlowHandler(config types.FaaSConfig, redisClient *redis.Client, resolver BaseURLResolver, flows types.Flows, verbose bool) http.HandlerFunc {
	// Check if the resolver is nil
	if resolver == nil {
		panic("NewFlowHandler: empty proxy handler resolver, cannot be nil")
	}

	// Create a new proxy client
	proxyClient := NewProxyClientFromConfig(config)

	// Create a new reverse proxy
	reverseProxy := httputil.ReverseProxy{}
	reverseProxy.Director = func(req *http.Request) {
		// At least an empty director is required to prevent runtime errors.
		req.URL.Scheme = "http"
	}
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
	}

	// Errors are common during disconnect of client, no need to log them.
	reverseProxy.ErrorLog = log.New(io.Discard, "", 0)

	// Return the handler function
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("the flow proxy handler is working properly.")

		// Fetch the name of the node (function)
		pathVars := mux.Vars(r)
		functionName := pathVars["name"]
		if functionName == "" {
			fhttputil.Errorf(w, http.StatusBadRequest, "Provide function name in the request path")
			return
		}

		// Fetch the flow of the node (function)
		flow, ok := flows.Flows[functionName]
		if !ok {
			fhttputil.Errorf(w, http.StatusBadRequest, "Can not find this kind of function in flows config")
		}

		// Read current request body
		var requestBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		if err != nil {
			log.Println("Error decoding JSON request body:", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Initialize the input flow variable
		flowInput := types.FlowInput{
			Args:     requestBody,
			Children: make(map[string]*types.FlowOutput),
		}

		// Try find the cache if it is enabled for function
		if config.EnableCaching && flow.Caching {
			requestBody["openfaas_flow_function_name"] = functionName
			reqBody, err := json.Marshal(requestBody)
			if err != nil {
				fmt.Printf("error in marshalling args of function %s for caching: %s\n", functionName, err.Error())
			}

			fmt.Printf("searching caching key of the %s function is %s", functionName, string(reqBody))

			// Generate SHA1 hash of the JSON string
			hashBytes := sha1.Sum(reqBody)
			hashString := fmt.Sprintf("%x", hashBytes)

			// Try to get cached response from Redis
			cachedResponseBytes, err := redisClient.Get(r.Context(), hashString).Bytes()
			if err == nil {
				// If cached response exists, parse the JSON string and return it
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.Copy(w, bytes.NewBuffer(cachedResponseBytes))
				fmt.Printf("get the response of %s from redis: %s\n", functionName, string(cachedResponseBytes))
				return
			}
		}

		// Iterate the children of the node (function)
		for alias, child := range flow.Children {
			// Recursively run the flow for these nodes
			fmt.Printf("processing the %s: [%+v], a child of %s\n", alias, child, functionName)

			// Grabbing the arguments of child
			args := make(map[string]interface{})
			for argField, mapField := range child.ArgsMap {
				args[argField] = flowInput.Args[mapField]
			}

			// Creating the URL of child for internal and third parties
			var destURL string
			if flows.Flows[child.Function].IsThirdParty {
				destURL = *flows.Flows[child.Function].ThirdPartyURL
			} else {
				destURL = fmt.Sprintf("http://127.0.0.1:8081/flow/%s", child.Function)
			}

			// Proxy the child function
			childRequestBody, err := json.Marshal(args)
			if err != nil {
				fmt.Printf("error in marshalling args of function %s: %s\n", alias, err.Error())
			}
			req, err := http.NewRequest(
				"POST",
				destURL,
				bytes.NewBuffer(childRequestBody),
			)
			if err != nil {
				fmt.Printf("error in creating new request of function %s: %s\n", alias, err.Error())
			}
			req.Header.Set("Content-Type", "application/json")

			// Do the request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("error in doing the request of function %s: %s\n", alias, err.Error())
			}

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("failed request of %s: %s\n", alias, resp.Body)
			}

			// Read the response body
			data, err := io.ReadAll(resp.Body)

			if err != nil {
				fmt.Printf("error in reading the response of function %s: %s\n", alias, err.Error())
			}

			fmt.Println(string(data))

			// Save the responses
			flowInput.Children[alias] = &types.FlowOutput{
				Data:     data,
				Function: child.Function,
			}
		}

		fmt.Printf("the flow input of the %s is: %+v\n", functionName, flowInput)

		// Create a new request body
		newRequestBody, _ := json.Marshal(flowInput)
		fmt.Printf("new body request of %s is: %s\n", functionName, string(newRequestBody))

		// Replace the existing request body with the new one
		r.Body = io.NopCloser(bytes.NewBuffer(newRequestBody))

		// Execute the target flow
		cacheResponse := proxyRequest(w, r, proxyClient, resolver, &reverseProxy, verbose, config.EnableCaching && flow.Caching)

		// Cache the response of the function
		if config.EnableCaching && flow.Caching {
			// Create a new request body for caching
			requestBody = flowInput.Args
			requestBody["openfaas_flow_function_name"] = functionName
			reqBody, err := json.Marshal(requestBody)
			if err != nil {
				fmt.Printf("error in marshalling args of function %s for caching: %s\n", functionName, err.Error())
			}

			fmt.Printf("the caching key of the %s function is %s", functionName, string(reqBody))

			// Generate SHA1 hash of the JSON string
			hashBytes := sha1.Sum(reqBody)
			hashString := fmt.Sprintf("%x", hashBytes)

			// Save the response
			responseBytes, _ := io.ReadAll(cacheResponse)
			redisClient.SetEx(r.Context(), hashString, responseBytes, time.Duration(flow.CacheTTL)*time.Second)
			fmt.Printf("caching the response of %s in redis: %s\n", functionName, string(responseBytes))
		}
	}
}

// NewProxyClientFromConfig creates a new http.Client designed for proxying requests and enforcing
// certain minimum configuration values.
func NewProxyClientFromConfig(config types.FaaSConfig) *http.Client {
	return NewProxyClient(config.GetReadTimeout(), config.GetMaxIdleConns(), config.GetMaxIdleConnsPerHost())
}

// NewProxyClient creates a new http.Client designed for proxying requests, this is exposed as a
// convenience method for internal or advanced uses. Most people should use NewProxyClientFromConfig.
func NewProxyClient(timeout time.Duration, maxIdleConns int, maxIdleConnsPerHost int) *http.Client {
	return &http.Client{
		// these Transport values ensure that the http Client will eventually timeout and prevents
		// infinite retries. The default http.Client configure these timeouts.  The specific
		// values tuned via performance testing/benchmarking
		//
		// Additional context can be found at
		// - https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		// - https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		//
		// Additionally, these overrides for the default client enable re-use of connections and prevent
		// CoreDNS from rate limiting under high traffic
		//
		// See also two similar projects where this value was updated:
		// https://github.com/prometheus/prometheus/pull/3592
		// https://github.com/minio/minio/pull/5860
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 1 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       120 * time.Millisecond,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// proxyRequest handles the actual resolution of and then request to the function service.
func proxyRequest(w http.ResponseWriter, originalReq *http.Request, proxyClient *http.Client, resolver BaseURLResolver, reverseProxy *httputil.ReverseProxy, verbose bool, returnBody bool) *bytes.Reader {
	ctx := originalReq.Context()

	pathVars := mux.Vars(originalReq)
	functionName := pathVars["name"]
	if functionName == "" {
		w.Header().Add(openFaaSInternalHeader, "proxy")

		fhttputil.Errorf(w, http.StatusBadRequest, "Provide function name in the request path")
		return nil
	}

	functionAddr, err := resolver.Resolve(functionName)
	if err != nil {
		w.Header().Add(openFaaSInternalHeader, "proxy")

		// TODO: Should record the 404/not found error in Prometheus.
		log.Printf("resolver error: no endpoints for %s: %s\n", functionName, err.Error())
		fhttputil.Errorf(w, http.StatusServiceUnavailable, "No endpoints available for: %s.", functionName)
		return nil
	}

	proxyReq, err := buildProxyRequest(originalReq, functionAddr, pathVars["params"])
	if err != nil {

		w.Header().Add(openFaaSInternalHeader, "proxy")

		fhttputil.Errorf(w, http.StatusInternalServerError, "Failed to resolve service: %s.", functionName)
		return nil
	}

	if proxyReq.Body != nil {
		defer proxyReq.Body.Close()
	}

	if verbose {
		start := time.Now()
		defer func() {
			seconds := time.Since(start)
			log.Printf("%s took %f seconds\n", functionName, seconds.Seconds())
		}()
	}

	if v := originalReq.Header.Get("Accept"); v == "text/event-stream" ||
		originalReq.Header.Get("Upgrade") == "websocket" {
		originalReq.URL = proxyReq.URL

		reverseProxy.ServeHTTP(w, originalReq)
		return nil
	}

	response, err := proxyClient.Do(proxyReq.WithContext(ctx))

	if err != nil {
		log.Printf("error with proxy request to: %s, %s\n", proxyReq.URL.String(), err.Error())

		w.Header().Add(openFaaSInternalHeader, "proxy")

		fhttputil.Errorf(w, http.StatusInternalServerError, "Can't reach service for: %s.", functionName)
		return nil
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	// Copy the headers and status code from the response
	clientHeader := w.Header()
	copyHeaders(clientHeader, &response.Header)
	w.Header().Set("Content-Type", getContentType(originalReq.Header, response.Header))
	w.WriteHeader(response.StatusCode)

	// Copy the response body to the client
	var cacheReader *bytes.Reader
	if response.Body != nil {
		if !returnBody {
			io.Copy(w, response.Body)
		} else {
			responseBody, _ := io.ReadAll(response.Body)
			responseReader := bytes.NewReader(responseBody)
			cacheReader = bytes.NewReader(responseBody)

			io.Copy(w, responseReader)
		}
	}

	return cacheReader
}

// buildProxyRequest creates a request object for the proxy request, it will ensure that
// the original request headers are preserved as well as setting openfaas system headers
func buildProxyRequest(originalReq *http.Request, baseURL url.URL, extraPath string) (*http.Request, error) {

	host := baseURL.Host
	if baseURL.Port() == "" {
		host = baseURL.Host + ":" + watchdogPort
	}

	url := url.URL{
		Scheme:   baseURL.Scheme,
		Host:     host,
		Path:     extraPath,
		RawQuery: originalReq.URL.RawQuery,
	}

	upstreamReq, err := http.NewRequest(originalReq.Method, url.String(), nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(upstreamReq.Header, &originalReq.Header)

	if len(originalReq.Host) > 0 && upstreamReq.Header.Get("X-Forwarded-Host") == "" {
		upstreamReq.Header["X-Forwarded-Host"] = []string{originalReq.Host}
	}
	if upstreamReq.Header.Get("X-Forwarded-For") == "" {
		upstreamReq.Header["X-Forwarded-For"] = []string{originalReq.RemoteAddr}
	}

	if originalReq.Body != nil {
		upstreamReq.Body = originalReq.Body
	}

	return upstreamReq, nil
}

// copyHeaders clones the header values from the source into the destination.
func copyHeaders(destination http.Header, source *http.Header) {
	for k, v := range *source {
		vClone := make([]string, len(v))
		copy(vClone, v)
		destination[k] = vClone
	}
}

// getContentType resolves the correct Content-Type for a proxied function.
func getContentType(request http.Header, proxyResponse http.Header) (headerContentType string) {
	responseHeader := proxyResponse.Get("Content-Type")
	requestHeader := request.Get("Content-Type")

	if len(responseHeader) > 0 {
		headerContentType = responseHeader
	} else if len(requestHeader) > 0 {
		headerContentType = requestHeader
	} else {
		headerContentType = defaultContentType
	}

	return headerContentType
}
