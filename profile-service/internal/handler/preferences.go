package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kurt4ins/drizzy/pkg/models"
	"github.com/kurt4ins/drizzy/profile-service/internal/repository"
	"github.com/redis/go-redis/v9"
)

type PrefsHandler struct {
	repo *repository.ProfileRepository
	rdb  *redis.Client
}

func NewPrefsHandler(repo *repository.ProfileRepository, rdb *redis.Client) *PrefsHandler {
	return &PrefsHandler{repo: repo, rdb: rdb}
}

func (h *PrefsHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")

	var req models.UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prefs, err := h.repo.UpsertPreferences(r.Context(), userID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update preferences")
		return
	}

	// Invalidate the discovery queue for this user on preference change.
	h.rdb.Del(r.Context(), fmt.Sprintf("discovery:queue:%s", userID))

	writeJSON(w, http.StatusOK, prefs)
}
