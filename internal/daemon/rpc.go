package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// WriteMessage writes a newline-delimited JSON message.
func WriteMessage(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// ReadMessage reads a newline-delimited JSON message.
func ReadMessage(r io.Reader, v interface{}) error {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	return json.Unmarshal(scanner.Bytes(), v)
}
