// Package jsonfile writes values as indented JSON files.
package jsonfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Write saves v as indented JSON to path, overwriting any existing file.
func Write(path string, v any) error {
	data, err := encode(path, v)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Create saves v as indented JSON to a new file at path. It never overwrites:
// if path already exists it returns an error matching fs.ErrExist.
func Create(path string, v any) error {
	data, err := encode(path, v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is a fixed file name chosen by the caller
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", path, err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func encode(path string, v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", path, err)
	}
	return append(data, '\n'), nil
}
