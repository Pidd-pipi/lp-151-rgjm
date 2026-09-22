package repository

import (
	"errors"
	"fmt"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrAddendumPendingExists 同一帖子已存在一条待审核追记。
	ErrAddendumPendingExists = errors.New("pending addendum already exists")
	// ErrAddendumLimitReached 该帖追记名额已满（两条）。
	ErrAddendumLimitReached = errors.New("addendum limit reached")
)

type PostAddendumRepository interface {
	// CreateIfSlotAvailable 在事务内锁定帖子行，校验"仅一条待审、最多两条已通过/待审"后创建追记。
	CreateIfSlotAvailable(addendum *model.PostAddendum) error
	FindByID(id uint) (*model.PostAddendum, error)
	// ListByPostID 返回帖子的全部追记（含各审核状态），按创建顺序返回。
	ListByPostID(postID uint) ([]model.PostAddendum, error)
	Update(addendum *model.PostAddendum) error
}

type postAddendumRepository struct {
	db *gorm.DB
}

func NewPostAddendumRepository(db *gorm.DB) PostAddendumRepository {
	return &postAddendumRepository{db: db}
}

// CreateIfSlotAvailable 通过对帖子行加写锁串行化同一帖子的并发追记提交：
// 已存在待审追记或名额（已通过 + 待审）已满时整次拒绝，任何记录都不会写入。
// 已占用的追记行同样使用加锁读，确保在 MySQL 可重复读级别下也能读到最新已提交数据。
// SQLite（单元测试）不支持 SELECT ... FOR UPDATE，自动退化为普通读。
func (r *postAddendumRepository) CreateIfSlotAvailable(addendum *model.PostAddendum) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		postQuery := tx.Model(&model.Post{}).Where("id = ?", addendum.PostID)
		occupiedQuery := tx.Model(&model.PostAddendum{}).
			Where("post_id = ? AND status IN ?", addendum.PostID,
				[]int{constants.AddendumStatusPublished, constants.AddendumStatusPending})
		if r.db.Dialector.Name() == "mysql" {
			postQuery = postQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			occupiedQuery = occupiedQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}

		var post model.Post
		if err := postQuery.First(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock post for addendum: %w", err)
		}

		var occupied []model.PostAddendum
		if err := occupiedQuery.Find(&occupied).Error; err != nil {
			return fmt.Errorf("lock occupied addenda: %w", err)
		}
		pendingCount := 0
		for _, a := range occupied {
			if a.Status == constants.AddendumStatusPending {
				pendingCount++
			}
		}
		if pendingCount > 0 {
			return ErrAddendumPendingExists
		}
		if len(occupied) >= constants.AddendumMaxPerPost {
			return ErrAddendumLimitReached
		}

		if err := tx.Create(addendum).Error; err != nil {
			return fmt.Errorf("create post addendum: %w", err)
		}
		return nil
	})
}

func (r *postAddendumRepository) FindByID(id uint) (*model.PostAddendum, error) {
	var addendum model.PostAddendum
	if err := r.db.First(&addendum, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find addendum by id: %w", err)
	}
	return &addendum, nil
}

func (r *postAddendumRepository) ListByPostID(postID uint) ([]model.PostAddendum, error) {
	var addenda []model.PostAddendum
	if err := r.db.Where("post_id = ?", postID).Order("id ASC").Find(&addenda).Error; err != nil {
		return nil, fmt.Errorf("list addenda by post: %w", err)
	}
	return addenda, nil
}

func (r *postAddendumRepository) Update(addendum *model.PostAddendum) error {
	if err := r.db.Save(addendum).Error; err != nil {
		return fmt.Errorf("update addendum: %w", err)
	}
	return nil
}
