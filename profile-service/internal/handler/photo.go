package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/kurt4ins/drizzy/pkg/models"
	"github.com/kurt4ins/drizzy/profile-service/internal/repository"
	"github.com/kurt4ins/drizzy/profile-service/internal/storage"
)

type PhotoHandler struct {
	repo  *repository.ProfileRepository
	store *storage.MinIO
}

func NewPhotoHandler(repo *repository.ProfileRepository, store *storage.MinIO) *PhotoHandler {
	return &PhotoHandler{repo: repo, store: store}
}

func (h *PhotoHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	telegramFileID := r.Header.Get("X-Telegram-File-ID")

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	s3Key := fmt.Sprintf("photos/%s/%s.jpg", userID, uuid.New().String())

	if err := h.store.PutObject(r.Context(), s3Key, contentType, r.ContentLength, r.Body); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store photo")
		return
	}

	ph, err := h.repo.AddPhoto(r.Context(), userID, s3Key, telegramFileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save photo record")
		return
	}

	writeJSON(w, http.StatusCreated, models.UploadPhotoResponse{
		Photo:          ph,
		TelegramFileID: ph.TelegramFileID,
	})
}

func (h *PhotoHandler) GetPrimaryPhotoMeta(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	ph, err := h.repo.GetPrimaryPhoto(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get photo")
		return
	}
	if ph.S3Key == "" {
		writeError(w, http.StatusNotFound, "no photo")
		return
	}
	writeJSON(w, http.StatusOK, ph)
}

func (h *PhotoHandler) GetPrimaryPhoto(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")

	ph, err := h.repo.GetPrimaryPhoto(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get photo")
		return
	}
	if ph.S3Key == "" {
		writeError(w, http.StatusNotFound, "no photo")
		return
	}

	if ph.TelegramFileID != "" {
		w.Header().Set("X-Telegram-File-ID", ph.TelegramFileID)
	}

	rc, size, ct, err := h.store.GetObject(r.Context(), ph.S3Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stream photo")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", ct)
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc) //nolint:errcheck
}
