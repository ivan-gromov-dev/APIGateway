package testenv

import (
	"io"
	"net/http"
)

// GET sends a GET request to the gateway's public server.
func (e *Environment) GET(path string) *Response {
	e.t.Helper()
	return e.Do(http.MethodGet, path, nil)
}

// POST sends a POST request to the gateway's public server.
func (e *Environment) POST(path string) *Response {
	e.t.Helper()
	return e.Do(http.MethodPost, path, nil)
}

// Do sends a request to the gateway's public server.
func (e *Environment) Do(method, path string, headers http.Header) *Response {
	e.t.Helper()
	return e.request(method, e.publicBaseURL+path, headers)
}

// AdminGET sends a GET request to the administrative server.
func (e *Environment) AdminGET(path string) *Response {
	e.t.Helper()
	return e.request(http.MethodGet, e.adminBaseURL+path, nil)
}

func (e *Environment) request(method, url string, headers http.Header) *Response {
	e.t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	request.Header = headers.Clone()
	response, err := e.client.Do(request)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return newResponse(e.t, response.StatusCode, response.Header.Clone(), body)
}
