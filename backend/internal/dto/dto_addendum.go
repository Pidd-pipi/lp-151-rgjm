package dto

// CreateAddendumRequest 楼主追记提交，正文 1~500 字，不可修改。
type CreateAddendumRequest struct {
	Content string `json:"content" validate:"required,min=1,max=500"`
}

// AddendumResponse 楼主追记视图。驳回原因 reviewNote 仅楼主本人可见，由服务端按身份过滤。
type AddendumResponse struct {
	ID         uint   `json:"id"`
	PostID     uint   `json:"postId"`
	Content    string `json:"content"`
	Status     int    `json:"status"` // 1 已发布 2 待审核 3 已驳回
	HitWords   string `json:"hitWords,omitempty"`
	ReviewNote string `json:"reviewNote,omitempty"` // 驳回/放行备注，仅作者可见
	CreatedAt  string `json:"createdAt"`
	ReviewedAt string `json:"reviewedAt,omitempty"`
}
