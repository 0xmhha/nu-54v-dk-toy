package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// ClientGrantFile opens the currently published pathname. Operators publish
// updates with atomic rename. Errors never fall back to a retained grant map.
type ClientGrantFile[T any] string

func (f ClientGrantFile[T]) Read() (map[string]T, error) {
	const max = 1 << 20
	file, err := os.Open(string(f))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || len(raw) > max {
		return nil, errors.New("invalid grant document size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var grants map[string]T
	if err := decoder.Decode(&grants); err != nil || grants == nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid grant document")
	}
	for key := range grants {
		hash, err := hex.DecodeString(key)
		if err != nil || len(hash) != 32 || key != strings.ToLower(key) {
			return nil, errors.New("invalid grant certificate fingerprint")
		}
	}
	return grants, nil
}

func (f ClientGrantFile[T]) Lookup(key string) (T, bool) {
	grants, err := f.Read()
	if err != nil {
		var zero T
		return zero, false
	}
	grant, ok := grants[key]
	return grant, ok
}
