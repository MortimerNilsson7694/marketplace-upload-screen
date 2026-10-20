package marketplace

import (
	"context"
	"fmt"
)

type Submission struct {
	ID       string
	Caption  string
	Filename string
	Image    []byte
	MIMEType string
}

type Decision struct {
	SubmissionID string `json:"submission_id"`
	State        string `json:"state"`
	AssetID      string `json:"asset_id,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
}

type ScreenResult struct {
	Flagged bool
}

type PublishedAsset struct {
	ID  string
	URL string
}

type Screener interface {
	Screen(ctx context.Context, caption string, image []byte, mimeType string) (ScreenResult, error)
}

type Transformer interface {
	Resize(ctx context.Context, submission Submission) (PublishedAsset, error)
}

type Workflow struct {
	screener    Screener
	transformer Transformer
}

func NewWorkflow(s Screener, t Transformer) *Workflow {
	return &Workflow{screener: s, transformer: t}
}

func (w *Workflow) Review(ctx context.Context, in Submission) (Decision, error) {
	result, err := w.screener.Screen(ctx, in.Caption, in.Image, in.MIMEType)
	if err != nil {
		return Decision{}, fmt.Errorf("screen submission: %w", err)
	}
	if result.Flagged {
		return Decision{SubmissionID: in.ID, State: "quarantined"}, nil
	}

	asset, err := w.transformer.Resize(ctx, in)
	if err != nil {
		return Decision{}, fmt.Errorf("prepare published asset: %w", err)
	}
	return Decision{SubmissionID: in.ID, State: "ready", AssetID: asset.ID, ImageURL: asset.URL}, nil
}
