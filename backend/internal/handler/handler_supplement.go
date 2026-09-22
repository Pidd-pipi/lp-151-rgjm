package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/dto"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/service"
)

type SupplementHandler struct {
	supplements service.SupplementService
	logger      *slog.Logger
}

func NewSupplementHandler(supplements service.SupplementService, logger *slog.Logger) *SupplementHandler {
	return &SupplementHandler{supplements: supplements, logger: logger}
}

// CreateSupplement 楼主追加追记
// @Summary 楼主追加追记
// @Tags supplement
// @Accept json
// @Produce json
// @Param id path int true "帖子ID"
// @Param request body dto.CreateSupplementRequest true "追记内容"
// @Success 200 {object} Response
// @Router /api/v1/posts/{id}/supplements [post]
func (h *SupplementHandler) CreateSupplement(c *gin.Context) {
	postID := parseID(c)
	if postID == 0 {
		return
	}
	identityID := c.GetUint("identityId")
	var req dto.CreateSupplementRequest
	if !BindAndValidate(c, &req) {
		return
	}
	supplement, hits, blocked, err := h.supplements.Create(identityID, postID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPostNotFound):
			Fail(c, http.StatusNotFound, constants.CodeNotFound, "post not found")
		case errors.Is(err, service.ErrNotPostAuthor):
			Fail(c, http.StatusForbidden, constants.CodeForbidden, "only post author can append supplement")
		case errors.Is(err, service.ErrPostNotPublished):
			Fail(c, http.StatusConflict, constants.CodeConflict, "post not published")
		case errors.Is(err, service.ErrSupplementLimit):
			Fail(c, http.StatusConflict, constants.CodeConflict, "supplement limit reached")
		case errors.Is(err, service.ErrSupplementPending):
			Fail(c, http.StatusConflict, constants.CodeConflict, "pending supplement exists")
		default:
			h.logger.Error("create supplement", "error", err)
			Fail(c, http.StatusInternalServerError, constants.CodeInternal, "create supplement failed")
		}
		return
	}
	OK(c, gin.H{"supplement": toSupplementResponse(supplement, true), "blocked": blocked, "hitWords": hits})
}

// toSupplementResponse includeReview 为 true 时（作者本人/创建响应）携带命中词与审核原因。
func toSupplementResponse(supplement *model.PostSupplement, includeReview bool) dto.SupplementResponse {
	resp := dto.SupplementResponse{
		ID:        supplement.ID,
		PostID:    supplement.PostID,
		Seq:       supplement.Seq,
		Content:   supplement.Content,
		Status:    supplement.Status,
		CreatedAt: supplement.CreatedAt.Format(time.RFC3339),
	}
	if includeReview {
		resp.HitWords = supplement.HitWords
		resp.ReviewNote = supplement.ReviewNote
	}
	return resp
}
