package testsupport

import (
	"context"
	"net/http"
)

type DummyHttpClient struct {
	Response      *http.Response
	Error         error
	AssertRequest func(req *http.Request)
}

type DummyHttpDoer struct {
	*HttpClientDoer
	Client *DummyHttpClient
}

func (c *DummyHttpClient) Do(req *http.Request) (*http.Response, error) {
	if c.AssertRequest != nil {
		c.AssertRequest(req)
	}
	return c.Response, c.Error
}

func NewDummyHttpDoer() *DummyHttpDoer {
	client := &DummyHttpClient{}
	doer := &HttpClientDoer{HttpClient: client}
	return &DummyHttpDoer{HttpClientDoer: doer, Client: client}
}
