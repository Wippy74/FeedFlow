package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteErrorContract(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "channel_not_found", "notification channel not found")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"error":{"code":"channel_not_found","message":"notification channel not found"}}`, w.Body.String())
}

func TestWriteJSONEncodingFailureDoesNotCommitSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, make(chan int))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":{"code":"internal_error","message":"internal server error"}}`, w.Body.String())
}

func TestDecodeJSON(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"valid", `{"name":"example"}`, 200},
		{"whitespace", " {\"name\":\"example\"} \n ", 200},
		{"malformed", `{"name":`, 400},
		{"empty", "", 400},
		{"array", `[]`, 400},
		{"null", `null`, 400},
		{"scalar", `"example"`, 400},
		{"unknown field", `{"id":"client","name":"example"}`, 400},
		{"two objects", `{"name":"example"}{}`, 400},
		{"trailing null", `{"name":"example"}null`, 400},
		{"trailing garbage", `{"name":"example"}invalid`, 400},
		{"oversized value", `{"name":"` + strings.Repeat("x", int(MaxJSONBodyBytes)) + `"}`, 413},
		{"oversized trailing whitespace", `{"name":"example"}` + strings.Repeat(" ", int(MaxJSONBodyBytes)), 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			defer r.Body.Close()
			var value struct {
				Name string `json:"name"`
			}
			err := DecodeJSON(w, r, &value)
			if tt.status == 200 {
				require.NoError(t, err)
				assert.Equal(t, "example", value.Name)
				return
			}
			require.Error(t, err)
			WriteDecodeError(w, err)
			assert.Equal(t, tt.status, w.Code)
			var response ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.NotEmpty(t, response.Error.Code)
		})
	}
}
