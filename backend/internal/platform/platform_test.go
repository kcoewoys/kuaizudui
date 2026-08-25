package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/database"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testPlatform(t *testing.T) (*Platform, *gorm.DB, *redis.Client, config.Config) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	cfg := config.Config{
		Business: config.BusinessConfig{
			AdminPhone: "13800000000", LuckyCodeMinLength: 8, LuckyCodeMaxLength: 9,
			ActivityContentMaxLength: 200,
			FirstVisitTTL:            config.Duration(time.Hour),
			LuckyClaimTTL:            config.Duration(time.Hour),
		},
		Security: config.SecurityConfig{
			AdminSessionTTL: config.Duration(time.Hour), AdminTokenSecret: "test-secret",
		},
	}
	return New(db, redisClient, cfg), db, redisClient, cfg
}

func setOrdinaryCredit(
	t *testing.T,
	app *Platform,
	db *gorm.DB,
	ctx context.Context,
	uid, activityType string,
	credit int64,
) {
	t.Helper()
	result := db.WithContext(ctx).Model(&domain.ActivityContent{}).
		Where("uid = ? AND type = ?", uid, activityType).
		Updates(map[string]any{"ordinary_credit": credit, "used_count": credit, "ordinary_rounds": 0})
	require.NoError(t, result.Error)
	require.Equal(t, int64(1), result.RowsAffected)
	if credit > 0 {
		require.NoError(t, app.activity.EnqueueOrdinary(ctx, activityType, uid))
	} else {
		require.NoError(t, app.activity.RemoveOrdinary(ctx, activityType, uid))
	}
}

func TestActivityTypesUseTheSameRulesAndIndependentRecords(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	types := []string{
		domain.ActivityBuyFood,
		domain.ActivityCashTurntable,
		domain.ActivityCashMonopoly,
		domain.ActivityDailyCash,
	}
	for _, activityType := range types {
		content := "invite-content-for-" + activityType
		published, err := app.PublishActivity(ctx, "user-a", activityType, content)
		require.NoError(t, err)
		require.True(t, published.Published)

		detail, err := app.ActivityDetail(ctx, "user-a", activityType)
		require.NoError(t, err)
		require.Equal(t, content, detail.Content)

		peerContent := "peer-content-for-" + activityType
		_, err = app.PublishActivity(ctx, "user-b", activityType, peerContent)
		require.NoError(t, err)
		setOrdinaryCredit(t, app, db, ctx, "user-b", activityType, 1)
		claimed, err := app.UseActivity(ctx, "user-a", activityType)
		require.NoError(t, err)
		require.Equal(t, peerContent, claimed.Content)
		require.Equal(t, "ordinary", claimed.Source)
		require.Equal(t, int64(1), claimed.State.ClaimCount)
	}

	_, err := app.PublishActivity(ctx, "user-a", domain.ActivityCashTurntable, strings.Repeat("现", 201))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestActivityClaimPublishesRealtimeUpdateToOwner(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityBuyFood

	_, err := app.PublishActivity(ctx, "owner-a", activityType, "owner invitation")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "claimant-b", activityType, "claimant invitation")
	require.NoError(t, err)
	setOrdinaryCredit(t, app, db, ctx, "owner-a", activityType, 1)
	subscription, err := app.updates.Subscribe(ctx, "owner-a")
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Close() })

	claimed, err := app.UseActivity(ctx, "claimant-b", activityType)
	require.NoError(t, err)
	require.Equal(t, "owner invitation", claimed.Content)

	select {
	case message := <-subscription.Channel():
		require.Equal(t, activityType, message.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the owner's realtime update")
	}
}

func TestActivityClaimIncrementsClaimantCountAndContentOwnerRound(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityBuyFood

	_, err := app.PublishActivity(ctx, "content-owner", activityType, "owner invitation")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "claimant", activityType, "claimant invitation")
	require.NoError(t, err)
	setOrdinaryCredit(t, app, db, ctx, "content-owner", activityType, 1)
	claimed, err := app.UseActivity(ctx, "claimant", activityType)
	require.NoError(t, err)
	require.Equal(t, "owner invitation", claimed.Content)
	require.Equal(t, int64(1), claimed.State.ClaimCount)
	require.Zero(t, claimed.State.OrdinaryRounds)

	owner, err := app.ActivityDetail(ctx, "content-owner", activityType)
	require.NoError(t, err)
	require.Equal(t, int64(1), owner.OrdinaryRounds)
	var claimantRecord domain.ActivityContent
	require.NoError(t, db.Where("uid = ? AND type = ?", "claimant", activityType).First(&claimantRecord).Error)
	require.Equal(t, int64(1), claimantRecord.OrdinaryCredit)
	var ownerRecord domain.ActivityContent
	require.NoError(t, db.Where("uid = ? AND type = ?", "content-owner", activityType).First(&ownerRecord).Error)
	require.Zero(t, ownerRecord.OrdinaryCredit)
}

