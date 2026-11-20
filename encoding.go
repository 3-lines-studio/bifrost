package bifrost

import (
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

// encodingPreference breaks ties between encodings the client accepts with the
// same quality. Brotli beats gzip on text, so it goes first.
var encodingPreference = []string{"br", "gzip"}

// negotiateEncoding returns the representation to serve and its encoding name,
// empty when the client has to take the identity artifact.
func negotiateEncoding(header string, identity protocol.FileRef, encoded map[string]protocol.FileRef) (protocol.FileRef, string) {
	if len(encoded) == 0 {
		return identity, ""
	}
	accepted := acceptedEncodings(header)
	best := ""
	bestQuality := 0.0
	for _, encoding := range encodingPreference {
		if _, exists := encoded[encoding]; !exists {
			continue
		}
		quality := accepted[encoding]
		if quality > bestQuality {
			best = encoding
			bestQuality = quality
		}
	}
	if best == "" {
		return identity, ""
	}
	return encoded[best], best
}

// acceptedEncodings maps an encoding to the quality the client assigned to it.
// Encodings the client did not name fall back to the wildcard quality.
func acceptedEncodings(header string) map[string]float64 {
	accepted := make(map[string]float64, len(encodingPreference)+1)
	wildcard := 0.0
	for _, part := range strings.Split(header, ",") {
		token, quality := parseEncodingToken(part)
		switch token {
		case "":
			continue
		case "x-gzip":
			accepted["gzip"] = quality
		case "*":
			wildcard = quality
		default:
			accepted[token] = quality
		}
	}
	for _, encoding := range encodingPreference {
		if _, exists := accepted[encoding]; !exists && wildcard > 0 {
			accepted[encoding] = wildcard
		}
	}
	return accepted
}

func parseEncodingToken(part string) (string, float64) {
	token, parameters, _ := strings.Cut(part, ";")
	token = strings.ToLower(strings.TrimSpace(token))
	quality := 1.0
	for _, parameter := range strings.Split(parameters, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(parameter), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return "", 0
		}
		quality = parsed
	}
	return token, quality
}
