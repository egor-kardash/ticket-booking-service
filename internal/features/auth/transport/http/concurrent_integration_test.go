//go:build integration

package http_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// tryRegister не трогает testing.T: его можно безопасно вызывать из горутин.
func (e *integrationEnv) tryRegister(email, password string) (response, error) {
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)

	resp, err := e.client.Post(
		e.server.URL+"/api/v1/auth/register",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return response{}, fmt.Errorf("post register: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, fmt.Errorf("read response: %w", err)
	}

	return response{status: resp.StatusCode, header: resp.Header.Clone(), body: data}, nil
}

type registerAttempt struct {
	resp response
	err  error
}

// TestIntegrationConcurrentRegister проверяет, что уникальность email держится
// на индексе БД, а не на проверке перед вставкой: 4 одновременных запроса
// с одним email дают ровно один 201 и три 409.
func TestIntegrationConcurrentRegister(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)

	const (
		workers = 4
		email   = "race@example.com"
	)

	attempts := make([]registerAttempt, workers)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			// Барьер вместо time.Sleep: все запросы уходят одновременно.
			<-start
			attempts[i].resp, attempts[i].err = env.tryRegister(email, integrationPassword)
		}(i)
	}

	close(start)
	wg.Wait()

	var created, taken int
	for i, attempt := range attempts {
		if attempt.err != nil {
			t.Fatalf("worker %d: %v", i, attempt.err)
		}

		switch attempt.resp.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			taken++

			var envelope errorEnvelope
			decodeJSONResponse(t, attempt.resp, &envelope)
			if envelope.Error.Code != "email_taken" {
				t.Errorf("worker %d: error code = %q, want email_taken", i, envelope.Error.Code)
			}
		default:
			t.Errorf("worker %d: status = %d, want 201 or 409 (body %q)",
				i, attempt.resp.status, attempt.resp.body)
		}
	}

	if created != 1 {
		t.Errorf("201 responses = %d, want exactly 1", created)
	}
	if taken != workers-1 {
		t.Errorf("409 responses = %d, want %d", taken, workers-1)
	}
	if count := countUsers(t); count != 1 {
		t.Errorf("users count = %d, want 1", count)
	}
}
