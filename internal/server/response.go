package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func writeJSONResponse(w http.ResponseWriter, status int, dto interface{}) (err error) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var b []byte
	if dto != nil {
		b, err = json.Marshal(dto)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		body := []byte(string(b) + "\n")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		_, err = w.Write(body)
		return err
	}
	w.WriteHeader(status)
	return nil
}

func notFound(w http.ResponseWriter, msg string) error {
	return writeJSONResponse(w, http.StatusNotFound, Error{
		Type:    "NotFound",
		Message: msg,
	})
}

func internalServerError(w http.ResponseWriter, err error) error {
	_ = writeJSONResponse(w, http.StatusNotFound, Error{
		Type:    "InternalServerError",
		Message: "(see server logs)",
	})
	return err
}

func stream(w http.ResponseWriter) *streamResponse {
	w.Header().Set("Content-Type", "application/json+stream; charset=UTF-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Keep-Alive", "timeout=60")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &streamResponse{writer: w}
}

type streamResponse struct {
	writer http.ResponseWriter
}

func (r *streamResponse) Write(dto interface{}) (err error) {
	var b []byte
	if dto != nil {
		b, err = json.Marshal(dto)
		if err != nil {
			return err
		}
		_, err = r.writer.Write([]byte(string(b) + "\n"))
		if err != nil {
			return err
		}
		f, ok := r.writer.(http.Flusher)
		if !ok {
			return fmt.Errorf("ResponseWriter is not a Flusher")
		}
		// The writer must be flushed to emit the event to the client immediately.
		f.Flush()
	}
	return nil
}
