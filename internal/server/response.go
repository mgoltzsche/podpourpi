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
