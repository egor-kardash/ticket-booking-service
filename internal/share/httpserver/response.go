package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
)

func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		LoggerFrom(ctx).Error("encode json response", "error", err)
		WriteError(ctx, w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	writeBody(ctx, w, status, body)
}

func WriteNoContent(w http.ResponseWriter) {
	w.Header().Del("Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

func writeBody(ctx context.Context, w http.ResponseWriter, status int, body []byte) {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		LoggerFrom(ctx).Error("write response", "error", err, "status", status)
	}
}
