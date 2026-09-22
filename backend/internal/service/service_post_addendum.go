package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
)

var (
	// ErrAddendumNotFound 追记不存在。
	ErrAddendumNotFound = errors.New("addendum not found")
	// ErrNotPostAuthor 非帖子作者，不能追加楼主追记。
	ErrNotPostAuthor = errors.New("not post author")
	// ErrPostNotPublished 只能对已公开发布的帖子追加追记。
	ErrPostNotPublished = errors.New("post not published")
	// ErrAddendumPendingExists 同一帖子已有一条待审核追记。
	ErrAddendumPendingExists = errors.New("a pending addendum already exists for this post")
	// ErrAddendumLimitReached 同一帖子最多保留两条追记。
	ErrAddendumLimitReached = errors.New("addendum limit reached for this post")
	// ErrAddendumContentEmpty 去除首尾空白后内容为空。
	ErrAddendumContentEmpty = errors.New("addendum content is empty")
	// ErrAddendumTooLong 内容超过 500 字。
	ErrAddendumTooLong = errors.New("addendum content too long")
)

// AddendumView 面向详情页的追记视图。
// 驳回原因仅在 IsOwner 为 true 时由 handler 填充，其他访问者永远拿不到。
type AddendumView struct {
	Addendum model.PostAddendum
	IsOwner  bool
}

type PostAddendumService interface {
	// Add 作者为自己的帖子追加一条追记。命中敏感词进入审核，原帖不受影响。
	Add(identityID, postID uint, content string) (*model.PostAddendum, []string, bool, error)
	// ListByPostID 返回帖子追记视图，viewerID 用于判定驳回原因可见性。
	ListByPostID(postID, viewerID uint) ([]AddendumView, error)
	// CountPublished 返回已通过、公开展示的追记数量。
	CountPublished(postID uint) (int, error)
}

type postAddendumService struct {
	addenda   repository.PostAddendumRepository
	posts     repository.PostRepository
	sensitive SensitiveWordService
	review    ReviewService
	logger    *slog.Logger
}

func NewPostAddendumService(addenda repository.PostAddendumRepository, posts repository.PostRepository, sensitive SensitiveWordService, review ReviewService, logger *slog.Logger) PostAddendumService {
	return &postAddendumService{addenda: addenda, posts: posts, sensitive: sensitive, review: review, logger: logger}
}

func (s *postAddendumService) Add(identityID, postID uint, content string) (*model.PostAddendum, []string, bool, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil, false, ErrAddendumContentEmpty
	}
	if len([]rune(content)) > constants.AddendumMaxLength {
		return nil, nil, false, ErrAddendumTooLong
	}
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
	addendum := &model.PostAddendum{
		PostID:     postID,
		IdentityID: identityID,
		Content:    content,
		Status:     constants.AddendumStatusPublished,
		HitWords:   strings.Join(hits, ","),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if blocked {
		addendum.Status = constants.AddendumStatusPending
	}

	// 事务内加锁校验：同一帖子只能有一条待审，已通过+待审最多两条。
	if err := s.addenda.CreateIfSlotAvailable(addendum); err != nil {
		switch {
		case errors.Is(err, repository.ErrAddendumPendingExists):
			return nil, hits, blocked, ErrAddendumPendingExists
		case errors.Is(err, repository.ErrAddendumLimitReached):
			return nil, hits, blocked, ErrAddendumLimitReached
		default:
			return nil, hits, blocked, fmt.Errorf("create addendum: %w", err)
		}
	}

	if blocked {
		if err := s.review.Enqueue("addendum", addendum.ID, content, hits); err != nil {
			// 追记已落库为待审，审核入队失败仅记录，后台仍可通过待审列表看到。
			s.logger.Error("enqueue addendum review", "addendumId", addendum.ID, "error", err)
		}
	}
	return addendum, hits, blocked, nil
}

func (s *postAddendumService) ListByPostID(postID, viewerID uint) ([]AddendumView, error) {
	addenda, err := s.addenda.ListByPostID(postID)
	if err != nil {
		return nil, err
	}
	views := make([]AddendumView, 0, len(addenda))
	for _, a := range addenda {
		views = append(views, AddendumView{Addendum: a, IsOwner: a.IdentityID == viewerID})
	}
	return views, nil
}

func (s *postAddendumService) CountPublished(postID uint) (int, error) {
	addenda, err := s.addenda.ListByPostID(postID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range addenda {
		if a.Status == constants.AddendumStatusPublished {
			count++
		}
	}
	return count, nil
}
