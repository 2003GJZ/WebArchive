package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"webarchive/internal/models"
)

type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	RefID string `json:"refId,omitempty"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int    `json:"value,omitempty"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

func (s *Server) getGraph(c *gin.Context) {
	var items []models.Archive
	if err := s.DB.Order("created_at desc").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}

	nodes := map[string]GraphNode{}
	links := make([]GraphLink, 0)

	addNode := func(id, label, group, refID string) {
		if _, ok := nodes[id]; ok {
			return
		}
		nodes[id] = GraphNode{ID: id, Label: label, Group: group, RefID: refID}
	}
	addLink := func(source, target string) {
		if source == "" || target == "" {
			return
		}
		links = append(links, GraphLink{Source: source, Target: target, Value: 1})
	}

	for _, item := range items {
		archiveNodeID := "arc:" + item.ID
		label := item.Title
		if label == "" {
			label = item.URL
		}
		addNode(archiveNodeID, label, "archive", item.ID)

		if item.Category != "" {
			catID := "cat:" + item.Category
			addNode(catID, item.Category, "category", "")
			addLink(catID, archiveNodeID)
		}

		tags := []string{}
		if len(item.TagsJSON) > 0 {
			_ = json.Unmarshal(item.TagsJSON, &tags)
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			tagID := "tag:" + tag
			addNode(tagID, tag, "tag", "")
			addLink(tagID, archiveNodeID)
		}

		path := []string{}
		if len(item.HierarchyJSON) > 0 {
			_ = json.Unmarshal(item.HierarchyJSON, &path)
		}
		if len(path) > 0 {
			prev := ""
			for idx, p := range path {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				pathID := "path:" + strings.Join(path[:idx+1], "/")
				addNode(pathID, p, "path", "")
				if prev != "" {
					addLink(prev, pathID)
				}
				prev = pathID
			}
			if prev != "" {
				addLink(prev, archiveNodeID)
			}
		}
	}

	out := GraphResponse{
		Nodes: make([]GraphNode, 0, len(nodes)),
		Links: links,
	}
	for _, node := range nodes {
		out.Nodes = append(out.Nodes, node)
	}
	c.JSON(http.StatusOK, out)
}
