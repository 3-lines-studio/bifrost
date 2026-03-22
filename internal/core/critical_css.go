package core

import (
	"regexp"
	"sort"
	"strings"
)

const DefaultCriticalCSSMaxBytes = 12 * 1024

var (
	htmlTagRegex   = regexp.MustCompile(`(?i)<([a-z][a-z0-9:-]*)\b`)
	htmlClassRegex = regexp.MustCompile(`(?i)\bclass\s*=\s*("([^"]*)"|'([^']*)')`)
	htmlIDRegex    = regexp.MustCompile(`(?i)\bid\s*=\s*("([^"]*)"|'([^']*)')`)
)

type criticalInventory struct {
	tags    map[string]struct{}
	classes map[string]struct{}
	ids     map[string]struct{}
}

func ExtractCriticalCSS(htmlDoc string, stylesheet string, maxBytes int) string {
	if strings.TrimSpace(htmlDoc) == "" || strings.TrimSpace(stylesheet) == "" {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultCriticalCSSMaxBytes
	}

	inventory := buildCriticalInventory(htmlDoc)
	keyframes := make(map[string]string)
	usedKeyframes := make(map[string]struct{})
	critical := strings.TrimSpace(extractCriticalCSSBlock(stylesheet, inventory, keyframes, usedKeyframes))
	if critical == "" {
		return ""
	}

	if len(usedKeyframes) > 0 {
		names := make([]string, 0, len(usedKeyframes))
		for name := range usedKeyframes {
			if _, ok := keyframes[name]; ok {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		var sb strings.Builder
		sb.WriteString(critical)
		for _, name := range names {
			sb.WriteString(keyframes[name])
		}
		critical = sb.String()
	}

	if len(critical) > maxBytes {
		return ""
	}
	return critical
}

func buildCriticalInventory(htmlDoc string) criticalInventory {
	inv := criticalInventory{
		tags:    map[string]struct{}{"html": {}, "body": {}, "head": {}},
		classes: make(map[string]struct{}),
		ids:     make(map[string]struct{}),
	}

	for _, match := range htmlTagRegex.FindAllStringSubmatch(htmlDoc, -1) {
		if len(match) > 1 {
			inv.tags[strings.ToLower(match[1])] = struct{}{}
		}
	}
	for _, match := range htmlClassRegex.FindAllStringSubmatch(htmlDoc, -1) {
		classValue := firstNonEmpty(match, 2, 3)
		for className := range strings.FieldsSeq(classValue) {
			inv.classes[className] = struct{}{}
		}
	}
	for _, match := range htmlIDRegex.FindAllStringSubmatch(htmlDoc, -1) {
		if id := firstNonEmpty(match, 2, 3); id != "" {
			inv.ids[id] = struct{}{}
		}
	}

	return inv
}

func extractCriticalCSSBlock(css string, inventory criticalInventory, keyframes map[string]string, usedKeyframes map[string]struct{}) string {
	var sb strings.Builder

	for i := 0; i < len(css); {
		i = skipCSSSpaceAndComments(css, i)
		if i >= len(css) {
			break
		}

		if css[i] == '@' {
			ruleStart := i
			headerEnd := findTopLevelCSSChar(css, i, '{', ';')
			if headerEnd == -1 {
				break
			}

			header := strings.TrimSpace(css[ruleStart:headerEnd])
			name := atRuleName(header)
			switch css[headerEnd] {
			case ';':
				if shouldKeepAtStatement(name, header) {
					sb.WriteString(css[ruleStart : headerEnd+1])
				}
				i = headerEnd + 1
			case '{':
				blockEnd := findMatchingCSSBrace(css, headerEnd)
				if blockEnd == -1 {
					return sb.String()
				}
				body := css[headerEnd+1 : blockEnd]
				block := css[ruleStart : blockEnd+1]
				switch name {
				case "font-face", "theme", "property":
					sb.WriteString(block)
				case "keyframes", "-webkit-keyframes":
					keyframes[keyframeName(header)] = block
				case "media", "supports", "layer", "container":
					inner := extractCriticalCSSBlock(body, inventory, keyframes, usedKeyframes)
					if inner != "" {
						sb.WriteString(header)
						sb.WriteByte('{')
						sb.WriteString(inner)
						sb.WriteByte('}')
					}
				default:
					if strings.Contains(body, "--") {
						sb.WriteString(block)
					}
				}
				i = blockEnd + 1
			}
			continue
		}

		ruleStart := i
		selectorEnd := findTopLevelCSSChar(css, i, '{')
		if selectorEnd == -1 {
			break
		}
		blockEnd := findMatchingCSSBrace(css, selectorEnd)
		if blockEnd == -1 {
			break
		}
		selectors := strings.TrimSpace(css[ruleStart:selectorEnd])
		declarations := css[selectorEnd+1 : blockEnd]
		if shouldKeepCSSRule(selectors, declarations, inventory) {
			sb.WriteString(css[ruleStart : blockEnd+1])
			collectUsedKeyframes(declarations, usedKeyframes)
		}
		i = blockEnd + 1
	}

	return sb.String()
}

func shouldKeepAtStatement(name string, header string) bool {
	switch name {
	case "import":
		return false
	case "charset":
		return false
	default:
		return strings.Contains(header, "--")
	}
}

func shouldKeepCSSRule(selectors string, declarations string, inventory criticalInventory) bool {
	if selectors == "" {
		return false
	}
	if strings.Contains(declarations, "--") {
		return true
	}
	for _, selector := range splitCSSSelectorList(selectors) {
		if selectorMatchesInventory(selector, inventory) {
			return true
		}
	}
	return false
}

func selectorMatchesInventory(selector string, inventory criticalInventory) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	if shouldAlwaysKeepSelector(selector) {
		return true
	}

	for i := 0; i < len(selector); i++ {
		switch selector[i] {
		case '.':
			name, end := readCSSIdent(selector, i+1)
			if name != "" {
				if _, ok := inventory.classes[name]; ok {
					return true
				}
			}
			i = end - 1
		case '#':
			name, end := readCSSIdent(selector, i+1)
			if name != "" {
				if _, ok := inventory.ids[name]; ok {
					return true
				}
			}
			i = end - 1
		default:
			if isSelectorIdentStart(selector[i]) && (i == 0 || isSelectorBoundary(selector[i-1])) {
				name, end := readCSSIdent(selector, i)
				if name != "" {
					if _, ok := inventory.tags[strings.ToLower(name)]; ok {
						return true
					}
				}
				i = end - 1
			}
		}
	}

	return false
}

func shouldAlwaysKeepSelector(selector string) bool {
	s := strings.ToLower(strings.TrimSpace(selector))
	if s == "" {
		return false
	}
	if strings.Contains(s, ":root") || strings.Contains(s, "html") || strings.Contains(s, "body") {
		return true
	}
	if strings.HasPrefix(s, "*") || strings.Contains(s, "::before") || strings.Contains(s, "::after") {
		return true
	}
	return false
}

func splitCSSSelectorList(selectors string) []string {
	var parts []string
	start := 0
	bracketDepth := 0
	parenDepth := 0
	inString := byte(0)

	for i := 0; i < len(selectors); i++ {
		ch := selectors[i]
		if inString != 0 {
			if ch == '\\' && i+1 < len(selectors) {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ',':
			if bracketDepth == 0 && parenDepth == 0 {
				parts = append(parts, strings.TrimSpace(selectors[start:i]))
				start = i + 1
			}
		}
	}

	if tail := strings.TrimSpace(selectors[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func collectUsedKeyframes(declarations string, used map[string]struct{}) {
	for _, value := range extractCSSPropertyValues(declarations, "animation-name") {
		for part := range strings.SplitSeq(value, ",") {
			if name := strings.TrimSpace(part); isUsableAnimationName(name) {
				used[name] = struct{}{}
			}
		}
	}
	for _, value := range extractCSSPropertyValues(declarations, "animation") {
		for part := range strings.SplitSeq(value, ",") {
			fields := strings.FieldsSeq(part)
			for field := range fields {
				field = strings.TrimSpace(field)
				if isUsableAnimationName(field) {
					used[field] = struct{}{}
					break
				}
			}
		}
	}
}
