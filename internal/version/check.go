package version

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	checkURL      = "https://api.github.com/repos/devenjarvis/moss/releases/latest"
	checkInterval = 24 * time.Hour
	checkTimeout  = 3 * time.Second
)

type checkCache struct {
	Latest    string `json:"latest"`
	CheckedAt int64  `json:"checked_at"`
}

// CheckLatest checks if a newer version is available.
// Returns the latest version string if newer, empty string otherwise.
// Results are cached for 24 hours.
func CheckLatest() string {
	if Version == "dev" {
		return ""
	}

	cachePath := cacheFilePath()
	if cachePath == "" {
		return ""
	}

	// Check cache first
	if cached, ok := readCache(cachePath); ok {
		if time.Since(time.Unix(cached.CheckedAt, 0)) < checkInterval {
			if isNewer(cached.Latest, Version) {
				return cached.Latest
			}
			return ""
		}
	}

	// Fetch latest version from GitHub
	latest := fetchLatest()
	if latest == "" {
		return ""
	}

	// Write cache
	writeCache(cachePath, checkCache{
		Latest:    latest,
		CheckedAt: time.Now().Unix(),
	})

	if isNewer(latest, Version) {
		return latest
	}
	return ""
}

func cacheFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "moss", ".version-check")
}

func readCache(path string) (checkCache, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return checkCache{}, false
	}
	var c checkCache
	if err := json.Unmarshal(data, &c); err != nil {
		return checkCache{}, false
	}
	return c, true
}

func writeCache(path string, c checkCache) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, data, 0644)
}

func fetchLatest() string {
	client := &http.Client{Timeout: checkTimeout}
	resp, err := client.Get(checkURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	return strings.TrimPrefix(release.TagName, "v")
}

// isNewer returns true if latest is a newer version than current.
// Uses simple string comparison of dot-separated version parts.
func isNewer(latest, current string) bool {
	if latest == "" || current == "" {
		return false
	}

	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	// Pad to same length
	for len(latestParts) < len(currentParts) {
		latestParts = append(latestParts, "0")
	}
	for len(currentParts) < len(latestParts) {
		currentParts = append(currentParts, "0")
	}

	for i := range latestParts {
		l := padLeft(latestParts[i], 10)
		c := padLeft(currentParts[i], 10)
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}

func padLeft(s string, length int) string {
	for len(s) < length {
		s = "0" + s
	}
	return s
}
