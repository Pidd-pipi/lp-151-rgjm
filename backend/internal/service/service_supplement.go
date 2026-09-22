package service

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
)

var (
	ErrNotPostAuthor     = errors.New("not post author")
	ErrPostNotPublished  = errors.New("post not published")
	ErrSupplementLimit   = errors.New("supplement limit reached")
	ErrSupplementPending = errors.New("supplement pending exists")
)

type SupplementService interface {
	Create(identityID, postID uint, content string) (*model.PostSupplement, []string, bool, error)
	ListForPost(postID uint, viewerIdentityID uint) ([]model.PostSupplement, error)
}

type supplementService struct {
	supplements repository.SupplementRepository
	posts       repository.PostRepository
	sensitive   SensitiveWordService
	review      ReviewService
	logger      *slog.Logger
}

func NewSupplementService(supplements repository.SupplementRepository, posts repository.PostRepository, sensitive SensitiveWordService, review ReviewService, logger *slog.Logger) SupplementService {
	return &supplementService{supplements: supplements, posts: posts, sensitive: sensitive, review: review, logger: logger}
}

// Create 楼主追加追记。命中敏感词时进入审核（blocked=true），原帖与已通过追记不受影响；
// 数量上限与待审唯一性由仓储层在事务内保证，并发或超限时整次拒绝。
func (s *supplementService) Create(identityID, postID uint, content string) (*model.PostSupplement, []string, bool, error) {
	post, err := s.posts.FindByID(postID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, false, ErrPostNotFound
		}
		return nil, nil, false, err
	}
	if post.IdentityID != identityID {
		return nil, nil, false, ErrNotPostAuthor
	}
	if post.Status != constants.PostStatusPublished {
		return nil, nil, false, ErrPostNotPublished
	}
	hits, blocked := s.sensitive.Detect(content)
	supplement := &model.PostSupplement{
		PostID:     postID,
		IdentityID: identityID,
		Content:    content,
		Status:     constants.SupplementStatusPublished,
		HitWords:   strings.Join(hits, ","),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if blocked {
		supplement.Status = constants.SupplementStatusPending
	}
	if err := s.supplements.CreateWithLimit(supplement); err != nil {
		if errors.Is(err, repository.ErrSupplementLimit) {
			return nil, hits, blocked, ErrSupplementLimit
		}
		if errors.Is(err, repository.ErrSupplementPending) {
			return nil, hits, blocked, ErrSupplementPending
		}
		return nil, hits, blocked, err
	}
	if blocked {
		if err := s.review.Enqueue("supplement", supplement.ID, content, hits); err != nil {
			s.logger.Error("enqueue supplement review", "error", err)
		}
	}
	return supplement, hits, blocked, nil
}

// ListForPost 楼主可见全部追记（含待审与已驳回及其审核原因），其他用户仅可见已发布追记。
func (s *supplementService) ListForPost(postID uint, viewerIdentityID uint) ([]model.PostSupplement, error) {
	post, err := s.posts.FindByID(postID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPostNotFound
		}
		return nil, err
	}
	statuses := []int{constants.SupplementStatusPublished}
	if viewerIdentityID > 0 && viewerIdentityID == post.IdentityID {
		statuses = nil
	}
	return s.supplements.ListByPostID(postID, statuses)
}
