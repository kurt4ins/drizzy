package models

import "time"

// ---------- domain entities ----------

type User struct {
	ID               string    `json:"id"`
	TelegramID       int64     `json:"telegram_id"`
	TelegramUsername string    `json:"telegram_username,omitempty"`
	RegisteredAt     time.Time `json:"registered_at"`
	IsActive         bool      `json:"is_active"`
	ReferralCode     string    `json:"referral_code"`
	ReferredByUserID *string   `json:"referred_by_user_id,omitempty"`
}

type Profile struct {
	UserID            string    `json:"user_id"`
	Name              string    `json:"name"`
	Bio               string    `json:"bio,omitempty"`
	Age               int       `json:"age"`
	Gender            string    `json:"gender"`
	City              string    `json:"city"`
	Latitude          *float64  `json:"latitude,omitempty"`
	Longitude         *float64  `json:"longitude,omitempty"`
	Interests         []string  `json:"interests,omitempty"`
	CompletenessScore float32   `json:"completeness_score"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Preferences struct {
	UserID       string   `json:"user_id"`
	PrefAgeMin   *int     `json:"pref_age_min,omitempty"`
	PrefAgeMax   *int     `json:"pref_age_max,omitempty"`
	PrefGender   []string `json:"pref_gender,omitempty"`
	PrefRadiusKM *int     `json:"pref_radius_km,omitempty"`
}

// ---------- request / response DTOs ----------

type CreateUserRequest struct {
	TelegramID       int64  `json:"telegram_id"`
	TelegramUsername string `json:"telegram_username,omitempty"`
}

type CreateUserResponse struct {
	User    User `json:"user"`
	Created bool `json:"created"` // false = user already existed
}

type UpdateProfileRequest struct {
	Name      string   `json:"name"`
	Age       int      `json:"age"`
	Gender    string   `json:"gender"`
	City      string   `json:"city"`
	Bio       string   `json:"bio,omitempty"`
	Interests []string `json:"interests,omitempty"`
}

type UpdatePreferencesRequest struct {
	PrefAgeMin   *int     `json:"pref_age_min,omitempty"`
	PrefAgeMax   *int     `json:"pref_age_max,omitempty"`
	PrefGender   []string `json:"pref_gender,omitempty"`
	PrefRadiusKM *int     `json:"pref_radius_km,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
