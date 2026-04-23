package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurt4ins/drizzy/pkg/models"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) RecordInteraction(ctx context.Context, actorID, targetID, action string) error {
	const q = `
		INSERT INTO interactions (actor_user_id, target_user_id, action_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (actor_user_id, target_user_id) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, actorID, targetID, action)
	if err != nil {
		return fmt.Errorf("record interaction: %w", err)
	}
	return nil
}

func (r *Repository) UpdateBehaviorStats(ctx context.Context, targetID, action string) error {
	var q string
	switch action {
	case "like":
		q = `INSERT INTO user_behavior_stats (user_id, likes_received, updated_at)
			 VALUES ($1, 1, NOW())
			 ON CONFLICT (user_id) DO UPDATE
			 SET likes_received = user_behavior_stats.likes_received + 1,
			     updated_at = NOW()`
	case "skip":
		q = `INSERT INTO user_behavior_stats (user_id, skips_received, updated_at)
			 VALUES ($1, 1, NOW())
			 ON CONFLICT (user_id) DO UPDATE
			 SET skips_received = user_behavior_stats.skips_received + 1,
			     updated_at = NOW()`
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
	_, err := r.pool.Exec(ctx, q, targetID)
	if err != nil {
		return fmt.Errorf("update behavior stats: %w", err)
	}
	return nil
}

func (r *Repository) IsLiked(ctx context.Context, actorID, targetID string) (bool, error) {
	const q = `
		SELECT 1 FROM interactions
		WHERE actor_user_id = $1 AND target_user_id = $2 AND action_type = 'like'`
	var dummy int
	err := r.pool.QueryRow(ctx, q, actorID, targetID).Scan(&dummy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check like: %w", err)
	}
	return true, nil
}

func (r *Repository) CreateMatch(ctx context.Context, userAID, userBID string) (models.Match, bool, error) {
	a, b := userAID, userBID
	if a > b {
		a, b = b, a
	}

	const q = `
		INSERT INTO matches (user_a_id, user_b_id)
		VALUES ($1, $2)
		ON CONFLICT (user_a_id, user_b_id) DO NOTHING
		RETURNING id, user_a_id, user_b_id, matched_at`

	var m models.Match
	err := r.pool.QueryRow(ctx, q, a, b).Scan(&m.ID, &m.UserAID, &m.UserBID, &m.MatchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Match{}, false, nil
	}
	if err != nil {
		return models.Match{}, false, fmt.Errorf("create match: %w", err)
	}

	const qStats = `
		INSERT INTO user_behavior_stats (user_id, matches_count, updated_at)
		VALUES ($1, 1, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET matches_count = user_behavior_stats.matches_count + 1, updated_at = NOW()`
	for _, uid := range []string{a, b} {
		if _, err = r.pool.Exec(ctx, qStats, uid); err != nil {
			return models.Match{}, false, fmt.Errorf("update match stats for %s: %w", uid, err)
		}
	}
	return m, true, nil
}

func (r *Repository) RecalculateAllScores(ctx context.Context) error {
	const q = `
		INSERT INTO user_ratings (user_id, score, algorithm_version, recalculated_at)
		SELECT
			user_id,
			(likes_received::float / GREATEST(likes_received + skips_received, 1))
			* LN(1 + likes_received + skips_received) AS score,
			'v1',
			NOW()
		FROM user_behavior_stats
		ON CONFLICT (user_id) DO UPDATE
		SET score             = EXCLUDED.score,
		    algorithm_version = EXCLUDED.algorithm_version,
		    recalculated_at   = EXCLUDED.recalculated_at`
	_, err := r.pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("recalculate scores: %w", err)
	}
	return nil
}

func (r *Repository) ActiveUserIDs(ctx context.Context) ([]string, error) {
	const q = `SELECT id FROM users WHERE is_active = TRUE`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("active users: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type CandidateRow struct {
	UserID string
}

func (r *Repository) TopCandidates(ctx context.Context, viewerID string, limit int) ([]string, error) {
	const q = `
		SELECT p.user_id
		FROM profiles p
		LEFT JOIN user_ratings ur ON ur.user_id = p.user_id
		LEFT JOIN LATERAL (
			SELECT city AS viewer_city FROM profiles WHERE user_id = $1 LIMIT 1
		) v ON TRUE
		WHERE p.user_id <> $1
		  AND p.user_id NOT IN (
		      SELECT target_user_id FROM interactions WHERE actor_user_id = $1
		  )
		  AND p.gender = COALESCE(
		      (SELECT pref_gender[1] FROM user_preferences
		       WHERE user_id = $1 AND array_length(pref_gender, 1) > 0),
		      p.gender
		  )
		  AND p.age BETWEEN
		      COALESCE((SELECT pref_age_min FROM user_preferences WHERE user_id = $1), p.age)
		      AND
		      COALESCE((SELECT pref_age_max FROM user_preferences WHERE user_id = $1), p.age)
		ORDER BY
		  COALESCE(ur.score, 0)
		  + COALESCE(p.completeness_score, 0)
		  + CASE
		      WHEN NULLIF(TRIM(BOTH FROM v.viewer_city), '') IS NOT NULL
		       AND NULLIF(TRIM(BOTH FROM p.city), '') IS NOT NULL
		       AND LOWER(TRIM(BOTH FROM p.city)) = LOWER(TRIM(BOTH FROM v.viewer_city))
		      THEN 2.0
		      ELSE 0
		    END
		  DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, viewerID, limit)
	if err != nil {
		return nil, fmt.Errorf("top candidates for %s: %w", viewerID, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) ListMatchesForUser(ctx context.Context, userID string) ([]models.UserMatchEntry, error) {
	const q = `
		SELECT id,
		       CASE WHEN user_a_id = $1 THEN user_b_id ELSE user_a_id END,
		       matched_at
		FROM matches
		WHERE user_a_id = $1 OR user_b_id = $1
		ORDER BY matched_at DESC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list matches for %s: %w", userID, err)
	}
	defer rows.Close()

	var out []models.UserMatchEntry
	for rows.Next() {
		var e models.UserMatchEntry
		if err := rows.Scan(&e.MatchID, &e.OtherUserID, &e.MatchedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) UserBehaviorStats(ctx context.Context, userID string) (models.BehaviorStats, error) {
	const q = `
		SELECT user_id, likes_received, skips_received, matches_count,
		       conversations_started, updated_at
		FROM user_behavior_stats WHERE user_id = $1`

	var s models.BehaviorStats
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&s.UserID, &s.LikesReceived, &s.SkipsReceived,
		&s.MatchesCount, &s.ConversationsStarted, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		s.UserID = userID
		s.UpdatedAt = time.Now()
		return s, nil
	}
	if err != nil {
		return models.BehaviorStats{}, fmt.Errorf("behavior stats: %w", err)
	}
	return s, nil
}
