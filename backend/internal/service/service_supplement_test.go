package service

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
)

type supplementFixture struct {
	svc       SupplementService
	review    ReviewService
	sensitive SensitiveWordService
	posts     repository.PostRepository
	authorID  uint
	postID    uint
}

func newSupplementFixture(t *testing.T) *supplementFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.Post{}, &model.Tag{}, &model.PostTag{}, &model.Comment{}, &model.Like{}, &model.SensitiveWord{}, &model.ReviewQueue{}, &model.PostSupplement{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	postRepo := repository.NewPostRepository(db)
	supplementRepo := repository.NewSupplementRepository(db)
	reviewRepo := repository.NewReviewQueueRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	sensitive := NewSensitiveWordService(repository.NewSensitiveWordRepository(db))
	review := NewReviewService(reviewRepo, postRepo, commentRepo, supplementRepo, logger)
	svc := NewSupplementService(supplementRepo, postRepo, sensitive, review, logger)

	const authorID uint = 7
	post := &model.Post{IdentityID: authorID, Content: "post", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}
	return &supplementFixture{svc: svc, review: review, sensitive: sensitive, posts: postRepo, authorID: authorID, postID: post.ID}
}

func TestSupplementCreateClean(t *testing.T) {
	f := newSupplementFixture(t)
	supplement, hits, blocked, err := f.svc.Create(f.authorID, f.postID, "补充一点后续进展")
	if err != nil {
		t.Fatalf("create supplement: %v", err)
	}
	if blocked || len(hits) != 0 {
		t.Fatalf("expected clean supplement, blocked=%v hits=%v", blocked, hits)
	}
	if supplement.Status != constants.SupplementStatusPublished {
		t.Fatalf("expected published, got %d", supplement.Status)
	}
	if supplement.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", supplement.Seq)
	}
}

func TestSupplementCreateNotAuthor(t *testing.T) {
	f := newSupplementFixture(t)
	if _, _, _, err := f.svc.Create(999, f.postID, "别人不能写"); err != ErrNotPostAuthor {
		t.Fatalf("expected ErrNotPostAuthor, got %v", err)
	}
}

func TestSupplementCreatePostNotFound(t *testing.T) {
	f := newSupplementFixture(t)
	if _, _, _, err := f.svc.Create(f.authorID, 999, "帖子不存在"); err != ErrPostNotFound {
		t.Fatalf("expected ErrPostNotFound, got %v", err)
	}
}

