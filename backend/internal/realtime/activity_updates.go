package realtime

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	activityUpdateChannelPrefix  = "activity_updates:"
	activityBroadcastPrefix      = "activity_updates:activity:"
)

type ActivityUpdates struct {
	client redis.UniversalClient
}

func NewActivityUpdates(client redis.UniversalClient) *ActivityUpdates {
	return &ActivityUpdates{client: client}
}

func (u *ActivityUpdates) Publish(ctx context.Context, uid, activityType string) error {
	if err := u.client.Publish(ctx, activityUpdateChannel(uid), activityType).Err(); err != nil {
		return fmt.Errorf("publish activity update: %w", err)
	}
	return nil
}

// PublishAll fans an event out to every client watching the activity type —
// queue composition changes (publish, boost, claim) reshape everyone's
// availability, not only the owner involved.
func (u *ActivityUpdates) PublishAll(ctx context.Context, activityType string) error {
	if err := u.client.Publish(ctx, activityBroadcastChannel(activityType), activityType).Err(); err != nil {
		return fmt.Errorf("publish activity broadcast: %w", err)
	}
	return nil
}

// Subscribe follows the user's personal channel plus the broadcast channel of
// each given activity type; empty types are ignored.
func (u *ActivityUpdates) Subscribe(ctx context.Context, uid string, activityTypes ...string) (*redis.PubSub, error) {
	channels := []string{activityUpdateChannel(uid)}
	for _, activityType := range activityTypes {
		if activityType != "" {
			channels = append(channels, activityBroadcastChannel(activityType))
		}
	}
	subscription := u.client.Subscribe(ctx, channels...)
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, fmt.Errorf("subscribe to activity updates: %w", err)
	}
	return subscription, nil
}

func activityUpdateChannel(uid string) string {
	return activityUpdateChannelPrefix + uid
}

func activityBroadcastChannel(activityType string) string {
	return activityBroadcastPrefix + activityType
}
