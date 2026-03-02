package e2e

import (
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestSvelteSSRHomePage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "E2E Test"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_home_dev", html)
}

func TestSvelteSSRHomePage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Production"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_home_prod", html)
}

func TestSvelteSSRNestedPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/nested", "./pages/nested/page.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "NestedPage"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/nested")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_nested_dev", html)
}

func TestSvelteSSRNestedPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/nested", "./pages/nested/page.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "NestedProd"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/nested")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_nested_prod", html)
}

func TestSvelteSSRDynamicParams_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/message/{message}", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			path := req.URL.Path
			message := "World"
			if len(path) > 9 && path[:9] == "/message/" {
				message = path[9:]
			}
			return map[string]any{"name": message}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/message/CustomName")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_dynamic_dev", html)
}

func TestSvelteSSRDynamicParams_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/message/{message}", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			path := req.URL.Path
			message := "World"
			if len(path) > 9 && path[:9] == "/message/" {
				message = path[9:]
			}
			return map[string]any{"name": message}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/message/DynamicParam")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_dynamic_prod", html)
}

func TestSvelteSSRWithAPIDemo_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/api-demo", "./pages/api-demo.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{
				"users": []map[string]any{
					{"id": 1, "name": "ServerAlice", "email": "alice@test.com"},
					{"id": 2, "name": "ServerBob", "email": "bob@test.com"},
				},
			}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/api-demo")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_api_demo_dev", html)
}

func TestSvelteSSRWithAPIDemo_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/api-demo", "./pages/api-demo.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{
				"users": []map[string]any{
					{"id": 1, "name": "Alice", "email": "alice@example.com"},
					{"id": 2, "name": "Bob", "email": "bob@example.com"},
				},
			}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/api-demo")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_api_demo_prod", html)
}

func TestSvelteSSRNoLoader_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/simple", "./pages/home.svelte"),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/simple")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_no_loader_dev", html)
}

func TestSvelteSSRNoLoader_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/simple", "./pages/home.svelte"),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/simple")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_no_loader_prod", html)
}

func TestSvelteSSRQueryParams_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/search", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			query := req.URL.Query().Get("q")
			if query == "" {
				query = "empty"
			}
			return map[string]any{"name": query}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/search?q=testquery")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_query_dev", html)
}

func TestSvelteSSRQueryParams_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/search", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			query := req.URL.Query().Get("q")
			if query == "" {
				query = "empty"
			}
			return map[string]any{"name": query}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/search?q=prodquery")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_query_prod", html)
}

func TestSvelteSSRPathValue_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/user/{id}", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return map[string]any{"name": req.PathValue("id")}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/user/123")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_path_value_dev", html)
}

func TestSvelteSSRPathValue_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/user/{id}", "./pages/home.svelte", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return map[string]any{"name": req.PathValue("id")}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/user/456")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_path_value_prod", html)
}

func TestSvelteSSRSharedComponent_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/shared-a", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route A"}, nil
		})),
		bifrost.Page("/shared-b", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route B"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/shared-a")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_shared_a_dev", html)

	resp2, html2 := server.get(t, "/shared-b")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "svelte_ssr_shared_b_dev", html2)
}

func TestSvelteSSRSharedComponent_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/shared-a", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route A"}, nil
		})),
		bifrost.Page("/shared-b", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route B"}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/shared-a")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_shared_a_prod", html)

	resp2, html2 := server.get(t, "/shared-b")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "svelte_ssr_shared_b_prod", html2)
}

func TestSvelteSSREmptyProps_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/empty", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/empty")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_empty_props_dev", html)
}

func TestSvelteSSREmptyProps_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/empty", "./pages/home.svelte", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{}, nil
		})),
	}

	server := newSvelteTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/empty")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_empty_props_prod", html)
}
