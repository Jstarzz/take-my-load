package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Register(ctx context.Context, reg protocol.WorkerRegistration) error {
	return c.post(ctx, "/api/v1/workers/register", reg, nil)
}

func (c *Client) Heartbeat(ctx context.Context, id string, hb protocol.WorkerHeartbeat) error {
	return c.post(ctx, "/api/v1/workers/"+url.PathEscape(id)+"/heartbeat", hb, nil)
}

func (c *Client) Assignments(ctx context.Context, workerID string) ([]protocol.WorkerAssignment, error) {
	var assignments []protocol.WorkerAssignment
	if err := c.get(ctx, "/api/v1/workers/"+url.PathEscape(workerID)+"/assignments", &assignments); err != nil {
		return nil, err
	}
	return assignments, nil
}

func (c *Client) Transition(ctx context.Context, workerID, assignmentID, action string) (protocol.TestJob, error) {
	path := "/api/v1/workers/" + url.PathEscape(workerID) + "/assignments/" + url.PathEscape(assignmentID) + "/" + url.PathEscape(action)
	var job protocol.TestJob
	if err := c.post(ctx, path, struct{}{}, &job); err != nil {
		return protocol.TestJob{}, err
	}
	return job, nil
}

func (c *Client) Complete(ctx context.Context, workerID, assignmentID string, summary protocol.ExecutionSummary) (protocol.TestJob, error) {
	path := "/api/v1/workers/" + url.PathEscape(workerID) + "/assignments/" + url.PathEscape(assignmentID) + "/result"
	var job protocol.TestJob
	if err := c.post(ctx, path, summary, &job); err != nil {
		return protocol.TestJob{}, err
	}
	return job, nil
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, dst)
}

func (c *Client) post(ctx context.Context, path string, payload, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, dst)
}

func (c *Client) do(req *http.Request, dst any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request control plane: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("control plane returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	if dst == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode control plane response: %w", err)
	}
	return nil
}
