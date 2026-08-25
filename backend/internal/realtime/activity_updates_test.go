package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestActivityUpdatesPublishToTheSelectedUser(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	updates := NewActivityUpdates(client)
	ctx := context.Background()

	subscription, err := updates.Subscribe(ctx, "owner-a")
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Close() })

	require.NoError(t, updates.Publish(ctx, "owner-a", "buy_food"))

	select {
	case message := <-subscription.Channel():
		require.Equal(t, "buy_food", message.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the activity update")
	}
}
