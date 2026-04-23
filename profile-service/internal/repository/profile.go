package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurt4ins/drizzy/pkg/models"
)

type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

func (r *ProfileRepository) Get(ctx context.Context, userID string) (models.Profile, error) {
	const query = `
		SELECT user_id, name, bio, age, gender, city, latitude, longitude,
		       interests, completeness_score, updated_at
		FROM profiles WHERE user_id = $1`

	var p models.Profile
	var interestsJSON []byte
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&p.UserID, &p.Name, &p.Bio, &p.Age, &p.Gender, &p.City,
		&p.Latitude, &p.Longitude, &interestsJSON, &p.CompletenessScore, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Profile{}, fmt.Errorf("profile not found")
	}
	if err != nil {
		return models.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	if len(interestsJSON) > 0 {
		_ = json.Unmarshal(interestsJSON, &p.Interests)
	}
	return p, nil
}

func (r *ProfileRepository) Upsert(ctx context.Context, userID string, req models.UpdateProfileRequest) (models.Profile, error) {
	hasPrefs, err := r.hasPreferences(ctx, userID)
	if err != nil {
		return models.Profile{}, err
	}
	score := calculateCompleteness(req, hasPrefs)

	interestsForJSON := req.Interests
	if interestsForJSON == nil {
		interestsForJSON = []string{}
	}
	interestsJSON, err := json.Marshal(interestsForJSON)
	if err != nil {
		return models.Profile{}, fmt.Errorf("marshal interests: %w", err)
	}

	const query = `
		INSERT INTO profiles (user_id, name, bio, age, gender, city, interests, completeness_score, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (user_id) DO UPDATE
			SET name=$2, bio=$3, age=$4, gender=$5, city=$6,
			    interests=$7, completeness_score=$8, updated_at=NOW()
		RETURNING user_id, name, bio, age, gender, city, latitude, longitude,
		          interests, completeness_score, updated_at`

	var p models.Profile
	var outInterestsJSON []byte
	err = r.pool.QueryRow(ctx, query,
		userID, req.Name, req.Bio, req.Age, req.Gender, req.City, interestsJSON, score,
	).Scan(
		&p.UserID, &p.Name, &p.Bio, &p.Age, &p.Gender, &p.City,
		&p.Latitude, &p.Longitude, &outInterestsJSON, &p.CompletenessScore, &p.UpdatedAt,
	)
	if err != nil {
		return models.Profile{}, fmt.Errorf("upsert profile: %w", err)
	}
	if len(outInterestsJSON) > 0 {
		_ = json.Unmarshal(outInterestsJSON, &p.Interests)
	}
	return p, nil
}

func (r *ProfileRepository) GetPreferences(ctx context.Context, userID string) (models.Preferences, error) {
	const query = `
		SELECT user_id, pref_age_min, pref_age_max, pref_gender, pref_radius_km
		FROM user_preferences WHERE user_id = $1`

	var p models.Preferences
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&p.UserID, &p.PrefAgeMin, &p.PrefAgeMax, &p.PrefGender, &p.PrefRadiusKM,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Preferences{UserID: userID}, nil
	}
	if err != nil {
		return models.Preferences{}, fmt.Errorf("get preferences: %w", err)
	}
	return p, nil
}

func (r *ProfileRepository) UpsertPreferences(ctx context.Context, userID string, req models.UpdatePreferencesRequest) (models.Preferences, error) {
	const query = `
		INSERT INTO user_preferences (user_id, pref_age_min, pref_age_max, pref_gender, pref_radius_km, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (user_id) DO UPDATE
			SET pref_age_min=$2, pref_age_max=$3, pref_gender=$4, pref_radius_km=$5, updated_at=NOW()
		RETURNING user_id, pref_age_min, pref_age_max, pref_gender, pref_radius_km`

	var p models.Preferences
	err := r.pool.QueryRow(ctx, query,
		userID, req.PrefAgeMin, req.PrefAgeMax, req.PrefGender, req.PrefRadiusKM,
	).Scan(&p.UserID, &p.PrefAgeMin, &p.PrefAgeMax, &p.PrefGender, &p.PrefRadiusKM)
	if err != nil {
		return models.Preferences{}, fmt.Errorf("upsert preferences: %w", err)
	}
	return p, nil
}

