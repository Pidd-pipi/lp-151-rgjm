package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/middleware"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
	"github.com/gbtreehole/backend/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type apiFixture struct {
	router    *gin.Engine
	tokenSvc  service.TokenService
	posts     repository.PostRepository
	addenda   repository.PostAddendumRepository
	review    service.ReviewService
	addSvc    service.PostAddendumService
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.UserIdentity{}, &model.Post{}, &model.Tag{}, &model.PostTag{},
		&model.Comment{}, &model.Like{}, &model.SensitiveWord{},
		&model.ReviewQueue{}, &model.PostAddendum{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	postRepo := repository.NewPostRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	addendumRepo := repository.NewPostAddendumRepository(db)
	tokenSvc := service.NewTokenService("test-secret", 60)
	sensitiveSvc := service.NewSensitiveWordService(repository.NewSensitiveWordRepository(db))
	if _, err := sensitiveSvc.Create("赌博"); err != nil {
		t.Fatalf("seed word: %v", err)
	}
	reviewSvc := service.NewReviewService(repository.NewReviewQueueRepository(db), postRepo, commentRepo, addendumRepo, logger)
	postSvc := service.NewPostService(postRepo, nil, sensitiveSvc, reviewSvc, logger)
	addendumSvc := service.NewPostAddendumService(addendumRepo, postRepo, sensitiveSvc, reviewSvc, logger)
	likeSvc := service.NewLikeService(repository.NewLikeRepository(db), postRepo, commentRepo, logger)

	postHandler := NewPostHandler(postSvc, addendumSvc, likeSvc, logger)
	addendumHandler := NewAddendumHandler(addendumSvc, logger)
	identityMW := middleware.NewIdentityAuthMiddleware(tokenSvc)

	r := gin.New()
	v1 := r.Group("/api/v1")
	posts := v1.Group("/posts")
	posts.Use(identityMW.OptionalAuth())
	{
		posts.GET("/:id", postHandler.GetPost)
		posts.POST("/:id/addenda", identityMW.RequireAuth(), addendumHandler.AddAddendum)
	}
	return &apiFixture{
		router:   r,
		tokenSvc: tokenSvc,
		posts:    postRepo,
		addenda:  addendumRepo,
		review:   reviewSvc,
		addSvc:   addendumSvc,
	}
}

func (f *apiFixture) token(t *testing.T, identityID uint) string {
	t.Helper()
	token, err := f.tokenSvc.Sign(identityID, "key-"+itoa(identityID))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func itoa(id uint) string {
	if id == 0 {
		return "0"
	}
	var b []byte
	for id > 0 {
		b = append([]byte{byte('0' + id%10)}, b...)
		id /= 10
	}
	return string(b)
}

func (f *apiFixture) createPostRow(identityID uint, status int) *model.Post {
	p := &model.Post{IdentityID: identityID, Content: "原帖", Status: status, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := f.posts.Create(p); err != nil {
		panic(err)
	}
	return p
}

func doJSON(t *testing.T, r *gin.Engine, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp
}

func TestHTTP_AddendumEndToEnd(t *testing.T) {
	f := newAPIFixture(t)
	post := f.createPostRow(1, constants.PostStatusPublished)
	ownerToken := f.token(t, 1)
	otherToken := f.token(t, 2)

	// 1. 未登录 -> 401
	if code, _ := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", "", map[string]string{"content": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("unauth code = %d, want 401", code)
	}

	// 2. 非楼主 -> 403
	if code, resp := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", otherToken, map[string]string{"content": "我不是楼主"}); code != http.StatusForbidden {
		t.Fatalf("forbidden code = %d, resp=%v", code, resp)
	}

	// 3. 楼主提交干净追记 -> 直接发布
	code, resp := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "第一条补充"})
	if code != http.StatusOK {
		t.Fatalf("clean add code = %d, resp=%v", code, resp)
	}
	data := resp["data"].(map[string]any)
	if data["blocked"].(bool) {
		t.Fatal("clean addendum should not be blocked")
	}

	// 4. 楼主提交命中敏感词追记 -> 待审，原帖仍公开
	code, resp = doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "含赌博内容"})
	if code != http.StatusOK {
		t.Fatalf("sensitive add code = %d, resp=%v", code, resp)
	}
	if !resp["data"].(map[string]any)["blocked"].(bool) {
		t.Fatal("expected blocked=true")
	}

	// 5. 已存在待审，再提交（并发/重复）-> 409 整次拒绝
	if code, resp = doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "又想补一条"}); code != http.StatusConflict {
		t.Fatalf("pending conflict code = %d, resp=%v", code, resp)
	}

	// 6. 他人刷新详情：只能看到已通过的追记，看不到待审内容/原因
	code, resp = doJSON(t, f.router, http.MethodGet, "/api/v1/posts/"+itoa(post.ID), otherToken, nil)
	if code != http.StatusOK {
		t.Fatalf("get post code = %d", code)
	}
	postData := resp["data"].(map[string]any)
	if postData["isOwner"].(bool) {
		t.Fatal("other viewer must not be owner")
	}
	if int(postData["addendumCount"].(float64)) != 1 {
		t.Fatalf("public addendumCount = %v, want 1", postData["addendumCount"])
	}
	addenda := postData["addenda"].([]any)
	if len(addenda) != 1 {
		t.Fatalf("public addenda len = %d, want 1 (pending hidden)", len(addenda))
	}

	// 7. 楼主刷新详情：能看到待审追记
	code, resp = doJSON(t, f.router, http.MethodGet, "/api/v1/posts/"+itoa(post.ID), ownerToken, nil)
	if code != http.StatusOK {
		t.Fatalf("owner get post code = %d", code)
	}
	postData = resp["data"].(map[string]any)
	if !postData["isOwner"].(bool) {
		t.Fatal("owner flag missing")
	}
	if len(postData["addenda"].([]any)) != 2 {
		t.Fatalf("owner should see 2 addenda (published + pending), got %d", len(postData["addenda"].([]any)))
	}

	// 8. 管理员放行待审追记
	items, _, err := f.review.List(1, 50, constants.ReviewStatusPending)
	if err != nil || len(items) != 1 {
		t.Fatalf("pending queue: %v %d", err, len(items))
	}
	if err := f.review.Approve(items[0].ID, 7, "核实放行"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// 9. 刷新一致：他人现在可见两条，数量为 2
	code, resp = doJSON(t, f.router, http.MethodGet, "/api/v1/posts/"+itoa(post.ID), otherToken, nil)
	if code != http.StatusOK {
		t.Fatalf("get after approve: %d", code)
	}
	postData = resp["data"].(map[string]any)
	if int(postData["addendumCount"].(float64)) != 2 || len(postData["addenda"].([]any)) != 2 {
		t.Fatalf("after approve count=%v len=%d", postData["addendumCount"], len(postData["addenda"].([]any)))
	}

	// 10. 两条已满 -> 再次提交整次拒绝
	if code, resp = doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "超额"}); code != http.StatusConflict {
		t.Fatalf("limit code = %d, resp=%v", code, resp)
	}
}

