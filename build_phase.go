package bifrost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const (
	buildPhaseEnv = "BIFROST_PHASE"
	buildFDEnv    = "BIFROST_FD"
)

func (a *App) handleBuildPhase() error {
	phase := os.Getenv(buildPhaseEnv)
	if phase == "" {
		return nil
	}
	fdValue := os.Getenv(buildFDEnv)
	fd, err := strconv.Atoi(fdValue)
	if err != nil || fd < 3 {
		return fmt.Errorf("bifrost: invalid build protocol file descriptor %q", fdValue)
	}
	output := os.NewFile(uintptr(fd), "bifrost-build-protocol")
	if output == nil {
		return fmt.Errorf("bifrost: open build protocol file descriptor %d", fd)
	}
	defer func() { _ = output.Close() }()
	encoder := json.NewEncoder(output)

	switch phase {
	case "describe":
		return encoder.Encode(protocol.DescribeResult{
			Spec:       a.spec,
			SpecHash:   a.specHash,
			SourceRoot: a.sourceRoot,
			Limits: protocol.RuntimeLimits{
				MaxPropsBytes: a.limits.MaxPropsBytes,
				MaxHeadBytes:  a.limits.MaxHeadBytes,
				MaxFrameBytes: a.limits.MaxFrameBytes,
			},
		})
	case "generate":
		generated, err := a.generateStatic(context.Background())
		if err != nil {
			return err
		}
		return encoder.Encode(protocol.GenerateResult{
			SpecHash: a.specHash,
			Limits: protocol.RuntimeLimits{
				MaxPropsBytes: a.limits.MaxPropsBytes,
				MaxHeadBytes:  a.limits.MaxHeadBytes,
				MaxFrameBytes: a.limits.MaxFrameBytes,
			},
			Pages: generated,
		})
	default:
		return fmt.Errorf("bifrost: unknown build phase %q", phase)
	}
}

func (a *App) generateStatic(ctx context.Context) ([]protocol.GeneratedPage, error) {
	mux := http.NewServeMux()
	marker := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, route := range a.routes {
		mux.Handle("GET "+route.pattern, marker)
	}

	var generated []protocol.GeneratedPage
	seenPaths := make(map[string]string)
	for _, route := range a.routes {
		if route.kind != routeStatic {
			continue
		}
		pages := []StaticPage{{Path: exactPatternPath(route.pattern)}}
		if route.generate != nil {
			var err error
			pages, err = route.generate(ctx)
			if err != nil {
				return nil, fmt.Errorf("bifrost: generate static route %q: %w", route.pattern, err)
			}
		}
		for i, page := range pages {
			if err := validateDocumentPath(page.Path); err != nil {
				return nil, fmt.Errorf("bifrost: static route %q page %d: %w", route.pattern, i, err)
			}
			request := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: page.Path}}
			_, matched := mux.Handler(request)
			if matched != "GET "+route.pattern {
				return nil, fmt.Errorf("bifrost: static path %q belongs to %q, not %q", page.Path, strings.TrimPrefix(matched, "GET "), route.pattern)
			}
			if previous, exists := seenPaths[page.Path]; exists {
				return nil, fmt.Errorf("bifrost: duplicate static path %q from %q and %q", page.Path, previous, route.pattern)
			}
			seenPaths[page.Path] = route.pattern
			document, err := normalizeDocument(page.Document)
			if err != nil {
				return nil, fmt.Errorf("bifrost: static path %q document: %w", page.Path, err)
			}
			props, err := marshalProps(page.Props)
			if err == nil && len(props) > a.limits.MaxPropsBytes {
				err = fmt.Errorf("props exceed %d bytes", a.limits.MaxPropsBytes)
			}
			if err != nil {
				return nil, fmt.Errorf("bifrost: static path %q props: %w", page.Path, err)
			}
			generated = append(generated, protocol.GeneratedPage{
				Pattern:  route.pattern,
				Path:     page.Path,
				Props:    props,
				Document: protocolDocument(document),
			})
		}
	}
	slices.SortFunc(generated, func(a, b protocol.GeneratedPage) int {
		if byPattern := strings.Compare(a.Pattern, b.Pattern); byPattern != 0 {
			return byPattern
		}
		return strings.Compare(a.Path, b.Path)
	})
	return generated, nil
}

func exactPatternPath(pattern string) string {
	if pattern == "/{$}" {
		return "/"
	}
	if before, ok := strings.CutSuffix(pattern, "{$}"); ok {
		return before
	}
	return pattern
}
