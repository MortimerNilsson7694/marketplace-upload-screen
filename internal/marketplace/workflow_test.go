package marketplace

import (
	"context"
	"testing"
)

type stubScreener struct{ flagged bool }

func (s stubScreener) Screen(context.Context, string, []byte, string) (ScreenResult, error) {
	return ScreenResult{Flagged: s.flagged}, nil
}

type recordingTransformer struct{ calls int }

func (t *recordingTransformer) Resize(context.Context, Submission) (PublishedAsset, error) {
	t.calls++
	return PublishedAsset{ID: "asset_42", URL: "https://cdn.example/item.jpg"}, nil
}

func TestReviewDecision(t *testing.T) {
	tests := []struct {
		name           string
		flagged        bool
		wantState      string
		wantTransforms int
	}{
		{name: "safe seller asset is prepared for buyer view", wantState: "ready", wantTransforms: 1},
		{name: "flagged image or caption is quarantined", flagged: true, wantState: "quarantined", wantTransforms: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := &recordingTransformer{}
			workflow := NewWorkflow(stubScreener{flagged: tt.flagged}, transformer)
			got, err := workflow.Review(context.Background(), Submission{
				ID: "listing-42", Caption: "Walnut desk", Filename: "desk.jpg", Image: []byte("image"), MIMEType: "image/jpeg",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.wantState {
				t.Fatalf("state = %q, want %q", got.State, tt.wantState)
			}
			if transformer.calls != tt.wantTransforms {
				t.Fatalf("resize calls = %d, want %d", transformer.calls, tt.wantTransforms)
			}
		})
	}
}
