package events

import "time"

type Envelope struct {
	EventID       string    `json:"event_id"`
	Type          string    `json:"type"`
	OccurredAt    time.Time `json:"occurred_at"`
	SchemaVersion int       `json:"schema_version"`
	Payload       any       `json:"payload"`
}

const (
	TypeInteractionLiked   = "interaction.liked"
	TypeInteractionSkipped = "interaction.skipped"
	TypeMatchCreated       = "match.created"
	TypeLikeReceived       = "like.received"
)

type InteractionPayload struct {
	ActorUserID  string `json:"actor_user_id"`
	TargetUserID string `json:"target_user_id"`
}

type MatchCreatedPayload struct {
	MatchID   string    `json:"match_id"`
	UserAID   string    `json:"user_a_id"`
	UserBID   string    `json:"user_b_id"`
	MatchedAt time.Time `json:"matched_at"`
}

type LikeReceivedPayload struct {
	TargetUserID string `json:"target_user_id"`
}
