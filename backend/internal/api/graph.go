package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
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
	Type   string `json:"type,omitempty"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

func (s *Server) getGraph(c *gin.Context) {
	if c.Query("mode") == "knowledge" {
		s.getKnowledgeGraph(c)
		return
	}
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

type knowledgeRelation struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

func (s *Server) getKnowledgeGraph(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 600)
	archiveLimit := parseLimit(c.Query("archives"), 200)

	var items []models.Archive
	query := s.DB.Order("created_at desc")
	if archiveLimit > 0 {
		query = query.Limit(archiveLimit)
	}
	if err := query.Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}

	type graphItem struct {
		archiveID string
		label     string
		url       string
		entities  []string
		relations []knowledgeRelation
	}
	itemData := make([]graphItem, 0, len(items))
	entityCounts := map[string]int{}

	for _, item := range items {
		label := item.Title
		if label == "" {
			label = item.URL
		}
		entities := []string{}
		if len(item.EntitiesJSON) > 0 {
			_ = json.Unmarshal(item.EntitiesJSON, &entities)
		}
		relations := []knowledgeRelation{}
		if len(item.RelationsJSON) > 0 {
			_ = json.Unmarshal(item.RelationsJSON, &relations)
		}
		for _, ent := range entities {
			ent = strings.TrimSpace(ent)
			if ent == "" {
				continue
			}
			entityCounts[ent]++
		}
		for _, rel := range relations {
			src := strings.TrimSpace(rel.Source)
			tgt := strings.TrimSpace(rel.Target)
			if src == "" || tgt == "" {
				continue
			}
			entityCounts[src] += 2
			entityCounts[tgt] += 2
		}
		itemData = append(itemData, graphItem{
			archiveID: item.ID,
			label:     label,
			url:       item.URL,
			entities:  entities,
			relations: relations,
		})
	}

	allowedEntities := buildTopEntities(entityCounts, limit)

	nodes := map[string]GraphNode{}
	links := make([]GraphLink, 0)

	addNode := func(id, label, group, refID string) {
		if id == "" || label == "" {
			return
		}
		if _, ok := nodes[id]; ok {
			return
		}
		nodes[id] = GraphNode{ID: id, Label: label, Group: group, RefID: refID}
	}
	addLink := func(source, target, relType string) {
		if source == "" || target == "" {
			return
		}
		links = append(links, GraphLink{Source: source, Target: target, Value: 1, Type: relType})
	}

	for _, item := range itemData {
		archiveNodeID := "arc:" + item.archiveID
		addNode(archiveNodeID, item.label, "archive", item.archiveID)

		for _, ent := range item.entities {
			ent = strings.TrimSpace(ent)
			if ent == "" || !allowedEntities[ent] {
				continue
			}
			entID := "ent:" + ent
			addNode(entID, ent, "entity", "")
			addLink(entID, archiveNodeID, "mentions")
		}

		for _, rel := range item.relations {
			src := strings.TrimSpace(rel.Source)
			tgt := strings.TrimSpace(rel.Target)
			if src == "" || tgt == "" {
				continue
			}
			if !allowedEntities[src] || !allowedEntities[tgt] {
				continue
			}
			srcID := "ent:" + src
			tgtID := "ent:" + tgt
			addNode(srcID, src, "entity", "")
			addNode(tgtID, tgt, "entity", "")
			addLink(srcID, tgtID, rel.Type)
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

func parseLimit(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func buildTopEntities(counts map[string]int, limit int) map[string]bool {
	if limit <= 0 || len(counts) <= limit {
		out := map[string]bool{}
		for key := range counts {
			out[key] = true
		}
		return out
	}

	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{key: key, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].count > pairs[j].count
	})
	out := map[string]bool{}
	for i := 0; i < limit && i < len(pairs); i++ {
		out[pairs[i].key] = true
	}
	return out
}
