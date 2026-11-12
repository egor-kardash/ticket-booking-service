//go:build integration

package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAppRouterEventsArePublic проверяет реальную проводку app.NewRouter:
// events смонтированы вне protected-группы и работают, а Auth middleware жив.
func TestAppRouterEventsArePublic(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	tests := []struct {
		name   string
		header string
	}{
		{name: "without authorization"},
		{name: "with broken token", header: "Bearer not-a-jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, env.server.URL+"/api/v1/events", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			resp, err := env.client.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer func() {
				_ = resp.Body.Close()
			}()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", resp.StatusCode, body)
			}

			var payload eventsListBody
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode response: %v (body %q)", err, body)
			}
			if payload.Items == nil {
				t.Errorf("items = null, want array (body %q)", body)
			}
			if len(payload.Items) != 0 {
				t.Errorf("items = %d, want 0 after reset (body %q)", len(payload.Items), body)
			}
			if strings.Contains(string(body), `"error"`) {
				t.Errorf("response contains error envelope: %q", body)
			}
		})
	}

	// Контрольная проверка: защищённый маршрут с тем же битым токеном даёт 401.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, env.server.URL+"/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer not-a-jwt")

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("protected route status = %d, want 401: Auth middleware is not active", resp.StatusCode)
	}
}
