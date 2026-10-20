package componentsdemo

import (
	"fmt"
	"net/http"
)

func Load(r *http.Request) (any, error) {
	items := make([]map[string]any, 20)
	for index := range items {
		items[index] = map[string]any{
			"Title": fmt.Sprintf("Card %02d", index+1),
			"Tags":  []string{"html", "css"},
		}
	}
	return map[string]any{
		"items": items,
		"menu": map[string]any{
			"Label": "Open menu",
			"Items": []map[string]string{
				{"Label": "Posts", "Href": "/posts"},
				{"Label": "Docs", "Href": "/docs"},
				{"Label": "About", "Href": "/about"},
			},
		},
	}, nil
}
