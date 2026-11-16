// Package ghtest is a fake GitHub for tests: a client whose requests never
// leave the process, answered by handlers keyed on what the request asks for.
package ghtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/chrismatix/gh-sift/internal/gh"
)

// Request is one call the client made, kept so tests can assert on writes.
type Request struct {
	Method    string
	Path      string
	Query     string
	Variables map[string]any
	Body      string
}

// Handler answers a request: a string is sent as the raw body, anything
// else as JSON. A GraphQL answer is wrapped in {"data": ...} for you.
type Handler func(request Request) (status int, body any)

type route struct {
	graphql bool
	method  string
	match   string
	handler Handler
}

type Server struct {
	mutex    sync.Mutex
	routes   []route
	Requests []Request
}

// GraphQL answers queries and mutations whose text contains match.
// Later routes win, so a test can override a default.
func (server *Server) GraphQL(match string, handler Handler) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.routes = append(server.routes, route{graphql: true, match: match, handler: handler})
}

// REST answers method requests whose path, query string included, starts with prefix.
func (server *Server) REST(method, prefix string, handler Handler) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.routes = append(server.routes, route{method: method, match: prefix, handler: handler})
}

// Reply is a handler that always gives the same answer.
func Reply(status int, body any) Handler {
	return func(Request) (int, any) { return status, body }
}

// Find returns the recorded requests whose GraphQL text or REST path contains text.
func (server *Server) Find(text string) []Request {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	var found []Request
	for _, request := range server.Requests {
		if strings.Contains(request.Query, text) || strings.Contains(request.Path, text) {
			found = append(found, request)
		}
	}
	return found
}

func (server *Server) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	request := Request{Method: httpRequest.Method, Path: strings.TrimPrefix(httpRequest.URL.RequestURI(), "/")}
	if httpRequest.Body != nil {
		body, _ := io.ReadAll(httpRequest.Body)
		request.Body = string(body)
	}
	isGraphQL := request.Path == "graphql"
	if isGraphQL {
		var payload struct {
			Query     string
			Variables map[string]any
		}
		_ = json.Unmarshal([]byte(request.Body), &payload)
		request.Query, request.Variables = payload.Query, payload.Variables
	}

	server.mutex.Lock()
	server.Requests = append(server.Requests, request)
	var handler Handler
	for index := len(server.routes) - 1; index >= 0; index-- {
		candidate := server.routes[index]
		if isGraphQL && candidate.graphql && strings.Contains(request.Query, candidate.match) ||
			!isGraphQL && !candidate.graphql && candidate.method == request.Method && strings.HasPrefix(request.Path, candidate.match) {
			handler = candidate.handler
			break
		}
	}
	server.mutex.Unlock()

	recorder := httptest.NewRecorder()
	if handler == nil {
		recorder.WriteHeader(http.StatusNotFound)
		recorder.WriteString(`{"message": "Not Found"}`)
		return response(recorder, httpRequest), nil
	}
	status, body := handler(request)
	if isGraphQL && status == http.StatusOK {
		if _, isText := body.(string); !isText {
			body = map[string]any{"data": body}
		}
	}
	recorder.WriteHeader(status)
	if text, isText := body.(string); isText {
		recorder.WriteString(text)
	} else {
		recorder.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(recorder).Encode(body)
	}
	return response(recorder, httpRequest), nil
}

func response(recorder *httptest.ResponseRecorder, request *http.Request) *http.Response {
	result := recorder.Result()
	result.Request = request
	return result
}

// New returns a fake GitHub and a client wired to it.
func New(t testing.TB) (*Server, *gh.Client) {
	t.Helper()
	server := &Server{}
	client, err := gh.NewClientWithOptions(api.ClientOptions{Host: "github.com", AuthToken: "test", Transport: server, LogIgnoreEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	return server, client
}
