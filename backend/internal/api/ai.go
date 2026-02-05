package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"webarchive/internal/ai"
	"webarchive/internal/models"
)

type AIConfigRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

func (s *Server) updateAIConfig(c *gin.Context) {
	var req AIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if s.LLM == nil {
		s.LLM = ai.NewClient(req.BaseURL, req.APIKey, req.Model, 30*time.Second)
	} else {
		if req.BaseURL != "" {
			s.LLM.BaseURL = req.BaseURL
		}
		if req.APIKey != "" {
			s.LLM.APIKey = req.APIKey
		}
		if req.Model != "" {
			s.LLM.Model = req.Model
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"baseUrl": s.LLM.BaseURL,
		"model":   s.LLM.Model,
		"enabled": s.LLM.Enabled(),
	})
}

func (s *Server) aiTagArchive(c *gin.Context) {
	if s.LLM == nil || !s.LLM.Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "llm not configured"})
		return
	}

	var item models.Archive
	if err := s.DB.First(&item, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	updated, err := s.classifyArchive(ctx, item)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "llm failed"})
		return
	}
	paths, _ := s.loadArchivePaths(updated.ID)
	c.JSON(http.StatusOK, toArchiveResponse(updated, paths))
}

func (s *Server) tagArchive(ctx context.Context, item models.Archive) (models.Archive, error) {
	input := ai.TagInput{
		Title:   item.Title,
		URL:     item.URL,
		Content: item.ContentText,
		Excerpt: item.Excerpt,
	}
	result, err := s.LLM.Tag(ctx, input)
	if err != nil {
		return item, err
	}

	tagsJSON, _ := json.Marshal(result.Tags)
	hierarchyJSON, _ := json.Marshal(result.Path)
	hierarchyPath := strings.Join(result.Path, "/")
	if hierarchyPath == "" && result.Category != "" {
		hierarchyPath = result.Category
		hierarchyJSON, _ = json.Marshal([]string{result.Category})
	}

	if err := s.DB.Model(&models.Archive{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"category":       result.Category,
			"tags_json":      tagsJSON,
			"hierarchy_json": hierarchyJSON,
			"hierarchy_path": hierarchyPath,
		}).Error; err != nil {
		return item, err
	}

	item.Category = result.Category
	item.TagsJSON = tagsJSON
	item.HierarchyJSON = hierarchyJSON
	item.HierarchyPath = hierarchyPath
	if hierarchyPath != "" {
		_ = s.replaceArchivePaths(item.ID, []string{hierarchyPath})
	}
	return item, nil
}

func (s *Server) classifyArchive(ctx context.Context, item models.Archive) (models.Archive, error) {
	nodes, err := s.loadTaxonomyNodes()
	if err != nil {
		return s.tagArchive(ctx, item)
	}
	path := []string{}
	if len(nodes) > 0 {
		path, _ = s.pickPath(ctx, item, nodes)
	}

	tagged, err := s.LLM.Tag(ctx, ai.TagInput{
		Title:   item.Title,
		URL:     item.URL,
		Content: item.ContentText,
		Excerpt: item.Excerpt,
	})
	if err != nil {
		return item, err
	}

	if len(path) == 0 && len(tagged.Path) > 0 {
		path = tagged.Path
	}
	var chosenPath string
	if len(path) > 0 {
		item.Category = path[0]
		if err := s.ensureTaxonomyPath(path); err != nil {
			return item, err
		}
		hierarchyJSON, _ := json.Marshal(path)
		item.HierarchyJSON = hierarchyJSON
		chosenPath = strings.Join(path, "/")
		item.HierarchyPath = chosenPath
	} else if tagged.Category != "" {
		item.Category = tagged.Category
		chosenPath = tagged.Category
		item.HierarchyPath = chosenPath
		item.HierarchyJSON, _ = json.Marshal([]string{tagged.Category})
	}
	tagsJSON, _ := json.Marshal(tagged.Tags)
	item.TagsJSON = tagsJSON

	if chosenPath != "" {
		_ = s.replaceArchivePaths(item.ID, []string{chosenPath})
	}

	if err := s.DB.Model(&models.Archive{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"category":       item.Category,
			"tags_json":      item.TagsJSON,
			"hierarchy_json": item.HierarchyJSON,
			"hierarchy_path": item.HierarchyPath,
		}).Error; err != nil {
		return item, err
	}

	return item, nil
}

type pickResponse struct {
	Choice string `json:"choice"`
	New    bool   `json:"new"`
	Stop   bool   `json:"stop"`
}

func (s *Server) pickPath(ctx context.Context, item models.Archive, nodes []models.TaxonomyNode) ([]string, error) {
	children := map[string][]string{}
	root := []string{}
	for _, n := range nodes {
		if n.ParentID == nil {
			root = append(root, n.Label)
		} else {
			children[*n.ParentID] = append(children[*n.ParentID], n.Label)
		}
	}

	path := []string{}
	parentID := ""
	options := root

	for depth := 0; depth < 4; depth++ {
		choice, isNew, stop, err := s.pickFromOptions(ctx, item, options, depth == 0)
		if err != nil {
			return path, err
		}
		if stop {
			break
		}
		if choice == "" {
			break
		}
		path = append(path, choice)
		if isNew {
			break
		}

		var nextID string
		for _, n := range nodes {
			if n.Label == choice && ((n.ParentID == nil && parentID == "") || (n.ParentID != nil && *n.ParentID == parentID)) {
				nextID = n.ID
				break
			}
		}
		if nextID == "" {
			break
		}
		parentID = nextID
		options = children[parentID]
		if len(options) == 0 {
			break
		}
	}
	return path, nil
}

func (s *Server) pickFromOptions(ctx context.Context, item models.Archive, options []string, topLevel bool) (string, bool, bool, error) {
	if len(options) == 0 {
		return "", true, false, nil
	}

	limited := options
	if len(limited) > 30 {
		limited = options[:30]
	}

	system := "You are a taxonomy router. Return strict JSON only."
	user := "Choose the best branch label for the content below.\n" +
		"If no label fits, set new=true and provide a new short label (<=6 words).\n" +
		"If you think it should stop here, set stop=true.\n" +
		"Return JSON: {\"choice\":\"label\",\"new\":false,\"stop\":false}\n" +
		"Available labels: " + strings.Join(limited, ", ") + "\n" +
		"Title: " + item.Title + "\nURL: " + item.URL + "\nExcerpt: " + item.Excerpt + "\nContent: " + trimContent(item.ContentText)

	raw, err := s.LLM.ChatJSON(ctx, system, user, 0.1)
	if err != nil {
		return "", false, false, err
	}
	raw = extractJSON(raw)
	if raw == "" {
		return "", false, false, errors.New("invalid json")
	}
	var resp pickResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", false, false, err
	}
	resp.Choice = strings.TrimSpace(resp.Choice)
	if resp.Stop {
		return "", false, true, nil
	}
	if resp.Choice == "" {
		return "", false, false, nil
	}
	if resp.New {
		return resp.Choice, true, false, nil
	}
	return resp.Choice, false, false, nil
}

func trimContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) > 1800 {
		return content[:1800]
	}
	return content
}

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return text[start : end+1]
}
