package bifrost

import (
	"regexp"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const componentRuntimePath = "/_bifrost/component-runtime.js"

const componentRuntime = `const registry = new Map();
const loading = new Map();

function manifest() {
  const node = document.getElementById("bifrost-component-manifest");
  if (!node) return {};
  try {
    return JSON.parse(node.textContent || "{}");
  } catch {
    return {};
  }
}

function unmountAll() {
  document.querySelectorAll("[data-bifrost-component]").forEach((host) => {
    if (!host.__bifrostMounted) return;
    host.__bifrostCleanup?.();
    delete host.__bifrostCleanup;
    delete host.__bifrostMounted;
  });
}

async function mountAll() {
  const components = manifest();
  await Promise.all(
    Object.entries(components).map(async ([name, url]) => {
      if (registry.has(name)) return;
      if (!loading.has(name)) {
        loading.set(
          name,
          import(url)
            .then((module) => {
              if (typeof module.mount === "function") registry.set(name, module.mount);
            })
            .catch((error) => {
              console.error("bifrost: component", name, error);
            }),
        );
      }
      await loading.get(name);
    }),
  );
  document.querySelectorAll("[data-bifrost-component]").forEach((host) => {
    const name = host.getAttribute("data-bifrost-component");
    const mount = registry.get(name);
    if (!mount || host.__bifrostMounted) return;
    host.__bifrostMounted = true;
    host.__bifrostCleanup = mount(host);
  });
}

mountAll();
document.addEventListener("turbo:load", mountAll);
document.addEventListener("turbo:before-render", unmountAll);
`

var componentCallPattern = regexp.MustCompile(`\{\{-?\s*component\s+["']([a-z0-9-]+)["']`)
var componentPropPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var componentScriptSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var componentTags = map[string]struct{}{
	"a":          {},
	"article":    {},
	"aside":      {},
	"button":     {},
	"dd":         {},
	"details":    {},
	"div":        {},
	"dl":         {},
	"dt":         {},
	"fieldset":   {},
	"figcaption": {},
	"figure":     {},
	"footer":     {},
	"form":       {},
	"header":     {},
	"label":      {},
	"legend":     {},
	"li":         {},
	"main":       {},
	"nav":        {},
	"ol":         {},
	"section":    {},
	"span":       {},
	"summary":    {},
	"table":      {},
	"tbody":      {},
	"td":         {},
	"tfoot":      {},
	"th":         {},
	"thead":      {},
	"tr":         {},
	"ul":         {},
}

func validComponentTag(tag string) bool {
	_, exists := componentTags[tag]
	return exists
}

func componentURL(component protocol.ComponentSource) string {
	if component.Hash == "" {
		return "/_bifrost/components/" + component.Name + "/" + component.Name + ".js"
	}
	return "/_bifrost/components/" + component.Name + "/" + component.Hash + "/" + component.Name + ".js"
}

func componentImportURL(component protocol.ComponentSource, relative string) string {
	if component.Hash == "" {
		return "/_bifrost/components/" + component.Name + "/" + relative
	}
	return "/_bifrost/components/" + component.Name + "/" + component.Hash + "/" + relative
}

func validComponentPropName(name string) bool {
	return componentPropPattern.MatchString(name)
}

func validComponentScriptPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || !componentScriptSegmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}

func componentScriptExtension(value string) string {
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return ""
	}
	return strings.ToLower(value[index:])
}

func usedComponentNames(sources []protocol.TemplateSource, components []protocol.ComponentSource) map[string]struct{} {
	byName := make(map[string]protocol.ComponentSource, len(components))
	for _, component := range components {
		byName[component.Name] = component
	}
	used := make(map[string]struct{})
	var visit func(string)
	visit = func(source string) {
		for _, match := range componentCallPattern.FindAllStringSubmatch(source, -1) {
			name := match[1]
			if _, seen := used[name]; seen {
				continue
			}
			used[name] = struct{}{}
			if component, exists := byName[name]; exists {
				visit(component.HTML.Source)
			}
		}
	}
	for _, source := range sources {
		visit(source.Source)
	}
	return used
}
