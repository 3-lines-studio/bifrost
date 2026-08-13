package bifrost

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

var emptyProps = json.RawMessage(`{}`)

func marshalProps(props any) (json.RawMessage, error) {
	if props == nil {
		return emptyProps, nil
	}
	if raw, ok := props.(RawProps); ok {
		return normalizeRawProps(json.RawMessage(raw))
	}
	data, err := json.Marshal(props)
	if err != nil {
		return nil, err
	}
	return requirePropsObject(safePropsJSON(data))
}

func normalizeRawProps(props json.RawMessage) (json.RawMessage, error) {
	if len(props) == 0 {
		return emptyProps, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, props); err != nil {
		return nil, err
	}
	return requirePropsObject(safePropsJSON(compact.Bytes()))
}

func requirePropsObject(data []byte) (json.RawMessage, error) {
	if len(data) == 0 || data[0] != '{' {
		return nil, errors.New("bifrost: page props must be a JSON object")
	}
	return data, nil
}

func safePropsJSON(data []byte) json.RawMessage {
	data = bytes.ReplaceAll(data, []byte("<"), []byte(`\u003c`))
	data = bytes.ReplaceAll(data, []byte("\u2028"), []byte(`\u2028`))
	data = bytes.ReplaceAll(data, []byte("\u2029"), []byte(`\u2029`))
	return data
}

func normalizeDocument(document Document) (Document, error) {
	if document == (Document{}) {
		return Document{Lang: "en"}, nil
	}
	if document.Lang == "" {
		document.Lang = "en"
	}
	if !validLanguageTag(document.Lang) {
		return Document{}, errors.New("invalid document language")
	}
	if document.Dir != "" && document.Dir != "ltr" && document.Dir != "rtl" && document.Dir != "auto" {
		return Document{}, errors.New("document direction must be ltr, rtl, or auto")
	}
	if len(document.Class) > 1024 {
		return Document{}, errors.New("document class exceeds 1024 bytes")
	}
	for _, value := range document.Class {
		if unicode.IsControl(value) && value != '\t' && value != '\n' && value != '\r' {
			return Document{}, errors.New("document class contains a control character")
		}
	}
	document.Class = strings.Join(strings.Fields(document.Class), " ")
	return document, nil
}

func validLanguageTag(value string) bool {
	if value == "" || len(value) > 64 || strings.TrimSpace(value) != value {
		return false
	}
	parts := strings.Split(value, "-")
	for index, part := range parts {
		if part == "" || len(part) > 8 {
			return false
		}
		for offset, char := range part {
			letter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
			digit := char >= '0' && char <= '9'
			if !letter && (!digit || index == 0 && offset == 0) {
				return false
			}
		}
	}
	return true
}

func protocolDocument(document Document) protocol.DocumentAttributes {
	return protocol.DocumentAttributes{Lang: document.Lang, Class: document.Class, Dir: document.Dir}
}

func documentFromProtocol(document protocol.DocumentAttributes) Document {
	return Document{Lang: document.Lang, Class: document.Class, Dir: document.Dir}
}

func splitPageData(value any) (any, Document, error) {
	document := Document{}
	switch data := value.(type) {
	case PageData:
		value = data.Props
		document = data.Document
	case *PageData:
		if data == nil {
			return nil, Document{}, errors.New("nil PageData")
		}
		value = data.Props
		document = data.Document
	}
	normalized, err := normalizeDocument(document)
	return value, normalized, err
}
