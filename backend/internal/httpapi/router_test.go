package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/database"
	"github.com/eaok-cn/kuaizudui/backend/internal/platform"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testRouter(t *testing.T) http.Handler {
	router, _, _ := testRouterComponents(t)
	return router
}

func testRouterComponents(t *testing.T) (http.Handler, *gorm.DB, *redis.Client) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	cfg := config.Config{
		Server: config.ServerConfig{Mode: "test", AllowedOrigins: []string{"http://localhost:5173"}},
		Business: config.BusinessConfig{
			AdminPhone: "13800000000", LuckyCodeMinLength: 8, LuckyCodeMaxLength: 9,
			ActivityContentMaxLength: 200,
			QRCodeUploadDir:          filepath.Join(t.TempDir(), "uploads"), QRCodeMaxUploadBytes: 5 * 1024 * 1024,
			FirstVisitTTL: config.Duration(time.Hour),
			LuckyClaimTTL: config.Duration(time.Hour),
		},
		Security: config.SecurityConfig{
			AdminSessionTTL: config.Duration(time.Hour), AdminTokenSecret: "test-secret",
		},
	}
	return NewRouter(platform.New(db, redisClient, cfg), db, redisClient, cfg), db, redisClient
}

func TestIdentityAndActivityRoundTrip(t *testing.T) {
	router, _, _ := testRouterComponents(t)

	info := httptest.NewRecorder()
	router.ServeHTTP(info, httptest.NewRequest(http.MethodGet, "/api/v1/user/info", nil))
	require.Equal(t, http.StatusOK, info.Code)
	uid := info.Header().Get("X-UID")
	require.Len(t, uid, 32)

	publish := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/activity/publish", bytes.NewBufferString(`{
  "type": "cash_monopoly",
  "content": "shared invitation logic"
}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-UID", uid)
	router.ServeHTTP(publish, request)
	require.Equal(t, http.StatusOK, publish.Code)
	require.Contains(t, publish.Body.String(), `"ordinary_rounds":0`)

	secondPublish := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/activity/publish", bytes.NewBufferString(`{
  "type": "cash_monopoly",
  "content": "another user's invitation"
}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-UID", "activity-user-b")
	router.ServeHTTP(secondPublish, request)
	require.Equal(t, http.StatusOK, secondPublish.Code)

	detail := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/activity/detail?type=cash_monopoly", nil)
	request.Header.Set("X-UID", uid)
	router.ServeHTTP(detail, request)
	require.Equal(t, http.StatusOK, detail.Code)
	require.Contains(t, detail.Body.String(), "shared invitation logic")

	claim := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/activity/use", bytes.NewBufferString(`{"type":"cash_monopoly"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-UID", uid)
	router.ServeHTTP(claim, request)
	require.Equal(t, http.StatusOK, claim.Code)
	require.Contains(t, claim.Body.String(), `"content":"another user's invitation"`)
	require.Contains(t, claim.Body.String(), `"source":"ordinary"`)
	// Publishing grants nothing; this claim click is the only count.
	require.Contains(t, claim.Body.String(), `"claim_count":1`)
}

func TestActivityEventsStreamUpdatesThePublisherAfterAClaim(t *testing.T) {
	router, _, _ := testRouterComponents(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	publish := func(uid, content string) {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/activity/publish", bytes.NewBufferString(
			fmt.Sprintf(`{"type":"buy_food","content":%q}`, content),
		))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-UID", uid)
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		defer func() { _ = response.Body.Close() }()
		require.Equal(t, http.StatusOK, response.StatusCode)
	}

	publish("event-owner", "owner invitation")
	publish("event-claimant", "claimant invitation")

	streamContext, cancelStream := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStream()
	streamRequest, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+"/api/v1/activity/events", nil)
	require.NoError(t, err)
	streamRequest.Header.Set("X-UID", "event-owner")
	streamResponse, err := server.Client().Do(streamRequest)
	require.NoError(t, err)
	defer func() { _ = streamResponse.Body.Close() }()
	require.Equal(t, http.StatusOK, streamResponse.StatusCode)
	require.Equal(t, "text/event-stream", streamResponse.Header.Get("Content-Type"))

	reader := bufio.NewReader(streamResponse.Body)
	connected, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, ": connected\n", connected)
	_, err = reader.ReadString('\n')
	require.NoError(t, err)

	claimRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/activity/use", bytes.NewBufferString(`{"type":"buy_food"}`))
	require.NoError(t, err)
	claimRequest.Header.Set("Content-Type", "application/json")
	claimRequest.Header.Set("X-UID", "event-claimant")
	claimResponse, err := server.Client().Do(claimRequest)
	require.NoError(t, err)
	defer func() { _ = claimResponse.Body.Close() }()
	require.Equal(t, http.StatusOK, claimResponse.StatusCode)

	eventName, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "event: activity\n", eventName)
	eventData, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"buy_food"}`, strings.TrimPrefix(strings.TrimSpace(eventData), "data: "))
}

func TestAdminEndpointsRequireTokenAndCanCreateCodes(t *testing.T) {
	router := testRouter(t)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	wrongPhone := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13911111111"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wrongPhone, request)
	require.Equal(t, http.StatusUnauthorized, wrongPhone.Code)
	require.NotContains(t, wrongPhone.Body.String(), "token")

	login := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13800000000"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	require.Equal(t, http.StatusOK, login.Code)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))
	require.NotEmpty(t, loginBody.Data.Token)

	created := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/exchange/create", bytes.NewBufferString(`{"points":20,"count":2,"prefix":"WEB-"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(created, request)
	require.Equal(t, http.StatusCreated, created.Code)
	require.Contains(t, created.Body.String(), `"count":2`)
}

