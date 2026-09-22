package model

import "time"

// PostAddendum 楼主追记：作者对自己已发布帖子追加的不可修改补充说明。
// 命中敏感词的追记进入审核（pending），通过后公开（published），驳回（rejected）后不占名额。
type PostAddendum struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	PostID     uint       `gorm:"index;not null" json:"postId"`
	Post       *Post      `gorm:"foreignKey:PostID" json:"post,omitempty"`
	IdentityID uint       `gorm:"index;not null" json:"identityId"`
	Content    string     `gorm:"type:varchar(500);not null" json:"content"`
	Status     int        `gorm:"default:1;index" json:"status"`
	HitWords   string     `gorm:"type:varchar(255)" json:"hitWords"`
	ReviewedBy *uint      `json:"reviewedBy,omitempty"`
	ReviewNote string     `gorm:"type:varchar(255)" json:"reviewNote"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
