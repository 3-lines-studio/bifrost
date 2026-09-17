package export

import (
	"fmt"
	"net/http"
	"strings"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Get(w http.ResponseWriter, r *http.Request) {
	var out strings.Builder
	out.WriteString("# Bifrost App Router demo\n\n")
	for _, project := range store.Projects() {
		fmt.Fprintf(&out, "- **%s** — %s\n", project.Name, project.Summary)
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(out.String()))
}
