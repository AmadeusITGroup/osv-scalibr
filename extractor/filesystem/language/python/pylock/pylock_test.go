package pylock_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/osv-scalibr/extractor"
	"github.com/google/osv-scalibr/extractor/filesystem/language/python/pylock"
	"github.com/google/osv-scalibr/extractor/filesystem/osv"
	"github.com/google/osv-scalibr/extractor/filesystem/simplefileapi"
	"github.com/google/osv-scalibr/inventory"
	"github.com/google/osv-scalibr/testing/extracttest"
)

func pkg(t *testing.T, name string, version string, location string) *extractor.Package {
	t.Helper()

	return &extractor.Package{
		Name:      name,
		Version:   version,
		Locations: []string{location},
		Metadata: osv.DepGroupMetadata{
			DepGroupVals: []string{},
		},
	}
}

func TestExtractor_FileRequired(t *testing.T) {
	tests := []struct {
		name      string
		inputPath string
		want      bool
	}{
		{
			name:      "",
			inputPath: "",
			want:      false,
		},
		{
			name:      "",
			inputPath: "pylock.toml",
			want:      true,
		},
		{
			name:      "",
			inputPath: "path/to/my/pylock.toml",
			want:      true,
		},
		{
			name:      "",
			inputPath: "path/to/my/pylock.toml/file",
			want:      false,
		},
		{
			name:      "",
			inputPath: "path/to/my/pylock.toml.file",
			want:      false,
		},
		{
			name:      "",
			inputPath: "path.to.my.pylock.toml",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := pylock.Extractor{}
			got := e.FileRequired(simplefileapi.New(tt.inputPath, nil))
			if got != tt.want {
				t.Errorf("FileRequired(%q, FileInfo) got = %v, want %v", tt.inputPath, got, tt.want)
			}
		})
	}
}

func TestExtractor_Extract(t *testing.T) {
	tests := []extracttest.TestTableEntry{
		{
			Name: "invalid toml",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/not-toml.txt",
			},
			WantErr:      extracttest.ContainsErrStr{Str: "could not extract from"},
			WantPackages: nil,
		},
		{
			Name: "empty toml",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/empty.toml",
			},
			WantErr:      nil,
			WantPackages: []*extractor.Package{},
		},
		{
			Name: "no packages",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/no-packages.toml",
			},
			WantPackages: []*extractor.Package{},
		},
		{
			Name: "no dependencies",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/no-dependencies.toml",
			},
			WantPackages: []*extractor.Package{},
		},
		{
			Name: "one package",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/one-package.toml",
			},
			WantPackages: []*extractor.Package{
				pkg(t, "emoji", "2.14.1", "testdata/one-package.toml"),
			},
		},
		{
			Name: "two packages",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/two-packages.toml",
			},
			WantPackages: []*extractor.Package{
				pkg(t, "emoji", "2.14.1", "testdata/two-packages.toml"),
				pkg(t, "protobuf", "6.30.2", "testdata/two-packages.toml"),
			},
		},
		{
			Name: "source git",
			InputConfig: extracttest.ScanInputMockConfig{
				Path: "testdata/source-git.toml",
			},
			WantPackages: []*extractor.Package{
				{
					Name:      "structlog",
					Version:   "25.3.1.dev1",
					Locations: []string{"testdata/source-git.toml"},
					SourceCode: &extractor.SourceCodeIdentifier{
						Commit: "677d00b8b7384b35fdde179093cfd5113894d75b",
					},
					Metadata: osv.DepGroupMetadata{
						DepGroupVals: []string{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			extr := pylock.Extractor{}

			scanInput := extracttest.GenerateScanInputMock(t, tt.InputConfig)
			defer extracttest.CloseTestScanInput(t, scanInput)

			got, err := extr.Extract(context.Background(), &scanInput)

			if diff := cmp.Diff(tt.WantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s.Extract(%q) error diff (-want +got):\n%s", extr.Name(), tt.InputConfig.Path, diff)
				return
			}

			wantInv := inventory.Inventory{Packages: tt.WantPackages}
			if diff := cmp.Diff(wantInv, got, cmpopts.SortSlices(extracttest.PackageCmpLess)); diff != "" {
				t.Errorf("%s.Extract(%q) diff (-want +got):\n%s", extr.Name(), tt.InputConfig.Path, diff)
			}
		})
	}
}
