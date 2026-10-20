package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResizeSendsIdempotencyKeyAsRequestField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got, want := r.FormValue("idempotency_key"), "marketplace-resize-listing-42"; got != want {
			t.Fatalf("idempotency_key = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"data": map[string]string{"id": "asset-42", "url": "https://example.test/asset.webp"},
		})
	}))
	defer server.Close()

	client := NewInfraiClient("test-key", server.URL, server.Client())
	_, err := client.Resize(context.Background(), Submission{
		ID: "listing-42", Filename: "image.jpg", Image: []byte("image"),
	})
	if err != nil {
		t.Fatal(err)
	}
}
