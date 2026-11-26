package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultReleaseAPIURL = "https://api.github.com/repos/kryx-linux/iso/releases/latest"

type ghReleasePayload struct {
	TagName     string           `json:"tag_name"`
	PublishedAt string           `json:"published_at"`
	Assets      []ghReleaseAsset `json:"assets"`
}

type ghReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LoadReleaseInfo loads baseline release metadata from fallbackPath and, when
// offline is false, refreshes real ISO/RootFS sizes, download URLs, date, and
// SHA-256 checksums from the kryx-linux/iso GitHub Release API.
func LoadReleaseInfo(fallbackPath, releaseAPIURL string, offline bool) (ReleaseInfo, string, error) {
	rel, err := loadReleaseInfo(fallbackPath)
	if err != nil {
		return rel, "", err
	}
	if offline || strings.TrimSpace(releaseAPIURL) == "" {
		return rel, fallbackPath + " (static baseline)", nil
	}

	client := &http.Client{Timeout: 12 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return rel, fallbackPath + " (fallback)", nil
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kryx-sitegen/1.0")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := client.Do(req)
	if err != nil {
		return rel, fallbackPath + " (fallback)", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fallbackPath + " (fallback)", nil
	}

	var payload ghReleasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return rel, fallbackPath + " (fallback)", nil
	}

	assetsByName := make(map[string]ghReleaseAsset, len(payload.Assets))
	var sumsURL string
	for _, a := range payload.Assets {
		assetsByName[a.Name] = a
		if a.Name == "SHA256SUMS.txt" || a.Name == "SHA256SUMS" {
			sumsURL = a.BrowserDownloadURL
		}
	}

	sums := make(map[string]string)
	if sumsURL != "" {
		if sreq, serr := http.NewRequest(http.MethodGet, sumsURL, nil); serr == nil {
			sreq.Header.Set("User-Agent", "kryx-sitegen/1.0")
			if sresp, serr := client.Do(sreq); serr == nil {
				if sresp.StatusCode == http.StatusOK {
					sums = parseSHA256Sums(io.LimitReader(sresp.Body, 1<<20))
				}
				_ = sresp.Body.Close()
			}
		}
	}

	updatedAny := false
	if payload.TagName != "" {
		rel.Version = payload.TagName
	}
	if len(payload.PublishedAt) >= 10 {
		rel.ReleaseDate = payload.PublishedAt[:10]
	}

	for i := range rel.Artifacts {
		name := rel.Artifacts[i].Name
		if a, ok := assetsByName[name]; ok {
			if a.Size > 0 {
				rel.Artifacts[i].Size = formatSize(a.Size)
			}
			if a.BrowserDownloadURL != "" {
				rel.Artifacts[i].URL = a.BrowserDownloadURL
			}
			updatedAny = true
		}
		if h, ok := sums[name]; ok && len(h) == 64 {
			rel.Artifacts[i].SHA256 = h
			updatedAny = true
		}
	}

	if updatedAny {
		return rel, fmt.Sprintf("%s (%s)", releaseAPIURL, payload.TagName), nil
	}
	return rel, fallbackPath + " (baseline)", nil
}

func parseSHA256Sums(r io.Reader) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && len(fields[0]) == 64 {
			fname := strings.TrimPrefix(fields[1], "*")
			out[fname] = strings.ToLower(fields[0])
		}
	}
	return out
}
