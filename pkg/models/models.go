package models

import "time"

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

type CreateUserRequest struct {
	TelegramID       int64  `json:"telegram_id"`
	TelegramUsername string `json:"telegram_username,omitempty"`
}

type CreateUserResponse struct {
	User    User `json:"user"`
	Created bool `json:"created"`
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

type ProfilePhoto struct {
	ID             string    `json:"id"`
	ProfileID      string    `json:"profile_id"`
	S3Key          string    `json:"s3_key"`
	TelegramFileID string    `json:"telegram_file_id,omitempty"`
	SortOrder      int       `json:"sort_order"`
	IsPrimary      bool      `json:"is_primary"`
	UploadedAt     time.Time `json:"uploaded_at"`
}

type UploadPhotoResponse struct {
	Photo          ProfilePhoto `json:"photo"`
	TelegramFileID string       `json:"telegram_file_id,omitempty"`
}

type Interaction struct {
	ID           string    `json:"id"`
	ActorUserID  string    `json:"actor_user_id"`
	TargetUserID string    `json:"target_user_id"`
	ActionType   string    `json:"action_type"`
	CreatedAt    time.Time `json:"created_at"`
}

type Match struct {
	ID                    string     `json:"id"`
	UserAID               string     `json:"user_a_id"`
	UserBID               string     `json:"user_b_id"`
	MatchedAt             time.Time  `json:"matched_at"`
	ConversationStarted   bool       `json:"conversation_started"`
	ConversationStartedAt *time.Time `json:"conversation_started_at,omitempty"`
}

type BehaviorStats struct {
	UserID               string    `json:"user_id"`
	LikesReceived        int       `json:"likes_received"`
	SkipsReceived        int       `json:"skips_received"`
	MatchesCount         int       `json:"matches_count"`
	ConversationsStarted int       `json:"conversations_started"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UserRating struct {
	UserID           string    `json:"user_id"`
	Score            float64   `json:"score"`
	AlgorithmVersion string    `json:"algorithm_version"`
	RecalculatedAt   time.Time `json:"recalculated_at"`
}
