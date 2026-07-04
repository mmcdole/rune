package session

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/rune/event"
	"github.com/mmcdole/rune/lua"
)

const (
	httpDefaultTimeout = 30 * time.Second
	// Responses larger than this fail the request rather than being
	// silently truncated. Scripts consume bodies as Lua strings; a
	// multi-megabyte page is almost certainly a mistake.
	httpMaxBodyBytes = 5 << 20
)

// HTTPRequest implements lua.Host. The request runs in its own
// goroutine and the outcome comes back through the event loop as an
// AsyncResult, so the Lua callback executes on the session goroutine
// under the watchdog like every other callback. Like Connect's dial
// goroutine, the channel send may block briefly; the session loop
// keeps draining while requests are in flight.
func (s *Session) HTTPRequest(id int, req lua.HTTPRequest) {
	go func() {
		resp, err := doHTTPRequest(req)
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.events <- event.Event{
			Type: event.AsyncResult,
			Payload: event.Callback(func() {
				s.engine.OnHTTPResult(id, resp, errMsg)
			}),
		}
	}()
}

func doHTTPRequest(req lua.HTTPRequest) (*lua.HTTPResponse, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = httpDefaultTimeout
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	httpReq, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		return nil, err
	}
	if httpReq.URL.Scheme != "http" && httpReq.URL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q (http or https only)", httpReq.URL.Scheme)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: timeout}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(httpResp.Body, httpMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > httpMaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds the %d MB cap", httpMaxBodyBytes>>20)
	}

	headers := make(map[string]string, len(httpResp.Header))
	for k := range httpResp.Header {
		headers[k] = httpResp.Header.Get(k)
	}
	return &lua.HTTPResponse{
		Status:  httpResp.StatusCode,
		Body:    string(data),
		Headers: headers,
	}, nil
}
