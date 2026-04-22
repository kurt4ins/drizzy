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

type ProfileClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewProfileClient(baseURL string) *ProfileClient {
	return &ProfileClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
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

func (c *ProfileClient) GetProfile(ctx context.Context, userID string) (models.Profile, error) {
	var resp models.Profile
	err := c.do(ctx, http.MethodGet, "/api/v1/profiles/"+userID, nil, &resp)
	return resp, err
}

func (c *ProfileClient) GetUser(ctx context.Context, userID string) (models.User, error) {
	var resp models.User
	err := c.do(ctx, http.MethodGet, "/api/v1/users/"+userID, nil, &resp)
	return resp, err
}

func (c *ProfileClient) UpdatePreferences(ctx context.Context, userID string, req models.UpdatePreferencesRequest) (models.Preferences, error) {
	var resp models.Preferences
	err := c.do(ctx, http.MethodPut, "/api/v1/preferences/"+userID, req, &resp)
	return resp, err
}

func (c *ProfileClient) UploadPhoto(ctx context.Context, userID, telegramFileID string, data []byte) (models.UploadPhotoResponse, error) {
	url := c.baseURL + "/api/v1/profiles/" + userID + "/photos"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return models.UploadPhotoResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if telegramFileID != "" {
		req.Header.Set("X-Telegram-File-ID", telegramFileID)
	}
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.UploadPhotoResponse{}, fmt.Errorf("upload photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var errResp models.ErrorResponse
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
			return models.UploadPhotoResponse{}, fmt.Errorf("profile-service upload photo: %s", errResp.Error)
		}
		return models.UploadPhotoResponse{}, fmt.Errorf("profile-service upload photo: status %d", resp.StatusCode)
	}

	var out models.UploadPhotoResponse
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return models.UploadPhotoResponse{}, fmt.Errorf("decode upload response: %w", err)
	}
	return out, nil
}

func (c *ProfileClient) GetPrimaryPhotoFileID(ctx context.Context, userID string) (string, error) {
	var ph models.ProfilePhoto
	err := c.do(ctx, http.MethodGet, "/api/v1/profiles/"+userID+"/photos/primary/meta", nil, &ph)
	if err != nil {
		if isNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return ph.TelegramFileID, nil
}

func isNotFound(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "status 404") ||
		strings.Contains(err.Error(), "not found"))
}

func (c *ProfileClient) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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
