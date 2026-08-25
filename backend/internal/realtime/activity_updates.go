package realtime

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const activityUpdateChannelPrefix = "activity_updates:"

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

func (u *ActivityUpdates) Subscribe(ctx context.Context, uid string) (*redis.PubSub, error) {
	subscription := u.client.Subscribe(ctx, activityUpdateChannel(uid))
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, fmt.Errorf("subscribe to activity updates: %w", err)
	}
	return subscription, nil
}

func activityUpdateChannel(uid string) string {
	return activityUpdateChannelPrefix + uid
}
