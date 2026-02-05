package models

import "time"

type TaxonomyNode struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Label     string    `gorm:"size:255;index" json:"label"`
	ParentID  *string   `gorm:"size:36;index" json:"parentId"`
	Path      string    `gorm:"size:512;uniqueIndex" json:"path"`
	Level     int       `json:"level"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
