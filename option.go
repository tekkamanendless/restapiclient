package restapiclient

import (
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/cenkalti/backoff/v5"
)

// RequestConfig is a structure that the Options operate on.
//
// The Client uses this to effectively apply its options.
type RequestConfig struct {
	BackOffIsPermanentErrorFunction func(err error) bool                // The function to use to determine if an error is permanent.
	BackOffOptions                  []backoff.RetryOption               // The options to use for the backoff algorithm.
	BaseURL                         string                              // The base URL to use for the request.
	ErrorFunction                   func(response *http.Response) error // The function to use on error responses (status code 400 or greater).
	Header                          http.Header                         // The request headers.
	HTTPClient                      *http.Client                        // The HTTP client to use for the request.
	QueryParameters                 url.Values                          // The request query parameters.
}

// Option is an option for a request.
//
// An option modifies a RequestConfig.
type Option func(*RequestConfig)

// OptionBackOffIsPermanentErrorFunction returns an Option that represents a function to determine if an error is permanent.
//
// If this function is provided, then exponential backoff will be used for every request.
// The function should return true if the error is "permanent", meaning that it should NOT be retried.
// Otherwise, it should return false, meaning that the request WILL be retried.
//
// Set this to nil to disable backoff.
func OptionBackOffIsPermanentErrorFunction(function func(err error) bool) Option {
	return func(c *RequestConfig) {
		c.BackOffIsPermanentErrorFunction = function
	}
}

// OptionBackOffOptions returns an Option that sets the backoff configuration.
//
// The default backoff configuration will retry for up to 1 minute.
func OptionBackOffOptions(options ...backoff.RetryOption) Option {
	return func(c *RequestConfig) {
		c.BackOffOptions = append(c.BackOffOptions, options...)
	}
}

// OptionBaseURL returns an Option that represents a base URL.
func OptionBaseURL(baseURL string) Option {
	return func(c *RequestConfig) {
		c.BaseURL = baseURL
	}
}

// OptionHeader returns an Option that represents a header.
func OptionHeader(name string, values ...string) Option {
	return func(c *RequestConfig) {
		c.Header.Del(name)
		for _, value := range values {
			c.Header.Add(name, value)
		}
	}
}

// OptionBasicAuth returns an Option that represents basic authentication with a username and password.
func OptionBasicAuth(username, password string) Option {
	return OptionHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
}

// OptionQuery returns an Option that represents a query parameter.
func OptionQuery(name string, values ...string) Option {
	return func(c *RequestConfig) {
		c.QueryParameters.Del(name)
		for _, value := range values {
			c.QueryParameters.Add(name, value)
		}
	}
}

// OptionErrorHandler returns an Option that represents a custom error handler.
//
// The function will be called if the response has a status code of 400 or greater.
// The error returned by the function will be returned by Client.Do.
//
// If the function is nil, then the default error handling will be used.
func OptionErrorHandler(function func(response *http.Response) error) Option {
	return func(c *RequestConfig) {
		c.ErrorFunction = function
	}
}

// OptionHTTPClient returns an Option that represents a custom HTTP client.
//
// If the client is nil, then a new http.Client will be used.
func OptionHTTPClient(client *http.Client) Option {
	if client == nil {
		client = &http.Client{}
	}
	return func(c *RequestConfig) {
		c.HTTPClient = client
	}
}
