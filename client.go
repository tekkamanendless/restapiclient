package restapiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/cenkalti/backoff/v5"
	"github.com/tekkamanendless/httperror"
)

// Client is a client for a REST API.
type Client interface {
	Do(ctx context.Context, method string, path string, input any, output any, options ...Option) error
	Config() RequestConfig
	WithOptions(options ...Option) Client
}

// clientStruct is an client for a REST API.
//
// It will also work with any HTTP server.
type clientStruct struct {
	Options []Option // This is a list of Options to apply, if any.
}

// New returns a new Client.
func New(options ...Option) Client {
	return &clientStruct{
		Options: options,
	}
}

// Config returns the fully-realized RequestConfig for the Client.
func (c *clientStruct) Config() RequestConfig {
	requestConfig := RequestConfig{
		BackOffIsPermanentErrorFunction: nil, // There is no permanent error function.
		BackOffOptions:                  nil, // There are no backoff options.
		BaseURL:                         "",  // There is no base URL.
		Header:                          http.Header{},
		QueryParameters:                 url.Values{},
		HTTPClient:                      &http.Client{},
		ErrorFunction:                   nil, // There is no custom error function.
	}
	for _, option := range c.Options {
		option(&requestConfig)
	}
	return requestConfig
}

// WithOptions returns a new Client with the given options already set.
func (c *clientStruct) WithOptions(options ...Option) Client {
	// Create a new Client.
	newClient := clientStruct{
		Options: make([]Option, 0, len(c.Options)+len(options)), // Allocate space for existing and new options.
	}
	// Copy the existing options.
	newClient.Options = append(newClient.Options, c.Options...)
	// Add the new options.
	newClient.Options = append(newClient.Options, options...)
	return &newClient
}

// defaultErrorFunction is the default error function if none is provided.
func defaultErrorFunction(response *http.Response) error {
	err := httperror.ErrorFromStatus(response.StatusCode)
	return err
}

