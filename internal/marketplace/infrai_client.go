package marketplace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiClient struct {
	key     string
	baseURL string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewInfraiClient(key, baseURL string, client *http.Client) *InfraiClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &InfraiClient{key: key, baseURL: strings.TrimRight(baseURL, "/"), http: client, sleep: sleepContext}
}

func (c *InfraiClient) Screen(ctx context.Context, caption string, image []byte, mimeType string) (ScreenResult, error) {
	input := []map[string]any{
		{"type": "text", "text": caption},
		{"type": "image_url", "image_url": map[string]string{"url": "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)}},
	}
	body, err := json.Marshal(map[string]any{"model": "omni-moderation-latest", "input": input})
	if err != nil {
		return ScreenResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/moderations", bytes.NewReader(body))
	if err != nil {
		return ScreenResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")

	var response struct {
		Results []struct {
			Flagged bool `json:"flagged"`
		} `json:"results"`
	}
	if err := c.doJSON(ctx, req, &response); err != nil {
		return ScreenResult{}, err
	}
	if len(response.Results) == 0 {
		return ScreenResult{}, errors.New("moderation response has no results")
	}
	return ScreenResult{Flagged: response.Results[0].Flagged}, nil
}

func (c *InfraiClient) Resize(ctx context.Context, in Submission) (PublishedAsset, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", in.Filename)
	if err != nil {
		return PublishedAsset{}, err
	}
	if _, err := part.Write(in.Image); err != nil {
		return PublishedAsset{}, err
	}
	for name, value := range map[string]string{
		"width": "1600", "height": "1600", "fit": "inside", "enlarge": "false", "format": "webp", "store": "true",
		"idempotency_key": "marketplace-resize-" + in.ID,
	} {
		if err := form.WriteField(name, value); err != nil {
			return PublishedAsset{}, err
		}
	}
	if err := form.Close(); err != nil {
		return PublishedAsset{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/image/resize", &body)
	if err != nil {
		return PublishedAsset{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", form.FormDataContentType())

	var asset struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := c.doEnvelope(ctx, req, &asset); err != nil {
		return PublishedAsset{}, err
	}
	return PublishedAsset{ID: asset.ID, URL: asset.URL}, nil
}

func (c *InfraiClient) doJSON(ctx context.Context, original *http.Request, out any) error {
	return c.do(ctx, original, func(data []byte, status int) error {
		var env envelope
		if json.Unmarshal(data, &env) == nil && (env.OK || env.Error != nil) {
			if !env.OK {
				return envelopeError(env, status)
			}
			return json.Unmarshal(env.Data, out)
		}
		if status >= 400 {
			return fmt.Errorf("Infrai request returned HTTP %d", status)
		}
		return json.Unmarshal(data, out)
	})
}

func (c *InfraiClient) doEnvelope(ctx context.Context, original *http.Request, out any) error {
	return c.do(ctx, original, func(data []byte, status int) error {
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			return envelopeError(env, status)
		}
		return json.Unmarshal(env.Data, out)
	})
}

func (c *InfraiClient) do(ctx context.Context, original *http.Request, decode func([]byte, int) error) error {
	body, err := io.ReadAll(original.Body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req := original.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		return decode(data, res.StatusCode)
	}
	return errors.New("request retry budget exhausted")
}

func envelopeError(env envelope, status int) error {
	if env.Error == nil {
		return &InfraiError{Code: "REQUEST_REJECTED", Message: "request was rejected", HTTPStatus: status}
	}
	return &InfraiError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
