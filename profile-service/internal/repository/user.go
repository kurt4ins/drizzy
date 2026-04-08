package repository

import (
	"context"
	"crypto/rand"
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

// Upsert inserts or updates a user by telegram_id.
// Returns the user and whether a new row was created.
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
			continue // retry with new code
		}
		return models.User{}, false, fmt.Errorf("upsert user: %w", err)
	}
	return models.User{}, false, fmt.Errorf("upsert user: failed to generate unique referral code")
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
		return models.User{}, fmt.Errorf("user not found")
	}
	if err != nil {
		return models.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}