// Do a request.
//
// The Client Options will be applied first, and then any Options given here will be applied.
//
// If `input` is provided, it will be handled according to its type:
//
//   - `restapiclient.RawBytes`: it will be sent as-is.
//   - `url.Values`: it will be serialized as a URL-encoded string.
//   - Any other type: it will be serialized based on the Content-Type header given to this function.
//
// If `output` is provided, it will be handled according to its type:
//
//   - `restapiclient.RawBytes`: the contents will be returned as-is.
//   - Any other type: the contents will be deserialized based on the response Content-Type header from the server.
//
// If the HTTP status code is non-2xx, then an httperror.Error will be returned.
// You can customize the exact error by specifying an ErrorFunction using OptionErrorFunction.
// This will allow you to parse the response body and return a more specific error.
//
// Consider wrapping that custom error in an httperror.Error to make it easier to handle.
//
//	httpErr := httperror.ErrorFromStatus(response.StatusCode)
//	yourErr := ...
//	return fmt.Errorf("%w: %w", httpErr, yourErr)
func (c *clientStruct) Do(ctx context.Context, method string, path string, input any, output any, options ...Option) error {
	requestConfig := c.Config()
	for _, option := range options {
		option(&requestConfig)
	}

	var reader io.Reader
	if input != nil {
		contentType := requestConfig.Header.Get("Content-Type")

		if rawBytes, ok := input.(RawBytes); ok {
			if contentType == "" {
				contentType = "application/json"
				requestConfig.Header.Set("Content-Type", contentType)
			}

			reader = bytes.NewReader(rawBytes)
		} else if rawBytes, ok := input.(*RawBytes); ok {
			if contentType == "" {
				contentType = "application/json"
				requestConfig.Header.Set("Content-Type", contentType)
			}

			reader = bytes.NewReader(*rawBytes)
		} else if urlValues, ok := input.(url.Values); ok {
			if contentType == "" {
				contentType = "application/x-www-form-urlencoded"
				requestConfig.Header.Set("Content-Type", contentType)
			}

			reader = bytes.NewReader([]byte(urlValues.Encode()))
		} else if urlValues, ok := input.(*url.Values); ok {
			if contentType == "" {
				contentType = "application/x-www-form-urlencoded"
				requestConfig.Header.Set("Content-Type", contentType)
			}

			reader = bytes.NewReader([]byte(urlValues.Encode()))
		} else {
			inputValue := reflect.ValueOf(input)

			if contentType == "" {
				contentType = "application/json"
				requestConfig.Header.Set("Content-Type", contentType)
			}

			switch true {
			case strings.HasPrefix(contentType, "application/json"):
				contents, err := json.Marshal(input)
				if err != nil {
					return fmt.Errorf("%w: could not serialize JSON: %w", ErrClientInvalidInput, err)
				}
				reader = bytes.NewReader(contents)
			case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
				inputBytes := []byte(fmt.Sprintf("%s", input))
				slog.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("Body: %s", inputBytes))
				reader = bytes.NewReader(inputBytes)
			case strings.HasPrefix(contentType, "multipart/form-data;"):
				inputBytes := []byte(fmt.Sprintf("%s", input))
				reader = bytes.NewReader(inputBytes)
			case strings.HasPrefix(contentType, "text/"):
				if inputValue.Kind() == reflect.String {
					inputBytes := []byte(fmt.Sprintf("%s", input))
					reader = bytes.NewReader(inputBytes)
				} else if inputValue.Type().String() == "[]uint8" {
					reader = bytes.NewReader(input.([]byte))
				}
			default:
				return fmt.Errorf("%w: unhandled content type: %s", ErrClientInvalidInput, contentType)
			}
		}
	}

	var targetURL string
	if strings.Contains(path, "://") {
		targetURL = path
	} else {
		targetURL = strings.TrimRight(requestConfig.BaseURL, "/")
		if strings.TrimLeft(path, "/") != "" {
			targetURL += "/" + strings.TrimLeft(path, "/")
		}
	}
	if value := requestConfig.QueryParameters.Encode(); value != "" {
		if strings.Contains(targetURL, "?") {
			targetURL += "&"
		} else {
			targetURL += "?"
		}
		targetURL += value
	}

	request, err := http.NewRequest(method, targetURL, reader)
	if err != nil {
		return err
	}
	for key, values := range requestConfig.Header {
		request.Header.Del(key)
		for _, value := range values {
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Header: %s: %s", key, value))
			request.Header.Add(key, value)
		}
	}

	retryCount := 0
	_, err = backoff.Retry(ctx, func() (bool, error) {
		response, err := func() (*http.Response, error) {
			dumpRequestIfEnabled(ctx, request)

			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Making request [retry %d]: %s %s %v", retryCount, method, targetURL, request.Header))
			response, err := requestConfig.HTTPClient.Do(request)
			if err != nil {
				return nil, err
			}

			dumpResponseIfEnabled(ctx, response)

			return response, nil
		}()
		// If we failed at any point so far, then this is a permanent error; we can't retry it.
		if err != nil {
			return false, backoff.Permanent(err)
		}

		contents, err := io.ReadAll(response.Body)
		if err != nil {
			return false, backoff.Permanent(err)
		}

		if response.StatusCode >= 200 && response.StatusCode <= 299 {
			err := func() error {
				if output != nil {
					if rawBytes, ok := output.(*RawBytes); ok {
						*rawBytes = contents
					} else {
						outputValue := reflect.ValueOf(output)
						if outputValue.Kind() != reflect.Pointer {
							return fmt.Errorf("%w: not a pointer: %T", ErrClientInvalidOutput, output)
						}
						outputValue = outputValue.Elem()

						contentType := response.Header.Get("Content-Type")
						switch true {
						case strings.HasPrefix(contentType, "application/json"):
							err = json.Unmarshal(contents, output)
							if err != nil {
								return fmt.Errorf("%w: could not deserialize JSON: %w", ErrClientInvalidOutput, err)
							}
						case strings.HasPrefix(contentType, "text/"):
							if outputValue.Kind() == reflect.String {
								outputValue.SetString(string(contents))
							} else if outputValue.Type().String() == "[]uint8" {
								outputValue.SetBytes(contents)
							} else {
								return fmt.Errorf("%w: invalid type for content type %s: %T", ErrClientInvalidOutput, contentType, outputValue.Type().String())
							}
						default:
							return fmt.Errorf("%w: unhandled content type: %s", ErrClientInvalidOutput, contentType)
						}
					}
				}

				return nil
			}()
			if err != nil {
				return false, backoff.Permanent(err)
			}
			return true, nil
		}

		// Restore the response body so the custom error handler can parse it.
		response.Body = io.NopCloser(bytes.NewReader(contents))

		// Figure out what error we want to return before handing it off to the custom function.
		//
		// There's a *chance* that the custom function could alter the response to change its error
		// code and then return nil, so we want to compute ours first, just in case.
		err = func() error {
			defaultErr := defaultErrorFunction(response)
			if requestConfig.ErrorFunction != nil {
				// If the user provided a custom error function, then use it.
				err = requestConfig.ErrorFunction(response)
				if err != nil {
					return err
				}
				// For whatever reason, the custom function did not actually provide an error.
				// Return the default error instead.
			}
			return defaultErr
		}()

		slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("BackoffIsPermanentErrorFunction: %t", requestConfig.BackOffIsPermanentErrorFunction != nil))
		retryCount++
		if requestConfig.BackOffIsPermanentErrorFunction != nil {
			isPermanentError := requestConfig.BackOffIsPermanentErrorFunction(err)
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("BackoffIsPermanentErrorFunction is set.  Error: %v.  Permanent: %t", err, isPermanentError))
			if isPermanentError {
				return false, backoff.Permanent(err)
			}
			return false, err
		}
		return false, backoff.Permanent(err)
	}, requestConfig.BackOffOptions...)
	if err != nil {
		return err
	}
	return nil
}
