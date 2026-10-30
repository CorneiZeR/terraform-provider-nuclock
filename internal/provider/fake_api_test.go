package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is the slice of the nuclock API the provider calls, in memory.
//
// Public CI cannot start the real server, so the resource tests run against
// this; the acceptance run against a real nuclock is `make testacc` with
// NUCLOCK_URL and a service key. It keeps the rules the provider leans on:
// closing rather than deleting, write-only secrets, and natural-key filters.
type fakeAPI struct {
	t            *testing.T
	mu           sync.Mutex
	next         int
	projects     map[string]map[string]any
	users        map[string]map[string]any
	seats        map[string]map[string]any
	integrations map[string]map[string]any
	// secrets holds what the server was sent, never what it answers with.
	secrets map[string]string
	// sentPrices records any rate a seat write carried — there must be none.
	sentPrices []string
	// refuse makes every write answer `ability_missing` for this ability.
	refuse string
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{
		t:            t,
		projects:     map[string]map[string]any{},
		users:        map[string]map[string]any{},
		seats:        map[string]map[string]any{},
		integrations: map[string]map[string]any{},
		secrets:      map[string]string{},
	}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fakeAPI) id(prefix string) string {
	f.next++
	return fmt.Sprintf("%s-%04d", prefix, f.next)
}

func refusal(w http.ResponseWriter, status int, key, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": message,
		"errors":  []map[string]any{{"key": key, "code": key, "message": message}},
	})
}

func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func pageOf(rows []map[string]any) map[string]any {
	if rows == nil {
		rows = []map[string]any{}
	}
	return map[string]any{"items": rows, "count": len(rows), "page": 1, "limit": 100}
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer test-key" {
		refusal(w, http.StatusUnauthorized, "login_required", "No key")
		return
	}
	if f.refuse != "" && r.Method != http.MethodGet {
		refusal(w, http.StatusForbidden, "ability_missing", "This credential was not granted "+f.refuse)
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	query := r.URL.Query()
	switch {
	case r.URL.Path == "/v1/project/projects" && r.Method == http.MethodGet:
		var rows []map[string]any
		for _, p := range f.projects {
			if prefix := query.Get("key_prefix"); prefix == "" || strings.EqualFold(prefix, p["key_prefix"].(string)) {
				rows = append(rows, p)
			}
		}
		answer(w, 200, pageOf(rows))
	case r.URL.Path == "/v1/project/projects" && r.Method == http.MethodPost:
		id := f.id("project")
		p := map[string]any{"id": id, "description": "", "currency": "USD", "manager_ids": []string{}, "owner_ids": []string{}, "is_active": true}
		for k, v := range body {
			p[k] = v
		}
		if p["key_prefix"] == nil || p["key_prefix"] == "" {
			p["key_prefix"] = "AUTO"
		}
		f.projects[id] = p
		answer(w, 201, p)
	case len(parts) == 4 && parts[2] == "projects":
		p, ok := f.projects[parts[3]]
		if !ok {
			refusal(w, 404, "", "Not found")
			return
		}
		for k, v := range body {
			p[k] = v
		}
		answer(w, 200, p)

	case r.URL.Path == "/v1/user" && r.Method == http.MethodGet:
		var rows []map[string]any
		for _, u := range f.users {
			if email := query.Get("email"); email == "" || strings.EqualFold(email, u["email"].(string)) {
				rows = append(rows, u)
			}
		}
		answer(w, 200, pageOf(rows))
	case r.URL.Path == "/v1/user" && r.Method == http.MethodPost:
		id := f.id("user")
		u := map[string]any{"id": id, "username": body["email"], "is_active": true, "area_ids": []string{},
			"has_2fa": false, "must_change_password": true, "password_version": 1}
		for k, v := range body {
			u[k] = v
		}
		f.users[id] = u
		answer(w, 201, u)
	case len(parts) == 3 && parts[1] == "user":
		u, ok := f.users[parts[2]]
		if !ok {
			refusal(w, 404, "", "Not found")
			return
		}
		for k, v := range body {
			u[k] = v
		}
		answer(w, 200, u)

	case r.URL.Path == "/v1/project/performers" && r.Method == http.MethodGet:
		var rows []map[string]any
		for _, s := range f.seats {
			if (query.Get("project_id") == "" || query.Get("project_id") == s["project_id"]) &&
				(query.Get("user_id") == "" || query.Get("user_id") == s["user_id"]) {
				rows = append(rows, s)
			}
		}
		answer(w, 200, pageOf(rows))
	case r.URL.Path == "/v1/project/performers" && r.Method == http.MethodPost:
		f.notePrices(body)
		id := f.id("seat")
		s := map[string]any{"id": id, "area_ids": []string{}, "closed_at": nil, "curator_id": nil,
			"inner_price": "40.00", "outer_price": "100.00"}
		for k, v := range body {
			s[k] = v
		}
		f.seats[id] = s
		answer(w, 201, s)
	case len(parts) >= 4 && parts[2] == "performers":
		s, ok := f.seats[parts[3]]
		if !ok {
			refusal(w, 404, "", "Not found")
			return
		}
		switch {
		case len(parts) == 5 && parts[4] == "close":
			if s["closed_at"] == nil {
				s["closed_at"] = "2026-10-30T12:00:00Z"
			}
		case len(parts) == 5 && parts[4] == "reopen":
			s["closed_at"] = nil
		default:
			f.notePrices(body)
			for k, v := range body {
				s[k] = v
			}
		}
		answer(w, 200, s)

	case r.URL.Path == "/v1/project/integrations" && r.Method == http.MethodPost:
		id := f.id("integration")
		i := map[string]any{"id": id, "is_active": true, "account": ""}
		for k, v := range body {
			if k == "api_token" || k == "webhook_secret" {
				f.secrets[id+"/"+k] = v.(string)
				continue
			}
			i[k] = v
		}
		f.integrations[id] = i
		answer(w, 201, f.integrationOut(id))
	case len(parts) == 4 && parts[2] == "integrations":
		i, ok := f.integrations[parts[3]]
		if !ok {
			refusal(w, 404, "", "Not found")
			return
		}
		if r.Method == http.MethodDelete {
			delete(f.integrations, parts[3])
			w.WriteHeader(204)
			return
		}
		for k, v := range body {
			if k == "api_token" || k == "webhook_secret" {
				f.secrets[parts[3]+"/"+k] = v.(string)
				continue
			}
			i[k] = v
		}
		answer(w, 200, f.integrationOut(parts[3]))
	default:
		refusal(w, 404, "", "No route "+r.Method+" "+r.URL.Path)
	}
}

func (f *fakeAPI) integrationOut(id string) map[string]any {
	out := map[string]any{}
	for k, v := range f.integrations[id] {
		out[k] = v
	}
	_, out["has_api_token"] = f.secrets[id+"/api_token"]
	_, out["has_webhook_secret"] = f.secrets[id+"/webhook_secret"]
	return out
}

func (f *fakeAPI) notePrices(body map[string]any) {
	for _, field := range []string{"inner_price", "outer_price", "tax_rate"} {
		if _, sent := body[field]; sent {
			f.sentPrices = append(f.sentPrices, field)
		}
	}
}
