package processor

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"webarchive/internal/storage"
)

type Asset struct {
	Original string `json:"original"`
	Stored   string `json:"stored"`
	Type     string `json:"type"`
}

type Result struct {
	HTML   []byte  `json:"html"`
	Assets []Asset `json:"assets"`
}

type Processor struct {
	Client  *http.Client
	Store   *storage.MinioStore
	BaseURL string
}

type assetInfo struct {
	Stored      string
	ContentType string
}

func New(store *storage.MinioStore, timeout time.Duration) *Processor {
	return &Processor{
		Store: store,
		Client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *Processor) Process(ctx context.Context, archiveID string, pageURL string, rawHTML []byte) (*Result, error) {
	if len(rawHTML) == 0 {
		return nil, errors.New("empty html")
	}

	cache := make(map[string]assetInfo)
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, err
	}

	base, _ := url.Parse(pageURL)
	assets := make([]Asset, 0)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "img", "source", "video", "audio", "script":
				for i := range n.Attr {
					if n.Attr[i].Key == "src" {
						updated, foundAssets := p.handleURL(ctx, archiveID, base, n.Attr[i].Val, cache)
						if updated != "" {
							n.Attr[i].Val = updated
						}
						if len(foundAssets) > 0 {
							assets = append(assets, foundAssets...)
						}
						break
					}
				}
				// handle common lazy-load attrs for images
				if strings.ToLower(n.Data) == "img" {
					lazyAttrs := []string{"data-src", "data-original", "data-lazy-src", "data-lazy"}
					for _, key := range lazyAttrs {
						for i := range n.Attr {
							if n.Attr[i].Key == key {
								updated, foundAssets := p.handleURL(ctx, archiveID, base, n.Attr[i].Val, cache)
								if updated != "" {
									n.Attr[i].Key = "src"
									n.Attr[i].Val = updated
								}
								if len(foundAssets) > 0 {
									assets = append(assets, foundAssets...)
								}
								break
							}
						}
					}
					for i := range n.Attr {
						if n.Attr[i].Key == "srcset" {
							updated, foundAssets := p.handleSrcset(ctx, archiveID, base, n.Attr[i].Val, cache)
							if updated != "" {
								n.Attr[i].Val = updated
							}
							if len(foundAssets) > 0 {
								assets = append(assets, foundAssets...)
							}
							break
						}
					}
				}
			case "link":
				rel := attrValue(n, "rel")
				if strings.Contains(rel, "stylesheet") || strings.Contains(rel, "icon") {
					for i := range n.Attr {
						if n.Attr[i].Key == "href" {
							updated, foundAssets := p.handleURL(ctx, archiveID, base, n.Attr[i].Val, cache)
							if updated != "" {
								n.Attr[i].Val = updated
							}
							if len(foundAssets) > 0 {
								assets = append(assets, foundAssets...)
							}
							break
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		return nil, err
	}

	return &Result{HTML: out.Bytes(), Assets: assets}, nil
}

func (p *Processor) handleSrcset(ctx context.Context, archiveID string, base *url.URL, raw string, cache map[string]assetInfo) (string, []Asset) {
	parts := strings.Split(raw, ",")
	assets := make([]Asset, 0)
	updatedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		urlPart := fields[0]
		descriptor := ""
		if len(fields) > 1 {
			descriptor = " " + strings.Join(fields[1:], " ")
		}
		updated, foundAssets := p.handleURL(ctx, archiveID, base, urlPart, cache)
		if updated == "" {
			updated = urlPart
		}
		if len(foundAssets) > 0 {
			assets = append(assets, foundAssets...)
		}
		updatedParts = append(updatedParts, updated+descriptor)
	}
	if len(updatedParts) == 0 {
		return raw, nil
	}
	return strings.Join(updatedParts, ", "), assets
}

func (p *Processor) handleURL(ctx context.Context, archiveID string, base *url.URL, raw string, cache map[string]assetInfo) (string, []Asset) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
		return raw, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw, nil
	}

	if base != nil {
		u = base.ResolveReference(u)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return raw, nil
	}

	storedPath, contentType, extraAssets, err := p.downloadAndStore(ctx, archiveID, u.String(), cache)
	if err != nil {
		return raw, nil
	}

	apiPath := fmt.Sprintf("/api/assets/%s/%s", archiveID, storedPath)
	assets := make([]Asset, 0, 1+len(extraAssets))
	assets = append(assets, Asset{Original: u.String(), Stored: storedPath, Type: contentType})
	if len(extraAssets) > 0 {
		assets = append(assets, extraAssets...)
	}
	return apiPath, assets
}

