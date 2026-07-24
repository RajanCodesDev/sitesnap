package crawler

import (
	"net/url"
	"strings"
)

func shouldExclude(rawURL string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		path = "/"
	}

	for _, ex := range excludes {
		ex = strings.TrimSuffix(ex, "/")

		if ex == "" {
			continue
		}

		if path == ex ||
			strings.HasPrefix(path, ex+"/") {
			return true
		}
	}

	return false
}