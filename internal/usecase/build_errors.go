package usecase

import "strings"

func parseBuildError(entryName string, err error) BuildError {
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
