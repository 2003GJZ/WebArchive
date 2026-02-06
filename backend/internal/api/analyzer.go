package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"webarchive/internal/models"
)

type AnalysisStatus struct {
	Running           bool       `json:"running"`
	LastRun           *time.Time `json:"lastRun,omitempty"`
	LastError         string     `json:"lastError,omitempty"`
	LoopCount         int        `json:"loopCount"`
	LastLoopScanned   int        `json:"lastLoopScanned"`
	LastLoopProcessed int        `json:"lastLoopProcessed"`
	TotalProcessed    int        `json:"totalProcessed"`
}

type AnalysisRequest struct {
	IDs []string `json:"ids"`
}

func (s *Server) analysisStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.getAnalysisStatus())
}

func (s *Server) startAnalysis(c *gin.Context) {
	if s.LLM == nil || !s.LLM.Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "llm not configured"})
		return
	}

	var req AnalysisRequest
	_ = c.ShouldBindJSON(&req)

	s.analyzeMu.Lock()
	if s.analyzeStatus.Running {
		status := s.analyzeStatus
		s.analyzeMu.Unlock()
		c.JSON(http.StatusOK, status)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.analyzeCancel = cancel
	s.analyzeStatus.Running = true
	s.analyzeStatus.LastError = ""
	s.analyzeStatus.LastLoopScanned = 0
	s.analyzeStatus.LastLoopProcessed = 0
	s.analyzeMu.Unlock()

	go s.runAnalyzerOnce(ctx, req.IDs)
	c.JSON(http.StatusOK, s.getAnalysisStatus())
}

func (s *Server) stopAnalysis(c *gin.Context) {
	s.analyzeMu.Lock()
	if s.analyzeCancel != nil {
		s.analyzeCancel()
		s.analyzeCancel = nil
	}
	s.analyzeStatus.Running = false
	status := s.analyzeStatus
	s.analyzeMu.Unlock()
	c.JSON(http.StatusOK, status)
}

func (s *Server) getAnalysisStatus() AnalysisStatus {
	s.analyzeMu.Lock()
	defer s.analyzeMu.Unlock()
	return s.analyzeStatus
}

func (s *Server) runAnalyzerOnce(ctx context.Context, ids []string) {
	loopStart := time.Now()
	scanned := 0
	processed := 0
	lastErr := ""

	defer func() {
		s.withAnalysisStatus(func(st *AnalysisStatus) {
			st.Running = false
			st.LastRun = &loopStart
			st.LastError = lastErr
			st.LoopCount++
			st.LastLoopScanned = scanned
			st.LastLoopProcessed = processed
			st.TotalProcessed += processed
		})
		s.analyzeMu.Lock()
		s.analyzeCancel = nil
		s.analyzeMu.Unlock()
	}()

	var items []models.Archive
	var err error
	if len(ids) > 0 {
		err = s.DB.Where("id IN ?", ids).Order("created_at desc").Find(&items).Error
	} else {
		err = s.DB.Order("created_at desc").Find(&items).Error
	}
	if err != nil {
		lastErr = err.Error()
		return
	}

	for _, item := range items {
		if ctx.Err() != nil {
			lastErr = "canceled"
			return
		}
		scanned++
		if !needsAnalysis(item) {
			s.withAnalysisStatus(func(st *AnalysisStatus) {
				st.LastLoopScanned = scanned
				st.LastLoopProcessed = processed
			})
			continue
		}

		taskCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		_, err := s.classifyArchive(taskCtx, item)
		cancel()
		if err != nil {
			lastErr = err.Error()
		} else {
			processed++
		}
		s.withAnalysisStatus(func(st *AnalysisStatus) {
			st.LastLoopScanned = scanned
			st.LastLoopProcessed = processed
		})

		select {
		case <-ctx.Done():
			lastErr = "canceled"
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (s *Server) withAnalysisStatus(update func(*AnalysisStatus)) {
	s.analyzeMu.Lock()
	defer s.analyzeMu.Unlock()
	update(&s.analyzeStatus)
}

func needsAnalysis(item models.Archive) bool {
	if item.HierarchyPath == "" || len(item.HierarchyJSON) == 0 {
		return true
	}
	raw := strings.TrimSpace(string(item.TagsJSON))
	if raw == "" || raw == "null" || raw == "[]" {
		return true
	}
	entities := strings.TrimSpace(string(item.EntitiesJSON))
	if entities == "" || entities == "null" || entities == "[]" {
		return true
	}
	return false
}
