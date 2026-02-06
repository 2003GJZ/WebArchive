package graphflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/eino/compose"

	"webarchive/internal/ai"
	"webarchive/internal/models"
)

type GraphInput struct {
	Archive  models.Archive
	Taxonomy []string
	LLM      *ai.Client
}

type Relation struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type GraphOutput struct {
	Category  string     `json:"category"`
	Tags      []string   `json:"tags"`
	Path      []string   `json:"path"`
	Entities  []string   `json:"entities"`
	Relations []Relation `json:"relations"`
	Summary   string     `json:"summary"`
}

type cleanedInput struct {
	Title    string
	URL      string
	Excerpt  string
	Content  string
	Taxonomy []string
	LLM      *ai.Client
}

type Analyzer struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

func NewAnalyzer() (*Analyzer, error) {
	graph := compose.NewGraph[GraphInput, GraphOutput]()
	if err := graph.AddLambdaNode("cleaner", compose.InvokableLambda(cleanerNode)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("extractor", compose.InvokableLambda(extractorNode)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("formatter", compose.InvokableLambda(formatterNode)); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(compose.START, "cleaner"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("cleaner", "extractor"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("extractor", "formatter"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("formatter", compose.END); err != nil {
		return nil, err
	}

	runnable, err := graph.Compile(context.Background(), compose.WithGraphName("knowledge_graph"))
	if err != nil {
		return nil, err
	}

	return &Analyzer{runnable: runnable}, nil
}

func (a *Analyzer) Analyze(ctx context.Context, input GraphInput) (GraphOutput, error) {
	if a == nil || a.runnable == nil {
		return GraphOutput{}, errors.New("eino graph not initialized")
	}
	return a.runnable.Invoke(ctx, input)
}

func cleanerNode(ctx context.Context, input GraphInput) (cleanedInput, error) {
	content := strings.TrimSpace(input.Archive.ContentText)
	if len(content) > 6000 {
		content = content[:6000]
	}
	excerpt := input.Archive.Excerpt
	if excerpt == "" {
		excerpt = strings.TrimSpace(content)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200]
		}
	}
	return cleanedInput{
		Title:    input.Archive.Title,
		URL:      input.Archive.URL,
		Excerpt:  excerpt,
		Content:  content,
		Taxonomy: input.Taxonomy,
		LLM:      input.LLM,
	}, nil
}

func extractorNode(ctx context.Context, input cleanedInput) (GraphOutput, error) {
	if input.LLM == nil || !input.LLM.Enabled() {
		return GraphOutput{}, errors.New("llm not configured")
	}

	taxonomyHint := "none"
	if len(input.Taxonomy) > 0 {
		taxonomyHint = strings.Join(input.Taxonomy, ", ")
	}

	system := "You are a knowledge graph analyst. Return strict JSON only."
	user := "Analyze the content and return JSON with fields: " +
		"category (string), tags (array of short strings), path (array of strings from high-level to low-level), " +
		"entities (array of key concepts), relations (array of {source,target,type}), summary (one sentence).\n" +
		"Relations type must be one of: is_a, part_of, related_to, prerequisite, based_on.\n" +
		"Prefer taxonomy branches if provided, otherwise create a concise path (2-4 levels).\n" +
		"Taxonomy options: " + taxonomyHint + "\n" +
		"Title: " + input.Title + "\nURL: " + input.URL + "\nExcerpt: " + input.Excerpt + "\nContent: " + input.Content

	raw, err := input.LLM.ChatJSON(ctx, system, user, 0.2)
	if err != nil {
		return GraphOutput{}, err
	}
	raw = extractJSON(raw)
	if raw == "" {
		return GraphOutput{}, errors.New("llm invalid json")
	}

	var out GraphOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return GraphOutput{}, err
	}
	out.Tags = normalizeList(out.Tags)
	out.Path = normalizeList(out.Path)
	out.Entities = normalizeList(out.Entities)
	out.Relations = normalizeRelations(out.Relations)
	out.Summary = strings.TrimSpace(out.Summary)

	if out.Category == "" && len(out.Path) > 0 {
		out.Category = out.Path[0]
	}
	return out, nil
}

func formatterNode(ctx context.Context, input GraphOutput) (GraphOutput, error) {
	if len(input.Path) > 6 {
		input.Path = input.Path[:6]
	}
	if len(input.Tags) > 12 {
		input.Tags = input.Tags[:12]
	}
	if len(input.Entities) > 20 {
		input.Entities = input.Entities[:20]
	}
	input.Category = strings.TrimSpace(input.Category)
	if input.Category == "" && len(input.Path) > 0 {
		input.Category = input.Path[0]
	}
	return input, nil
}

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func normalizeList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizeRelations(items []Relation) []Relation {
	out := make([]Relation, 0, len(items))
	for _, r := range items {
		r.Source = strings.TrimSpace(r.Source)
		r.Target = strings.TrimSpace(r.Target)
		r.Type = strings.TrimSpace(r.Type)
		if r.Source == "" || r.Target == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}
