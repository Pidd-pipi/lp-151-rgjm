package dto

type CreateSupplementRequest struct {
	Content string `json:"content" validate:"required,min=1,max=500"`
}

type SupplementResponse struct {
	ID         uint   `json:"id"`
	PostID     uint   `json:"postId"`
	Seq        int    `json:"seq"`
	Content    string `json:"content"`
	Status     int    `json:"status"`
	HitWords   string `json:"hitWords,omitempty"`
	ReviewNote string `json:"reviewNote,omitempty"`
	CreatedAt  string `json:"createdAt"`
}
