package repository

import (
	"sync"
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"gorm.io/gorm"
)

func seedPostForAddendum(t *testing.T, db *gorm.DB, identityID uint, postID uint) {
	t.Helper()
	post := &model.Post{
		ID:         postID,
		IdentityID: identityID,
		Content:    "原帖内容",
		Status:     constants.PostStatusPublished,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.Create(post).Error; err != nil {
		t.Fatalf("seed post: %v", err)
	}
}

func TestAddendumRepository_SlotRules(t *testing.T) {
	db := newTestDB(t)
	repo := NewPostAddendumRepository(db)
	seedPostForAddendum(t, db, 1, 100)

	// 第一条：直接放行（已发布），占一个名额
	first := &model.PostAddendum{PostID: 100, IdentityID: 1, Content: "补充一", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(first); err != nil {
		t.Fatalf("create first addendum: %v", err)
	}

	// 第二条：命中敏感词进入待审，占第二个名额
	second := &model.PostAddendum{PostID: 100, IdentityID: 1, Content: "待审补充", Status: constants.AddendumStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(second); err != nil {
		t.Fatalf("create second addendum: %v", err)
	}

	// 第三条：已有待审 -> 拒绝
	third := &model.PostAddendum{PostID: 100, IdentityID: 1, Content: "再补一条", Status: constants.AddendumStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(third); err != ErrAddendumPendingExists {
		t.Fatalf("expected ErrAddendumPendingExists, got %v", err)
	}

	// 驳回待审那条后释放名额（已发布仍占 1 个）
	second.Status = constants.AddendumStatusRejected
	if err := repo.Update(second); err != nil {
		t.Fatalf("reject second: %v", err)
	}
	retry := &model.PostAddendum{PostID: 100, IdentityID: 1, Content: "驳回后另写一条", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(retry); err != nil {
		t.Fatalf("create replacement after rejection: %v", err)
	}

	// 名额已满（两条已发布）-> 整次拒绝
	fourth := &model.PostAddendum{PostID: 100, IdentityID: 1, Content: "超额追记", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(fourth); err != ErrAddendumLimitReached {
		t.Fatalf("expected ErrAddendumLimitReached, got %v", err)
	}

	list, err := repo.ListByPostID(100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 addendum rows (2 published + 1 rejected), got %d", len(list))
	}
}

func TestAddendumRepository_TwoPublishedRejectNew(t *testing.T) {
	db := newTestDB(t)
	repo := NewPostAddendumRepository(db)
	seedPostForAddendum(t, db, 1, 200)

	for i, content := range []string{"第一条", "第二条"} {
		a := &model.PostAddendum{PostID: 200, IdentityID: 1, Content: content, Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := repo.CreateIfSlotAvailable(a); err != nil {
			t.Fatalf("create addendum %d: %v", i, err)
		}
	}
	extra := &model.PostAddendum{PostID: 200, IdentityID: 1, Content: "不该存在", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(extra); err != ErrAddendumLimitReached {
		t.Fatalf("expected ErrAddendumLimitReached, got %v", err)
	}
}

func TestAddendumRepository_PendingBlocksPublished(t *testing.T) {
	db := newTestDB(t)
	repo := NewPostAddendumRepository(db)
	seedPostForAddendum(t, db, 1, 300)

	pending := &model.PostAddendum{PostID: 300, IdentityID: 1, Content: "待审", Status: constants.AddendumStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(pending); err != nil {
		t.Fatalf("create pending: %v", err)
	}
	// 即便另一条不含敏感词（会直接发布），有待审存在也必须整次拒绝
	clean := &model.PostAddendum{PostID: 300, IdentityID: 1, Content: "干净补充", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateIfSlotAvailable(clean); err != ErrAddendumPendingExists {
		t.Fatalf("expected ErrAddendumPendingExists, got %v", err)
	}
}

// TestAddendumRepository_ConcurrentSubmit 并发提交时，至多两条成功、其余整次拒绝。
func TestAddendumRepository_ConcurrentSubmit(t *testing.T) {
	db := newTestDB(t)
	// SQLite 内存库需要单连接共享，事务天然串行，等效于 MySQL 下行锁的临界区。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	repo := NewPostAddendumRepository(db)
	seedPostForAddendum(t, db, 1, 400)

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			a := &model.PostAddendum{PostID: 400, IdentityID: 1, Content: "并发追记", Status: constants.AddendumStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
			errs[i] = repo.CreateIfSlotAvailable(a)
		}()
	}
	wg.Wait()

	success, rejected := 0, 0
	for _, err := range errs {
		switch err {
		case nil:
			success++
		case ErrAddendumLimitReached, ErrAddendumPendingExists:
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success != constants.AddendumMaxPerPost {
		t.Fatalf("expected exactly %d successful inserts, got %d (rejected=%d)", constants.AddendumMaxPerPost, success, rejected)
	}
	if success+rejected != workers {
		t.Fatalf("all attempts must be success or business reject")
	}
	list, err := repo.ListByPostID(400)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != constants.AddendumMaxPerPost {
		t.Fatalf("expected %d rows, got %d", constants.AddendumMaxPerPost, len(list))
	}
}
