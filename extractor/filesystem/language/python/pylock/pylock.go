package pylock

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/osv-scalibr/extractor"
	"github.com/google/osv-scalibr/extractor/filesystem"
	"github.com/google/osv-scalibr/extractor/filesystem/language/python/internal/pypipurl"
	"github.com/google/osv-scalibr/extractor/filesystem/osv"
	"github.com/google/osv-scalibr/inventory"
	"github.com/google/osv-scalibr/plugin"
	"github.com/google/osv-scalibr/purl"
)

const (
	// Name is the unique name of this extractor.
	Name = "python/pylock"
)

type pylockPackageDirectory struct {
	Path     string `toml:"path"`
	Editable bool   `toml:"editable"`
}

type pylockVcs struct {
	VcsType  string `toml:"type"`
	Url      string `toml:"url"`
	CommitId string `toml:"commit-id"`
}

type pylockPackage struct {
	Name      string
	Version   string
	Index     string
	Directory pylockPackageDirectory `toml:"directory"`
	Vcs       pylockVcs              `toml:"vcs"`
}

type pylockFile struct {
	Version  string                 `toml:"lock-version"`
	Packages []pylockPackage        `toml:"packages"`
	Groups   map[string]interface{} `toml:"dependency-groups"`
}

// Extractor extracts python packages from uv.lock files.
type Extractor struct{}

// New returns a new instance of the extractor.
func New() filesystem.Extractor { return &Extractor{} }

// Name of the extractor
func (e Extractor) Name() string { return Name }

// Version of the extractor
func (e Extractor) Version() int { return 0 }

// Requirements of the extractor
func (e Extractor) Requirements() *plugin.Capabilities {
	return &plugin.Capabilities{}
}

// FileRequired returns true if the specified file matches uv lockfile patterns
func (e Extractor) FileRequired(api filesystem.FileAPI) bool {
	return filepath.Base(api.Path()) == "pylock.toml"
}

// Extract extracts packages from uv.lock files passed through the scan input.
func (e Extractor) Extract(ctx context.Context, input *filesystem.ScanInput) (inventory.Inventory, error) {
	var parsedLockfile *pylockFile

	_, err := toml.NewDecoder(input.Reader).Decode(&parsedLockfile)

	if err != nil {
		return inventory.Inventory{}, fmt.Errorf("could not extract from %s: %w", input.Path, err)
	}

	packages := make([]*extractor.Package, 0, len(parsedLockfile.Packages))

	for _, lockPackage := range parsedLockfile.Packages {

		// skip including the root "package"
		if lockPackage.Directory.Path == "." && lockPackage.Directory.Editable {
			continue
		}

		pkgDetails := &extractor.Package{
			Name:      lockPackage.Name,
			Version:   lockPackage.Version,
			Locations: []string{input.Path},
			Metadata: osv.DepGroupMetadata{
				DepGroupVals: []string{},
			},
		}

		// Specify git repository URL if source is a git one
		if lockPackage.Vcs.CommitId != "" {
			pkgDetails.SourceCode = &extractor.SourceCodeIdentifier{
				Commit: lockPackage.Vcs.CommitId,
			}
		}

		packages = append(packages, pkgDetails)
	}

	return inventory.Inventory{Packages: packages}, nil
}

// ToPURL converts a package created by this extractor into a PURL.
func (e Extractor) ToPURL(p *extractor.Package) *purl.PackageURL {
	return pypipurl.MakePackageURL(p)
}

// Ecosystem returns the OSV ecosystem ('PyPI') of the software extracted by this extractor.
func (e Extractor) Ecosystem(p *extractor.Package) string {
	return "PyPI"
}
