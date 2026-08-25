package queue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestActivityQueueIsUniqueRoundRobinAndSkipsClaimant(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a"))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b"))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c"))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b"))

	uid, err := activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-a")
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-a")
	require.NoError(t, err)
	require.Equal(t, "user-c", uid)

	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-c")
	require.NoError(t, err)
	require.Equal(t, "user-a", uid)
}

func TestActivityPriorityQueueFallsEmptyWhenItOnlyContainsClaimant(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityDailyCash, "user-a"))
	_, err := activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a")
	require.ErrorIs(t, err, domain.ErrQueueEmpty)

	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityDailyCash, "user-b"))
	uid, err := activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a")
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	require.NoError(t, activityQueue.RemovePriority(ctx, domain.ActivityDailyCash, "user-b"))
	_, err = activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a")
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityQueueResetClearsEntriesAndSeed(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a"))
	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityBuyFood, "user-b"))
	require.NoError(t, activityQueue.MarkSeeded(ctx, domain.ActivityBuyFood))
	require.NoError(t, activityQueue.Reset(ctx, domain.ActivityBuyFood))

	seeded, err := activityQueue.Seeded(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.False(t, seeded)
	_, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other")
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
	_, err = activityQueue.NextPriority(ctx, domain.ActivityBuyFood, "other")
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}
