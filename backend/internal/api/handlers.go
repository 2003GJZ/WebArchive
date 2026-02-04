package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"webarchive/internal/models"
	"webarchive/internal/processor"
	"webarchive/internal/storage"
)

type Server struct {
	DB        *gorm.DB
	Store     *storage.MinioStore
	Processor *processor.Processor
}

type CreateArchiveRequest struct {
	URL        string     `json:"url"`
	Title      string     `json:"title"`
	HTML       string     `json:"html"`
	Content    string     `json:"content"`
	Excerpt    string     `json:"excerpt"`
	Byline     string     `json:"byline"`
	SiteName   string     `json:"siteName"`
	Favicon    string     `json:"favicon"`
	CapturedAt *time.Time `json:"capturedAt"`
	Category   string     `json:"category"`
	Tags       []string   `json:"tags"`
}

type UpdateArchiveRequest struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	api := r.Group("/api")
	api.POST("/archives", s.createArchive)
	api.GET("/archives", s.listArchives)
	api.GET("/archives/:id", s.getArchive)
	api.PATCH("/archives/:id", s.updateArchive)
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
	tagsJSON, _ := json.Marshal(req.Tags)

	archive := models.Archive{
		ID:          id,
		Title:       req.Title,
		URL:         req.URL,
		SiteName:    req.SiteName,
		Byline:      req.Byline,
		Excerpt:     req.Excerpt,
		Favicon:     req.Favicon,
		Category:    req.Category,
		TagsJSON:    tagsJSON,
		ContentText: req.Content,
		CapturedAt:  req.CapturedAt,
		HTMLPath:    "index.html",
		AssetsJSON:  assetsJSON,
	}

	if err := s.DB.Create(&archive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert failed"})
		return
	}

	c.JSON(http.StatusOK, archive)
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
	c.JSON(http.StatusOK, items)
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
	c.JSON(http.StatusOK, item)
}

func (s *Server) updateArchive(c *gin.Context) {
	var req UpdateArchiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	tagsJSON, _ := json.Marshal(req.Tags)

	if err := s.DB.Model(&models.Archive{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{
			"category":  req.Category,
			"tags_json": tagsJSON,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db update failed"})
		return
	}

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
