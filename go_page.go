package bifrost

import (
	"errors"
	"log"
	"net/http"
	"runtime/debug"
)

func RecoverPage(w http.ResponseWriter, r *http.Request) {
	recovered := recover()
	if recovered == nil {
		return
	}
	log.Printf("bifrost: go page panic: %v\n%s", recovered, debug.Stack())
	if DataRequested(r) {
		WriteDataError(w, errors.New(http.StatusText(http.StatusInternalServerError)))
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
