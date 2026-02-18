package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, cssHref string, chunks []string) (string, error) {
	if scriptSrc == "" {
		return "", fmt.Errorf("missing script src")
	}

	title := "Bifrost"
	if headHTML != "" {
		headHTMLLower := strings.ToLower(headHTML)
		if strings.Contains(headHTMLLower, "<title") {
			title = ""
		}
	}

	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", err
	}

	escapedProps := strings.ReplaceAll(string(propsJSON), "</", "<\\/")

	cssLink := ""
	if cssHref != "" {
		cssLink = "<link rel=\"stylesheet\" href=\"" + cssHref + "\" />"
	}

	head := "<meta charset=\"UTF-8\" /><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\" />"
	if title != "" {
		head += "<title>" + title + "</title>"
	}
	if headHTML != "" {
		head += headHTML
	}
	if cssLink != "" {
		head += cssLink
	}

	var buf bytes.Buffer
	if err := PageTemplate.Execute(&buf, map[string]any{
		"Head":      template.HTML(head),
		"Body":      template.HTML(bodyHTML),
		"Props":     template.JS(escapedProps),
		"ScriptSrc": scriptSrc,
		"Chunks":    chunks,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const clientEntryTemplate = `import * as React from "react";
import { hydrateRoot } from "react-dom/client";
import * as Mod from "{{.ComponentImport}}";

const root = document.getElementById("app");
if (!root) {
  throw new Error("Missing #app element");
}

const propsScript = document.getElementById("__BIFROST_PROPS__");
const propsText = propsScript?.textContent;
const props = propsText ? JSON.parse(propsText) : {};

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");
if (!Component) {
  throw new Error("No component export found in {{.ComponentImport}}");
}

const doHydrate = () => hydrateRoot(root, <Component {...props} />);

if ("requestIdleCallback" in window) {
  requestIdleCallback(doHydrate, { timeout: 2000 });
} else {
  setTimeout(doHydrate, 0);
}
`

const staticClientEntryTemplate = `import * as React from "react";
import { createRoot } from "react-dom/client";
import * as Mod from "{{.ComponentImport}}";

const root = document.getElementById("app");
if (!root) {
  throw new Error("Missing #app element");
}

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");
if (!Component) {
  throw new Error("No component export found in {{.ComponentImport}}");
}

const doRender = () => {
  createRoot(root).render(<Component />);
};

if ("requestIdleCallback" in window) {
  requestIdleCallback(doRender, { timeout: 2000 });
} else {
  setTimeout(doRender, 0);
}
`

var (
	clientEntryTemplateParsed       = template.Must(template.New("client-entry").Parse(clientEntryTemplate))
	staticClientEntryTemplateParsed = template.Must(template.New("static-client-entry").Parse(staticClientEntryTemplate))
)

func WriteClientEntry(path string, componentImport string) error {
	if path == "" {
		return fmt.Errorf("missing entry path")
	}

	if componentImport == "" {
		return fmt.Errorf("missing component import")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := clientEntryTemplateParsed.Execute(&buf, map[string]string{
		"ComponentImport": componentImport,
	}); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func WriteStaticClientEntry(path string, componentImport string) error {
	if path == "" {
		return fmt.Errorf("missing entry path")
	}

	if componentImport == "" {
		return fmt.Errorf("missing component import")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := staticClientEntryTemplateParsed.Execute(&buf, map[string]string{
		"ComponentImport": componentImport,
	}); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
