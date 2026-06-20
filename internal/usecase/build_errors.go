package usecase

import (
	"errors"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func parseBuildError(entryName string, err error) BuildError {
	var se *core.StructuredError
	if errors.As(err, &se) {
		var details []string
		for _, sub := range se.SubErrors {
			line := sub.Message
			if sub.File != "" {
				line += " (" + sub.File + ":" + strconv.Itoa(sub.Line) + ":" + strconv.Itoa(sub.Column) + ")"
			}
			details = append(details, line)
		}
		return BuildError{
			Page:    entryName,
			Message: se.Message,
			Details: details,
		}
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	var message string
	var details []string

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if i == 0 {
			message = line
			continue
		}

		details = append(details, line)
	}

	if message == "" && len(details) > 0 {
		message = details[0]
		details = details[1:]
	}

	return BuildError{
		Page:    entryName,
		Message: message,
		Details: details,
	}
}
