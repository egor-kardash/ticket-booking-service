package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
	authhttp "github.com/egor-kardash/ticket-booking-service/internal/features/auth/transport/http"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

const (
	testUserID   int64 = 42
	testEmail          = "user@example.com"
	testPassword       = "secret-password"
)

type fakeUserService struct {
	registerResult *domain.User
	registerErr    error
	registerInput  service.RegisterInput
	registerCalls  int

	loginResult *service.LoginOutput
	loginErr    error
	loginInput  service.LoginInput
	loginCalls  int

	getMeResult *domain.User
	getMeErr    error
	getMeID     int64
}

func (f *fakeUserService) Register(_ context.Context, in service.RegisterInput) (*domain.User, error) {
	f.registerCalls++
	f.registerInput = in

	if f.registerErr != nil {
		return nil, f.registerErr
	}

	return f.registerResult, nil
}

func (f *fakeUserService) Login(_ context.Context, in service.LoginInput) (*service.LoginOutput, error) {
	f.loginCalls++
	f.loginInput = in

	if f.loginErr != nil {
		return nil, f.loginErr
	}

	return f.loginResult, nil
}

func (f *fakeUserService) GetMe(_ context.Context, userID int64) (*domain.User, error) {
	f.getMeID = userID

	if f.getMeErr != nil {
		return nil, f.getMeErr
	}

	return f.getMeResult, nil
}

type fakeTokenVerifier struct {
	userID int64
	err    error
}

func (f *fakeTokenVerifier) VerifyToken(string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}

	return f.userID, nil
}

// newTestAPI повторяет проводку app.New: те же пути, тот же /api/v1,
// тот же Auth только на protected-группе.
// verifier == nil означает unprotected-сборку: нужна для проверки me без user id.
func newTestAPI(svc authhttp.UserService, verifier httpserver.TokenVerifier) chi.Router {
	handler := authhttp.New(svc)
	root := chi.NewRouter()
	root.Use(httpserver.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	root.Route("/api/v1", func(router chi.Router) {
		protected := router.Group(func(protected chi.Router) {
			if verifier != nil {
				protected.Use(httpserver.Auth(verifier))
			}
		})

		handler.Routes(router, protected)
	})

	return root
}

func serve(t *testing.T, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func postJSON(t *testing.T, path, body string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return req
}

type userBody struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type loginBody struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func requireError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) errorEnvelope {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not a JSON error envelope: %v (body %q)", err, rec.Body.String())
	}
	if envelope.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", envelope.Error.Code, wantCode)
	}
	if envelope.Error.Message == "" {
		t.Error("error message = empty, want human readable message")
	}

	return envelope
}

// decodeBody декодирует успешный ответ строго: лишнее поле в DTO — это утечка
// (например, password_hash) и должно ронять тест.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(rec.Body.String()))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("response contains more than one JSON value: %v (body %q)", err, rec.Body.String())
	}
}

func requireNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want %q", got, "no-cache")
	}
}

func requireNoLeak(t *testing.T, rec *httptest.ResponseRecorder, secrets ...string) {
	t.Helper()

	body := rec.Body.String()
	for _, secret := range secrets {
		if strings.Contains(body, secret) {
			t.Errorf("response leaks %q: %q", secret, body)
		}
	}
}