func (r *ProfileRepository) AddPhoto(ctx context.Context, userID, s3Key, telegramFileID string) (models.ProfilePhoto, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.ProfilePhoto{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx,
		`UPDATE profile_photos SET is_primary = FALSE WHERE profile_id = $1`, userID,
	); err != nil {
		return models.ProfilePhoto{}, fmt.Errorf("demote photos: %w", err)
	}

	const qInsert = `
		INSERT INTO profile_photos (profile_id, s3_key, telegram_file_id, sort_order, is_primary)
		VALUES ($1, $2, $3,
		    COALESCE((SELECT MAX(sort_order)+1 FROM profile_photos WHERE profile_id = $1), 0),
		    TRUE
		)
		RETURNING id, profile_id, s3_key, COALESCE(telegram_file_id,''), sort_order, is_primary, uploaded_at`

	var ph models.ProfilePhoto
	err = tx.QueryRow(ctx, qInsert, userID, s3Key, nullableString(telegramFileID)).Scan(
		&ph.ID, &ph.ProfileID, &ph.S3Key, &ph.TelegramFileID,
		&ph.SortOrder, &ph.IsPrimary, &ph.UploadedAt,
	)
	if err != nil {
		return models.ProfilePhoto{}, fmt.Errorf("insert photo: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return models.ProfilePhoto{}, fmt.Errorf("commit tx: %w", err)
	}

	if err = r.recalculateCompleteness(ctx, userID); err != nil {
		return ph, err
	}
	return ph, nil
}

func (r *ProfileRepository) GetPrimaryPhoto(ctx context.Context, userID string) (models.ProfilePhoto, error) {
	const q = `
		SELECT id, profile_id, s3_key, COALESCE(telegram_file_id,''),
		       sort_order, is_primary, uploaded_at
		FROM profile_photos
		WHERE profile_id = $1 AND is_primary = TRUE
		LIMIT 1`

	var ph models.ProfilePhoto
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&ph.ID, &ph.ProfileID, &ph.S3Key, &ph.TelegramFileID,
		&ph.SortOrder, &ph.IsPrimary, &ph.UploadedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ProfilePhoto{}, nil
	}
	if err != nil {
		return models.ProfilePhoto{}, fmt.Errorf("get primary photo: %w", err)
	}
	return ph, nil
}

func (r *ProfileRepository) recalculateCompleteness(ctx context.Context, userID string) error {
	const q = `
		UPDATE profiles SET
			completeness_score = (
				CASE WHEN name <> ''                                THEN 0.10 ELSE 0 END +
				CASE WHEN bio IS NOT NULL AND bio <> ''             THEN 0.15 ELSE 0 END +
				CASE
					WHEN interests IS NULL THEN 0
					WHEN jsonb_typeof(interests) <> 'array' THEN 0
					WHEN jsonb_array_length(interests) > 0 THEN 0.15
					ELSE 0
				END +
				CASE WHEN EXISTS(SELECT 1 FROM user_preferences WHERE user_id = $1)    THEN 0.20 ELSE 0 END +
				CASE WHEN EXISTS(SELECT 1 FROM profile_photos  WHERE profile_id = $1)  THEN 0.25 ELSE 0 END
			),
			updated_at = NOW()
		WHERE user_id = $1`
	_, err := r.pool.Exec(ctx, q, userID)
	return err
}

func (r *ProfileRepository) hasPreferences(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_preferences WHERE user_id = $1)`, userID,
	).Scan(&exists)
	return exists, err
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func calculateCompleteness(req models.UpdateProfileRequest, hasPreferences bool) float32 {
	var score float32
	if strings.TrimSpace(req.Name) != "" {
		score += 0.10
	}
	if strings.TrimSpace(req.Bio) != "" {
		score += 0.15
	}
	if len(req.Interests) > 0 {
		score += 0.15
	}
	if hasPreferences {
		score += 0.20
	}
	return score
}
