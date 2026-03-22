package core

import (
	"strings"
	"unicode"
)

func skipCSSSpaceAndComments(css string, i int) int {
	for i < len(css) {
		switch {
		case unicode.IsSpace(rune(css[i])):
			i++
		case i+1 < len(css) && css[i] == '/' && css[i+1] == '*':
			end := strings.Index(css[i+2:], "*/")
			if end == -1 {
				return len(css)
			}
			i += end + 4
		default:
			return i
		}
	}
	return i
}

func findTopLevelCSSChar(css string, start int, targets ...byte) int {
	targetSet := make(map[byte]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target] = struct{}{}
	}

	parenDepth := 0
	bracketDepth := 0
	inString := byte(0)
	for i := start; i < len(css); i++ {
		ch := css[i]
		if inString != 0 {
			if ch == '\\' && i+1 < len(css) {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if i+1 < len(css) && ch == '/' && css[i+1] == '*' {
			end := strings.Index(css[i+2:], "*/")
			if end == -1 {
				return -1
			}
			i += end + 3
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		default:
			if parenDepth == 0 && bracketDepth == 0 {
				if _, ok := targetSet[ch]; ok {
					return i
				}
			}
		}
	}
	return -1
}

func findMatchingCSSBrace(css string, open int) int {
	depth := 0
	inString := byte(0)
	for i := open; i < len(css); i++ {
		ch := css[i]
		if inString != 0 {
			if ch == '\\' && i+1 < len(css) {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if i+1 < len(css) && ch == '/' && css[i+1] == '*' {
			end := strings.Index(css[i+2:], "*/")
			if end == -1 {
				return -1
			}
			i += end + 3
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func readCSSIdent(s string, start int) (string, int) {
	i := start
	for i < len(s) {
		ch := s[i]
		if !unicode.IsLetter(rune(ch)) && !unicode.IsDigit(rune(ch)) && ch != '-' && ch != '_' {
			break
		}
		i++
	}
	return s[start:i], i
}

func isSelectorIdentStart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_' || ch == '-'
}

func isSelectorBoundary(ch byte) bool {
	switch ch {
	case ' ', '>', '+', '~', ',', '(', '[', ':':
		return true
	default:
		return false
	}
}

func extractCSSPropertyValues(declarations string, property string) []string {
	var values []string
	lowerDecls := strings.ToLower(declarations)
	property = strings.ToLower(property)
	search := property + ":"
	for offset := 0; offset < len(lowerDecls); {
		idx := strings.Index(lowerDecls[offset:], search)
		if idx == -1 {
			break
		}
		idx += offset
		valueStart := idx + len(search)
		valueEnd := valueStart
		parenDepth := 0
		inString := byte(0)
		for valueEnd < len(declarations) {
			ch := declarations[valueEnd]
			if inString != 0 {
				if ch == '\\' && valueEnd+1 < len(declarations) {
					valueEnd += 2
					continue
				}
				if ch == inString {
					inString = 0
				}
				valueEnd++
				continue
			}
			switch ch {
			case '\'', '"':
				inString = ch
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
			case ';':
				if parenDepth == 0 {
					values = append(values, strings.TrimSpace(declarations[valueStart:valueEnd]))
					offset = valueEnd + 1
					goto nextProperty
				}
			}
			valueEnd++
		}
		values = append(values, strings.TrimSpace(declarations[valueStart:valueEnd]))
		offset = valueEnd
	nextProperty:
	}
	return values
}

func isUsableAnimationName(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" || name == "none" {
		return false
	}
	switch name {
	case "linear", "ease", "ease-in", "ease-out", "ease-in-out", "infinite", "normal", "reverse", "alternate", "alternate-reverse", "forwards", "backwards", "both", "running", "paused", "step-start", "step-end":
		return false
	}
	if unicode.IsDigit(rune(name[0])) || strings.HasSuffix(name, "ms") || strings.HasSuffix(name, "s") {
		return false
	}
	return true
}

func atRuleName(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "@") {
		return ""
	}
	header = header[1:]
	for i, r := range header {
		if unicode.IsSpace(r) {
			return strings.ToLower(header[:i])
		}
	}
	return strings.ToLower(header)
}

func keyframeName(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	if idx := strings.IndexFunc(header, unicode.IsSpace); idx >= 0 {
		return strings.TrimSpace(header[idx:])
	}
	return ""
}

func firstNonEmpty(match []string, indexes ...int) string {
	for _, idx := range indexes {
		if idx < len(match) && match[idx] != "" {
			return match[idx]
		}
	}
	return ""
}
