package service

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type addendumFixture struct {
	db       *gorm.DB
	posts    repository.PostRepository
	svc      PostAddendumService
	review   ReviewService
	addenda  repository.PostAddendumRepository
}

func newAddendumFixture(t *testing.T) *addendumFixture {
	t.Helper()
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
	sensitiveSvc := NewSensitiveWordService(repository.NewSensitiveWordRepository(db))
	if _, err := sensitiveSvc.Create("赌博"); err != nil {
		t.Fatalf("seed sensitive word: %v", err)
	}
	reviewSvc := NewReviewService(repository.NewReviewQueueRepository(db), postRepo, commentRepo, addendumRepo, logger)
	addendumSvc := NewPostAddendumService(addendumRepo, postRepo, sensitiveSvc, reviewSvc, logger)
	return &addendumFixture{
		db:      db,
		posts:   postRepo,
		svc:     addendumSvc,
		review:  reviewSvc,
		addenda: addendumRepo,
	}
}

func (f *addendumFixture) createPost(t *testing.T, identityID uint, status int) *model.Post {
	t.Helper()
	post := &model.Post{
		IdentityID: identityID,
		Content:    "原帖正文",
		Status:     status,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := f.posts.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}
	return post
}

func (f *addendumFixture) pendingQueueID(t *testing.T, addendumID uint) uint {
	t.Helper()
	items, _, err := f.review.List(1, 50, constants.ReviewStatusPending)
	if err != nil {
		t.Fatalf("list pending review: %v", err)
	}
	for _, it := range items {
		if it.TargetType == "addendum" && it.TargetID == addendumID {
			return it.ID
		}
	}
	t.Fatalf("pending review for addendum %d not found", addendumID)
	return 0
}

func TestAddendum_CleanPublishedImmediately(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	a, hits, blocked, err := f.svc.Add(1, post.ID, "这是一条干净的补充说明")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if blocked || len(hits) != 0 {
		t.Fatalf("clean addendum should not be blocked, hits=%v", hits)
	}
	if a.Status != constants.AddendumStatusPublished {
		t.Fatalf("status = %d, want published", a.Status)
	}

	views, err := f.svc.ListByPostID(post.ID, 1)
	if err != nil || len(views) != 1 {
		t.Fatalf("list views: %v %d", err, len(views))
	}
	count, err := f.svc.CountPublished(post.ID)
	if err != nil || count != 1 {
		t.Fatalf("published count = %d, err=%v", count, err)
	}
}

func TestAddendum_SensitiveGoesPendingKeepsPostPublic(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	a, _, blocked, err := f.svc.Add(1, post.ID, "补充里提到了赌博内容")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !blocked {
		t.Fatal("expected blocked")
	}
	if a.Status != constants.AddendumStatusPending {
		t.Fatalf("status = %d, want pending", a.Status)
	}
	// 原帖保持公开
	refreshed, err := f.posts.FindByID(post.ID)
	if err != nil {
		t.Fatalf("reload post: %v", err)
	}
	if refreshed.Status != constants.PostStatusPublished {
		t.Fatalf("post status = %d, want published", refreshed.Status)
	}
}

