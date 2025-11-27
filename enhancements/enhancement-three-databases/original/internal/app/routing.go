package app

import (
	"fmt"
	"strings"
)

func (app *Application) findMatchingRoute(path string) (matchedPrefix string, baseURL string, ok bool) {
	longestPrefix := ""
	var matchedURL string

	for _, server := range app.Registry.ListRegistered() {
		for _, prefix := range server.Prefixes {
			if strings.HasPrefix(path, prefix) && len(prefix) > len(longestPrefix) {
				longestPrefix = prefix
				matchedURL = server.BaseURL
			}
		}
	}

	if longestPrefix != "" {
		return longestPrefix, matchedURL, true
	}

	return "", "", false
}

func (app *Application) determineBackendURL(requestPath string) (string, error) {
	prefix, baseURL, ok := app.findMatchingRoute(requestPath)
	if !ok {
		app.Logger.Error("no matching route found", "path", requestPath)
		return "", fmt.Errorf("no matching bakend for path: %s", requestPath)
	}

	trimmedPath := strings.TrimPrefix(requestPath, prefix)
	return baseURL + trimmedPath, nil
}
