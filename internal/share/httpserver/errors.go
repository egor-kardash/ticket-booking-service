package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func WriteError(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	WriteErrorFields(ctx, w, status, code, message, nil)
}

func WriteErrorFields(ctx context.Context, w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	body, err := json.Marshal(errorEnvelope{
		Error: errorBody{
			Code:    code,
			Message: message,
			Fields:  fields,
		},
	})
	if err != nil {
		LoggerFrom(ctx).Error("encode error response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeBody(ctx, w, status, body)
}