func TestSupplementCreatePostNotPublished(t *testing.T) {
	f := newSupplementFixture(t)
	pending := &model.Post{IdentityID: f.authorID, Content: "pending", Status: constants.PostStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := f.posts.Create(pending); err != nil {
		t.Fatalf("create pending post: %v", err)
	}
	if _, _, _, err := f.svc.Create(f.authorID, pending.ID, "未过审帖子"); err != ErrPostNotPublished {
		t.Fatalf("expected ErrPostNotPublished, got %v", err)
	}
}

func TestSupplementListVisibility(t *testing.T) {
	f := newSupplementFixture(t)
	if _, err := f.sensitive.Create("赌博"); err != nil {
		t.Fatalf("create word: %v", err)
	}
	if _, _, _, err := f.svc.Create(f.authorID, f.postID, "公开追记"); err != nil {
		t.Fatalf("create clean supplement: %v", err)
	}
	if _, _, blocked, err := f.svc.Create(f.authorID, f.postID, "涉及赌博的追记"); err != nil || !blocked {
		t.Fatalf("create blocked supplement: blocked=%v err=%v", blocked, err)
	}

	// 陌生人只能看到已发布的一条
	strangerItems, err := f.svc.ListForPost(f.postID, 999)
	if err != nil {
		t.Fatalf("list for stranger: %v", err)
	}
	if len(strangerItems) != 1 || strangerItems[0].Status != constants.SupplementStatusPublished {
		t.Fatalf("stranger should see 1 published supplement, got %+v", strangerItems)
	}
	// 匿名访客同样只能看到已发布的一条
	guestItems, err := f.svc.ListForPost(f.postID, 0)
	if err != nil {
		t.Fatalf("list for guest: %v", err)
	}
	if len(guestItems) != 1 {
		t.Fatalf("guest should see 1 supplement, got %d", len(guestItems))
	}
	// 楼主可见全部（含待审）
	authorItems, err := f.svc.ListForPost(f.postID, f.authorID)
	if err != nil {
		t.Fatalf("list for author: %v", err)
	}
	if len(authorItems) != 2 {
		t.Fatalf("author should see 2 supplements, got %d", len(authorItems))
	}
}

func TestSupplementReviewFlow(t *testing.T) {
	f := newSupplementFixture(t)
	if _, err := f.sensitive.Create("赌博"); err != nil {
		t.Fatalf("create word: %v", err)
	}
	// 命中敏感词 → 待审并进入审核队列
	supplement, hits, blocked, err := f.svc.Create(f.authorID, f.postID, "这里有赌博内容")
	if err != nil || !blocked || len(hits) == 0 {
		t.Fatalf("expected blocked supplement, blocked=%v hits=%v err=%v", blocked, hits, err)
	}
	if supplement.Status != constants.SupplementStatusPending {
		t.Fatalf("expected pending, got %d", supplement.Status)
	}
	queue, total, err := f.review.List(1, 10, constants.ReviewStatusPending)
	if err != nil || total != 1 {
		t.Fatalf("expected 1 pending review item, total=%d err=%v", total, err)
	}
	if queue[0].TargetType != "supplement" || queue[0].TargetID != supplement.ID {
		t.Fatalf("review item mismatch: %+v", queue[0])
	}
	// 待审期间不允许再提交
	if _, _, _, err := f.svc.Create(f.authorID, f.postID, "再来一条"); err != ErrSupplementPending {
		t.Fatalf("expected ErrSupplementPending, got %v", err)
	}
	// 放行 → 追记公开
	if err := f.review.Approve(queue[0].ID, 1, "没问题"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	items, err := f.svc.ListForPost(f.postID, 0)
	if err != nil {
		t.Fatalf("list after approve: %v", err)
	}
	if len(items) != 1 || items[0].Status != constants.SupplementStatusPublished {
		t.Fatalf("expected approved supplement visible, got %+v", items)
	}

	// 再次命中 → 驳回并记录原因
	second, _, blocked, err := f.svc.Create(f.authorID, f.postID, "还是赌博内容")
	if err != nil || !blocked {
		t.Fatalf("create second blocked supplement: blocked=%v err=%v", blocked, err)
	}
	queue, _, err = f.review.List(1, 10, constants.ReviewStatusPending)
	if err != nil || len(queue) != 1 {
		t.Fatalf("expected 1 pending review item, got %d", len(queue))
	}
	if err := f.review.Reject(queue[0].ID, 1, "仍包含敏感词"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	authorItems, err := f.svc.ListForPost(f.postID, f.authorID)
	if err != nil {
		t.Fatalf("list for author after reject: %v", err)
	}
	var rejectedItem *model.PostSupplement
	for i := range authorItems {
		if authorItems[i].ID == second.ID {
			rejectedItem = &authorItems[i]
		}
	}
	if rejectedItem == nil || rejectedItem.Status != constants.SupplementStatusRejected {
		t.Fatalf("expected rejected supplement, got %+v", rejectedItem)
	}
	if rejectedItem.ReviewNote != "仍包含敏感词" {
		t.Fatalf("expected review note persisted, got %q", rejectedItem.ReviewNote)
	}
	// 驳回后允许另写一条
	third, _, _, err := f.svc.Create(f.authorID, f.postID, "重新写的追记")
	if err != nil {
		t.Fatalf("create after reject: %v", err)
	}
	if third.Status != constants.SupplementStatusPublished {
		t.Fatalf("expected published, got %d", third.Status)
	}
	// 已有两条有效追记（放行 + 重提），再次提交整次拒绝
	if _, _, _, err := f.svc.Create(f.authorID, f.postID, "第三条"); err != ErrSupplementLimit {
		t.Fatalf("expected ErrSupplementLimit, got %v", err)
	}
}
