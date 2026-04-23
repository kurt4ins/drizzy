package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kurt4ins/drizzy/pkg/models"
)

type RankingClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRankingClient(baseURL string) *RankingClient {
	return &RankingClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *RankingClient) RefillQueue(ctx context.Context, userID string) error {
	body, _ := json.Marshal(map[string]string{"user_id": userID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/queue/refill", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("ranking service returned %d", resp.StatusCode)
	}
	return nil
}

func (c *RankingClient) ListMatches(ctx context.Context, userID string) ([]models.UserMatchEntry, error) {
	url := strings.TrimSuffix(c.baseURL, "/") + "/api/v1/users/" + userID + "/matches"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ranking service list matches: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out []models.UserMatchEntry
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode matches: %w", err)
	}
	if out == nil {
		out = []models.UserMatchEntry{}
	}
	return out, nil
}
