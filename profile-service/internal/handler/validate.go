package handler

import (
	"strings"

	"github.com/kurt4ins/drizzy/pkg/models"
)

// validatePreferencesRequest returns "" on success or the first error message.
// Empty pref_gender means "any". Each entry, when present, must be male/female.
func validatePreferencesRequest(req models.UpdatePreferencesRequest) string {
	if req.PrefAgeMin != nil && (*req.PrefAgeMin < 1 || *req.PrefAgeMin > 100) {
		return "pref_age_min must be between 1 and 100"
	}
	if req.PrefAgeMax != nil && (*req.PrefAgeMax < 1 || *req.PrefAgeMax > 100) {
		return "pref_age_max must be between 1 and 100"
	}
	if req.PrefAgeMin != nil && req.PrefAgeMax != nil && *req.PrefAgeMin > *req.PrefAgeMax {
		return "pref_age_min must be <= pref_age_max"
	}
	for _, g := range req.PrefGender {
		if g != "male" && g != "female" {
			return "pref_gender entries must be male or female"
		}
	}
	if req.PrefRadiusKM != nil && *req.PrefRadiusKM < 0 {
		return "pref_radius_km must be non-negative"
	}
	return ""
}

// validateProfileRequest returns an empty string on success, otherwise a
// human-readable error message describing the first failure.
func validateProfileRequest(req models.UpdateProfileRequest) string {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if req.Age < 1 || req.Age > 100 {
		return "age must be between 1 and 100"
	}
	if req.Gender != "male" && req.Gender != "female" {
		return "gender must be male or female"
	}
	if strings.TrimSpace(req.City) == "" {
		return "city is required"
	}
	return ""
}