func TestAddendum_OnlyOnePendingRejectSecond(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	if _, _, _, err := f.svc.Add(1, post.ID, "第一条含赌博待审"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	// 并发/重复提交一条干净追记：因已有待审，整次拒绝
	_, _, _, err := f.svc.Add(1, post.ID, "另一条干净补充")
	if !errors.Is(err, ErrAddendumPendingExists) {
		t.Fatalf("expected ErrAddendumPendingExists, got %v", err)
	}
}

func TestAddendum_ApproveFlow(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	a, _, _, err := f.svc.Add(1, post.ID, "含赌博的补充")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	queueID := f.pendingQueueID(t, a.ID)
	if err := f.review.Approve(queueID, 99, "核实无误，放行"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approved, err := f.addenda.FindByID(a.ID)
	if err != nil {
		t.Fatalf("reload addendum: %v", err)
	}
	if approved.Status != constants.AddendumStatusPublished {
		t.Fatalf("status = %d, want published", approved.Status)
	}
	if approved.ReviewNote != "核实无误，放行" || approved.ReviewedBy == nil || *approved.ReviewedBy != 99 {
		t.Fatalf("review metadata not synced: %+v", approved)
	}
	if count, _ := f.svc.CountPublished(post.ID); count != 1 {
		t.Fatalf("published count = %d, want 1", count)
	}
}

func TestAddendum_RejectFreesSlotThenResubmit(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	first, _, _, err := f.svc.Add(1, post.ID, "第一条已通过")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.Status != constants.AddendumStatusPublished {
		t.Fatal("first should publish immediately")
	}

	pending, _, _, err := f.svc.Add(1, post.ID, "第二条含赌博")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	queueID := f.pendingQueueID(t, pending.ID)
	if err := f.review.Reject(queueID, 99, "内容不当，驳回"); err != nil {
		t.Fatalf("reject: %v", err)
	}

	rejected, err := f.addenda.FindByID(pending.ID)
	if err != nil || rejected.Status != constants.AddendumStatusRejected {
		t.Fatalf("rejected status: %v %+v", err, rejected)
	}

	// 驳回不占名额：可另写一条
	retry, _, blocked, err := f.svc.Add(1, post.ID, "重写后的干净补充")
	if err != nil {
		t.Fatalf("resubmit after reject: %v", err)
	}
	if blocked || retry.Status != constants.AddendumStatusPublished {
		t.Fatalf("resubmit should publish cleanly, status=%d blocked=%v", retry.Status, blocked)
	}

	// 此时两条已通过，名额满，再写整次拒绝
	_, _, _, err = f.svc.Add(1, post.ID, "第三条超额")
	if !errors.Is(err, ErrAddendumLimitReached) {
		t.Fatalf("expected ErrAddendumLimitReached, got %v", err)
	}
}

func TestAddendum_NonAuthorForbidden(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	_, _, _, err := f.svc.Add(2, post.ID, "别人来补一条")
	if !errors.Is(err, ErrNotPostAuthor) {
		t.Fatalf("expected ErrNotPostAuthor, got %v", err)
	}
}

func TestAddendum_PostNotFoundAndNotPublished(t *testing.T) {
	f := newAddendumFixture(t)

	_, _, _, err := f.svc.Add(1, 99999, "补充")
	if !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("expected ErrPostNotFound, got %v", err)
	}

	pendingPost := f.createPost(t, 1, constants.PostStatusPending)
	_, _, _, err = f.svc.Add(1, pendingPost.ID, "原帖还没过审就想补充")
	if !errors.Is(err, ErrPostNotPublished) {
		t.Fatalf("expected ErrPostNotPublished, got %v", err)
	}
}

func TestAddendum_LengthBoundary(t *testing.T) {
	// 服务层对存储长度的兜底：DTO 校验在 handler 层，这里验证 500 字以内可正常写入。
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	within := strings.Repeat("字", constants.AddendumMaxLength)
	a, _, _, err := f.svc.Add(1, post.ID, within)
	if err != nil {
		t.Fatalf("500 chars add: %v", err)
	}
	if len([]rune(a.Content)) != constants.AddendumMaxLength {
		t.Fatalf("content rune length = %d", len([]rune(a.Content)))
	}

	// 空白内容拒绝
	if _, _, _, err := f.svc.Add(1, post.ID, "   \n\t "); !errors.Is(err, ErrAddendumContentEmpty) {
		t.Fatalf("blank content expected ErrAddendumContentEmpty, got %v", err)
	}
	// 超过 500 字拒绝
	overlong := strings.Repeat("字", constants.AddendumMaxLength+1)
	if _, _, _, err := f.svc.Add(1, post.ID, overlong); !errors.Is(err, ErrAddendumTooLong) {
		t.Fatalf("overlong expected ErrAddendumTooLong, got %v", err)
	}
}

func TestAddendum_ReviewNoteOwnerOnlyVisibility(t *testing.T) {
	f := newAddendumFixture(t)
	post := f.createPost(t, 1, constants.PostStatusPublished)

	pending, _, _, err := f.svc.Add(1, post.ID, "含赌博")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	queueID := f.pendingQueueID(t, pending.ID)
	if err := f.review.Reject(queueID, 99, "驳回原因X"); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// 作者视角：能看到被驳回的追记
	ownerViews, err := f.svc.ListByPostID(post.ID, 1)
	if err != nil || len(ownerViews) != 1 {
		t.Fatalf("owner views: %v %d", err, len(ownerViews))
	}
	if !ownerViews[0].IsOwner {
		t.Fatal("owner should see IsOwner=true")
	}

	// 他人视角：IsOwner=false，handler 的 toAddendumResponse 将据此剔除驳回追记与原因
	otherViews, err := f.svc.ListByPostID(post.ID, 2)
	if err != nil || len(otherViews) != 1 {
		t.Fatalf("other views: %v %d", err, len(otherViews))
	}
	if otherViews[0].IsOwner {
		t.Fatal("other viewer must not be treated as owner")
	}
	if otherViews[0].Addendum.ReviewNote != "驳回原因X" {
		t.Fatalf("service layer still holds raw note, got %q", otherViews[0].Addendum.ReviewNote)
	}
}
