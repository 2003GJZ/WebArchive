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
						updated, asset := p.handleURL(ctx, archiveID, base, n.Attr[i].Val)
						if asset != nil {
							assets = append(assets, *asset)
							n.Attr[i].Val = updated
						}
						break
					}
				}
			case "link":
				rel := attrValue(n, "rel")
				if strings.Contains(rel, "stylesheet") || strings.Contains(rel, "icon") {
					for i := range n.Attr {
						if n.Attr[i].Key == "href" {
							updated, asset := p.handleURL(ctx, archiveID, base, n.Attr[i].Val)
							if asset != nil {
								assets = append(assets, *asset)
								n.Attr[i].Val = updated
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

func (p *Processor) handleURL(ctx context.Context, archiveID string, base *url.URL, raw string) (string, *Asset) {
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

	storedPath, contentType, err := p.downloadAndStore(ctx, archiveID, u.String())
	if err != nil {
		return raw, nil
	}

	apiPath := fmt.Sprintf("/api/assets/%s/%s", archiveID, storedPath)
	return apiPath, &Asset{Original: u.String(), Stored: storedPath, Type: contentType}
}

func (p *Processor) downloadAndStore(ctx context.Context, archiveID string, rawURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "WebArchiveBot/0.1")

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", "", err
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
	if err := p.Store.PutBytes(ctx, objectPath, body, contentType); err != nil {
		return "", "", err
	}

	return path.Join("assets", name), contentType, nil
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
