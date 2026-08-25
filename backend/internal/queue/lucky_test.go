package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLuckyQueueIsFIFOAndSkipsOwnEntries(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	q := NewLuckyQueue(client, time.Hour)
	ctx := context.Background()

	require.NoError(t, q.Publish(ctx, LuckyEntry{ID: 1, UID: "owner-a"}))
	require.NoError(t, q.Publish(ctx, LuckyEntry{ID: 2, UID: "owner-b"}))
	require.NoError(t, q.Publish(ctx, LuckyEntry{ID: 3, UID: "owner-c"}))

	entry, err := q.Receive(ctx, "owner-a")
	require.NoError(t, err)
	require.Equal(t, uint(2), entry.ID, "the oldest foreign entry must be returned")

	entry, err = q.Receive(ctx, "new-user")
	require.NoError(t, err)
	require.Equal(t, uint(1), entry.ID, "skipped own entry must retain FIFO priority")
}

func TestLuckyQueueDoesNotReturnAlreadyClaimedEntry(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	q := NewLuckyQueue(client, time.Hour)
	ctx := context.Background()

	require.NoError(t, q.Publish(ctx, LuckyEntry{ID: 1, UID: "owner"}))
	claimed, err := q.Claim(ctx, 1, "first-user")
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = q.Receive(ctx, "second-user")
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
}
