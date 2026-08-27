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

	// Mixed counts must not affect the cursor's publish order, but a zero
	// count parks the member: only publishers holding chances take turns.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 0))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 2))

	for _, expected := range []string{"user-a", "user-c", "user-a", "user-c"} {
		uid, err := activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
		require.NoError(t, err)
		require.Equal(t, expected, uid)
	}

	// Earning a chance back re-activates the parked member at its original
	// publish position; the wrap-around resumes from the oldest active member.
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-b", 1))
	for _, expected := range []string{"user-a", "user-b", "user-c"} {
		uid, err := activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
		require.NoError(t, err)
		require.Equal(t, expected, uid)
	}

	// A late publisher only ever joins behind the pending rotation: serving
	// the tail member resets the cursor to the head, so the newcomer waits
	// their turn instead of cutting in front of members not yet served this
	// lap — the cursor is moved by claims alone, never by publishing.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-d", 2))
	for _, expected := range []string{"user-a", "user-b", "user-c", "user-d", "user-a"} {
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

	// ...and once nobody fresh is left the cursor stops: an already-served
	// member is never handed out a second time.
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.ErrorIs(t, err, domain.ErrQueueEmpty)

	// Members whose ordinary chances are used up are parked (negative score)
	// and skipped until a claim click earns them a fresh chance.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-d", 0))
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-d", 1))
	uid, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "user-a", []string{"user-b", "user-c"})
	require.NoError(t, err)
	require.Equal(t, "user-d", uid)

	// A queue holding only the claimant serves nobody.
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityDailyCash, "solo", 1))
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityDailyCash, "solo", nil)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}

func TestActivityQueueStatusReportsCursorRankAndTotals(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	activityQueue := NewActivityQueue(client)
	ctx := context.Background()

	// Without any member the sorted sets do not exist, which is what the
	// admin panel treats as "queue not created yet".
	status, err := activityQueue.OrdinaryStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.False(t, status.Created)
	require.Zero(t, status.Total)
	require.Zero(t, status.Position)
	require.Zero(t, status.CursorSeq)
	status, err = activityQueue.PriorityStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.False(t, status.Created)
	require.Zero(t, status.Total)

	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-a", 1))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-b", 0))
	require.NoError(t, activityQueue.EnqueueOrdinary(ctx, domain.ActivityBuyFood, "user-c", 2))

	// A fresh queue has no cursor yet, so every cursor number reads zero —
	// parked members still count toward the total.
	status, err = activityQueue.OrdinaryStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.True(t, status.Created)
	require.Equal(t, int64(3), status.Total)
	require.Zero(t, status.Position)
	require.Zero(t, status.CursorSeq)

	// The reported rank follows the member the cursor last landed on. The raw
	// sequence is timestamp-seeded, so only its growth is asserted. The parked
	// member earns a chance first because the cursor only walks active
	// publishers.
	require.NoError(t, activityQueue.AddOrdinary(ctx, domain.ActivityBuyFood, "user-b", 1))
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	status, err = activityQueue.OrdinaryStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.Positive(t, status.CursorSeq)
	require.Equal(t, int64(1), status.Position)
	firstCursor := status.CursorSeq

	// The second claim lands past the first; the third serves the tail member
	// and completes the lap, resetting the stored cursor to zero so a member
	// publishing later joins behind the rotation instead of in front of it.
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	status, err = activityQueue.OrdinaryStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.Greater(t, status.CursorSeq, firstCursor)
	_, err = activityQueue.NextByCursor(ctx, domain.ActivityBuyFood, "other", nil)
	require.NoError(t, err)
	status, err = activityQueue.OrdinaryStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.Zero(t, status.CursorSeq)
	require.Zero(t, status.Position)

	// The priority queue keeps no cursor of its own: only existence and the
	// count of members still holding chances are meaningful there.
	require.NoError(t, activityQueue.EnqueuePriority(ctx, domain.ActivityBuyFood, "user-d", 1))
	status, err = activityQueue.PriorityStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.True(t, status.Created)
	require.Equal(t, int64(1), status.Total)
	require.Zero(t, status.Position)
	require.Zero(t, status.CursorSeq)

	// Serving out a member's chances parks them in place: the queue still
	// exists, but the admin count reads zero again.
	require.NoError(t, activityQueue.AddPriority(ctx, domain.ActivityBuyFood, "user-d", -1))
	status, err = activityQueue.PriorityStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.True(t, status.Created)
	require.Zero(t, status.Total)

	require.NoError(t, activityQueue.RemovePriority(ctx, domain.ActivityBuyFood, "user-d"))
	status, err = activityQueue.PriorityStatus(ctx, domain.ActivityBuyFood)
	require.NoError(t, err)
	require.False(t, status.Created)
	require.Zero(t, status.Total)
}
