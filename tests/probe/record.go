package main

import (
	"encoding/json"
	"os"
	"strings"
)

// Record is what the probe writes to PID.json and tmux.json.
type Record struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

// writeRecord writes the probe's Record to path.
func writeRecord(path string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	data, err := json.Marshal(Record{Argv: os.Args[1:], Cwd: cwd, Env: environment()})
	if err != nil {
		return err
	}
	// Renamed into place, so a reader never sees a partial file.
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// environment is the probe's environment, by variable.
func environment() map[string]string {
	variables := map[string]string{}
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		variables[name] = value
	}
	return variables
}

// appendJSON appends value to the file at path as a line of JSON, creating the file.
func appendJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return appendLine(path, data)
}

// appendLine appends line and a newline to the file at path, creating the file.
func appendLine(path string, line []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
