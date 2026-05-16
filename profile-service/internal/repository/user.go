package repository

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurt4ins/drizzy/pkg/models"
)

const referralCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func generateReferralCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = referralCharset[b[i]%byte(len(referralCharset))]
	}
	return string(b)
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Upsert(ctx context.Context, req models.CreateUserRequest) (models.User, bool, error) {
	const query = `
		INSERT INTO users (telegram_id, telegram_username, referral_code)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO UPDATE
			SET telegram_username = EXCLUDED.telegram_username
		RETURNING id, telegram_id, telegram_username, registered_at, is_active,
		          referral_code, referred_by_user_id, (xmax = 0) AS created`

	for attempt := 0; attempt < 3; attempt++ {
		code := generateReferralCode()
		row := r.pool.QueryRow(ctx, query, req.TelegramID, req.TelegramUsername, code)

		var u models.User
		var created bool
		err := row.Scan(
			&u.ID, &u.TelegramID, &u.TelegramUsername, &u.RegisteredAt,
			&u.IsActive, &u.ReferralCode, &u.ReferredByUserID, &created,
		)
		if err == nil {
			return u, created, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_referral_code_key" {
			continue
		}
		return models.User{}, false, fmt.Errorf("upsert user: %w", err)
	}
	return models.User{}, false, fmt.Errorf("upsert user: failed to generate unique referral code")
}

// Register inserts the user (or returns the existing one for the same telegram_id)
// and upserts their profile in a single transaction. Returns the user, the upserted
// profile, and whether the user row was newly created.
func (r *UserRepository) Register(
	ctx context.Context,
	userReq models.CreateUserRequest,
	profileReq models.UpdateProfileRequest,
	hasPreferences bool,
) (models.User, models.Profile, bool, error) {
	score := calculateCompleteness(profileReq, hasPreferences)

	interestsForJSON := profileReq.Interests
	if interestsForJSON == nil {
		interestsForJSON = []string{}
	}
	interestsJSON, err := json.Marshal(interestsForJSON)
	if err != nil {
		return models.User{}, models.Profile{}, false, fmt.Errorf("marshal interests: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.User{}, models.Profile{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const qUser = `
		INSERT INTO users (telegram_id, telegram_username, referral_code)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO UPDATE
			SET telegram_username = EXCLUDED.telegram_username
		RETURNING id, telegram_id, telegram_username, registered_at, is_active,
		          referral_code, referred_by_user_id, (xmax = 0) AS created`

	var u models.User
	var created bool
	for attempt := 0; attempt < 3; attempt++ {
		code := generateReferralCode()
		err = tx.QueryRow(ctx, qUser, userReq.TelegramID, userReq.TelegramUsername, code).Scan(
			&u.ID, &u.TelegramID, &u.TelegramUsername, &u.RegisteredAt,
			&u.IsActive, &u.ReferralCode, &u.ReferredByUserID, &created,
		)
		if err == nil {
			break
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_referral_code_key" {
			continue
		}
		return models.User{}, models.Profile{}, false, fmt.Errorf("upsert user: %w", err)
	}
	if err != nil {
		return models.User{}, models.Profile{}, false, fmt.Errorf("upsert user: failed to generate unique referral code")
	}

	const qProfile = `
		INSERT INTO profiles (user_id, name, bio, age, gender, city, interests, completeness_score, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (user_id) DO UPDATE
			SET name=$2, bio=$3, age=$4, gender=$5, city=$6,
			    interests=$7, completeness_score=$8, updated_at=NOW()
		RETURNING user_id, name, bio, age, gender, city, latitude, longitude,
		          interests, completeness_score, updated_at`

	var p models.Profile
	var outInterestsJSON []byte
	err = tx.QueryRow(ctx, qProfile,
		u.ID, profileReq.Name, profileReq.Bio, profileReq.Age, profileReq.Gender,
		profileReq.City, interestsJSON, score,
	).Scan(
		&p.UserID, &p.Name, &p.Bio, &p.Age, &p.Gender, &p.City,
		&p.Latitude, &p.Longitude, &outInterestsJSON, &p.CompletenessScore, &p.UpdatedAt,
	)
	if err != nil {
		return models.User{}, models.Profile{}, false, fmt.Errorf("upsert profile: %w", err)
	}
	if len(outInterestsJSON) > 0 {
		_ = json.Unmarshal(outInterestsJSON, &p.Interests)
	}

	if err = tx.Commit(ctx); err != nil {
		return models.User{}, models.Profile{}, false, fmt.Errorf("commit: %w", err)
	}
	return u, p, created, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (models.User, error) {
	const query = `
		SELECT id, telegram_id, telegram_username, registered_at, is_active,
		       referral_code, referred_by_user_id
		FROM users WHERE id = $1`

	var u models.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.TelegramID, &u.TelegramUsername, &u.RegisteredAt,
		&u.IsActive, &u.ReferralCode, &u.ReferredByUserID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}
