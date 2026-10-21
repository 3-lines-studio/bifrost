package ui

import (
	"fmt"
	"strconv"
	"sync/atomic"
)

const (
	Default = "default"
	Primary = "primary"
	Danger  = "danger"
	Ghost   = "ghost"
	Outline = "outline"

	Small  = "sm"
	Medium = "md"
	Large  = "lg"
)

var idCounter atomic.Uint64

func ID(prefix string) string {
	return fmt.Sprintf("ui-%s-%d", prefix, idCounter.Add(1))
}

func quote(value string) string {
	return strconv.Quote(value)
}
