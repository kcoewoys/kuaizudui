package queue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestActivityQueueSkipsZeroCountAndKeepsPosition(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 0))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 1))

	uid, err := activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	require.Equal(t, "user-a", uid)

	// Zero-count members stay parked: exhausting a's count skips it while
	// keeping the entry, and raising b's count restores its original position
	// between a and c.
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-a", -1))
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	require.Equal(t, "user-c", uid)

	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-b", 1))
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-c", -1))
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)
}

func TestActivityQueueNextSkipsClaimantAndIgnoresDeadSelf(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 1))

	uid, err := activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-a", nil)
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-b", nil)
	require.NoError(t, err)
	require.Equal(t, "user-a", uid)

	// The claimant itself at the head is skipped, and a zero-count claimant
	// does not shadow the rest of the queue.
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-a", -1))
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-a", nil)
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	// Excluded members are passed over even with credit left; the selection
	// keeps the FIFO order of everyone still eligible.
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	uid, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-c", []string{"user-b"})
	require.NoError(t, err)
	require.Equal(t, "user-a", uid)
	_, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "user-c", []string{"user-a", "user-b"})
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityPriorityQueueFallsEmptyWhenItOnlyContainsClaimant(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityDailyCash, "user-a", 2))
	_, err := activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)

	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityDailyCash, "user-b", 1))
	uid, err := activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a", nil)
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	require.NoError(t, activityQueue.RemovePriority(ctx, domain.ActivityDailyCash, "user-b"))
	_, err = activityQueue.NextPriority(ctx, domain.ActivityDailyCash, "user-a", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityQueueEnqueueIsIdempotent(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	// Re-publishing must not reset the count or jump the queue position.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 5))
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-a", -1))

	_, err := activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityQueueResetClearsEntriesAndSeed(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityBuyFood, "user-b", 1))
	require.NoError(t, activityQueue.MarkSeeded(ctx, domain.ActivityBuyFood))
	require.NoError(t, activityQueue.Reset(ctx, domain.ActivityBuyFood))

	seeded, err := activityQueue.Seeded(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.False(t, seeded)
	_, err = activityQueue.NextOrdinary(ctx, domain.ActivityBuyFood, "other", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
	_, err = activityQueue.NextPriority(ctx, domain.ActivityBuyFood, "other", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityQueueCursorWalksFIFOAndLoops(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	// Mixed counts must not affect the cursor's publish order, and a zero
	// count parks nobody — every member keeps receiving turns.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 0))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 2))

	for _, expected := range []string{"user-a", "user-b", "user-c", "user-a", "user-b", "user-c"} {
		uid, err := activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
		require.NoError(t, err)
		require.Equal(t, expected, uid)
	}
}

func TestActivityQueueCursorSkipsSelfAndAlreadyServed(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 1))

	// The claimant itself is never served.
	uid, err := activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-b", nil)
	require.NoError(t, err)
	require.Equal(t, "user-a", uid)
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-b", nil)
	require.NoError(t, err)
	require.Equal(t, "user-c", uid)

	// Already-served members are skipped while someone fresher remains...
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b"})
	require.NoError(t, err)
	require.Equal(t, "user-c", uid)

	// ...and once nobody fresh is left the cursor keeps rotating through the
	// already-served members instead of parking on the oldest one.
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.NoError(t, err)
	require.Equal(t, "user-c", uid)
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.NoError(t, err)
	require.Equal(t, "user-b", uid)

	// A queue holding only the claimant serves nobody.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityDailyCash, "solo", 1))
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityDailyCash, "solo", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}
