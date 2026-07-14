package usecase

import (
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

func convertHTMLToMarkdown(html string) (string, error) {
	return htmltomarkdown.ConvertString(html)
}
