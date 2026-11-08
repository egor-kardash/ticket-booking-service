package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MiB

var (
	ErrNotJSONObject        = errors.New("request body must be a JSON object")
	ErrUnsupportedMediaType = errors.New("unsupported media type")
)

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil ||
		(mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return ErrUnsupportedMediaType
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrNotJSONObject
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after JSON object")
	}

	obj := json.NewDecoder(bytes.NewReader(trimmed))
	obj.DisallowUnknownFields()
	return obj.Decode(dst)
}
