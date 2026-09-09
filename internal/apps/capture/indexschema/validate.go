// Package indexschema validates Capture index metadata independently of the TUI.
package indexschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"
)

var sha256Pattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

// Attachment is the portion of an attachment entry needed by the Scan tree.
type Attachment struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
}

type Device struct {
	OS            string `json:"os"`
	SystemVersion string `json:"systemVesion"`
	Name          string `json:"name"`
}

type Source struct {
	App    string `json:"app"`
	Device Device `json:"device"`
}

type Position struct {
	Altitude     float64 `json:"altitude"`
	Longitude    float64 `json:"longitude"`
	Latitude     float64 `json:"latitude"`
	Address      string  `json:"address"`
	Locality     string  `json:"locality"`
	City         string  `json:"city"`
	Region       string  `json:"region"`
	LegacyRegion string  `json:"region "`
	Country      string  `json:"country"`
}

// Index is the typed subset used by Capture Scan after validation. Payload is
// deliberately dynamic because its fields are defined by the capture type.
type Index struct {
	Schema      string         `json:"schema"`
	Source      Source         `json:"source"`
	Position    Position       `json:"position"`
	ID          string         `json:"id"`
	IsDone      string         `json:"isDone"`
	Payload     map[string]any `json:"payload"`
	Attachments []Attachment   `json:"attachments"`
	CreatedAt   string         `json:"createdAt"`
	Type        string         `json:"type"`
	Dir         string         `json:"dir"`
}

// ReadFile validates and decodes a Capture index.
func ReadFile(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Index{}, fmt.Errorf("read index: %w", err)
	}
	if err := Validate(bytes.NewReader(data)); err != nil {
		return Index{}, fmt.Errorf("validate index: %w", err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("decode index: %w", err)
	}
	return index, nil
}

// ReadAttachments validates a Capture index and returns its attachment
// references in manifest order.
func ReadAttachments(path string) ([]Attachment, error) {
	index, err := ReadFile(path)
	return index.Attachments, err
}

// ValidateFile validates one Capture index against the v1 contract.
func ValidateFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer file.Close()
	if err := Validate(file); err != nil {
		return fmt.Errorf("validate index: %w", err)
	}
	return nil
}

// Validate validates a single JSON document against the Capture index v1
// schema recorded in docs/apps/capture/index-v1.schema.json.
func Validate(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}

	root, err := asObject(value, "index")
	if err != nil {
		return err
	}
	if err := requireOnly(root,
		[]string{"schema", "source", "position", "id", "isDone", "createdAt", "type", "dir"},
		[]string{"schema", "source", "position", "id", "isDone", "payload", "attachments", "createdAt", "type", "dir"},
		"index"); err != nil {
		return err
	}
	if schema, err := nonEmptyString(root, "schema", "index"); err != nil || schema != "v1" {
		if err != nil {
			return err
		}
		return fmt.Errorf("index.schema must equal %q", "v1")
	}
	if _, err := nonEmptyString(root, "id", "index"); err != nil {
		return err
	}
	if done, err := nonEmptyString(root, "isDone", "index"); err != nil || (done != "true" && done != "false") {
		if err != nil {
			return err
		}
		return fmt.Errorf("index.isDone must equal %q or %q", "true", "false")
	}
	if payload, exists := root["payload"]; exists {
		if _, err := asObject(payload, "index.payload"); err != nil {
			return err
		}
	}
	createdAt, err := nonEmptyString(root, "createdAt", "index")
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return fmt.Errorf("index.createdAt must be an RFC 3339 date-time: %w", err)
	}
	if _, err := nonEmptyString(root, "type", "index"); err != nil {
		return err
	}
	if _, err := nonEmptyString(root, "dir", "index"); err != nil {
		return err
	}
	if err := validateSource(root["source"]); err != nil {
		return err
	}
	if err := validatePosition(root["position"]); err != nil {
		return err
	}
	if attachments, ok := root["attachments"]; ok {
		if err := validateAttachments(attachments); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(value any) error {
	source, err := asObject(value, "index.source")
	if err != nil {
		return err
	}
	if err := requireOnly(source, []string{"app", "device"}, []string{"app", "device"}, "index.source"); err != nil {
		return err
	}
	if _, err := nonEmptyString(source, "app", "index.source"); err != nil {
		return err
	}
	device, err := asObject(source["device"], "index.source.device")
	if err != nil {
		return err
	}
	fields := []string{"os", "systemVesion", "name"}
	if err := requireOnly(device, fields, fields, "index.source.device"); err != nil {
		return err
	}
	for _, field := range fields {
		if _, err := nonEmptyString(device, field, "index.source.device"); err != nil {
			return err
		}
	}
	return nil
}

func validatePosition(value any) error {
	position, err := asObject(value, "index.position")
	if err != nil {
		return err
	}
	required := []string{"altitude", "longitude", "latitude"}
	allowed := []string{"altitude", "longitude", "latitude", "address", "locality", "city", "region", "region ", "country"}
	if err := requireOnly(position, required, allowed, "index.position"); err != nil {
		return err
	}
	for _, field := range []string{"address", "locality", "city", "region", "region ", "country"} {
		if _, exists := position[field]; !exists {
			continue
		}
		if _, err := nonEmptyString(position, field, "index.position"); err != nil {
			return err
		}
	}
	if _, err := number(position, "altitude", "index.position"); err != nil {
		return err
	}
	longitude, err := number(position, "longitude", "index.position")
	if err != nil {
		return err
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("index.position.longitude must be between -180 and 180")
	}
	latitude, err := number(position, "latitude", "index.position")
	if err != nil {
		return err
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("index.position.latitude must be between -90 and 90")
	}
	return nil
}

func validateAttachments(value any) error {
	attachments, ok := value.([]any)
	if !ok {
		return fmt.Errorf("index.attachments must be an array")
	}
	for index, value := range attachments {
		path := fmt.Sprintf("index.attachments[%d]", index)
		attachment, err := asObject(value, path)
		if err != nil {
			return err
		}
		if _, err := nonEmptyString(attachment, "kind", path); err != nil {
			return err
		}
		if value, ok := attachment["sha256"]; ok {
			digest, ok := value.(string)
			if !ok || !sha256Pattern.MatchString(digest) {
				return fmt.Errorf("%s.sha256 must be a 64-digit hexadecimal string", path)
			}
		}
		if value, ok := attachment["name"]; ok {
			name, ok := value.(string)
			if !ok || name == "" {
				return fmt.Errorf("%s.name must be a non-empty string", path)
			}
		}
	}
	return nil
}

func asObject(value any, path string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	return object, nil
}

func requireOnly(object map[string]any, required, allowed []string, path string) error {
	for _, field := range required {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("%s.%s is required", path, field)
		}
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowedSet[field]; !ok {
			return fmt.Errorf("%s.%s is not allowed", path, field)
		}
	}
	return nil
}

func nonEmptyString(object map[string]any, field, path string) (string, error) {
	value, ok := object[field].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s.%s must be a non-empty string", path, field)
	}
	return value, nil
}

func number(object map[string]any, field, path string) (float64, error) {
	value, ok := object[field].(float64)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be a number", path, field)
	}
	return value, nil
}
