package repository

import (
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
)

func newTestPost(t *testing.T, posts PostRepository) *model.Post {
	t.Helper()
	post := &model.Post{IdentityID: 1, Content: "post", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := posts.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}
	return post
}

func TestSupplementRepositoryCreateWithLimit(t *testing.T) {
	db := newTestDB(t)
	postRepo := NewPostRepository(db)
	repo := NewSupplementRepository(db)
	post := newTestPost(t, postRepo)

	first := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "第一条", Status: constants.SupplementStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(first); err != nil {
		t.Fatalf("create first supplement: %v", err)
	}
	if first.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", first.Seq)
	}

	second := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "第二条", Status: constants.SupplementStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(second); err != nil {
		t.Fatalf("create second supplement: %v", err)
	}
	if second.Seq != 2 {
		t.Fatalf("expected seq 2, got %d", second.Seq)
	}

	// 已满两条（1 已发布 + 1 待审），整次拒绝
	third := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "第三条", Status: constants.SupplementStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(third); err != ErrSupplementLimit {
		t.Fatalf("expected ErrSupplementLimit, got %v", err)
	}
	items, err := repo.ListByPostID(post.ID, nil)
	if err != nil {
		t.Fatalf("list supplements: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 supplements after rejected create, got %d", len(items))
	}
}

func TestSupplementRepositoryPendingUnique(t *testing.T) {
	db := newTestDB(t)
	postRepo := NewPostRepository(db)
	repo := NewSupplementRepository(db)
	post := newTestPost(t, postRepo)

	pending := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "待审", Status: constants.SupplementStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(pending); err != nil {
		t.Fatalf("create pending supplement: %v", err)
	}
	// 已存在一条待审，第二条（无论是否命中敏感词）整次拒绝
	another := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "再来一条", Status: constants.SupplementStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(another); err != ErrSupplementPending {
		t.Fatalf("expected ErrSupplementPending, got %v", err)
	}
}

func TestSupplementRepositoryRejectedNotCounted(t *testing.T) {
	db := newTestDB(t)
	postRepo := NewPostRepository(db)
	repo := NewSupplementRepository(db)
	post := newTestPost(t, postRepo)

	rejected := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "被驳回", Status: constants.SupplementStatusRejected, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(rejected); err != nil {
		t.Fatalf("create rejected supplement: %v", err)
	}
	// 已驳回不占额度，仍可写满两条有效追记
	for i, content := range []string{"有效一", "有效二"} {
		s := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: content, Status: constants.SupplementStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := repo.CreateWithLimit(s); err != nil {
			t.Fatalf("create active supplement %d: %v", i, err)
		}
		if s.Seq != i+1 {
			t.Fatalf("expected seq %d, got %d", i+1, s.Seq)
		}
	}
	overflow := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "超限", Status: constants.SupplementStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(overflow); err != ErrSupplementLimit {
		t.Fatalf("expected ErrSupplementLimit, got %v", err)
	}
}

func TestSupplementRepositoryListByPostID(t *testing.T) {
	db := newTestDB(t)
	postRepo := NewPostRepository(db)
	repo := NewSupplementRepository(db)
	post := newTestPost(t, postRepo)

	seed := []struct {
		content string
		status  int
	}{
		{"已发布", constants.SupplementStatusPublished},
		{"待审核", constants.SupplementStatusPending},
	}
	for _, item := range seed {
		s := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: item.content, Status: item.status, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := repo.CreateWithLimit(s); err != nil {
			t.Fatalf("create supplement: %v", err)
		}
	}
	// 待审被拒绝后补一条已驳回记录（保留其待审时的序号）
	rejected := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Seq: 2, Content: "已驳回", Status: constants.SupplementStatusRejected, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(rejected).Error; err != nil {
		t.Fatalf("create rejected supplement: %v", err)
	}

	published, err := repo.ListByPostID(post.ID, []int{constants.SupplementStatusPublished})
	if err != nil {
		t.Fatalf("list published: %v", err)
	}
	if len(published) != 1 || published[0].Content != "已发布" {
		t.Fatalf("expected only published supplement, got %+v", published)
	}
	all, err := repo.ListByPostID(post.ID, nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 supplements, got %d", len(all))
	}
	if all[0].Seq != 1 || all[1].Seq != 2 {
		t.Fatalf("expected seq order, got %d, %d", all[0].Seq, all[1].Seq)
	}
}

func TestSupplementRepositoryFindAndUpdate(t *testing.T) {
	db := newTestDB(t)
	postRepo := NewPostRepository(db)
	repo := NewSupplementRepository(db)
	post := newTestPost(t, postRepo)

	s := &model.PostSupplement{PostID: post.ID, IdentityID: 1, Content: "待审内容", Status: constants.SupplementStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateWithLimit(s); err != nil {
		t.Fatalf("create supplement: %v", err)
	}
	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("find supplement: %v", err)
	}
	got.Status = constants.SupplementStatusRejected
	got.ReviewNote = "包含敏感词"
	if err := repo.Update(got); err != nil {
		t.Fatalf("update supplement: %v", err)
	}
	again, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("find supplement again: %v", err)
	}
	if again.Status != constants.SupplementStatusRejected || again.ReviewNote != "包含敏感词" {
		t.Fatalf("update not persisted: %+v", again)
	}
	if _, err := repo.FindByID(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
