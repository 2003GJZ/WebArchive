package api

import (
	"strings"

	"github.com/google/uuid"

	"webarchive/internal/models"
)

func (s *Server) loadArchivePaths(archiveID string) ([]string, error) {
	var rows []models.ArchivePath
	if err := s.DB.Where("archive_id = ?", archiveID).Order("path asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Path != "" {
			out = append(out, row.Path)
		}
	}
	return out, nil
}

func (s *Server) replaceArchivePaths(archiveID string, rawPaths []string) error {
	if err := s.DB.Where("archive_id = ?", archiveID).Delete(&models.ArchivePath{}).Error; err != nil {
		return err
	}

	paths := normalizePaths(rawPaths)
	for _, path := range paths {
		parts := strings.Split(path, "/")
		if err := s.ensureTaxonomyPath(parts); err != nil {
			return err
		}
		node, err := s.getNodeByPath(path)
		if err != nil {
			continue
		}
		row := models.ArchivePath{
			ID:        uuid.New().String(),
			ArchiveID: archiveID,
			NodeID:    node.ID,
			Path:      path,
		}
		if err := s.DB.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizePaths(raw []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.Trim(p, "/")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
