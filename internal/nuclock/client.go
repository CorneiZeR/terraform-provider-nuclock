// Package nuclock is the provider's HTTP client for the nuclock API.
//
// It knows the API's shapes and its refusals and nothing about Terraform, so
// the provider's schema code and the wire stay apart and the client can be
// tested against a fake server on its own.
package nuclock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one nuclock organisation with one API key.
type Client struct {
	base      string
	key       string
	userAgent string
	http      *http.Client
}

// New returns a client for the API at base (e.g. https://api.clock.example.com).
func New(base, key, userAgent string) *Client {
	return &Client{
		base:      strings.TrimRight(base, "/"),
		key:       key,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

// APIError is a refusal from the API, with the machine-readable key it sent.
//
// The message is the API's own sentence and is repeated whole: for
// `ability_missing` it names the ability the key was minted without, which is
// exactly what whoever reads the error needs to mint the right one.
type APIError struct {
	Status  int
	Key     string
	Message string
}

func (e *APIError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("nuclock answered %d %s: %s", e.Status, e.Key, e.Message)
	}
	return fmt.Sprintf("nuclock answered %d: %s", e.Status, e.Message)
}

// IsNotFound reports whether err is the API saying the object is not there.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// HasKey reports whether err is a refusal carrying this machine-readable key.
func HasKey(err error, key string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Key == key
}

type errorBody struct {
	Message string `json:"message"`
	Detail  string `json:"detail"`
	Errors  []struct {
		Key     string `json:"key"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
		var parsed errorBody
		if json.Unmarshal(raw, &parsed) == nil {
			if len(parsed.Errors) > 0 {
				apiErr.Key = parsed.Errors[0].Key
				apiErr.Message = parsed.Errors[0].Message
			} else if parsed.Message != "" {
				apiErr.Message = parsed.Message
			} else if parsed.Detail != "" {
				apiErr.Message = parsed.Detail
			}
		}
		return apiErr
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// page is the envelope every paged listing answers with.
type page[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

// listAll walks a paged listing to its end. A loop that reads the first page
// silently manages a prefix of the organisation.
func listAll[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	const limit = 100
	if query == nil {
		query = url.Values{}
	}
	var all []T
	for number := 1; ; number++ {
		query.Set("limit", strconv.Itoa(limit))
		query.Set("page", strconv.Itoa(number))
		var got page[T]
		if err := c.do(ctx, http.MethodGet, path, query, nil, &got); err != nil {
			return nil, err
		}
		all = append(all, got.Items...)
		if len(got.Items) < limit || len(all) >= got.Count {
			return all, nil
		}
	}
}

// one is the answer of a natural-key lookup: the row, or nil when there is none.
func one[T any](rows []T) *T {
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}
