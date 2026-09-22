package model

import "time"

// PostSupplement 楼主追记：作者为自己的帖子追加的补充说明，创建后不可修改。
// 命中敏感词的追记进入审核（Status=pending），不影响原帖与已通过追记的公开状态；
// 被驳回的追记保留审核原因，仅作者本人可见。
type PostSupplement struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	PostID     uint       `gorm:"index;not null" json:"postId"`
	Post       *Post      `gorm:"foreignKey:PostID" json:"post,omitempty"`
	IdentityID uint       `gorm:"index;not null" json:"identityId"`
	Seq        int        `gorm:"not null" json:"seq"`
	Content    string     `gorm:"type:varchar(500);not null" json:"content"`
	Status     int        `gorm:"default:1;index" json:"status"`
	HitWords   string     `gorm:"type:varchar(255)" json:"hitWords"`
	ReviewNote string     `gorm:"type:varchar(255)" json:"reviewNote"`
	ReviewedBy *uint      `json:"reviewedBy,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
