package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kurt4ins/drizzy/pkg/events"
	"github.com/kurt4ins/drizzy/pkg/rabbitmq"
	"github.com/kurt4ins/drizzy/ranking-service/internal/repository"
)

type InteractionConsumer struct {
	repo      *repository.Repository
	publisher *rabbitmq.Publisher
	consumer  *rabbitmq.Consumer
}

func NewInteractionConsumer(
	repo *repository.Repository,
	pub *rabbitmq.Publisher,
	rabbitURL string,
) (*InteractionConsumer, error) {
	c, err := rabbitmq.NewConsumer(rabbitURL, rabbitmq.QueueBehaviorAggregate, rabbitmq.RoutingKeyInteractionAll)
	if err != nil {
		return nil, fmt.Errorf("new consumer: %w", err)
	}
	return &InteractionConsumer{repo: repo, publisher: pub, consumer: c}, nil
}

func (ic *InteractionConsumer) Close() { ic.consumer.Close() }

func (ic *InteractionConsumer) Run(ctx context.Context) error {
	log.Println("interaction consumer started")
	return ic.consumer.Consume(ctx, func(body []byte) error {
		return ic.handle(ctx, body)
	})
}

func (ic *InteractionConsumer) handle(ctx context.Context, body []byte) error {
	var env events.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	payloadBytes, err := json.Marshal(env.Payload)
	if err != nil {
		return fmt.Errorf("re-marshal payload: %w", err)
	}
	var p events.InteractionPayload
	if err = json.Unmarshal(payloadBytes, &p); err != nil {
		return fmt.Errorf("unmarshal interaction payload: %w", err)
	}

	action := ""
	switch env.Type {
	case events.TypeInteractionLiked:
		action = "like"
	case events.TypeInteractionSkipped:
		action = "skip"
	default:
		log.Printf("interaction consumer: unknown event type %q, skipping", env.Type)
		return nil
	}

	if err = ic.repo.RecordInteraction(ctx, p.ActorUserID, p.TargetUserID, action); err != nil {
		return err
	}
	if err = ic.repo.UpdateBehaviorStats(ctx, p.TargetUserID, action); err != nil {
		return err
	}

	if action != "like" {
		return nil
	}

	reciprocal, err := ic.repo.IsLiked(ctx, p.TargetUserID, p.ActorUserID)
	if err != nil {
		return err
	}
	if !reciprocal {
		likeEnv := events.Envelope{
			EventID:       newUUID(),
			Type:          events.TypeLikeReceived,
			OccurredAt:    time.Now(),
			SchemaVersion: 1,
			Payload:       events.LikeReceivedPayload{TargetUserID: p.TargetUserID},
		}
		if err = ic.publisher.Publish(ctx, events.TypeLikeReceived, likeEnv); err != nil {
			log.Printf("interaction consumer: publish like.received: %v", err)
		}
		return nil
	}

	match, created, err := ic.repo.CreateMatch(ctx, p.ActorUserID, p.TargetUserID)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}

	log.Printf("match created: %s (%s <-> %s)", match.ID, match.UserAID, match.UserBID)

	matchEnv := events.Envelope{
		EventID:       newUUID(),
		Type:          events.TypeMatchCreated,
		OccurredAt:    match.MatchedAt,
		SchemaVersion: 1,
		Payload: events.MatchCreatedPayload{
			MatchID:   match.ID,
			UserAID:   match.UserAID,
			UserBID:   match.UserBID,
			MatchedAt: match.MatchedAt,
		},
	}
	if err = ic.publisher.Publish(ctx, events.TypeMatchCreated, matchEnv); err != nil {
		return fmt.Errorf("publish match.created: %w", err)
	}
	return nil
}