func TestFeedbackCanBeSubmittedAndListedByAdmin(t *testing.T) {
	router := testRouter(t)

	submit := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", bytes.NewBufferString(`{"content":"页面使用很顺手"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-UID", "feedback-user")
	router.ServeHTTP(submit, request)
	require.Equal(t, http.StatusCreated, submit.Code)
	require.Contains(t, submit.Body.String(), `"uid":"feedback-user"`)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/feedback", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	login := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13800000000"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	require.Equal(t, http.StatusOK, login.Code)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))

	list := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/feedback", nil)
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(list, request)
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), `"content":"页面使用很顺手"`)
	require.Contains(t, list.Body.String(), `"uid":"feedback-user"`)
	require.NotContains(t, list.Body.String(), `"phone"`)
}

func TestAdminCanUploadServeAndRemoveGroupQRCode(t *testing.T) {
	router := testRouter(t)

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13800000000"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	require.Equal(t, http.StatusOK, login.Code)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	part, err := writer.CreateFormFile("image", "group.png")
	require.NoError(t, err)
	picture := image.NewRGBA(image.Rect(0, 0, 180, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 180; x++ {
			picture.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 32, A: 255})
		}
	}
	require.NoError(t, png.Encode(part, picture))
	require.NoError(t, writer.Close())

	upload := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/qrcode", &uploadBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(upload, request)
	require.Equal(t, http.StatusOK, upload.Code)
	var uploadResponse struct {
		Data struct {
			QRCode string `json:"qrcode"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(upload.Body.Bytes(), &uploadResponse))
	require.Contains(t, uploadResponse.Data.QRCode, "/api/v1/group-qrcode/image?v=")

	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/group-qrcode", nil))
	require.Equal(t, http.StatusOK, public.Code)
	require.Contains(t, public.Body.String(), "/api/v1/group-qrcode/image?v=")

	served := httptest.NewRecorder()
	router.ServeHTTP(served, httptest.NewRequest(http.MethodGet, uploadResponse.Data.QRCode, nil))
	require.Equal(t, http.StatusOK, served.Code)
	require.Equal(t, "image/png", served.Header().Get("Content-Type"))
	require.NotEmpty(t, served.Body.Bytes())

	remove := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/qrcode", nil)
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(remove, request)
	require.Equal(t, http.StatusOK, remove.Code)
	require.Contains(t, remove.Body.String(), `"qrcode":""`)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, uploadResponse.Data.QRCode, nil))
	require.Equal(t, http.StatusNotFound, missing.Code)
}

func TestAdminQRCodeUploadRejectsNonImage(t *testing.T) {
	router := testRouter(t)

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13800000000"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	part, err := writer.CreateFormFile("image", "not-an-image.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("not an image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	upload := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/qrcode", &uploadBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(upload, request)
	require.Equal(t, http.StatusBadRequest, upload.Code)
	require.Contains(t, upload.Body.String(), "only PNG and JPEG")
}

func TestAdminActivityQueuesRequiresTokenAndListsEveryType(t *testing.T) {
	router := testRouter(t)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/activity-queues", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewBufferString(`{"phone":"13800000000"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	require.Equal(t, http.StatusOK, login.Code)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))

	list := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/activity-queues", nil)
	request.Header.Set("Authorization", "Bearer "+loginBody.Data.Token)
	router.ServeHTTP(list, request)
	require.Equal(t, http.StatusOK, list.Code)
	var body struct {
		Data struct {
			Items []struct {
				Type     string `json:"type"`
				Ordinary struct {
					Created bool  `json:"created"`
					Total   int64 `json:"total"`
				} `json:"ordinary"`
				Priority struct {
					Created bool `json:"created"`
				} `json:"priority"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 4)
	for index, expected := range []string{"buy_food", "cash_turntable", "cash_monopoly", "daily_cash"} {
		require.Equal(t, expected, body.Data.Items[index].Type)
		require.False(t, body.Data.Items[index].Ordinary.Created)
		require.False(t, body.Data.Items[index].Priority.Created)
	}
}