func TestActivityClaimStopsWhenOwnerRoundsReachClaimCount(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityBuyFood

	_, err := app.PublishActivity(ctx, "content-owner", activityType, "owner invitation")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "claimant", activityType, "claimant invitation")
	require.NoError(t, err)
	require.NoError(t, db.Model(&domain.ActivityContent{}).
		Where("uid = ? AND type = ?", "content-owner", activityType).
		Updates(map[string]any{"used_count": 1, "ordinary_rounds": 1, "ordinary_credit": 1}).Error)
	require.NoError(t, app.activity.EnqueueOrdinary(ctx, activityType, "content-owner"))

	_, err = app.UseActivity(ctx, "claimant", activityType)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
	owner, err := app.ActivityDetail(ctx, "content-owner", activityType)
	require.NoError(t, err)
	require.Equal(t, int64(1), owner.OrdinaryRounds)
}

func TestActivityAvailabilityRequiresAnotherEligiblePublisher(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityBuyFood

	_, err := app.PublishActivity(ctx, "user-a", activityType, "invite-a")
	require.NoError(t, err)
	userA, err := app.ActivityDetail(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.False(t, userA.CanClaim)

	_, err = app.PublishActivity(ctx, "user-b", activityType, "invite-b")
	require.NoError(t, err)
	setOrdinaryCredit(t, app, db, ctx, "user-b", activityType, 1)
	userA, err = app.ActivityDetail(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.True(t, userA.CanClaim)
	userB, err := app.ActivityDetail(ctx, "user-b", activityType)
	require.NoError(t, err)
	require.False(t, userB.CanClaim)

	claimed, err := app.UseActivity(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.False(t, claimed.State.CanClaim)
	userB, err = app.ActivityDetail(ctx, "user-b", activityType)
	require.NoError(t, err)
	require.True(t, userB.CanClaim)
}

func TestLuckyReceiveIsFIFOAndNeverReturnsOwnCode(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()
	first, err := app.PublishLucky(ctx, "user-a", "12345678")
	require.NoError(t, err)
	second, err := app.PublishLucky(ctx, "user-b", "23456789")
	require.NoError(t, err)
	_, err = app.PublishLucky(ctx, "user-c", "34567890")
	require.NoError(t, err)

	listed, err := app.ListLucky(ctx, "user-a", 20)
	require.NoError(t, err)
	require.Len(t, listed, 3)
	require.True(t, listed[0].IsOwn)
	require.Equal(t, "我", listed[0].Source)
	require.False(t, listed[1].IsOwn)
	require.Equal(t, "u***b", listed[1].Source)
	require.Equal(t, "u***c", listed[2].Source)

	received, err := app.ReceiveLucky(ctx, "user-a")
	require.NoError(t, err)
	require.Equal(t, second.ID, received.ID)
	require.NotEqual(t, first.ID, received.ID)

	received, err = app.ReceiveLucky(ctx, "user-d")
	require.NoError(t, err)
	require.Equal(t, first.ID, received.ID)
}

func TestMaskIdentifierKeepsACompactPrefixAndSuffix(t *testing.T) {
	require.Equal(t, "abc***890", maskIdentifier("abcdefghijklmnop1234567890"))
	require.Equal(t, "u***b", maskIdentifier("user-b"))
	require.Equal(t, "我***", maskIdentifier("我"))
}

func TestExchangeCodeCanOnlyBeUsedOnce(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&domain.ExchangeCode{
		Code: "EAOK-ONCE", Points: 50, Status: domain.ExchangeStatusUnused,
	}).Error)

	result, err := app.Exchange(ctx, "user-a", "eaok-once")
	require.NoError(t, err)
	require.Equal(t, int64(50), result.TotalPoints)

	_, err = app.Exchange(ctx, "user-b", "EAOK-ONCE")
	require.True(t, errors.Is(err, domain.ErrAlreadyUsed))
}

func TestAdminLoginRechargeAndCodeGeneration(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()
	bound, err := app.BindPhone(ctx, "user-a", "13900000000")
	require.NoError(t, err)
	require.NotNil(t, bound.Phone)

	login, err := app.AdminLogin(ctx, "13800000000")
	require.NoError(t, err)
	adminPhone, err := app.AuthenticateAdmin(ctx, login.Token)
	require.NoError(t, err)

	user, err := app.AdminRecharge(ctx, adminPhone, "13900000000", 25)
	require.NoError(t, err)
	require.Equal(t, int64(25), user.Points)

	codes, err := app.AdminCreateExchangeCodes(ctx, 10, 3, "TEST-")
	require.NoError(t, err)
	require.Len(t, codes, 3)
	require.True(t, strings.HasPrefix(codes[0].Code, "TEST-"))
}

func TestFeedbackSubmissionAndAdminListing(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()

	first, err := app.SubmitFeedback(ctx, "anonymous-user", "  希望增加夜间模式  ")
	require.NoError(t, err)
	require.Equal(t, "希望增加夜间模式", first.Content)

	_, err = app.BindPhone(ctx, "bound-user", "13900000001")
	require.NoError(t, err)
	second, err := app.SubmitFeedback(ctx, "bound-user", "活动入口很好找")
	require.NoError(t, err)

	items, err := app.AdminListFeedback(ctx, 20, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, second.ID, items[0].ID)
	require.Equal(t, "13900000001", *items[0].Phone)
	require.Equal(t, first.ID, items[1].ID)
	require.Nil(t, items[1].Phone)
	require.Equal(t, "anonymous-user", items[1].UID)

	_, err = app.SubmitFeedback(ctx, "anonymous-user", "   ")
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = app.SubmitFeedback(ctx, "anonymous-user", strings.Repeat("反", 501))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestPhoneBindingIsIdempotentButCannotBeReassigned(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()

	first, err := app.BindPhone(ctx, "user-a", "13900000000")
	require.NoError(t, err)
	require.Equal(t, "13900000000", *first.Phone)

	second, err := app.BindPhone(ctx, "user-a", "13900000000")
	require.NoError(t, err)
	require.Equal(t, first.UID, second.UID)

	_, err = app.BindPhone(ctx, "user-a", "13900000001")
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestReferralUsesBoundPhoneAndCannotBeReassigned(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()

	_, err := app.BindPhone(ctx, "inviter", "13900000000")
	require.NoError(t, err)
	_, err = app.BindPhone(ctx, "other-inviter", "13900000001")
	require.NoError(t, err)

	bound, err := app.ApplyReferral(ctx, "new-user", "13900000000")
	require.NoError(t, err)
	require.NotNil(t, bound.InvitedByPhone)
	require.Equal(t, "13900000000", *bound.InvitedByPhone)
	require.Zero(t, bound.Points)
	inviter, err := app.UserInfo(ctx, "inviter")
	require.NoError(t, err)
	require.Zero(t, inviter.Points)

	bound, err = app.BindPhone(ctx, "new-user", "13900000002")
	require.NoError(t, err)
	require.Equal(t, int64(10), bound.Points)
	inviter, err = app.UserInfo(ctx, "inviter")
	require.NoError(t, err)
	require.Equal(t, int64(10), inviter.Points)
	history, err := app.PointsHistory(ctx, "inviter", 20, 0)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "invite", history[0].Source)
	require.Equal(t, "好友绑定手机号奖励", history[0].Description)

	repeated, err := app.ApplyReferral(ctx, "new-user", "13900000001")
	require.NoError(t, err)
	require.NotNil(t, repeated.InvitedByPhone)
	require.Equal(t, "13900000000", *repeated.InvitedByPhone)

	_, err = app.ApplyReferral(ctx, "another-user", "13900000003")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestReferralIsIgnoredForUsersAlreadyBoundToAPhone(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()

	_, err := app.BindPhone(ctx, "inviter", "13900000000")
	require.NoError(t, err)
	bound, err := app.BindPhone(ctx, "already-bound", "13900000001")
	require.NoError(t, err)
	require.Zero(t, bound.Points)

	result, err := app.ApplyReferral(ctx, "already-bound", "13900000000")
	require.NoError(t, err)
	require.Nil(t, result.InvitedByPhone)
	require.Zero(t, result.Points)

	inviter, err := app.UserInfo(ctx, "inviter")
	require.NoError(t, err)
	require.Zero(t, inviter.Points)
}

func TestActivityClaimUsesPriorityBeforeOrdinaryAndSkipsSelf(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityCashTurntable
	for _, publication := range []struct {
		uid     string
		content string
	}{{"user-a", "invite-a"}, {"user-b", "invite-b"}, {"user-c", "invite-c"}} {
		result, err := app.PublishActivity(ctx, publication.uid, activityType, publication.content)
		require.NoError(t, err)
		require.Zero(t, result.OrdinaryRounds)
		require.Zero(t, result.ClaimCount)
	}
	setOrdinaryCredit(t, app, db, ctx, "user-b", activityType, 1)
	ordinary, err := app.UseActivity(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.Equal(t, "ordinary", ordinary.Source)
	require.Equal(t, "invite-b", ordinary.Content)
	require.Equal(t, int64(1), ordinary.State.ClaimCount)
	require.Zero(t, ordinary.State.OrdinaryRounds)
	userB, err := app.ActivityDetail(ctx, "user-b", activityType)
	require.NoError(t, err)
	require.Equal(t, int64(1), userB.OrdinaryRounds)

	require.NoError(t, db.Model(&domain.User{}).Where("uid = ?", "user-c").Update("points", 10).Error)
	boosted, err := app.BoostActivity(ctx, "user-c", activityType, 2)
	require.NoError(t, err)
	require.Zero(t, boosted.PriorityRounds)
	require.Equal(t, int64(2), boosted.PointsCommitted)
	require.Equal(t, int64(2), boosted.PriorityCredit)

	priority, err := app.UseActivity(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.Equal(t, "priority", priority.Source)
	require.Equal(t, "invite-c", priority.Content)
	require.Equal(t, int64(2), priority.State.ClaimCount)
	userC, err := app.ActivityDetail(ctx, "user-c", activityType)
	require.NoError(t, err)
	require.Equal(t, int64(1), userC.PriorityRounds)
	require.Equal(t, int64(1), userC.PriorityCredit)

	priorityOnlySelf, err := app.UseActivity(ctx, "user-c", activityType)
	require.NoError(t, err)
	require.Equal(t, "ordinary", priorityOnlySelf.Source)
	require.Equal(t, "invite-a", priorityOnlySelf.Content)
	require.Equal(t, int64(1), priorityOnlySelf.State.ClaimCount)

	priority, err = app.UseActivity(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.Equal(t, "priority", priority.Source)
	userC, err = app.ActivityDetail(ctx, "user-c", activityType)
	require.NoError(t, err)
	require.Equal(t, int64(2), userC.PriorityRounds)
	require.Zero(t, userC.PriorityCredit)

	points, err := app.Points(ctx, "user-c")
	require.NoError(t, err)
	require.Equal(t, int64(8), points)
	history, err := app.PointsHistory(ctx, "user-c", 20, 0)
	require.NoError(t, err)
	require.Empty(t, history)
}

func TestPointsHistoryOnlyReturnsPositiveRecordsAndPaginatesAfterFiltering(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	_, err := app.EnsureUser(ctx, "user-a")
	require.NoError(t, err)

	records := []domain.PointRecord{
		{UID: "user-a", Source: "exchange_code", Description: "兑换码奖励", Points: 1},
		{UID: "user-a", Source: "activity_boost", Description: "活动积分插队", Points: -2},
		{UID: "user-a", Source: "invite", Description: "邀请奖励", Points: 3},
		{UID: "user-a", Source: "adjustment", Description: "零积分记录", Points: 0},
		{UID: "user-a", Source: "exchange_code", Description: "兑换码奖励", Points: 5},
		{UID: "user-b", Source: "exchange_code", Description: "其他用户奖励", Points: 9},
	}
	require.NoError(t, db.Create(&records).Error)

	firstPage, err := app.PointsHistory(ctx, "user-a", 2, 0)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Equal(t, []int64{5, 3}, []int64{firstPage[0].Points, firstPage[1].Points})

	secondPage, err := app.PointsHistory(ctx, "user-a", 2, 2)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, int64(1), secondPage[0].Points)
}

func TestActivityBoostValidatesPointAmount(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	_, err := app.PublishActivity(ctx, "user-a", domain.ActivityBuyFood, "invite")
	require.NoError(t, err)
	require.NoError(t, db.Model(&domain.User{}).Where("uid = ?", "user-a").Update("points", 2).Error)

	_, err = app.BoostActivity(ctx, "user-a", domain.ActivityBuyFood, 0)
	require.ErrorIs(t, err, domain.ErrInvalidInput)

	_, err = app.BoostActivity(ctx, "user-a", domain.ActivityBuyFood, 3)
	require.ErrorIs(t, err, domain.ErrInsufficientPoints)

	boosted, err := app.BoostActivity(ctx, "user-a", domain.ActivityBuyFood, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), boosted.PointsCommitted)
	require.Equal(t, int64(2), boosted.PriorityCredit)
}

func TestActivityClaimWithOnlyOwnPublicationReturnsQueueEmpty(t *testing.T) {
	app, _, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityDailyCash
	_, err := app.PublishActivity(ctx, "user-a", activityType, "invite-a")
	require.NoError(t, err)

	_, err = app.UseActivity(ctx, "user-a", activityType)
	require.ErrorIs(t, err, domain.ErrQueueEmpty)
	detail, err := app.ActivityDetail(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.Zero(t, detail.OrdinaryRounds)
	require.Zero(t, detail.ClaimCount)
}

func TestActivityClaimRebuildsQueuesFromDatabaseAfterRedisFlush(t *testing.T) {
	app, db, redisClient, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityCashMonopoly
	_, err := app.PublishActivity(ctx, "user-a", activityType, "invite-a")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "user-b", activityType, "invite-b")
	require.NoError(t, err)
	setOrdinaryCredit(t, app, db, ctx, "user-b", activityType, 1)
	require.NoError(t, redisClient.FlushDB(ctx).Err())

	claimed, err := app.UseActivity(ctx, "user-a", activityType)
	require.NoError(t, err)
	require.Equal(t, "invite-b", claimed.Content)
	require.Equal(t, "ordinary", claimed.Source)
}

func TestActivityClaimMovesOrdinaryCreditToClaimant(t *testing.T) {
	app, db, _, _ := testPlatform(t)
	ctx := context.Background()
	activityType := domain.ActivityBuyFood

	_, err := app.PublishActivity(ctx, "zero-owner", activityType, "zero invitation")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "funded-owner", activityType, "funded invitation")
	require.NoError(t, err)
	_, err = app.PublishActivity(ctx, "claimant", activityType, "claimant invitation")
	require.NoError(t, err)
	setOrdinaryCredit(t, app, db, ctx, "funded-owner", activityType, 1)

	claimed, err := app.UseActivity(ctx, "claimant", activityType)
	require.NoError(t, err)
	require.Equal(t, "funded invitation", claimed.Content)
	require.Equal(t, int64(1), claimed.State.ClaimCount)
	require.Zero(t, claimed.State.OrdinaryRounds)

	var funded domain.ActivityContent
	require.NoError(t, db.Where("uid = ? AND type = ?", "funded-owner", activityType).First(&funded).Error)
	require.Equal(t, int64(1), funded.OrdinaryRounds)
	require.Zero(t, funded.OrdinaryCredit)
	var claimant domain.ActivityContent
	require.NoError(t, db.Where("uid = ? AND type = ?", "claimant", activityType).First(&claimant).Error)
	require.Equal(t, int64(1), claimant.OrdinaryCredit)
	var zero domain.ActivityContent
	require.NoError(t, db.Where("uid = ? AND type = ?", "zero-owner", activityType).First(&zero).Error)
	require.Zero(t, zero.OrdinaryRounds)
	require.Zero(t, zero.OrdinaryCredit)

	claimedBack, err := app.UseActivity(ctx, "funded-owner", activityType)
	require.NoError(t, err)
	require.Equal(t, "claimant invitation", claimedBack.Content)
	require.Equal(t, int64(2), claimedBack.State.ClaimCount)
	require.Equal(t, int64(1), claimedBack.State.OrdinaryRounds)

	require.NoError(t, db.Where("uid = ? AND type = ?", "claimant", activityType).First(&claimant).Error)
	require.Equal(t, int64(1), claimant.OrdinaryRounds)
	require.Zero(t, claimant.OrdinaryCredit)
	require.NoError(t, db.Where("uid = ? AND type = ?", "funded-owner", activityType).First(&funded).Error)
	require.Equal(t, int64(1), funded.OrdinaryCredit)
}
