package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
)

const activityQueuePrefix = "activity_queue:"

type ActivityQueue struct {
	client redis.UniversalClient
}

func NewActivityQueue(client redis.UniversalClient) *ActivityQueue {
	return &ActivityQueue{client: client}
}

func (q *ActivityQueue) EnqueueOrdinary(ctx context.Context, activityType, uid string) error {
	return q.enqueue(ctx, activityType, "ordinary", uid)
}

func (q *ActivityQueue) EnqueuePriority(ctx context.Context, activityType, uid string) error {
	return q.enqueue(ctx, activityType, "priority", uid)
}

func (q *ActivityQueue) NextOrdinary(ctx context.Context, activityType, claimantUID string) (string, error) {
	return q.nextOther(ctx, activityType, "ordinary", claimantUID)
}

func (q *ActivityQueue) NextPriority(ctx context.Context, activityType, claimantUID string) (string, error) {
	return q.nextOther(ctx, activityType, "priority", claimantUID)
}

func (q *ActivityQueue) RemoveOrdinary(ctx context.Context, activityType, uid string) error {
	return q.remove(ctx, activityType, "ordinary", uid)
}

func (q *ActivityQueue) RemovePriority(ctx context.Context, activityType, uid string) error {
	return q.remove(ctx, activityType, "priority", uid)
}

func (q *ActivityQueue) Seeded(ctx context.Context, activityType string) (bool, error) {
	exists, err := q.client.Exists(ctx, activitySeedKey(activityType)).Result()
	if err != nil {
		return false, fmt.Errorf("check activity queue seed: %w", err)
	}
	return exists > 0, nil
}

func (q *ActivityQueue) MarkSeeded(ctx context.Context, activityType string) error {
	if err := q.client.Set(ctx, activitySeedKey(activityType), "1", 0).Err(); err != nil {
		return fmt.Errorf("mark activity queue seeded: %w", err)
	}
	return nil
}

func (q *ActivityQueue) InvalidateSeed(ctx context.Context, activityType string) error {
	if err := q.client.Del(ctx, activitySeedKey(activityType)).Err(); err != nil {
		return fmt.Errorf("invalidate activity queue seed: %w", err)
	}
	return nil
}

func (q *ActivityQueue) Reset(ctx context.Context, activityType string) error {
	if err := q.client.Del(ctx,
		activityListKey(activityType, "ordinary"),
		activityMemberKey(activityType, "ordinary"),
		activityListKey(activityType, "priority"),
		activityMemberKey(activityType, "priority"),
		activitySeedKey(activityType),
	).Err(); err != nil {
		return fmt.Errorf("reset activity queues: %w", err)
	}
	return nil
}

func (q *ActivityQueue) enqueue(ctx context.Context, activityType, queueType, uid string) error {
	if err := enqueueActivityScript.Run(
		ctx,
		q.client,
		[]string{activityListKey(activityType, queueType), activityMemberKey(activityType, queueType)},
		uid,
	).Err(); err != nil {
		return fmt.Errorf("enqueue %s activity entry: %w", queueType, err)
	}
	return nil
}

func (q *ActivityQueue) nextOther(ctx context.Context, activityType, queueType, claimantUID string) (string, error) {
	uid, err := nextOtherActivityScript.Run(
		ctx,
		q.client,
		[]string{activityListKey(activityType, queueType)},
		claimantUID,
	).Text()
	if errors.Is(err, redis.Nil) || uid == "" {
		return "", domain.ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("select %s activity entry: %w", queueType, err)
	}
	return uid, nil
}

func (q *ActivityQueue) remove(ctx context.Context, activityType, queueType, uid string) error {
	if err := removeActivityScript.Run(
		ctx,
		q.client,
		[]string{activityListKey(activityType, queueType), activityMemberKey(activityType, queueType)},
		uid,
	).Err(); err != nil {
		return fmt.Errorf("remove %s activity entry: %w", queueType, err)
	}
	return nil
}

func activityListKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":list"
}

func activityMemberKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":members"
}

func activitySeedKey(activityType string) string {
	return activityQueuePrefix + activityType + ":seeded"
}

var enqueueActivityScript = redis.NewScript(`
if redis.call('SADD', KEYS[2], ARGV[1]) == 1 then
  redis.call('RPUSH', KEYS[1], ARGV[1])
end
return 1
`)

var nextOtherActivityScript = redis.NewScript(`
local queueLength = redis.call('LLEN', KEYS[1])

for i = 1, queueLength do
  local uid = redis.call('LPOP', KEYS[1])
  if not uid then
    break
  end
  redis.call('RPUSH', KEYS[1], uid)
  if uid ~= ARGV[1] then
    return uid
  end
end

return ''
`)

var removeActivityScript = redis.NewScript(`
redis.call('LREM', KEYS[1], 0, ARGV[1])
redis.call('SREM', KEYS[2], ARGV[1])
return 1
`)
