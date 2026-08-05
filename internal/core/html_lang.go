package core

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

const DefaultHTMLLang = "en"

const PropHTMLLang = "__bifrost_html_lang"

const PropHTMLClass = "__bifrost_html_class"

var htmlLangTagPattern = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8})*$`)

func SanitizeHTMLLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultHTMLLang
	}
	if htmlLangTagPattern.MatchString(s) {
		return s
	}
	return DefaultHTMLLang
}

func SanitizeHTMLClass(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func stripReservedKeysFromMap(m map[string]any) (cleaned map[string]any, lang string, class string, hasReserved bool) {
	rawLang, hasLang := m[PropHTMLLang].(string)
	rawClass, hasClass := m[PropHTMLClass].(string)
	if !hasLang && !hasClass {
		return m, "", "", false
	}
	cleaned = make(map[string]any, len(m))
	for k, v := range m {
		switch k {
		case PropHTMLLang, PropHTMLClass:
			continue
		}
		cleaned[k] = v
	}
	return cleaned, rawLang, rawClass, true
}

// reservedKeysFromStruct looks up the reserved HTML lang/class keys in struct props
// without a JSON round trip. When any reserved key is found it returns a map of the
// remaining fields, matching the legacy JSON-decoded output shape.
func reservedKeysFromStruct(props any) (cleaned map[string]any, lang string, class string, hasReserved bool) {
	t := reflect.TypeOf(props)
	v := reflect.ValueOf(props)
	if t == nil {
		return nil, "", "", false
	}
	for t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, "", "", false
		}
		t = t.Elem()
		v = v.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, "", "", false
	}
	lang, class, hasReserved = findReservedStructFields(t, v)
	if !hasReserved {
		return nil, "", "", false
	}
	return structFieldsAsMap(t, v), lang, class, true
}

func findReservedStructFields(t reflect.Type, v reflect.Value) (lang string, class string, found bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous {
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if name == "" {
				ft := f.Type
				if ft.Kind() == reflect.Pointer {
					if fv.IsNil() {
						continue
					}
					ft = ft.Elem()
					fv = fv.Elem()
				}
				if ft.Kind() == reflect.Struct {
					l, c, f2 := findReservedStructFields(ft, fv)
					if f2 {
						lang, class, found = l, c, true
					}
				}
				continue
			}
		}
		if f.PkgPath != "" {
			continue
		}
		name := jsonFieldName(f)
		switch name {
		case PropHTMLLang:
			if fv.Kind() == reflect.String {
				lang, found = fv.String(), true
			}
		case PropHTMLClass:
			if fv.Kind() == reflect.String {
				class, found = fv.String(), true
			}
		}
	}
	return lang, class, found
}

func structFieldsAsMap(t reflect.Type, v reflect.Value) map[string]any {
	cleaned := make(map[string]any, t.NumField())
	var walk func(t reflect.Type, v reflect.Value)
	walk = func(t reflect.Type, v reflect.Value) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fv := v.Field(i)
			if f.Anonymous {
				tag := f.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, _, _ := strings.Cut(tag, ",")
				if name == "" {
					ft := f.Type
					if ft.Kind() == reflect.Pointer {
						if fv.IsNil() {
							continue
						}
						ft = ft.Elem()
						fv = fv.Elem()
					}
					if ft.Kind() == reflect.Struct {
						walk(ft, fv)
					}
					continue
				}
			}
			if f.PkgPath != "" {
				continue
			}
			name := jsonFieldName(f)
			if name == PropHTMLLang || name == PropHTMLClass || name == "-" {
				continue
			}
			if jsonFieldOmitEmpty(f) && fv.IsZero() {
				continue
			}
			cleaned[name] = fv.Interface()
		}
	}
	walk(t, v)
	return cleaned
}

func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return f.Name
	}
	return name
}

func jsonFieldOmitEmpty(f reflect.StructField) bool {
	tag := f.Tag.Get("json")
	_, _, has := strings.Cut(tag, ",")
	return has && strings.Contains(tag, "omitempty")
}

func isPropsMap(props any) bool {
	_, ok := props.(map[string]any)
	return ok
}

func isStructProps(props any) bool {
	t := reflect.TypeOf(props)
	if t == nil {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}

func jsonRoundTrip(props any) map[string]any {
	data, err := json.Marshal(props)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func ResolveHTMLDocumentAttrs(appDefaultLang, pageLang, pageClass string, props any) (lang string, htmlClass string, propsForReact any) {
	var fromLoaderLang string
	var fromLoaderClass string

	if props != nil {
		switch {
		case isPropsMap(props):
			m := props.(map[string]any)
			cleaned, rawLang, rawClass, hasReserved := stripReservedKeysFromMap(m)
			fromLoaderLang = rawLang
			fromLoaderClass = rawClass
			if hasReserved {
				propsForReact = cleaned
			} else {
				propsForReact = props
			}
		default:
			cleaned, rawLang, rawClass, hasReserved := reservedKeysFromStruct(props)
			if hasReserved {
				fromLoaderLang = rawLang
				fromLoaderClass = rawClass
				propsForReact = cleaned
			} else if isStructProps(props) {
				propsForReact = props
			} else {
				// Typed maps, slices, scalars: legacy JSON round trip to find reserved keys.
				cleaned, rawLang, rawClass, hasReserved := stripReservedKeysFromMap(jsonRoundTrip(props))
				if hasReserved {
					fromLoaderLang = rawLang
					fromLoaderClass = rawClass
					propsForReact = cleaned
				} else {
					propsForReact = props
				}
			}
		}
	}

	switch {
	case strings.TrimSpace(fromLoaderLang) != "":
		lang = SanitizeHTMLLang(fromLoaderLang)
	case strings.TrimSpace(pageLang) != "":
		lang = SanitizeHTMLLang(pageLang)
	case strings.TrimSpace(appDefaultLang) != "":
		lang = SanitizeHTMLLang(appDefaultLang)
	default:
		lang = DefaultHTMLLang
	}

	switch {
	case strings.TrimSpace(fromLoaderClass) != "":
		htmlClass = SanitizeHTMLClass(fromLoaderClass)
	case strings.TrimSpace(pageClass) != "":
		htmlClass = SanitizeHTMLClass(pageClass)
	default:
		htmlClass = ""
	}

	return lang, htmlClass, propsForReact
}
