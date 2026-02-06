package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"webarchive/internal/ai"
	"webarchive/internal/graphflow"
	"webarchive/internal/models"
	"webarchive/internal/processor"
	"webarchive/internal/storage"
)

type Server struct {
	DB            *gorm.DB
	Store         *storage.MinioStore
	Processor     *processor.Processor
	LLM           *ai.Client
	AutoTag       bool
	Eino          *graphflow.Analyzer
	analyzeMu     sync.Mutex
	analyzeCancel context.CancelFunc
	analyzeStatus AnalysisStatus
}

type CreateArchiveRequest struct {
	URL            string     `json:"url"`
	Title          string     `json:"title"`
	HTML           string     `json:"html"`
	Content        string     `json:"content"`
	Excerpt        string     `json:"excerpt"`
	Byline         string     `json:"byline"`
	SiteName       string     `json:"siteName"`
	Favicon        string     `json:"favicon"`
	CapturedAt     *time.Time `json:"capturedAt"`
	Category       string     `json:"category"`
	Tags           []string   `json:"tags"`
	Hierarchy      []string   `json:"hierarchy"`
	HierarchyPaths []string   `json:"hierarchyPaths"`
	AutoTag        bool       `json:"autoTag"`
}

type UpdateArchiveRequest struct {
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
	Hierarchy      []string `json:"hierarchy"`
	HierarchyPaths []string `json:"hierarchyPaths"`
}

