package restapiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tekkamanendless/httperror"
)

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("Error %d: %s", e.Code, e.Message)
}

func TestClient(t *testing.T) {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	testUsername := "testuser"
	testPassword := "testpassword"
	type TestStruct struct {
		Header http.Header `json:"header"`
		Method string      `json:"method"`
		Path   string      `json:"path"`
		Query  url.Values  `json:"query"`
		Body   []byte      `json:"body"`
		Code   int         `json:"code"`
	}
	writeResponse := func(code int, w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			body = nil
		}

		testStruct := TestStruct{
			Header: r.Header,
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Body:   body,
			Code:   code,
		}
		content, err := json.Marshal(testStruct)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write(content)
	}
	// Check the request method.
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/public/text":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/public/json":
			switch r.Method {
			case http.MethodDelete, http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodPut:
				writeResponse(http.StatusOK, w, r)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/public/custom":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "x-custom/something")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/public/error":
			switch r.Method {
			case http.MethodDelete, http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodPut:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"code":1234,"message":"This is a custom error message."}`))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/public/redirect":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Location", "/public/text")
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/public/temporary-error":
			switch r.Method {
			case http.MethodDelete, http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodPut:
				parsedURL, err := url.Parse(r.URL.String())
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal Server Error"))
					return
				}

				if stringValue := parsedURL.Query().Get("fail_until"); stringValue != "" {
					failUntil, err := time.Parse(time.RFC3339, stringValue)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"code":1,"message":"Could not parse fail_until."}`))
						return
					}

					failWith := http.StatusTooManyRequests
					if stringValue := parsedURL.Query().Get("fail_with"); stringValue != "" {
						intValue, err := strconv.Atoi(stringValue)
						if err != nil {
							w.WriteHeader(http.StatusBadRequest)
							w.Write([]byte(`{"code":1,"message":"Could not parse fail_with."}`))
							return
						}
						failWith = intValue
					}

					if time.Now().Before(failUntil) {
						w.WriteHeader(failWith)
						w.Write([]byte(http.StatusText(failWith)))
						return
					}
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		case "/private/json":
			username, password, ok := r.BasicAuth()
			if !ok {
				writeResponse(http.StatusUnauthorized, w, r)
				return
			}
			if username != testUsername || password != testPassword {
				writeResponse(http.StatusUnauthorized, w, r)
				return
			}

			switch r.Method {
			case http.MethodDelete, http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodPut:
				writeResponse(http.StatusOK, w, r)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	})
	testServer := httptest.NewServer(testHandler)
	defer testServer.Close()

	t.Run("Unauthenticated", func(t *testing.T) {
		client := New(OptionBaseURL(testServer.URL))
		assert.Equal(t, client.Config().BaseURL, testServer.URL)

		t.Run("Bogus path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/bogus", nil, nil)
			require.ErrorIs(t, err, httperror.ErrStatusNotFound)
		})
		t.Run("Public path", func(t *testing.T) {
			t.Run("Text", func(t *testing.T) {
				t.Run("No output", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, nil)
					require.NoError(t, err)
				})
				t.Run("Bad raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
				t.Run("Good raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "Success", string(output))
				})
				t.Run("Good string output", func(t *testing.T) {
					var output string
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "Success", output)
				})
				t.Run("Good byte output", func(t *testing.T) {
					var output []byte
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "Success", string(output))
				})
				t.Run("Bad int output", func(t *testing.T) {
					var output int
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, &output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
			})
			t.Run("Custom", func(t *testing.T) {
				t.Run("Unhandled custom type", func(t *testing.T) {
					var output int
					err := client.Do(ctx, http.MethodGet, "/public/custom", nil, &output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
			})
			t.Run("Error", func(t *testing.T) {
				t.Run("Default error handler", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/error", nil, nil)
					require.ErrorIs(t, err, httperror.ErrStatusBadRequest)
				})
				t.Run("Custom error handler that returns nil just uses the default one", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/error", nil, nil, OptionErrorHandler(func(response *http.Response) error {
						return nil
					}))
					require.ErrorIs(t, err, httperror.ErrStatusBadRequest)
				})
				t.Run("Custom error handler that returns a custom error", func(t *testing.T) {
					var errCustom = fmt.Errorf("my custom error")
					err := client.Do(ctx, http.MethodGet, "/public/error", nil, nil, OptionErrorHandler(func(response *http.Response) error {
						return fmt.Errorf("%w: testing", errCustom)
					}))
					require.NotErrorIs(t, err, httperror.ErrStatusBadRequest)
					require.ErrorIs(t, err, errCustom)
					require.True(t, strings.HasSuffix(err.Error(), "testing"))
				})
				t.Run("Custom error handler can parse body", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/error", nil, nil, OptionErrorHandler(func(response *http.Response) error {
						contents, err := io.ReadAll(response.Body)
						if err != nil {
							return nil // Oh well; use the default error.
						}
						var errorResponse ErrorResponse
						err = json.Unmarshal(contents, &errorResponse)
						if err != nil {
							return nil // Oh well; use the default error.
						}
						return &errorResponse
					}))
					require.NotErrorIs(t, err, httperror.ErrStatusBadRequest)
					var errorResponse *ErrorResponse
					if assert.ErrorAs(t, err, &errorResponse) {
						assert.Equal(t, 1234, errorResponse.Code)
						assert.Equal(t, "This is a custom error message.", errorResponse.Message)
					}
				})
				t.Run("Custom error handler can parse body and return httperror", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/error", nil, nil, OptionErrorHandler(func(response *http.Response) error {
						contents, err := io.ReadAll(response.Body)
						if err != nil {
							return nil // Oh well; use the default error.
						}
						var errorResponse ErrorResponse
						err = json.Unmarshal(contents, &errorResponse)
						if err != nil {
							return nil // Oh well; use the default error.
						}
						return fmt.Errorf("%w: %w", httperror.ErrorFromStatus(response.StatusCode), &errorResponse)
					}))
					require.ErrorIs(t, err, httperror.ErrStatusBadRequest)
					var errorResponse *ErrorResponse
					if assert.ErrorAs(t, err, &errorResponse) {
						assert.Equal(t, 1234, errorResponse.Code)
						assert.Equal(t, "This is a custom error message.", errorResponse.Message)
					}
				})
				t.Run("Backoff", func(t *testing.T) {
					t.Run("No backoff", func(t *testing.T) {
						err := client.Do(ctx, http.MethodGet, "/public/temporary-error?fail_until="+time.Now().Add(5*time.Second).Format(time.RFC3339)+"&fail_with="+strconv.Itoa(http.StatusTooManyRequests), nil, nil)
						require.ErrorIs(t, err, httperror.ErrStatusTooManyRequests)
					})
					t.Run("Default backoff function", func(t *testing.T) {
						err := client.Do(ctx, http.MethodGet, "/public/temporary-error?fail_until="+time.Now().Add(5*time.Second).Format(time.RFC3339)+"&fail_with="+strconv.Itoa(http.StatusTooManyRequests), nil, nil,
							OptionBackOffIsPermanentErrorFunction(DefaultBackOffIsPermanentErrorFunction),
						)
						require.NoError(t, err)
					})
					t.Run("Default backoff function that doesn't apply", func(t *testing.T) {
						err := client.Do(ctx, http.MethodGet, "/public/temporary-error?fail_until="+time.Now().Add(5*time.Second).Format(time.RFC3339)+"&fail_with="+strconv.Itoa(http.StatusNotFound), nil, nil,
							OptionBackOffIsPermanentErrorFunction(DefaultBackOffIsPermanentErrorFunction),
						)
						require.ErrorIs(t, err, httperror.ErrStatusNotFound)
					})
					t.Run("Default backoff function without enough time", func(t *testing.T) {
						err := client.Do(ctx, http.MethodGet, "/public/temporary-error?fail_until="+time.Now().Add(5*time.Second).Format(time.RFC3339)+"&fail_with="+strconv.Itoa(http.StatusTooManyRequests), nil, nil,
							OptionBackOffIsPermanentErrorFunction(DefaultBackOffIsPermanentErrorFunction),
							OptionBackOffOptions(backoff.WithMaxElapsedTime(1*time.Second)),
						)
						require.ErrorIs(t, err, httperror.ErrStatusTooManyRequests)
					})
					t.Run("Default backoff function without enough tries", func(t *testing.T) {
						err := client.Do(ctx, http.MethodGet, "/public/temporary-error?fail_until="+time.Now().Add(5*time.Second).Format(time.RFC3339)+"&fail_with="+strconv.Itoa(http.StatusTooManyRequests), nil, nil,
							OptionBackOffIsPermanentErrorFunction(DefaultBackOffIsPermanentErrorFunction),
							OptionBackOffOptions(backoff.WithMaxTries(1)),
						)
						require.ErrorIs(t, err, httperror.ErrStatusTooManyRequests)
					})
				})
			})
			t.Run("JSON", func(t *testing.T) {
				t.Run("No output", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, nil)
					require.NoError(t, err)
				})
				t.Run("Bad raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
				t.Run("Good raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, &output)
					require.NoError(t, err)
					require.True(t, strings.HasPrefix(string(output), "{"))
				})
				t.Run("Bad output type", func(t *testing.T) {
					var output string
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, &output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
				t.Run("Good output type", func(t *testing.T) {
					var output TestStruct
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodGet, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
				})
				t.Run("With header", func(t *testing.T) {
					var output TestStruct
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, &output, OptionHeader("X-Test-Header", "TestValue"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodGet, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, "TestValue", output.Header.Get("X-Test-Header"))
				})
				t.Run("With raw query", func(t *testing.T) {
					var output TestStruct
					err := client.Do(ctx, http.MethodGet, "/public/json?param1=value1", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodGet, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, "value1", output.Query.Get("param1"))
				})
				t.Run("With query", func(t *testing.T) {
					var output TestStruct
					err := client.Do(ctx, http.MethodGet, "/public/json", nil, &output, OptionQuery("param1", "value1"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodGet, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, "value1", output.Query.Get("param1"))
				})
				t.Run("With raw query and option query", func(t *testing.T) {
					var output TestStruct
					err := client.Do(ctx, http.MethodGet, "/public/json?param1=value1", nil, &output, OptionQuery("param2", "value2"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodGet, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, "value1", output.Query.Get("param1"))
					require.Equal(t, "value2", output.Query.Get("param2"))
				})
				t.Run("Post implied JSON", func(t *testing.T) {
					input := "test"
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `"test"`, string(output.Body))
				})
				t.Run("Post raw JSON", func(t *testing.T) {
					input := RawBytes(`{"key1":"value1"}`)
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `{"key1":"value1"}`, string(output.Body))
				})
				t.Run("Post raw JSON pointer", func(t *testing.T) {
					input := RawBytes(`{"key1":"value1"}`)
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", &input, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `{"key1":"value1"}`, string(output.Body))
				})
				t.Run("Post actual JSON", func(t *testing.T) {
					input := map[string]string{"key1": "value1"}
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output)
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `{"key1":"value1"}`, string(output.Body))
				})
				t.Run("Post bad JSON", func(t *testing.T) {
					input := map[string]any{"key1": func() {}}
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidInput)
				})
				t.Run("Post basic form data", func(t *testing.T) {
					input := "key1=value1"
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "application/x-www-form-urlencoded"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `key1=value1`, string(output.Body))
				})
				t.Run("Post multipart form data", func(t *testing.T) {
					input := "key1=value1"
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `key1=value1`, string(output.Body))
				})
				t.Run("Post plain text", func(t *testing.T) {
					input := "key1=value1"
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "text/plain"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `key1=value1`, string(output.Body))
				})
				t.Run("Post any text", func(t *testing.T) {
					input := "key1=value1"
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "text/whatever-who-cares"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `key1=value1`, string(output.Body))
				})
				t.Run("Post any text as bytes", func(t *testing.T) {
					input := []byte("key1=value1")
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "text/whatever-who-cares"))
					require.NoError(t, err)
					require.Equal(t, "/public/json", output.Path)
					require.Equal(t, http.MethodPost, output.Method)
					require.Equal(t, http.StatusOK, output.Code)
					require.Equal(t, `key1=value1`, string(output.Body))
				})

				t.Run("Post bad content type", func(t *testing.T) {
					input := []byte("key1=value1")
					var output TestStruct
					err := client.Do(ctx, http.MethodPost, "/public/json", input, &output, OptionHeader("Content-Type", "custom/whatever-who-cares"))
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidInput)
				})
			})
		})
		t.Run("Private path", func(t *testing.T) {
			t.Run("Text", func(t *testing.T) {
				t.Run("No output", func(t *testing.T) {
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, nil)
					require.NoError(t, err)
				})
				t.Run("Bad raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, output)
					require.ErrorIs(t, err, ErrClient)
					require.ErrorIs(t, err, ErrClientInvalidOutput)
				})
				t.Run("Good raw output", func(t *testing.T) {
					var output RawBytes
					err := client.Do(ctx, http.MethodGet, "/public/text", nil, &output)
					require.NoError(t, err)
					require.Equal(t, "Success", string(output))
				})
			})
		})
		t.Run("Full path", func(t *testing.T) {
			t.Run("Good", func(t *testing.T) {
				err := client.Do(ctx, http.MethodGet, testServer.URL+"/public/text", nil, nil)
				require.NoError(t, err)
			})
			t.Run("Bogus", func(t *testing.T) {
				err := client.Do(ctx, http.MethodGet, "bogus://bogus", nil, nil)
				require.Error(t, err)
				assert.NotErrorIs(t, err, ErrClient)
			})
		})
		t.Run("Private path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/private/json", nil, nil)
			require.NotErrorIs(t, err, ErrClient)
			require.ErrorIs(t, err, httperror.ErrStatusUnauthorized)
		})
		t.Run("Redirect", func(t *testing.T) {
			t.Run("Default client", func(t *testing.T) {
				var output RawBytes
				err := client.Do(ctx, http.MethodGet, "/public/redirect", nil, &output)
				require.NoError(t, err)
				require.Equal(t, "Success", string(output))
			})
			t.Run("Custom client", func(t *testing.T) {
				var output RawBytes
				err := client.Do(ctx, http.MethodGet, "/public/redirect", nil, &output,
					OptionHTTPClient(&http.Client{
						CheckRedirect: func(req *http.Request, via []*http.Request) error {
							return http.ErrUseLastResponse
						},
					}),
				)
				require.NoError(t, err)
				require.Equal(t, "", string(output))
			})
			t.Run("Custom client with error", func(t *testing.T) {
				customError := fmt.Errorf("redirect not allowed")

				var output RawBytes
				err := client.Do(ctx, http.MethodGet, "/public/redirect", nil, &output,
					OptionHTTPClient(&http.Client{
						CheckRedirect: func(req *http.Request, via []*http.Request) error {
							return customError
						},
					}),
				)
				require.ErrorIs(t, err, customError)
				require.Equal(t, "", string(output))
			})
		})
	})
	t.Run("Authenticated", func(t *testing.T) {
		client := New(OptionBaseURL(testServer.URL), OptionBasicAuth(testUsername, testPassword))
		assert.Equal(t, client.Config().BaseURL, testServer.URL)

		t.Run("Bogus path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/bogus", nil, nil)
			require.ErrorIs(t, err, httperror.ErrStatusNotFound)
		})
		t.Run("Public path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/public/json", nil, nil)
			require.NoError(t, err)
		})
		t.Run("Private path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/private/json", nil, nil)
			require.NoError(t, err)
		})
	})
	t.Run("Authenticated on the request itself", func(t *testing.T) {
		client := New(OptionBaseURL(testServer.URL))
		assert.Equal(t, client.Config().BaseURL, testServer.URL)

		t.Run("Private path", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/private/json", nil, nil, OptionBasicAuth(testUsername, testPassword))
			require.NoError(t, err)
		})
	})
	t.Run("WithOptions", func(t *testing.T) {
		client := New(OptionBaseURL(testServer.URL))
		assert.Equal(t, client.Config().BaseURL, testServer.URL)

		t.Run("Initial", func(t *testing.T) {
			err := client.Do(ctx, http.MethodGet, "/public/json", nil, nil)
			require.NoError(t, err)
		})
		t.Run("Three levels", func(t *testing.T) {
			newClient := client.WithOptions(
				OptionHeader("header-1", "value-1"),
				OptionHeader("header-2", "value-2"),
			).WithOptions(
				OptionHeader("header-3", "value-3"),
				OptionHeader("header-4", "value-4"),
			).WithOptions(
				OptionHeader("header-1", "value-5"),
			)

			var output TestStruct
			err := newClient.Do(ctx, http.MethodGet, "/public/json", nil, &output)
			require.NoError(t, err)

			assert.Equal(t, "value-5", output.Header.Get("header-1"))
			assert.Equal(t, "value-2", output.Header.Get("header-2"))
			assert.Equal(t, "value-3", output.Header.Get("header-3"))
			assert.Equal(t, "value-4", output.Header.Get("header-4"))
		})
		t.Run("Three levels with request override", func(t *testing.T) {
			newClient := client.WithOptions(
				OptionHeader("header-1", "value-1"),
				OptionHeader("header-2", "value-2"),
			).WithOptions(
				OptionHeader("header-3", "value-3"),
				OptionHeader("header-4", "value-4"),
			).WithOptions(
				OptionHeader("header-1", "value-5"),
			)

			var output TestStruct
			err := newClient.Do(ctx, http.MethodGet, "/public/json", nil, &output, OptionHeader("header-2", "value-6"))
			require.NoError(t, err)

			assert.Equal(t, "value-5", output.Header.Get("header-1"))
			assert.Equal(t, "value-6", output.Header.Get("header-2"))
			assert.Equal(t, "value-3", output.Header.Get("header-3"))
			assert.Equal(t, "value-4", output.Header.Get("header-4"))
		})
	})
	t.Run("Dump request and response", func(t *testing.T) {
		buildFileRequestMap := func(dumpDirectory string) map[string]map[string]string {
			files, err := filepath.Glob(dumpDirectory + "/*.*")
			require.NoError(t, err)

			fileRequestMap := map[string]map[string]string{}
			for _, file := range files {
				fileName := filepath.Base(file)
				parts := strings.SplitN(fileName, ".", 2)
				require.Len(t, parts, 2)

				requestName := parts[0]
				fileSuffix := parts[1]
				if _, ok := fileRequestMap[requestName]; !ok {
					fileRequestMap[requestName] = map[string]string{}
				}
				fileRequestMap[requestName][fileSuffix] = file
			}
			return fileRequestMap
		}

		client := New(OptionBaseURL(testServer.URL))
		assert.Equal(t, client.Config().BaseURL, testServer.URL)

		t.Run("GET", func(t *testing.T) {
			t.Run("Short URL", func(t *testing.T) {
				t.Run("Directory", func(t *testing.T) {
					dumpDirectory := t.TempDir()
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					err := client.Do(ctx, http.MethodGet, "/public/json", nil, nil)
					require.NoError(t, err)

					fileRequestMap := buildFileRequestMap(dumpDirectory)
					require.Len(t, fileRequestMap, 1)
					for _, fileMap := range fileRequestMap {
						require.Len(t, fileMap, 3)
						require.Contains(t, fileMap, "request.head")
						require.Contains(t, fileMap, "response.head")
						require.Contains(t, fileMap, "response.body")
					}
				})
				t.Run("Stderr", func(t *testing.T) {
					dumpDirectory := "-"
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					err := client.Do(ctx, http.MethodGet, "/public/json", nil, nil)
					require.NoError(t, err)
				})
			})
			t.Run("Long URL", func(t *testing.T) {
				t.Run("Directory", func(t *testing.T) {
					dumpDirectory := t.TempDir()
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					queryParams := url.Values{}
					queryParams.Add("param1", strings.Repeat("x", 500))
					assert.True(t, len(queryParams.Encode()) > 200)
					err := client.Do(ctx, http.MethodGet, "/public/json?"+queryParams.Encode(), nil, nil)
					require.NoError(t, err)

					fileRequestMap := buildFileRequestMap(dumpDirectory)
					require.Len(t, fileRequestMap, 1)
					for _, fileMap := range fileRequestMap {
						require.Len(t, fileMap, 3)
						require.Contains(t, fileMap, "request.head")
						require.Contains(t, fileMap, "response.head")
						require.Contains(t, fileMap, "response.body")
					}
				})
				t.Run("Stderr", func(t *testing.T) {
					dumpDirectory := "-"
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					queryParams := url.Values{}
					queryParams.Add("param1", strings.Repeat("x", 500))
					assert.True(t, len(queryParams.Encode()) > 200)
					err := client.Do(ctx, http.MethodGet, "/public/json?"+queryParams.Encode(), nil, nil)
					require.NoError(t, err)
				})
			})
		})
		t.Run("POST", func(t *testing.T) {
			t.Run("No body", func(t *testing.T) {
				t.Run("Directory", func(t *testing.T) {
					dumpDirectory := t.TempDir()
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					err := client.Do(ctx, http.MethodPost, "/public/json", nil, nil)
					require.NoError(t, err)

					fileRequestMap := buildFileRequestMap(dumpDirectory)
					require.Len(t, fileRequestMap, 1)
					for _, fileMap := range fileRequestMap {
						require.Len(t, fileMap, 3)
						require.Contains(t, fileMap, "request.head")
						require.Contains(t, fileMap, "response.head")
						require.Contains(t, fileMap, "response.body")
					}
				})
				t.Run("Stderr", func(t *testing.T) {
					dumpDirectory := "-"
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					err := client.Do(ctx, http.MethodPost, "/public/json", nil, nil)
					require.NoError(t, err)
				})
			})
			t.Run("Body", func(t *testing.T) {
				t.Run("Directory", func(t *testing.T) {
					dumpDirectory := t.TempDir()
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					input := "test"
					err := client.Do(ctx, http.MethodPost, "/public/json", input, nil)
					require.NoError(t, err)

					fileRequestMap := buildFileRequestMap(dumpDirectory)
					require.Len(t, fileRequestMap, 1)
					for _, fileMap := range fileRequestMap {
						require.Len(t, fileMap, 4)
						require.Contains(t, fileMap, "request.head")
						require.Contains(t, fileMap, "request.body")
						require.Contains(t, fileMap, "response.head")
						require.Contains(t, fileMap, "response.body")
					}
				})
				t.Run("Stderr", func(t *testing.T) {
					dumpDirectory := "-"
					t.Setenv("RESTAPICLIENT_DUMP_DIRECTORY", dumpDirectory)

					input := "test"
					err := client.Do(ctx, http.MethodPost, "/public/json", input, nil)
					require.NoError(t, err)
				})
			})
		})
	})
}
