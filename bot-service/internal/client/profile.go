package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kurt4ins/drizzy/pkg/models"
)

type ProfileClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewProfileClient(baseURL string) *ProfileClient {
	return &ProfileClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ProfileClient) CreateUser(ctx context.Context, req models.CreateUserRequest) (models.CreateUserResponse, error) {
	var resp models.CreateUserResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/users", req, &resp)
	return resp, err
}

func (c *ProfileClient) UpdateProfile(ctx context.Context, userID string, req models.UpdateProfileRequest) (models.Profile, error) {
	var resp models.Profile
	err := c.do(ctx, http.MethodPut, "/api/v1/profiles/"+userID, req, &resp)
	return resp, err
}

func (c *ProfileClient) do(ctx context.Context, method, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var errResp models.ErrorResponse
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("profile-service %s %s: %s", method, path, errResp.Error)
		}
		return fmt.Errorf("profile-service %s %s: status %d", method, path, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
