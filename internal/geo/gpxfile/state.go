package gpxfile

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
)

// DGS state is stored in a GPX extension so a file can be reopened without
// a companion JSON file. The value is base64-encoded JSON, not a secret.
var stateElement = regexp.MustCompile(`(?s)\s*<extensions><dgs:state xmlns:dgs="urn:dgs-toolbox:gpx:1">([A-Za-z0-9+/=]*)</dgs:state></extensions>`)

// ReadState returns the embedded dgs JSON, if any.
func ReadState(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if _, err := Parse(bytes.NewReader(data)); err != nil {
		return nil, false, err
	}
	match := stateElement.FindSubmatch(data)
	if match == nil {
		return nil, false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(string(match[1]))
	return decoded, true, err
}

// WriteState atomically replaces the embedded state of a dgs-created file.
// An empty value removes the extension.
func WriteState(path string, state []byte) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, err := Parse(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if !file.IsOurs() {
		return fmt.Errorf("%s: %w", path, ErrNotOurs)
	}
	data = stateElement.ReplaceAll(data, nil)
	end := bytes.LastIndex(data, []byte("</gpx>"))
	if end < 0 {
		return errors.New("GPX root is not closed")
	}
	if len(state) == 0 {
		return replaceFile(path, data)
	}
	var out bytes.Buffer
	out.Write(data[:end])
	out.WriteString("  <extensions><dgs:state xmlns:dgs=\"urn:dgs-toolbox:gpx:1\">")
	out.WriteString(base64.StdEncoding.EncodeToString(state))
	out.WriteString("</dgs:state></extensions>\n")
	out.Write(data[end:])
	return replaceFile(path, out.Bytes())
}
