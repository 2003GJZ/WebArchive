package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"webarchive/internal/models"
)

type TaxonomyNodeResponse struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	ParentID *string                `json:"parentId"`
	Path     string                 `json:"path"`
	Level    int                    `json:"level"`
	Children []TaxonomyNodeResponse `json:"children,omitempty"`
}

func (s *Server) getTaxonomy(c *gin.Context) {
	nodes, err := s.loadTaxonomyNodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}
	tree := buildTaxonomyTree(nodes)
	c.JSON(http.StatusOK, tree)
}

func (s *Server) getTaxonomyNode(c *gin.Context) {
	id := c.Param("id")
	includeDesc := c.Query("desc") == "1"
	var node models.TaxonomyNode
	if err := s.DB.First(&node, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	var children []models.TaxonomyNode
	if err := s.DB.Where("parent_id = ?", id).Order("label asc").Find(&children).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed"})
		return
	}

	archives := []models.Archive{}
	if node.Path != "" {
		nodeIDs := []string{node.ID}
		if includeDesc {
			var descendants []models.TaxonomyNode
			like := node.Path + "%"
			if err := s.DB.Where("path LIKE ?", like).Find(&descendants).Error; err == nil {
				nodeIDs = []string{}
				for _, d := range descendants {
					nodeIDs = append(nodeIDs, d.ID)
				}
			}
		}
		if len(nodeIDs) > 0 {
			var paths []models.ArchivePath
			if err := s.DB.Where("node_id IN ?", nodeIDs).Find(&paths).Error; err == nil {
				ids := []string{}
				seen := map[string]bool{}
				for _, p := range paths {
					if !seen[p.ArchiveID] {
						seen[p.ArchiveID] = true
						ids = append(ids, p.ArchiveID)
					}
				}
				if len(ids) > 0 {
					_ = s.DB.Where("id IN ?", ids).Order("created_at desc").Find(&archives).Error
				}
			}
		}
	}

	resp := struct {
		Node     TaxonomyNodeResponse   `json:"node"`
		Children []TaxonomyNodeResponse `json:"children"`
		Archives []ArchiveResponse      `json:"archives"`
	}{
		Node: TaxonomyNodeResponse{
			ID:       node.ID,
			Label:    node.Label,
			ParentID: node.ParentID,
			Path:     node.Path,
			Level:    node.Level,
		},
		Children: make([]TaxonomyNodeResponse, 0, len(children)),
		Archives: make([]ArchiveResponse, 0, len(archives)),
	}
	for _, child := range children {
		resp.Children = append(resp.Children, TaxonomyNodeResponse{
			ID:       child.ID,
			Label:    child.Label,
			ParentID: child.ParentID,
			Path:     child.Path,
			Level:    child.Level,
		})
	}
	for _, item := range archives {
		paths, _ := s.loadArchivePaths(item.ID)
		resp.Archives = append(resp.Archives, toArchiveResponse(item, paths))
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) loadTaxonomyNodes() ([]models.TaxonomyNode, error) {
	var nodes []models.TaxonomyNode
	if err := s.DB.Order("level asc, label asc").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Server) getNodeByPath(path string) (models.TaxonomyNode, error) {
	var node models.TaxonomyNode
	err := s.DB.Where("path = ?", path).Limit(1).Find(&node).Error
	return node, err
}

func buildTaxonomyTree(nodes []models.TaxonomyNode) []TaxonomyNodeResponse {
	index := map[string]*TaxonomyNodeResponse{}
	childrenMap := map[string][]*TaxonomyNodeResponse{}
	roots := []*TaxonomyNodeResponse{}

	for _, node := range nodes {
		n := &TaxonomyNodeResponse{
			ID:       node.ID,
			Label:    node.Label,
			ParentID: node.ParentID,
			Path:     node.Path,
			Level:    node.Level,
		}
		index[node.ID] = n
		if node.ParentID != nil {
			childrenMap[*node.ParentID] = append(childrenMap[*node.ParentID], n)
		} else {
			roots = append(roots, n)
		}
	}

	var attach func(*TaxonomyNodeResponse)
	attach = func(n *TaxonomyNodeResponse) {
		if kids, ok := childrenMap[n.ID]; ok {
			n.Children = make([]TaxonomyNodeResponse, 0, len(kids))
			for _, child := range kids {
				attach(child)
				n.Children = append(n.Children, *child)
			}
		}
	}

	out := make([]TaxonomyNodeResponse, 0, len(roots))
	for _, root := range roots {
		attach(root)
		out = append(out, *root)
	}
	return out
}

func (s *Server) ensureTaxonomyPath(path []string) error {
	clean := make([]string, 0, len(path))
	for _, p := range path {
		p = strings.TrimSpace(p)
		if len(p) > 80 {
			p = p[:80]
		}
		if p == "" {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return nil
	}

	var parentID *string
	for i, label := range clean {
		nodePath := strings.Join(clean[:i+1], "/")
		if len(nodePath) > 500 {
			break
		}
		var node models.TaxonomyNode
		tx := s.DB.Where("path = ?", nodePath).Limit(1).Find(&node)
		if tx.Error != nil {
			return tx.Error
		}
		if tx.RowsAffected == 0 {
			id := uuid.New().String()
			node = models.TaxonomyNode{
				ID:       id,
				Label:    label,
				ParentID: parentID,
				Path:     nodePath,
				Level:    i,
			}
			if err := s.DB.Create(&node).Error; err != nil {
				return err
			}
		}
		parentID = &node.ID
	}
	return nil
}
