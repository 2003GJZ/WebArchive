package settings

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"webarchive/internal/models"
)

const (
	KeyLLMBaseURL = "llm.base_url"
	KeyLLMAPIKey  = "llm.api_key"
	KeyLLMModel   = "llm.model"
)

type LLMSettings struct {
	BaseURL string
	APIKey  string
	Model   string
}

func LoadLLM(db *gorm.DB) (LLMSettings, error) {
	out := LLMSettings{}
	keys := []string{KeyLLMBaseURL, KeyLLMAPIKey, KeyLLMModel}
	var rows []models.AppSetting
	if err := db.Where("setting_key IN ?", keys).Find(&rows).Error; err != nil {
		return out, err
	}
	for _, row := range rows {
		switch row.Key {
		case KeyLLMBaseURL:
			out.BaseURL = row.Value
		case KeyLLMAPIKey:
			out.APIKey = row.Value
		case KeyLLMModel:
			out.Model = row.Value
		}
	}
	return out, nil
}

func SaveLLM(db *gorm.DB, cfg LLMSettings) error {
	rows := []models.AppSetting{
		{Key: KeyLLMBaseURL, Value: cfg.BaseURL},
		{Key: KeyLLMAPIKey, Value: cfg.APIKey},
		{Key: KeyLLMModel, Value: cfg.Model},
	}
	for _, row := range rows {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "setting_key"}},
			UpdateAll: true,
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
