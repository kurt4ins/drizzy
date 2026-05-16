package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kurt4ins/drizzy/pkg/models"
)

// HTTPError carries the HTTP status and decoded error body for a failed
// profile-service request.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("profile-service status %d: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("profile-service status %d", e.Status)
}

// IsNotFound reports whether err is an HTTPError with status 404.
func IsNotFound(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound
}

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

// Register atomically creates (or updates) the user and upserts their profile in
// a single round-trip.
func (c *ProfileClient) Register(ctx context.Context, req models.RegisterRequest) (models.RegisterResponse, error) {
	var resp models.RegisterResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/users/register", req, &resp)
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

func (c *ProfileClient) UploadPhoto(ctx context.Context, userID, telegramFileID string, body io.Reader, size int64) (models.UploadPhotoResponse, error) {
	url := c.baseURL + "/api/v1/profiles/" + userID + "/photos"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return models.UploadPhotoResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	if size > 0 {
		req.ContentLength = size
	}
	if telegramFileID != "" {
		req.Header.Set("X-Telegram-File-ID", telegramFileID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.UploadPhotoResponse{}, fmt.Errorf("upload photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return models.UploadPhotoResponse{}, httpErrorFrom(resp)
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
		if IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return ph.TelegramFileID, nil
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
		return httpErrorFrom(resp)
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func httpErrorFrom(resp *http.Response) *HTTPError {
	raw, _ := io.ReadAll(resp.Body)
	body := strings.TrimSpace(string(raw))
	var errResp models.ErrorResponse
	if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
		body = errResp.Error
	}
	return &HTTPError{Status: resp.StatusCode, Body: body}
}