func (p *Processor) downloadAndStore(ctx context.Context, archiveID string, rawURL string, cache map[string]assetInfo) (string, string, []Asset, error) {
	if info, ok := cache[rawURL]; ok {
		return info.Stored, info.ContentType, nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", nil, err
	}
	req.Header.Set("User-Agent", "WebArchiveBot/0.1")

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", "", nil, err
	}

	parsed, _ := url.Parse(rawURL)
	ext := ""
	if parsed != nil {
		ext = path.Ext(parsed.Path)
	}
	if ext == "" {
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			if exts, _ := mimeExtensions(ct); len(exts) > 0 {
				ext = exts[0]
			}
		}
	}
	if ext == "" {
		ext = ".bin"
	}

	hash := sha1.Sum([]byte(rawURL))
	name := hex.EncodeToString(hash[:]) + ext

	objectPath := path.Join(storage.ArchivePrefix(archiveID), "assets", name)
	contentType := storage.GuessContentType(name, resp.Header.Get("Content-Type"))

	extraAssets := []Asset{}
	if strings.Contains(contentType, "text/css") || strings.EqualFold(ext, ".css") {
		rewritten, assets, err := p.rewriteCSS(ctx, archiveID, rawURL, body, cache)
		if err == nil {
			body = rewritten
			extraAssets = append(extraAssets, assets...)
		}
	}

	if err := p.Store.PutBytes(ctx, objectPath, body, contentType); err != nil {
		return "", "", nil, err
	}

	storedPath := path.Join("assets", name)
	cache[rawURL] = assetInfo{Stored: storedPath, ContentType: contentType}
	return storedPath, contentType, extraAssets, nil
}

func (p *Processor) rewriteCSS(ctx context.Context, archiveID string, cssURL string, css []byte, cache map[string]assetInfo) ([]byte, []Asset, error) {
	base, err := url.Parse(cssURL)
	if err != nil {
		return css, nil, err
	}

	assets := make([]Asset, 0)
	reURL := regexp.MustCompile(`url\(([^)]+)\)`)
	reImport := regexp.MustCompile(`@import\s+(?:url\()?\s*['"]?([^'")\s]+)['"]?\s*\)?`)

	replaceFn := func(raw string) (string, *Asset, []Asset) {
		raw = strings.TrimSpace(raw)
		raw = strings.Trim(raw, `"'`)
		if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
			return "", nil, nil
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", nil, nil
		}
		if base != nil {
			u = base.ResolveReference(u)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", nil, nil
		}
		storedPath, contentType, extraAssets, err := p.downloadAndStore(ctx, archiveID, u.String(), cache)
		if err != nil {
			return "", nil, nil
		}
		apiPath := fmt.Sprintf("/api/assets/%s/%s", archiveID, storedPath)
		return apiPath, &Asset{Original: u.String(), Stored: storedPath, Type: contentType}, extraAssets
	}

	cssText := string(css)
	cssText = reURL.ReplaceAllStringFunc(cssText, func(m string) string {
		matches := reURL.FindStringSubmatch(m)
		if len(matches) < 2 {
			return m
		}
		apiPath, asset, extra := replaceFn(matches[1])
		if asset != nil {
			assets = append(assets, *asset)
		}
		if len(extra) > 0 {
			assets = append(assets, extra...)
		}
		if apiPath == "" {
			return m
		}
		return fmt.Sprintf("url(\"%s\")", apiPath)
	})

	cssText = reImport.ReplaceAllStringFunc(cssText, func(m string) string {
		matches := reImport.FindStringSubmatch(m)
		if len(matches) < 2 {
			return m
		}
		apiPath, asset, extra := replaceFn(matches[1])
		if asset != nil {
			assets = append(assets, *asset)
		}
		if len(extra) > 0 {
			assets = append(assets, extra...)
		}
		if apiPath == "" {
			return m
		}
		return fmt.Sprintf("@import url(\"%s\")", apiPath)
	})

	return []byte(cssText), assets, nil
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.ToLower(strings.TrimSpace(a.Val))
		}
	}
	return ""
}

func mimeExtensions(contentType string) ([]string, error) {
	if i := strings.Index(contentType, ";"); i > -1 {
		contentType = contentType[:i]
	}
	return mimeExtensionsMap(contentType)
}

var mimeExtMap = map[string][]string{
	"image/jpeg":             {".jpg"},
	"image/png":              {".png"},
	"image/gif":              {".gif"},
	"image/webp":             {".webp"},
	"image/svg+xml":          {".svg"},
	"text/css":               {".css"},
	"application/javascript": {".js"},
	"text/javascript":        {".js"},
}

func mimeExtensionsMap(ct string) ([]string, error) {
	if exts, ok := mimeExtMap[strings.ToLower(ct)]; ok {
		return exts, nil
	}
	return nil, errors.New("unknown content type")
}
