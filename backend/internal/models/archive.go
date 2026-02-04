package models

import (
	"time"

	"gorm.io/datatypes"
)

type Archive struct {
	ID          string         `gorm:"primaryKey;size:36" json:"id"`
	Title       string         `gorm:"size:500" json:"title"`
	URL         string         `gorm:"size:2000" json:"url"`
	SiteName    string         `gorm:"size:255" json:"siteName"`
	Byline      string         `gorm:"size:255" json:"byline"`
	Excerpt     string         `gorm:"type:text" json:"excerpt"`
	Favicon     string         `gorm:"size:2000" json:"favicon"`
	Category    string         `gorm:"size:255" json:"category"`
	TagsJSON    datatypes.JSON `gorm:"type:json" json:"tags"`
	ContentText string         `gorm:"type:longtext" json:"contentText,omitempty"`
	CapturedAt  *time.Time     `json:"capturedAt"`
	HTMLPath    string         `gorm:"size:1024" json:"htmlPath"`
	AssetsJSON  datatypes.JSON `gorm:"type:json" json:"assets"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}
