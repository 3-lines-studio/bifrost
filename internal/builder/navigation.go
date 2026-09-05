package builder

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func writeNavigation(sourceRoot, output string, plans []viewPlan) error {
	var modules strings.Builder
	for _, plan := range plans {
		if !plan.Navigation {
			continue
		}
		source, err := resolveSourceFile(sourceRoot, plan.Source)
		if err != nil {
			return err
		}
		modules.WriteString(strconv.Quote(plan.ID) + ": () => import(" + strconv.Quote(filepath.ToSlash(source)) + "),\n")
	}
	if modules.Len() == 0 {
		return nil
	}
	if err := os.WriteFile(filepath.Join(output, "entries", "navigation-api.ts"), []byte(navigationAPI), 0o644); err != nil {
		return err
	}
	source := "const modules = {\n" + modules.String() + "};\n" + navigationClient
	return os.WriteFile(filepath.Join(output, "entries", "navigation.tsx"), []byte(source), 0o644)
}

const navigationClient = `import { hydrateRoot } from "react-dom/client";
import { flushSync } from "react-dom";
import { setRouter } from "./navigation-api";

export function start(page) {
  const app = document.getElementById("app");
  const propsNode = document.getElementById("__BIFROST_PROPS__");
  const buildNode = document.querySelector('meta[name="bifrost-build"]');
  const build = buildNode?.content;
  let documentClasses = (buildNode?.dataset.documentClass || "").split(/\s+/).filter(Boolean);
  const headStart = document.querySelector('meta[name="bifrost-head-start"]');
  const headEnd = document.querySelector('meta[name="bifrost-head-end"]');
  const props = JSON.parse(propsNode.textContent || "{}");
  let pending;
  let renderedURL = new URL(location.href);
  const root = hydrateRoot(app, page.renderPage(props, renderedURL.pathname), {
    onUncaughtError(error) {
      console.error(error);
      if (pending) {
        location.replace(renderedURL.href);
      }
    },
  });
  let sequence = 0;
  let currentKey = history.state?.__bifrostKey || createKey();
  const positions = new Map();
  history.replaceState({ ...history.state, __bifrostKey: currentKey }, "");
  history.scrollRestoration = "manual";

  function createKey() {
    sequence += 1;
    return performance.timeOrigin + ":" + sequence;
  }

  function saveScroll() {
    positions.set(currentKey, [scrollX, scrollY]);
  }

  function restoreScroll(url, position) {
    if (position) {
      scrollTo(...position);
      return;
    }
    if (url.hash) {
      let id;
      try {
        id = decodeURIComponent(url.hash.slice(1));
      } catch {
        id = url.hash.slice(1);
      }
      const target = document.getElementById(id) || document.getElementsByName(id)[0];
      if (target) {
        target.scrollIntoView();
        return;
      }
    }
    scrollTo(0, 0);
  }

  function sameHeadNode(previous, next, destination) {
    if (!previous.isEqualNode(next)) {
      return false;
    }
    if (previous instanceof Element) {
      for (const name of ["href", "src"]) {
        const value = previous.getAttribute(name);
        if (value !== null && new URL(value, renderedURL).href !== new URL(value, destination).href) {
          return false;
        }
      }
    }
    return true;
  }

  async function transition(url, mode = "push", position) {
    pending?.abort();
    const controller = new AbortController();
    pending = controller;
    saveScroll();
    app.setAttribute("aria-busy", "true");
    let destination = url;
    try {
      for (let node = headStart.nextSibling; node !== headEnd; node = node.nextSibling) {
        if (node instanceof Element && node.matches("base, meta[http-equiv]")) {
          throw new Error("Head requires document navigation");
        }
      }
      const response = await fetch(url, {
        headers: { Accept: "application/vnd.bifrost.navigation+json" },
        credentials: "same-origin",
        cache: "no-store",
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (response.redirected) {
        destination = new URL(response.url);
      }
      if (destination.origin !== location.origin || response.headers.has("Content-Disposition") || response.headers.get("Content-Type")?.split(";")[0] !== "application/vnd.bifrost.navigation+json") {
        throw new Error("Document navigation required");
      }
      const data = await response.json();
      if (data.build !== build || !Object.hasOwn(modules, data.view)) {
        throw new Error("Build changed");
      }
      const next = await modules[data.view]();
      if (controller.signal.aborted) {
        return;
      }
      const head = document.createElement("template");
      head.innerHTML = data.head;
      if (head.content.querySelector('base, meta[http-equiv]')) {
        throw new Error("Head requires document navigation");
      }
      const oldHead = [];
      for (let node = headStart.nextSibling; node !== headEnd; node = node.nextSibling) {
        oldHead.push(node);
      }
      const nextHead = Array.from(head.content.childNodes);
      const oldScripts = oldHead.filter(node => node instanceof Element && node.matches("script"));
      const nextScripts = Array.from(head.content.querySelectorAll("script"));
      for (const script of nextScripts) {
        if (!oldScripts.some(old => sameHeadNode(old, script, destination))) {
          throw new Error("New scripts require document navigation");
        }
      }
      const unusedHead = [...oldHead];
      const updatedHead = nextHead.map(node => {
        const index = unusedHead.findIndex(old => sameHeadNode(old, node, destination));
        if (index === -1) {
          return node;
        }
        return unusedHead.splice(index, 1)[0];
      });
      const preserveView = mode === "refresh" && destination.href === renderedURL.href;
      const scroll = [scrollX, scrollY];
      const tree = next.renderPage(data.props, destination.pathname);
      if (mode !== "push") {
        currentKey = history.state?.__bifrostKey || createKey();
        history.replaceState({ ...history.state, __bifrostKey: currentKey }, "", destination);
      } else {
        currentKey = createKey();
        history.pushState({ __bifrostKey: currentKey }, "", destination);
      }
      renderedURL = destination;
      for (const node of unusedHead) {
        if (node instanceof Element && node.matches("script")) {
          continue;
        }
        node.remove();
      }
      let cursor = headStart.nextSibling;
      for (const node of updatedHead) {
        if (node === cursor) {
          cursor = cursor.nextSibling;
        } else {
          cursor.before(node);
        }
      }
      document.documentElement.lang = data.document.lang;
      const nextClasses = (data.document.class || "").split(/\s+/).filter(Boolean);
      for (const name of documentClasses) {
        if (!nextClasses.includes(name)) {
          document.documentElement.classList.remove(name);
        }
      }
      for (const name of nextClasses) {
        document.documentElement.classList.add(name);
      }
      documentClasses = nextClasses;
      if (data.document.dir) {
        document.documentElement.dir = data.document.dir;
      } else {
        document.documentElement.removeAttribute("dir");
      }
      propsNode.textContent = JSON.stringify(data.props);
      flushSync(() => root.render(tree));
      if (preserveView) {
        scrollTo(...scroll);
        return;
      }
      const focus = app.querySelector("h1") || app.querySelector("main") || app;
      const tabIndex = focus.getAttribute("tabindex");
      focus.setAttribute("tabindex", "-1");
      focus.focus({ preventScroll: true });
      focus.addEventListener("blur", () => {
        if (tabIndex === null) {
          focus.removeAttribute("tabindex");
        } else {
          focus.setAttribute("tabindex", tabIndex);
        }
      }, { once: true });
      restoreScroll(destination, position);
    } catch {
      if (!controller.signal.aborted) {
        if (mode !== "push") {
          location.replace(destination.href);
        } else {
          location.assign(destination.href);
        }
      }
    } finally {
      if (pending === controller) {
        pending = undefined;
        app.removeAttribute("aria-busy");
      }
    }
  }

  document.addEventListener("click", event => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
    if (!link || link.hasAttribute("download") || link.hasAttribute("data-bifrost-reload") || link.relList.contains("external")) {
      return;
    }
    const target = link.getAttribute("target") || document.querySelector("base[target]")?.getAttribute("target");
    if (target && target.toLowerCase() !== "_self") {
      return;
    }
    const url = new URL(link.href);
    if (url.origin !== location.origin || !["http:", "https:"].includes(url.protocol)) {
      return;
    }
    event.preventDefault();
    navigate(url.href);
  });

  async function navigate(href) {
    const url = new URL(href, location.href);
    if (!["http:", "https:"].includes(url.protocol)) {
      throw new TypeError("Bifrost navigation requires an HTTP or HTTPS URL");
    }
    if (url.origin !== location.origin) {
      pending?.abort();
      location.assign(url.href);
      return;
    }
    if (url.pathname === renderedURL.pathname && url.search === renderedURL.search && (url.hash || url.href.endsWith("#"))) {
      pending?.abort();
      saveScroll();
      if (url.href !== location.href) {
        currentKey = createKey();
        history.pushState({ __bifrostKey: currentKey }, "", url);
      }
      renderedURL = url;
      restoreScroll(url);
      return;
    }
    await transition(url);
  }

  setRouter({
    navigate,
    refresh() {
      return transition(new URL(location.href), "refresh");
    },
  });

  addEventListener("scroll", saveScroll, { passive: true });
  addEventListener("popstate", () => {
    const url = new URL(location.href);
    const position = positions.get(history.state?.__bifrostKey);
    if (url.pathname === renderedURL.pathname && url.search === renderedURL.search) {
      pending?.abort();
      currentKey = history.state?.__bifrostKey || currentKey;
      renderedURL = url;
      restoreScroll(url, position);
      return;
    }
    transition(url, "traverse", position);
  });
}
`

const navigationAPI = `type Router = {
  navigate(href: string): Promise<void>;
  refresh(): Promise<void>;
};

let router: Router | undefined;

export function setRouter(value: Router) {
  router = value;
}

export async function navigate(href: string): Promise<void> {
  if (!router) {
    throw new Error("Bifrost navigation requires a mounted convention app");
  }
  await router.navigate(href);
}

export async function refresh(): Promise<void> {
  if (!router) {
    throw new Error("Bifrost navigation requires a mounted convention app");
  }
  await router.refresh();
}
`
