package restapiclient

import (
	"encoding/base64"
	"net/http"
	"net/url"
)

// RequestConfig is a structure that the Options operate on.
//
// The Client uses this to effectively apply its options.
type RequestConfig struct {
	Header          http.Header                         // The request headers.
	QueryParameters url.Values                          // The request query parameters.
	ErrorFunction   func(response *http.Response) error // The function to use on error responses (status code 400 or greater).
	HTTPClient      *http.Client                        // The HTTP client to use for the request.
}

// Option is an option for a request.
//
// An option modifies a RequestConfig.
type Option func(*RequestConfig)

// OptionHeader returns an Option that represents a header.
func OptionHeader(name, value string) Option {
	return func(c *RequestConfig) {
		c.Header.Set(name, value)
	}
}

// OptionBasicAuth returns an Option that represents basic authentication with a username and password.
func OptionBasicAuth(username, password string) Option {
	return OptionHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
}

// OptionQuery returns an Option that represents a query parameter.
func OptionQuery(name, value string) Option {
	return func(c *RequestConfig) {
		c.QueryParameters.Add(name, value)
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
// If the client is nil, then the default HTTP client will be used.
func OptionHTTPClient(client *http.Client) Option {
	return func(c *RequestConfig) {
		c.HTTPClient = client
	}
}
