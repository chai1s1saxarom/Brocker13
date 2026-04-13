package gosdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Message struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) CreateStream(ctx context.Context, name, stype, mode string) error {
	body := map[string]any{"name": name, "type": stype, "queue_mode": mode}
	return c.post(ctx, "/streams", body, nil)
}

func (c *Client) Subscribe(ctx context.Context, stream, subscriber string) error {
	body := map[string]any{"stream": stream, "subscriber": subscriber}
	return c.post(ctx, "/subscribe", body, nil)
}

func (c *Client) Publish(ctx context.Context, stream, payload string, priority int, ttlSeconds int) (Message, error) {
	body := map[string]any{
		"stream":      stream,
		"payload":     payload,
		"priority":    priority,
		"ttl_seconds": ttlSeconds,
	}
	var msg Message
	if err := c.post(ctx, "/publish", body, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (c *Client) Pull(ctx context.Context, stream, subscriber string, batch int) ([]Message, error) {
	body := map[string]any{
		"stream":     stream,
		"subscriber": subscriber,
		"batch":      batch,
	}
	var resp struct {
		Messages []Message `json:"messages"`
	}
	if err := c.post(ctx, "/pull", body, &resp); err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

func (c *Client) Ack(ctx context.Context, stream, subscriber, messageID string) error {
	body := map[string]any{
		"stream":     stream,
		"subscriber": subscriber,
		"message_id": messageID,
	}
	return c.post(ctx, "/ack", body, nil)
}

func (c *Client) post(ctx context.Context, path string, reqBody any, out any) error {
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewBuffer(raw))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("broker error: %s", string(payload))
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
