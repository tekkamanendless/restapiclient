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
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tekkamanendless/httperror"
)

// Client is an client for a REST API.
//
// This will basically work with anything, including ours.
type Client struct {
	BaseURL    string      // This is the base URL; all paths will be appended to this.
	Options    []Option    // This is a list of Options to apply, if any.
	httpClient http.Client // This is used internally for making requests.
}

// New returns a new Client.
func New(baseURL string, options ...Option) *Client {
	return &Client{
		BaseURL: baseURL,
		Options: options,
	}
}

// HTTPClient returns a pointer to the HTTP client that will be used for the next request.
//
// If no option overrides it, then the internal HTTP client will be returned.
// Otherwise, the HTTP client from the option will be returned.
//
// You may alter this client, change its transport, etc.
func (c *Client) HTTPClient() *http.Client {
	requestConfig := RequestConfig{
		Header:          http.Header{},
		QueryParameters: url.Values{},
		HTTPClient:      &c.httpClient,
	}
	for _, option := range c.Options {
		option(&requestConfig)
	}
	if requestConfig.HTTPClient == nil {
		requestConfig.HTTPClient = &c.httpClient
	}
	return requestConfig.HTTPClient
}

// WithOptions returns a new Client with the given options already set.
func (c *Client) WithOptions(options ...Option) *Client {
	// Create a new Client.
	newClient := Client{
		BaseURL: c.BaseURL,
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
// If `input` is provided, it will be serialized based on the Content-Type header.
// If `input` is `RawBytes`, then it will be sent as-is.
//
// If `output` is provided, it will be deserialized based on the response Content-Type header.
// If `output` is `RawBytes`, then the contents will be returned as-is.
func (c *Client) Do(ctx context.Context, method string, path string, input any, output any, options ...Option) error {
	requestConfig := RequestConfig{
		Header:          http.Header{},
		QueryParameters: url.Values{},
		HTTPClient:      &c.httpClient,
	}
	for _, option := range c.Options {
		option(&requestConfig)
	}
	for _, option := range options {
		option(&requestConfig)
	}
	if requestConfig.HTTPClient == nil {
		requestConfig.HTTPClient = &c.httpClient
	}

	var reader io.Reader
	if input != nil {
		contentType := requestConfig.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
			requestConfig.Header.Set("Content-Type", contentType)
		}

		if rawBytes, ok := input.(RawBytes); ok {
			reader = bytes.NewReader(rawBytes)
		} else if rawBytes, ok := input.(*RawBytes); ok {
			reader = bytes.NewReader(*rawBytes)
		} else {
			inputValue := reflect.ValueOf(input)

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
		targetURL = strings.TrimRight(c.BaseURL, "/")
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

	slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Making request: %s %s %v", method, targetURL, request.Header))
	response, err := requestConfig.HTTPClient.Do(request)
	if err != nil {
		return err
	}

	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if dumpDirectory := os.Getenv("RESTAPICLIENT_DUMP_DIRECTORY"); dumpDirectory != "" {
		filename := fmt.Sprintf("%d_%s_%s", time.Now().Unix(), method, targetURL)
		{
			reg, _ := regexp.Compile("[^a-zA-Z0-9_ -]+")
			filename = reg.ReplaceAllString(filename, "_")
		}

		dumpFile := dumpDirectory + string(filepath.Separator) + filename
		err = os.WriteFile(dumpFile, contents, 0644)
		if err != nil {
			// Oh well.
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump contents to %q: %v", dumpFile, err))
		}
	}

	if response.StatusCode >= 200 && response.StatusCode <= 299 {
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
	}

	// Figure out what error we want to return before handing it off to the custom function.
	//
	// There's a *chance* that the custom function could alter the response to change its error
	// code and then return nil, so we want to compute ours first, just in case.
	defaultErr := defaultErrorFunction(response)
	if requestConfig.ErrorFunction != nil {
		// Restore the response body so the custom error handler can parse it.
		response.Body = io.NopCloser(bytes.NewReader(contents))

		// If the user provided a custom error function, then use it.
		err = requestConfig.ErrorFunction(response)
		if err != nil {
			return err
		}
		// For whatever reason, the custom function did not actually provide an error.
		// Return the default error instead.
	}
	return defaultErr
}
