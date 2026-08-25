package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
)

const activityQueuePrefix = "activity_queue:"

// Each queue is one sorted set: the score is the enqueue sequence number whose
// absolute value fixes the member's FIFO position forever, and its sign carries
// the state — positive means the member's count is above zero and selectable,
// negative parks the member in place with a zero count until the count rises
// again. Counts live in a separate hash so they can move freely without ever
// touching the member's queue position.
type ActivityQueue struct {
	client redis.UniversalClient
}

func NewActivityQueue(client redis.UniversalClient) *ActivityQueue {
	return &ActivityQueue{client: client}
}

func (q *ActivityQueue) EnqueueOrdinary(ctx context.Context, activityType, uid string, count int64) error {
	return q.enqueue(ctx, activityType, "ordinary", uid, count)
}

func (q *ActivityQueue) EnqueuePriority(ctx context.Context, activityType, uid string, count int64) error {
	return q.enqueue(ctx, activityType, "priority", uid, count)
}

func (q *ActivityQueue) AddOrdinary(ctx context.Context, activityType, uid string, delta int64) error {
	return q.add(ctx, activityType, "ordinary", uid, delta)
}

func (q *ActivityQueue) AddPriority(ctx context.Context, activityType, uid string, delta int64) error {
	return q.add(ctx, activityType, "priority", uid, delta)
}

// Next returns the oldest active member that is neither the claimant itself
// nor one of the excluded members (publishers the claimant already received).
func (q *ActivityQueue) NextOrdinary(ctx context.Context, activityType, claimantUID string, exclude []string) (string, error) {
	return q.next(ctx, activityType, "ordinary", claimantUID, exclude)
}

// NextByCursor advances the ordinary queue's FIFO cursor and returns the
// member it lands on. Traversal follows the enqueue sequence (the absolute
// score) so publish order survives count changes; the claimant itself is
// always skipped, excluded members are skipped while anyone fresher remains,
// and once the queue runs out of fresh faces the cursor wraps to the oldest
// member and keeps cycling — entries are never consumed, parked, or removed
// by this, so the same queue serves claims forever.
func (q *ActivityQueue) NextByCursor(ctx context.Context, activityType, claimantUID string, exclude []string) (string, error) {
	args := make([]any, 0, len(exclude)+1)
	args = append(args, claimantUID)
	for _, uid := range exclude {
		args = append(args, uid)
	}
	uid, err := nextByCursorScript.Run(
		ctx,
		q.client,
		[]string{activityZSetKey(activityType, "ordinary"), activityCursorKey(activityType)},
		args...,
	).Text()
	if errors.Is(err, redis.Nil) || uid == "" {
		return "", domain.ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("select activity cursor entry: %w", err)
	}
	return uid, nil
}

func (q *ActivityQueue) NextPriority(ctx context.Context, activityType, claimantUID string, exclude []string) (string, error) {
	return q.next(ctx, activityType, "priority", claimantUID, exclude)
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

// Flush drops the queue data so the caller can rebuild it from the database.
// The cursor belongs to that state and resets with it. The list/member keys
// belong to the pre-zset layout and are swept here so a deploy over an
// existing Redis leaves no stale entries behind.
func (q *ActivityQueue) Flush(ctx context.Context, activityType string) error {
	if err := q.client.Del(ctx,
		activityZSetKey(activityType, "ordinary"),
		activityCountsKey(activityType, "ordinary"),
		activityCursorKey(activityType),
		activityLegacyListKey(activityType, "ordinary"),
		activityLegacyMemberKey(activityType, "ordinary"),
		activityZSetKey(activityType, "priority"),
		activityCountsKey(activityType, "priority"),
		activityLegacyListKey(activityType, "priority"),
		activityLegacyMemberKey(activityType, "priority"),
	).Err(); err != nil {
		return fmt.Errorf("flush activity queues: %w", err)
	}
	return nil
}

func (q *ActivityQueue) Reset(ctx context.Context, activityType string) error {
	if err := q.Flush(ctx, activityType); err != nil {
		return err
	}
	if err := q.client.Del(ctx, activitySeedKey(activityType)).Err(); err != nil {
		return fmt.Errorf("reset activity queues: %w", err)
	}
	return nil
}

func (q *ActivityQueue) enqueue(ctx context.Context, activityType, queueType, uid string, count int64) error {
	if err := enqueueActivityScript.Run(
		ctx,
		q.client,
		[]string{activityZSetKey(activityType, queueType), activityCountsKey(activityType, queueType), activitySeqKey()},
		uid, count, time.Now().UnixMilli(),
	).Err(); err != nil {
		return fmt.Errorf("enqueue %s activity entry: %w", queueType, err)
	}
	return nil
}

func (q *ActivityQueue) add(ctx context.Context, activityType, queueType, uid string, delta int64) error {
	if err := adjustActivityScript.Run(
		ctx,
		q.client,
		[]string{activityZSetKey(activityType, queueType), activityCountsKey(activityType, queueType), activitySeqKey()},
		uid, delta, time.Now().UnixMilli(),
	).Err(); err != nil {
		return fmt.Errorf("adjust %s activity count: %w", queueType, err)
	}
	return nil
}

func (q *ActivityQueue) next(ctx context.Context, activityType, queueType, claimantUID string, exclude []string) (string, error) {
	args := make([]any, 0, len(exclude)+1)
	args = append(args, claimantUID)
	for _, uid := range exclude {
		args = append(args, uid)
	}
	uid, err := nextActivityScript.Run(
		ctx,
		q.client,
		[]string{activityZSetKey(activityType, queueType)},
		args...,
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
		[]string{activityZSetKey(activityType, queueType), activityCountsKey(activityType, queueType)},
		uid,
	).Err(); err != nil {
		return fmt.Errorf("remove %s activity entry: %w", queueType, err)
	}
	return nil
}

func activityZSetKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":zset"
}

func activityCountsKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":counts"
}

func activityCursorKey(activityType string) string {
	return activityQueuePrefix + activityType + ":cursor"
}

func activitySeqKey() string {
	return activityQueuePrefix + "seq"
}

func activitySeedKey(activityType string) string {
	return activityQueuePrefix + activityType + ":seeded"
}

func activityLegacyListKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":list"
}

func activityLegacyMemberKey(activityType, queueType string) string {
	return activityQueuePrefix + activityType + ":" + queueType + ":members"
}

var enqueueActivityScript = redis.NewScript(`
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then
  return 0
end
redis.call('SET', KEYS[3], ARGV[3], 'NX')
local seq = redis.call('INCR', KEYS[3])
local count = tonumber(ARGV[2]) or 0
redis.call('HSET', KEYS[2], ARGV[1], count)
if count > 0 then
  redis.call('ZADD', KEYS[1], seq, ARGV[1])
else
  redis.call('ZADD', KEYS[1], -seq, ARGV[1])
end
return 1
`)

var adjustActivityScript = redis.NewScript(`
local count = tonumber(redis.call('HGET', KEYS[2], ARGV[1])) or 0
local updated = count + (tonumber(ARGV[2]) or 0)
redis.call('HSET', KEYS[2], ARGV[1], updated)
local score = redis.call('ZSCORE', KEYS[1], ARGV[1])
if score then
  score = tonumber(score)
  if (updated > 0 and score < 0) or (updated <= 0 and score > 0) then
    redis.call('ZADD', KEYS[1], -score, ARGV[1])
  end
else
  redis.call('SET', KEYS[3], ARGV[3], 'NX')
  local seq = redis.call('INCR', KEYS[3])
  if updated > 0 then
    redis.call('ZADD', KEYS[1], seq, ARGV[1])
  else
    redis.call('ZADD', KEYS[1], -seq, ARGV[1])
  end
end
return updated
`)

var nextActivityScript = redis.NewScript(`
local excluded = {[ARGV[1]] = true}
for i = 2, #ARGV do excluded[ARGV[i]] = true end
local members = redis.call('ZRANGEBYSCORE', KEYS[1], '(0', '+inf', 'LIMIT', '0', #ARGV + 1)
for _, uid in ipairs(members) do
  if not excluded[uid] then
    return uid
  end
end
return ''
`)

// nextByCursorScript advances the cursor stored at KEYS[2] and returns the
// member it lands on, in four tiers: the first member past the cursor that is
// neither the claimant nor excluded; failing that the earliest such member
// from the top of the queue (the wrap-around onto a fresh lap); failing that
// the next member past the cursor even though already served; failing that
// the earliest member that is merely not the claimant. The last two tiers
// keep the rotation spinning instead of parking on one member once everyone
// eligible has been served.
var nextByCursorScript = redis.NewScript(`
local self = ARGV[1]
local excluded = {}
for i = 2, #ARGV do excluded[ARGV[i]] = true end
local members = redis.call('ZRANGE', KEYS[1], 0, -1, 'WITHSCORES')
if #members == 0 then return '' end
local cursor = tonumber(redis.call('GET', KEYS[2]) or '0')
local freshAfter, freshAfterSeq, freshAny, freshAnySeq, anyAfter, anyAfterSeq, any, anySeq
for i = 1, #members, 2 do
  local uid = members[i]
  if uid ~= self then
    local seq = math.abs(tonumber(members[i + 1]))
    if not excluded[uid] then
      if not freshAnySeq or seq < freshAnySeq then freshAny, freshAnySeq = uid, seq end
      if seq > cursor and (not freshAfterSeq or seq < freshAfterSeq) then freshAfter, freshAfterSeq = uid, seq end
    end
    if seq > cursor and (not anyAfterSeq or seq < anyAfterSeq) then anyAfter, anyAfterSeq = uid, seq end
    if not anySeq or seq < anySeq then any, anySeq = uid, seq end
  end
end
local chosen, chosenSeq
if freshAfter then chosen, chosenSeq = freshAfter, freshAfterSeq
elseif freshAny then chosen, chosenSeq = freshAny, freshAnySeq
elseif anyAfter then chosen, chosenSeq = anyAfter, anyAfterSeq
else chosen, chosenSeq = any, anySeq end
if not chosen then return '' end
redis.call('SET', KEYS[2], chosenSeq)
return chosen
`)

var removeActivityScript = redis.NewScript(`
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('HDEL', KEYS[2], ARGV[1])
return 1
`)
