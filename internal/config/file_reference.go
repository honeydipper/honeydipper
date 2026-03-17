// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

// ErrFileError is all errors when accessing a file.
var ErrFileError = errors.New("file error")

func (c *Repo) normalizeFilePath(cwd string, file string) string {
	fullpath := path.Clean(path.Join(cwd, file))
	if !strings.HasPrefix(fullpath, c.root+"/") {
		panic(fmt.Errorf("%w: invalid path: %s", ErrFileError, fullpath))
	}

	return fullpath
}

func (c *Repo) normalizeFilePaths(currentFile string, content *DataSet) {
	var processor func(key string, val interface{}) (interface{}, bool)

	cwd := path.Dir(currentFile)

	processor = func(_ string, val interface{}) (interface{}, bool) {
		switch v := val.(type) {
		case string:
			if len(v) > 2 && v[0:2] == "@:" {
				text, err := os.ReadFile(c.normalizeFilePath(cwd, v[2:]))
				if err != nil {
					panic(err)
				}

				return string(text), true
			}

			return nil, false
		case Trigger:
			dipper.Recursive(v.Match, processor)
			dipper.Recursive(v.Parameters, processor)
			dipper.Recursive(v.Export, processor)

			return nil, false
		case Function:
			dipper.Recursive(v.Parameters, processor)
			dipper.Recursive(v.Export, processor)
			dipper.Recursive(v.ExportOnSuccess, processor)
			dipper.Recursive(v.ExportOnFailure, processor)

			return nil, false
		case System:
			dipper.Recursive(v.Triggers, processor)
			dipper.Recursive(v.Functions, processor)
			dipper.Recursive(v.Data, processor)

			return nil, false
		case Rule:
			dipper.Recursive(&v.Do, processor)
			dipper.Recursive(&v.When, processor)

			return nil, false
		case Workflow:
			dipper.Recursive(v.Match, processor)
			dipper.Recursive(v.UnlessMatch, processor)
			dipper.Recursive(v.Local, processor)
			dipper.Recursive(v.Export, processor)
			dipper.Recursive(v.ExportOnSuccess, processor)
			dipper.Recursive(v.ExportOnFailure, processor)

			dipper.Recursive(v.Else, processor)
			dipper.Recursive(v.Steps, processor)
			dipper.Recursive(v.Threads, processor)
			dipper.Recursive(v.Cases, processor)

			return nil, false
		}

		return nil, false
	}

	dipper.Recursive(content, processor)
}
