package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/dto"
	"github.com/gbtreehole/backend/internal/service"
)

type AddendumHandler struct {
	addenda service.PostAddendumService
	logger  *slog.Logger
}

func NewAddendumHandler(addenda service.PostAddendumService, logger *slog.Logger) *AddendumHandler {
	return &AddendumHandler{addenda: addenda, logger: logger}
}

// AddAddendum 楼主为自己的帖子追加一条不可修改的补充说明（最多两条）。
// 命中敏感词进入审核；已有待审追记或名额已满时整次拒绝（409）。
//
//	@Summary	追加楼主追记
//	@Tags		addendum
//	@Accept		json
//	@Produce	json
//	@Param		id		path		int							true	"帖子ID"
//	@Param		request	body		dto.CreateAddendumRequest	true	"追记内容（≤500字）"
//	@Success	200		{object}	Response
//	@Router		/api/v1/posts/{id}/addenda [post]
func (h *AddendumHandler) AddAddendum(c *gin.Context) {
	postID := parseID(c)
	if postID == 0 {
		return
	}
	identityID := c.GetUint("identityId")
	var req dto.CreateAddendumRequest
	if !BindAndValidate(c, &req) {
		return
	}
	addendum, hits, blocked, err := h.addenda.Add(identityID, postID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAddendumContentEmpty), errors.Is(err, service.ErrAddendumTooLong):
			Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "addendum content must be 1-500 non-blank characters")
		case errors.Is(err, service.ErrPostNotFound):
			Fail(c, http.StatusNotFound, constants.CodeNotFound, "post not found")
		case errors.Is(err, service.ErrNotPostAuthor):
			Fail(c, http.StatusForbidden, constants.CodeForbidden, "only the post author can add addenda")
		case errors.Is(err, service.ErrPostNotPublished):
			Fail(c, http.StatusConflict, constants.CodeConflict, "post is not published")
		case errors.Is(err, service.ErrAddendumPendingExists):
			Fail(c, http.StatusConflict, constants.CodeConflict, "a pending addendum already exists, wait for review")
		case errors.Is(err, service.ErrAddendumLimitReached):
			Fail(c, http.StatusConflict, constants.CodeConflict, "addendum limit reached, at most 2 per post")
		default:
			h.logger.Error("add addendum", "error", err)
			Fail(c, http.StatusInternalServerError, constants.CodeInternal, "add addendum failed")
		}
		return
	}
	view := service.AddendumView{Addendum: *addendum, IsOwner: true}
	OK(c, gin.H{"addendum": toAddendumResponse(view), "blocked": blocked, "hitWords": hits})
}

// toAddendumResponse 将追记视图转为响应；非楼主访问者拿不到驳回原因与命中词。
func toAddendumResponse(view service.AddendumView) dto.AddendumResponse {
	a := view.Addendum
	resp := dto.AddendumResponse{
		ID:        a.ID,
		PostID:    a.PostID,
		Content:   a.Content,
		Status:    a.Status,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
	if a.ReviewedAt != nil {
		resp.ReviewedAt = a.ReviewedAt.Format(time.RFC3339)
	}
	// 审核结果细节（命中词、审核备注/驳回原因）仅作者可见。
	if view.IsOwner {
		resp.HitWords = a.HitWords
		resp.ReviewNote = a.ReviewNote
	}
	return resp
}
