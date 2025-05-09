// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package lockfile provides methods for parsing and writing lockfiles.
package lockfile

import (
	"deps.dev/util/resolve"
	scalibrfs "github.com/google/osv-scalibr/fs"
	"github.com/google/osv-scalibr/guidedremediation/result"
	"github.com/google/osv-scalibr/guidedremediation/strategy"
)

// ReadWriter is the interface for parsing and applying remediation patches to a lockfile.
type ReadWriter interface {
	System() resolve.System
	Read(path string, fsys scalibrfs.FS) (*resolve.Graph, error)
	SupportedStrategies() []strategy.Strategy

	// Write writes the lockfile after applying the patches to outputPath.
	//
	// path is the path to the original (unpatched) lockfile in fsys.
	// outputPath is the path on disk (*not* in fsys) to write the entire patched lockfile to (this can overwrite the original lockfile).
	Write(path string, fsys scalibrfs.FS, patches []result.Patch, outputPath string) error
}