type ArchiveResponse struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	URL            string          `json:"url"`
	SiteName       string          `json:"siteName"`
	Byline         string          `json:"byline"`
	Excerpt        string          `json:"excerpt"`
	Favicon        string          `json:"favicon"`
	Category       string          `json:"category"`
	Tags           []string        `json:"tags"`
	Hierarchy      []string        `json:"hierarchy"`
	HierarchyPath  string          `json:"hierarchyPath"`
	HierarchyPaths []string        `json:"hierarchyPaths"`
	ContentText    string          `json:"contentText,omitempty"`
	CapturedAt     *time.Time      `json:"capturedAt"`
	HTMLPath       string          `json:"htmlPath"`
	AssetsJSON     json.RawMessage `json:"assets"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func toArchiveResponse(item models.Archive, paths []string) ArchiveResponse {
	tags := []string{}
	if len(item.TagsJSON) > 0 {
		_ = json.Unmarshal(item.TagsJSON, &tags)
	}
	hierarchy := []string{}
	if len(item.HierarchyJSON) > 0 {
		_ = json.Unmarshal(item.HierarchyJSON, &hierarchy)
	}
	if paths == nil {
		if item.HierarchyPath != "" {
			paths = []string{item.HierarchyPath}
		} else {
			paths = []string{}
		}
	}
	return ArchiveResponse{
		ID:             item.ID,
		Title:          item.Title,
		URL:            item.URL,
		SiteName:       item.SiteName,
		Byline:         item.Byline,
		Excerpt:        item.Excerpt,
		Favicon:        item.Favicon,
		Category:       item.Category,
		Tags:           tags,
		Hierarchy:      hierarchy,
		HierarchyPath:  item.HierarchyPath,
		HierarchyPaths: paths,
		ContentText:    item.ContentText,
		CapturedAt:     item.CapturedAt,
		HTMLPath:       item.HTMLPath,
		AssetsJSON:     json.RawMessage(item.AssetsJSON),
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	api := r.Group("/api")
	api.POST("/archives", s.createArchive)
	api.GET("/archives", s.listArchives)
	api.GET("/archives/:id", s.getArchive)
	api.PATCH("/archives/:id", s.updateArchive)
	api.DELETE("/archives/:id", s.deleteArchive)
	api.POST("/archives/:id/ai-tag", s.aiTagArchive)
	api.POST("/ai/config", s.updateAIConfig)
	api.POST("/ai/analyze/start", s.startAnalysis)
	api.POST("/ai/analyze/stop", s.stopAnalysis)
	api.GET("/ai/analyze/status", s.analysisStatus)
	api.GET("/taxonomy", s.getTaxonomy)
	api.GET("/taxonomy/:id", s.getTaxonomyNode)
	api.GET("/graph", s.getGraph)
	api.GET("/archives/:id/html", s.getArchiveHTML)
	api.GET("/assets/:id/*path", s.getAsset)
}

func (s *Server) createArchive(c *gin.Context) {
	var req CreateArchiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url required"})
		return
	}

	html := req.HTML
	if html == "" {
		html = req.Content
	}
	if html == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "html required"})
		return
	}

	id := uuid.New().String()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	result, err := s.Processor.Process(ctx, id, req.URL, []byte(html))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
		return
	}

	htmlObject := storage.ArchivePrefix(id) + "/index.html"
	if err := s.Store.PutBytes(ctx, htmlObject, result.HTML, "text/html; charset=utf-8"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store html failed"})
		return
	}

	assetsJSON, _ := json.Marshal(result.Assets)
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.HierarchyPaths == nil {
		req.HierarchyPaths = []string{}
	}
	if req.Hierarchy == nil {
		req.Hierarchy = []string{}
	}
	if len(req.HierarchyPaths) == 0 && len(req.Hierarchy) > 0 {
		req.HierarchyPaths = []string{strings.Join(req.Hierarchy, "/")}
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	hierarchyJSON, _ := json.Marshal(req.Hierarchy)
	hierarchyPath := strings.Join(req.Hierarchy, "/")
	if hierarchyPath == "" && len(req.HierarchyPaths) > 0 {
		hierarchyPath = req.HierarchyPaths[0]
		hierarchyJSON, _ = json.Marshal(strings.Split(req.HierarchyPaths[0], "/"))
	}
	if hierarchyPath == "" && req.Category != "" {
		hierarchyPath = req.Category
		hierarchyJSON, _ = json.Marshal([]string{req.Category})
	}

	archive := models.Archive{
		ID:            id,
		Title:         req.Title,
		URL:           req.URL,
		SiteName:      req.SiteName,
		Byline:        req.Byline,
		Excerpt:       req.Excerpt,
		Favicon:       req.Favicon,
		Category:      req.Category,
		TagsJSON:      tagsJSON,
		HierarchyJSON: hierarchyJSON,
		HierarchyPath: hierarchyPath,
		ContentText:   req.Content,
		CapturedAt:    req.CapturedAt,
		HTMLPath:      "index.html",
		AssetsJSON:    assetsJSON,
	}

	if err := s.DB.Create(&archive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert failed"})
		return
	}

	if len(req.HierarchyPaths) > 0 {
		_ = s.replaceArchivePaths(archive.ID, req.HierarchyPaths)
	} else if len(req.Hierarchy) > 0 {
		_ = s.replaceArchivePaths(archive.ID, []string{strings.Join(req.Hierarchy, "/")})
	} else if req.Category != "" {
		_ = s.replaceArchivePaths(archive.ID, []string{req.Category})
	}

	if (req.AutoTag || s.AutoTag) && s.LLM != nil && s.LLM.Enabled() {
		item := archive
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_, _ = s.classifyArchive(ctx, item)
		}()
	}

	c.JSON(http.StatusOK, toArchiveResponse(archive, nil))
}

func (s *Server) listArchives(c *gin.Context) {
	var items []models.Archive
	query := c.Query("q")
	category := c.Query("category")
	tag := c.Query("tag")

	db := s.DB
	if query != "" {
		like := "%" + query + "%"
		db = db.Where("title LIKE ? OR url LIKE ? OR content_text LIKE ?", like, like, like)
	}
	if category != "" {
		db = db.Where("category = ?", category)
	}
	if tag != "" {
		db = db.Where("JSON_CONTAINS(tags_json, ?)", fmt.Sprintf("\"%s\"", tag))
	}

	if err := db.Order("created_at desc").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}
	resp := make([]ArchiveResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toArchiveResponse(item, nil))
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) getArchive(c *gin.Context) {
	var item models.Archive
	if err := s.DB.First(&item, "id = ?", c.Param("id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}
	paths, _ := s.loadArchivePaths(item.ID)
	c.JSON(http.StatusOK, toArchiveResponse(item, paths))
}

func (s *Server) updateArchive(c *gin.Context) {
	var req UpdateArchiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	var current models.Archive
	if err := s.DB.First(&current, "id = ?", c.Param("id")).Error; err == nil {
		if req.Tags == nil && len(current.TagsJSON) > 0 {
			_ = json.Unmarshal(current.TagsJSON, &req.Tags)
		}
		if req.Hierarchy == nil && len(current.HierarchyJSON) > 0 {
			_ = json.Unmarshal(current.HierarchyJSON, &req.Hierarchy)
		}
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.HierarchyPaths == nil {
		req.HierarchyPaths = []string{}
	}
	if req.Hierarchy == nil {
		req.Hierarchy = []string{}
	}
	if len(req.HierarchyPaths) == 0 && len(req.Hierarchy) > 0 {
		req.HierarchyPaths = []string{strings.Join(req.Hierarchy, "/")}
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	hierarchyJSON, _ := json.Marshal(req.Hierarchy)
	hierarchyPath := strings.Join(req.Hierarchy, "/")
	if hierarchyPath == "" && len(req.HierarchyPaths) > 0 {
		hierarchyPath = req.HierarchyPaths[0]
		hierarchyJSON, _ = json.Marshal(strings.Split(req.HierarchyPaths[0], "/"))
	}
	if hierarchyPath == "" && req.Category != "" {
		hierarchyPath = req.Category
		hierarchyJSON, _ = json.Marshal([]string{req.Category})
	}

	if err := s.DB.Model(&models.Archive{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{
			"category":       req.Category,
			"tags_json":      tagsJSON,
			"hierarchy_json": hierarchyJSON,
			"hierarchy_path": hierarchyPath,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db update failed"})
		return
	}

	var updated models.Archive
	if err := s.DB.First(&updated, "id = ?", c.Param("id")).Error; err == nil {
		if len(req.HierarchyPaths) > 0 {
			_ = s.replaceArchivePaths(updated.ID, req.HierarchyPaths)
		} else if len(req.Hierarchy) > 0 {
			_ = s.replaceArchivePaths(updated.ID, []string{strings.Join(req.Hierarchy, "/")})
		} else if req.Category != "" {
			_ = s.replaceArchivePaths(updated.ID, []string{req.Category})
		}
		paths, _ := s.loadArchivePaths(updated.ID)
		c.JSON(http.StatusOK, toArchiveResponse(updated, paths))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) deleteArchive(c *gin.Context) {
	id := c.Param("id")
	var item models.Archive
	if err := s.DB.First(&item, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}

	if err := s.DB.Delete(&models.Archive{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db delete failed"})
		return
	}

	_ = s.DB.Where("archive_id = ?", id).Delete(&models.ArchivePath{}).Error
	_ = s.Store.RemovePrefix(c.Request.Context(), storage.ArchivePrefix(id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) getArchiveHTML(c *gin.Context) {
	id := c.Param("id")
	objectPath := storage.ArchivePrefix(id) + "/index.html"
	obj, err := s.Store.Get(c.Request.Context(), objectPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	defer obj.Close()

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", "default-src 'self' data: blob:; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline' data:; font-src 'self' data:; media-src 'self' data:; script-src 'self' 'unsafe-inline'")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, obj)
}

func (s *Server) getAsset(c *gin.Context) {
	id := c.Param("id")
	p := c.Param("path")
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	objectPath := storage.ArchivePrefix(id) + "/" + p
	obj, err := s.Store.Get(c.Request.Context(), objectPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err == nil && stat.ContentType != "" {
		c.Header("Content-Type", stat.ContentType)
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, obj)
}
