package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

type fakeTokenVerifier struct {
	userID int64
	err    error

	token string
	calls int
}

func (f *fakeTokenVerifier) VerifyToken(tokenStr string) (int64, error) {
	f.calls++
	f.token = tokenStr

	if f.err != nil {
		return 0, f.err
	}

	return f.userID, nil
}

// nextHandler фиксирует, дошёл ли запрос до защищённого хендлера
// и что лежало в контексте.
type nextHandler struct {
	calls  int
	userID int64
	ok     bool
}

func (h *nextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls++
	h.userID, h.ok = httpserver.UserIDFrom(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func serveAuth(t *testing.T, verifier httpserver.TokenVerifier, headers ...string) (*httptest.ResponseRecorder, *nextHandler) {
	t.Helper()

	next := &nextHandler{}
	handler := httpserver.Auth(verifier)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, header := range headers {
		req.Header.Add("Authorization", header)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec, next
}

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func decodeErrorEnvelope(t *testing.T, body []byte) errorEnvelope {
	t.Helper()

	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("response is not a JSON error envelope: %v (body %q)", err, body)
	}

	return envelope
}

func TestAuthRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name          string
		headers       []string
		verifierErr   error
		wantChallenge string
		wantVerify    int
	}{
		{
			name:          "missing authorization header",
			wantChallenge: "Bearer",
		},
		{
			name:          "two authorization headers",
			headers:       []string{"Bearer one", "Bearer two"},
			wantChallenge: "Bearer",
		},
		{
			name:          "basic scheme",
			headers:       []string{"Basic dXNlcjpwYXNz"},
			wantChallenge: "Bearer",
		},
		{
			name:          "bearer without token",
			headers:       []string{"Bearer"},
			wantChallenge: "Bearer",
		},
		{
			name:          "bearer with extra parts",
			headers:       []string{"Bearer one two"},
			wantChallenge: "Bearer",
		},
		{
			// Единственный случай, где верификатор вызывается: заголовок
			// синтаксически верный, но токен отвергнут.
			name:          "invalid token",
			headers:       []string{"Bearer bad-token"},
			verifierErr:   errors.New("bad signature"),
			wantChallenge: `Bearer error="invalid_token"`,
			wantVerify:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeTokenVerifier{err: tt.verifierErr}
			rec, next := serveAuth(t, verifier, tt.headers...)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tt.wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tt.wantChallenge)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			envelope := decodeErrorEnvelope(t, rec.Body.Bytes())
			if envelope.Error.Code != "unauthorized" {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, "unauthorized")
			}
			if envelope.Error.Message == "" {
				t.Error("error message = empty, want human readable message")
			}
			if len(envelope.Error.Fields) != 0 {
				t.Errorf("error fields = %v, want none", envelope.Error.Fields)
			}

			if tt.verifierErr != nil && strings.Contains(rec.Body.String(), tt.verifierErr.Error()) {
				t.Errorf("response leaks internal error %q: body %q", tt.verifierErr, rec.Body.String())
			}

			if verifier.calls != tt.wantVerify {
				t.Errorf("VerifyToken() calls = %d, want %d", verifier.calls, tt.wantVerify)
			}
			if next.calls != 0 {
				t.Errorf("next handler calls = %d, want 0 on rejected request", next.calls)
			}
		})
	}
}

func TestAuthAcceptsValidHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
	}{
		{name: "canonical scheme", header: "Bearer abc.def", wantToken: "abc.def"},
		{name: "lowercase scheme", header: "bearer abc.def", wantToken: "abc.def"},
		{name: "extra spaces", header: "Bearer   abc.def", wantToken: "abc.def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeTokenVerifier{userID: 42}
			rec, next := serveAuth(t, verifier, tt.header)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if verifier.calls != 1 {
				t.Fatalf("VerifyToken() calls = %d, want 1", verifier.calls)
			}
			if verifier.token != tt.wantToken {
				t.Errorf("VerifyToken() token = %q, want %q", verifier.token, tt.wantToken)
			}
			if next.calls != 1 {
				t.Fatalf("next handler calls = %d, want 1", next.calls)
			}
			if !next.ok {
				t.Fatal("UserIDFrom() ok = false, want true inside authenticated handler")
			}
			if next.userID != 42 {
				t.Errorf("UserIDFrom() userID = %d, want 42", next.userID)
			}
		})
	}
}

func TestUserIDFromWithoutValue(t *testing.T) {
	userID, ok := httpserver.UserIDFrom(context.Background())
	if ok {
		t.Error("UserIDFrom() ok = true on empty context, want false")
	}
	if userID != 0 {
		t.Errorf("UserIDFrom() userID = %d, want 0", userID)
	}
}
