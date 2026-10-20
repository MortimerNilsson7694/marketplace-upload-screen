package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"example.com/marketplace-upload-screen/internal/marketplace"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := marketplace.NewInfraiClient(key, os.Getenv("INFRAI_BASE_URL"), nil)
	workflow := marketplace.NewWorkflow(client, client)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /submissions", submissionHandler(workflow))
	addr := ":8080"
	log.Printf("marketplace screen listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func submissionHandler(workflow *marketplace.Workflow) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const maxImageBytes = 10 << 20

		if err := r.ParseMultipartForm(12 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
			return
		}
		defer file.Close()
		image, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		if err != nil || len(image) > maxImageBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image must be at most 10 MiB"})
			return
		}
		submissionID := r.FormValue("submission_id")
		if submissionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "submission_id is required"})
			return
		}

		decision, err := workflow.Review(r.Context(), marketplace.Submission{
			ID: submissionID, Caption: r.FormValue("caption"), Filename: header.Filename,
			Image: image, MIMEType: header.Header.Get("Content-Type"),
		})
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *marketplace.InfraiError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, decision)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
