package repository

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
)

var (
	// ErrSupplementLimit 有效追记（已发布 + 待审）已达上限。
	ErrSupplementLimit = errors.New("supplement limit reached")
	// ErrSupplementPending 同一帖子已存在待审核的追记。
	ErrSupplementPending = errors.New("supplement pending exists")
)

type SupplementRepository interface {
	CreateWithLimit(supplement *model.PostSupplement) error
	Update(supplement *model.PostSupplement) error
	FindByID(id uint) (*model.PostSupplement, error)
	ListByPostID(postID uint, statuses []int) ([]model.PostSupplement, error)
}

type supplementRepository struct {
	db *gorm.DB
}

func NewSupplementRepository(db *gorm.DB) SupplementRepository {
	return &supplementRepository{db: db}
}

// CreateWithLimit 在事务中锁定帖子行后校验数量约束并写入追记：
// 有效追记达到上限、或已存在待审追记时整次拒绝（不写入任何数据）；
// 并发提交在帖子行锁上串行化，后到的请求会看到先到请求写入的结果并被拒绝。
func (r *supplementRepository) CreateWithLimit(supplement *model.PostSupplement) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		lockTx := tx
		if tx.Dialector.Name() == "mysql" {
			// MySQL 下通过帖子行锁串行化同一帖子的并发追记写入；sqlite（单测）不支持该语法
			lockTx = lockTx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var post model.Post
		if err := lockTx.Select("id").First(&post, supplement.PostID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock post for supplement: %w", err)
		}
		var active int64
		if err := tx.Model(&model.PostSupplement{}).
			Where("post_id = ? AND status IN ?", supplement.PostID,
				[]int{constants.SupplementStatusPublished, constants.SupplementStatusPending}).
			Count(&active).Error; err != nil {
			return fmt.Errorf("count active supplements: %w", err)
		}
		if active >= constants.SupplementMaxActive {
			return ErrSupplementLimit
		}
		var pending int64
		if err := tx.Model(&model.PostSupplement{}).
			Where("post_id = ? AND status = ?", supplement.PostID, constants.SupplementStatusPending).
			Count(&pending).Error; err != nil {
			return fmt.Errorf("count pending supplements: %w", err)
		}
		if pending >= constants.SupplementMaxPending {
			return ErrSupplementPending
		}
		supplement.Seq = int(active) + 1
		if err := tx.Create(supplement).Error; err != nil {
			return fmt.Errorf("create supplement: %w", err)
		}
		return nil
	})
}

func (r *supplementRepository) Update(supplement *model.PostSupplement) error {
	if err := r.db.Save(supplement).Error; err != nil {
		return fmt.Errorf("update supplement: %w", err)
	}
	return nil
}

func (r *supplementRepository) FindByID(id uint) (*model.PostSupplement, error) {
	var supplement model.PostSupplement
	if err := r.db.First(&supplement, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find supplement by id: %w", err)
	}
	return &supplement, nil
}

// ListByPostID 按帖子查询追记；statuses 为空时返回全部状态。
func (r *supplementRepository) ListByPostID(postID uint, statuses []int) ([]model.PostSupplement, error) {
	var items []model.PostSupplement
	q := r.db.Where("post_id = ?", postID)
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	if err := q.Order("seq ASC, id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list supplements: %w", err)
	}
	return items, nil
}
