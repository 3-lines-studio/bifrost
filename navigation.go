package bifrost

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const navigationMediaType = "application/vnd.bifrost.navigation+json"

type navigationPage struct {
	Build    string                      `json:"build"`
	View     string                      `json:"view"`
	Props    json.RawMessage             `json:"props"`
	Document protocol.DocumentAttributes `json:"document"`
	Head     string                      `json:"head"`
}

type navigationRenderSink struct {
	writer   http.ResponseWriter
	page     navigationPage
	status   int
	limits   Limits
	started  bool
	finished bool
}

func (s *navigationRenderSink) Head(head []byte) error {
	if s.started {
		return errors.New("bifrost: renderer emitted head more than once")
	}
	if len(head) > s.limits.MaxHeadBytes {
		return fmt.Errorf("renderer head exceeds %d bytes", s.limits.MaxHeadBytes)
	}
	s.page.Head = string(head)
	s.started = true
	return nil
}

func (s *navigationRenderSink) Body(body []byte) error {
	if !s.started || s.finished {
		return errors.New("bifrost: renderer body outside head and completion")
	}
	if len(body) > s.limits.MaxFrameBytes {
		return fmt.Errorf("renderer frame exceeds %d bytes", s.limits.MaxFrameBytes)
	}
	return nil
}

func (s *navigationRenderSink) finish() error {
	if !s.started || s.finished {
		return errors.New("bifrost: invalid navigation completion")
	}
	s.writer.Header().Set("Content-Type", navigationMediaType)
	s.writer.Header().Set("Cache-Control", "no-store")
	s.writer.WriteHeader(s.status)
	s.finished = true
	return json.NewEncoder(s.writer).Encode(s.page)
}

func (s *navigationRenderSink) committed() bool {
	return s.finished
}

func (h *serverPageHandler) newSink(w http.ResponseWriter, request *http.Request, props json.RawMessage, document Document, status int) pageRenderSink {
	if h.navigationView != "" && request.Header.Get("Accept") == navigationMediaType && request.Method == http.MethodGet {
		return &navigationRenderSink{
			writer: w,
			page:   navigationPage{Build: h.navigationBuild, View: h.navigationView, Props: props, Document: protocolDocument(document)},
			status: status,
			limits: h.limits,
		}
	}
	return &httpRenderSink{writer: w, shell: h.shell, props: props, document: document, status: status, limits: h.limits}
}
