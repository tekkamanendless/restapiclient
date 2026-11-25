package restapiclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// getDumpFilename returns the base filename for the request.
//
// This will be truncated to 200 characters.
func getDumpFilename(request *http.Request) string {
	filename := fmt.Sprintf("%d_%s_%s", time.Now().Unix(), request.Method, request.URL.String())
	{
		reg, _ := regexp.Compile("[^a-zA-Z0-9_ -]+")
		filename = reg.ReplaceAllString(filename, "_")

		if len(filename) > 200 {
			filename = filename[:200]
		}
	}
	return filename
}

// dumpRequestIfEnabled dumps the request to the dump directory if enabled.
//
// ctx is used for logging.
func dumpRequestIfEnabled(ctx context.Context, request *http.Request) {
	dumpDirectory := os.Getenv("RESTAPICLIENT_DUMP_DIRECTORY")
	if dumpDirectory == "" {
		return
	}

	filename := getDumpFilename(request)

	{
		dumpFile := dumpDirectory + string(filepath.Separator) + filename + ".request.head"
		dumpContents, err := httputil.DumpRequest(request, false)
		if err != nil {
			// Oh well.
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump request to %q: %v", dumpFile, err))
		}

		if dumpDirectory == "-" {
			fmt.Fprintf(os.Stderr, "%s\n", string(dumpContents))
		} else {
			err = os.WriteFile(dumpFile, dumpContents, 0644)
			if err != nil {
				// Oh well.
				slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump request contents to %q: %v", dumpFile, err))
			}
		}
	}
	if request.Body != nil {
		requestBody, err := io.ReadAll(request.Body)
		if err != nil {
			// Oh well.
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not read request body: %v", err))
		} else {
			// Restore the request body so the next request can use it.
			request.Body = io.NopCloser(bytes.NewReader(requestBody))

			if dumpDirectory == "-" {
				fmt.Fprintf(os.Stderr, "%s\n", string(requestBody))
			} else {
				dumpFile := dumpDirectory + string(filepath.Separator) + filename + ".request.body"
				err = os.WriteFile(dumpFile, requestBody, 0644)
				if err != nil {
					// Oh well.
					slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump request contents to %q: %v", dumpFile, err))
				}
			}
		}
	}
}

func dumpResponseIfEnabled(ctx context.Context, response *http.Response) {
	dumpDirectory := os.Getenv("RESTAPICLIENT_DUMP_DIRECTORY")
	if dumpDirectory == "" {
		return
	}

	filename := getDumpFilename(response.Request)

	{
		dumpFile := dumpDirectory + string(filepath.Separator) + filename + ".response.head"
		dumpContents, err := httputil.DumpResponse(response, false)
		if err != nil {
			// Oh well.
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump response to %q: %v", dumpFile, err))
		}

		if dumpDirectory == "-" {
			fmt.Fprintf(os.Stderr, "%s\n", string(dumpContents))
		} else {
			err = os.WriteFile(dumpFile, dumpContents, 0644)
			if err != nil {
				// Oh well.
				slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump response contents to %q: %v", dumpFile, err))
			}
		}
	}

	if response.Body != nil {
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			// Oh well.
			slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not read response body: %v", err))
		} else {
			// Restore the response body so the next response can use it.
			response.Body = io.NopCloser(bytes.NewReader(responseBody))

			if dumpDirectory == "-" {
				fmt.Fprintf(os.Stderr, "%s\n", string(responseBody))
			} else {
				dumpFile := dumpDirectory + string(filepath.Separator) + filename + ".response.body"
				err = os.WriteFile(dumpFile, responseBody, 0644)
				if err != nil {
					// Oh well.
					slog.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Could not dump response contents to %q: %v", dumpFile, err))
				}
			}
		}
	}
}
