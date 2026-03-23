package usecase

import (
	"fmt"
	"os"
	"path/filepath"
)

func normalizeSSRBundle(ssrDir, entryName string) (string, error) {
	expectedPath := filepath.Join(ssrDir, entryName+"-ssr.js")
	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to verify SSR bundle %s: %w", expectedPath, err)
	}

	candidates, err := findNestedSSRBundles(ssrDir, entryName)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("expected SSR bundle at %s", expectedPath)
	}
	if len(candidates) > 1 {
		return "", fmt.Errorf("multiple nested SSR bundles found for %s: %v", entryName, candidates)
	}

	if err := os.Rename(candidates[0], expectedPath); err != nil {
		return "", fmt.Errorf("failed to move SSR bundle from %s to %s: %w", candidates[0], expectedPath, err)
	}

	return expectedPath, nil
}

func findNestedSSRBundles(ssrDir, entryName string) ([]string, error) {
	targetName := entryName + "-ssr.js"
	matches := make([]string, 0, 1)

	err := filepath.WalkDir(ssrDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != targetName {
			return nil
		}
		if filepath.Dir(path) == ssrDir {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan SSR bundle directory %s: %w", ssrDir, err)
	}

	return matches, nil
}
