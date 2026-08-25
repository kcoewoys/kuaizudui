package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	luckyQueueKey = "lucky_queue"
	luckyClaimKey = "lucky_used:"
)

type LuckyEntry struct {
	ID  uint
	UID string
}

type LuckyQueue struct {
	client   redis.UniversalClient
	claimTTL time.Duration
}

func NewLuckyQueue(client redis.UniversalClient, claimTTL time.Duration) *LuckyQueue {
	return &LuckyQueue{client: client, claimTTL: claimTTL}
}

func (q *LuckyQueue) Publish(ctx context.Context, entry LuckyEntry) error {
	if err := q.client.LPush(ctx, luckyQueueKey, encode(entry)).Err(); err != nil {
		return fmt.Errorf("publish lucky queue entry: %w", err)
	}
	return nil
}

func (q *LuckyQueue) Receive(ctx context.Context, requesterUID string) (LuckyEntry, error) {
	result, err := receiveLuckyScript.Run(
		ctx,
		q.client,
		[]string{luckyQueueKey, luckyClaimKey},
		requesterUID,
		q.claimTTL.Milliseconds(),
	).Text()
	if errors.Is(err, redis.Nil) {
		return LuckyEntry{}, domain.ErrQueueEmpty
	}
	if err != nil {
		return LuckyEntry{}, fmt.Errorf("receive lucky queue entry: %w", err)
	}
	if result == "" {
		return LuckyEntry{}, domain.ErrQueueEmpty
	}
	entry, err := decode(result)
	if err != nil {
		return LuckyEntry{}, err
	}
	return entry, nil
}

func (q *LuckyQueue) Claim(ctx context.Context, id uint, requesterUID string) (bool, error) {
	claimed, err := q.client.SetNX(ctx, claimKey(id), requesterUID, q.claimTTL).Result()
	if err != nil {
		return false, fmt.Errorf("claim lucky code: %w", err)
	}
	return claimed, nil
}

func (q *LuckyQueue) Release(ctx context.Context, entry LuckyEntry, requeue bool) error {
	pipe := q.client.TxPipeline()
	pipe.Del(ctx, claimKey(entry.ID))
	if requeue {
		pipe.RPush(ctx, luckyQueueKey, encode(entry))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("release lucky code: %w", err)
	}
	return nil
}

func (q *LuckyQueue) Claimed(ctx context.Context, id uint) (bool, error) {
	exists, err := q.client.Exists(ctx, claimKey(id)).Result()
	if err != nil {
		return false, fmt.Errorf("check lucky claim: %w", err)
	}
	return exists > 0, nil
}

func claimKey(id uint) string { return luckyClaimKey + strconv.FormatUint(uint64(id), 10) }

func encode(entry LuckyEntry) string {
	return strconv.FormatUint(uint64(entry.ID), 10) + "|" + entry.UID
}

func decode(value string) (LuckyEntry, error) {
	idText, uid, ok := strings.Cut(value, "|")
	if !ok || uid == "" {
		return LuckyEntry{}, fmt.Errorf("decode lucky queue entry %q", value)
	}
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil {
		return LuckyEntry{}, fmt.Errorf("decode lucky queue id: %w", err)
	}
	return LuckyEntry{ID: uint(id), UID: uid}, nil
}

var receiveLuckyScript = redis.NewScript(`
local queueLength = redis.call('LLEN', KEYS[1])
local skipped = {}

for i = 1, queueLength do
  local item = redis.call('RPOP', KEYS[1])
  if not item then
    break
  end

  local separator = string.find(item, '|')
  if separator then
    local codeID = string.sub(item, 1, separator - 1)
    local ownerUID = string.sub(item, separator + 1)

    if ownerUID == ARGV[1] then
      table.insert(skipped, item)
    else
      local claimed = redis.call('SET', KEYS[2] .. codeID, ARGV[1], 'NX', 'PX', ARGV[2])
      if claimed then
        for j = #skipped, 1, -1 do
          redis.call('RPUSH', KEYS[1], skipped[j])
        end
        return item
      end
    end
  end
end

for j = #skipped, 1, -1 do
  redis.call('RPUSH', KEYS[1], skipped[j])
end
return ''
`)