func TestHTTP_AddendumRejectResubmit(t *testing.T) {
	f := newAPIFixture(t)
	post := f.createPostRow(1, constants.PostStatusPublished)
	ownerToken := f.token(t, 1)

	// 提交命中敏感词追记并被驳回（带原因）
	if _, _, _, err := f.addSvc.Add(1, post.ID, "含赌博待审"); err != nil {
		t.Fatalf("add: %v", err)
	}
	items, _, _ := f.review.List(1, 50, constants.ReviewStatusPending)
	if err := f.review.Reject(items[0].ID, 7, "违规驳回原因"); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// 楼主详情：能看到驳回状态和驳回原因
	code, resp := doJSON(t, f.router, http.MethodGet, "/api/v1/posts/"+itoa(post.ID), ownerToken, nil)
	if code != http.StatusOK {
		t.Fatalf("owner get: %d", code)
	}
	addenda := resp["data"].(map[string]any)["addenda"].([]any)
	if len(addenda) != 1 {
		t.Fatalf("owner addenda len = %d", len(addenda))
	}
	first := addenda[0].(map[string]any)
	if int(first["status"].(float64)) != constants.AddendumStatusRejected {
		t.Fatalf("status = %v, want rejected", first["status"])
	}
	if first["reviewNote"].(string) != "违规驳回原因" {
		t.Fatalf("owner should see reject reason, got %v", first["reviewNote"])
	}

	// 他人详情：看不到驳回追记，也看不到原因
	otherToken := f.token(t, 2)
	code, resp = doJSON(t, f.router, http.MethodGet, "/api/v1/posts/"+itoa(post.ID), otherToken, nil)
	if code != http.StatusOK {
		t.Fatalf("other get: %d", code)
	}
	postData := resp["data"].(map[string]any)
	if int(postData["addendumCount"].(float64)) != 0 {
		t.Fatalf("public count = %v, want 0", postData["addendumCount"])
	}
	if len(postData["addenda"].([]any)) != 0 {
		t.Fatalf("rejected addendum must be hidden from others")
	}

	// 驳回释放名额：可另写一条干净追记
	code, resp = doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "驳回后重新补充"})
	if code != http.StatusOK {
		t.Fatalf("resubmit code = %d, resp=%v", code, resp)
	}
	if resp["data"].(map[string]any)["blocked"].(bool) {
		t.Fatal("resubmitted clean addendum must publish")
	}
}

func TestHTTP_AddendumValidation(t *testing.T) {
	f := newAPIFixture(t)
	post := f.createPostRow(1, constants.PostStatusPublished)
	ownerToken := f.token(t, 1)

	// 空内容 -> 400
	if code, _ := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": ""}); code != http.StatusBadRequest {
		t.Fatalf("empty content should be 400")
	}
	// 纯空白内容 -> 400
	if code, _ := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": "   \n\t "}); code != http.StatusBadRequest {
		t.Fatalf("blank content should be 400")
	}
	// 超过 500 字 -> 400
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'a'
	}
	if code, _ := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/"+itoa(post.ID)+"/addenda", ownerToken, map[string]string{"content": string(long)}); code != http.StatusBadRequest {
		t.Fatalf("overlength content should be 400")
	}
	// 帖子不存在 -> 404
	if code, _ := doJSON(t, f.router, http.MethodPost, "/api/v1/posts/99999/addenda", ownerToken, map[string]string{"content": "x"}); code != http.StatusNotFound {
		t.Fatalf("missing post should be 404")
	}
}
