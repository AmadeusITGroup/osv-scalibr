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

// Package proto provides protobuf related utilities for the SCALIBR binary.
package proto

import (
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/osv-scalibr/converter"
	"github.com/google/osv-scalibr/detector"
	"github.com/google/osv-scalibr/log"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	scalibr "github.com/google/osv-scalibr"
	"github.com/google/osv-scalibr/extractor"
	ctrdfs "github.com/google/osv-scalibr/extractor/filesystem/containers/containerd"
	"github.com/google/osv-scalibr/extractor/filesystem/containers/podman"
	"github.com/google/osv-scalibr/extractor/filesystem/language/dotnet/depsjson"
	archivemeta "github.com/google/osv-scalibr/extractor/filesystem/language/java/archive/metadata"
	"github.com/google/osv-scalibr/extractor/filesystem/language/java/javalockfile"
	"github.com/google/osv-scalibr/extractor/filesystem/language/javascript/packagejson"
	"github.com/google/osv-scalibr/extractor/filesystem/language/python/requirements"
	"github.com/google/osv-scalibr/extractor/filesystem/language/python/setup"
	"github.com/google/osv-scalibr/extractor/filesystem/language/python/wheelegg"
	chromeextensions "github.com/google/osv-scalibr/extractor/filesystem/misc/chrome/extensions"
	"github.com/google/osv-scalibr/extractor/filesystem/misc/vscodeextensions"
	apkmeta "github.com/google/osv-scalibr/extractor/filesystem/os/apk/metadata"
	cosmeta "github.com/google/osv-scalibr/extractor/filesystem/os/cos/metadata"
	dpkgmeta "github.com/google/osv-scalibr/extractor/filesystem/os/dpkg/metadata"
	flatpakmeta "github.com/google/osv-scalibr/extractor/filesystem/os/flatpak/metadata"
	"github.com/google/osv-scalibr/extractor/filesystem/os/homebrew"
	modulemeta "github.com/google/osv-scalibr/extractor/filesystem/os/kernel/module/metadata"
	vmlinuzmeta "github.com/google/osv-scalibr/extractor/filesystem/os/kernel/vmlinuz/metadata"
	"github.com/google/osv-scalibr/extractor/filesystem/os/macapps"
	nixmeta "github.com/google/osv-scalibr/extractor/filesystem/os/nix/metadata"
	pacmanmeta "github.com/google/osv-scalibr/extractor/filesystem/os/pacman/metadata"
	portagemeta "github.com/google/osv-scalibr/extractor/filesystem/os/portage/metadata"
	rpmmeta "github.com/google/osv-scalibr/extractor/filesystem/os/rpm/metadata"
	snapmeta "github.com/google/osv-scalibr/extractor/filesystem/os/snap/metadata"
	"github.com/google/osv-scalibr/extractor/filesystem/osv"
	cdxmeta "github.com/google/osv-scalibr/extractor/filesystem/sbom/cdx/metadata"
	spdxmeta "github.com/google/osv-scalibr/extractor/filesystem/sbom/spdx/metadata"
	ctrdruntime "github.com/google/osv-scalibr/extractor/standalone/containers/containerd"
	"github.com/google/osv-scalibr/extractor/standalone/containers/docker"
	winmetadata "github.com/google/osv-scalibr/extractor/standalone/windows/common/metadata"
	"github.com/google/osv-scalibr/plugin"
	"github.com/google/osv-scalibr/purl"

	spb "github.com/google/osv-scalibr/binary/proto/scan_result_go_proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fileType represents the type of a proto result file.
type fileType struct {
	isGZipped  bool
	isBinProto bool
}

// typeForPath returns the proto type of a path, or an error if the path is not a valid proto file.
func typeForPath(filePath string) (*fileType, error) {
	ext := filepath.Ext(filePath)
	if ext == "" {
		return nil, errors.New("invalid filename: Doesn't have an extension")
	}

	isGZipped := false
	if ext == ".gz" {
		isGZipped = true
		ext = filepath.Ext(strings.TrimSuffix(filePath, ext))
		if ext == "" {
			return nil, errors.New("invalid filename: Gzipped file doesn't have an extension")
		}
	}

	var isBinProto bool
	switch ext {
	case ".binproto":
		isBinProto = true
	case ".textproto":
		isBinProto = false
	default:
		return nil, errors.New("invalid filename: not a .textproto or .binproto")
	}

	return &fileType{isGZipped: isGZipped, isBinProto: isBinProto}, nil
}

// ValidExtension returns an error if the file extension is not a proto file.
func ValidExtension(path string) error {
	_, err := typeForPath(path)
	return err
}

// Write writes a proto message to a .textproto or .binproto file, based on the file extension.
// If the file name additionally has the .gz suffix, it's zipped before writing.
func Write(filePath string, outputProto proto.Message) error {
	ft, err := typeForPath(filePath)
	if err != nil {
		return err
	}
	return write(filePath, outputProto, ft)
}

// WriteWithFormat writes a proto message to a .textproto or .binproto file, based
// on the value of the format parameter ("textproto" or "binproto")
func WriteWithFormat(filePath string, outputProto proto.Message, format string) error {
	ft := &fileType{isGZipped: false, isBinProto: format == "binproto"}
	return write(filePath, outputProto, ft)
}

func write(filePath string, outputProto proto.Message, ft *fileType) error {
	var p []byte
	var err error
	if ft.isBinProto {
		if p, err = proto.Marshal(outputProto); err != nil {
			return err
		}
	} else {
		opts := prototext.MarshalOptions{Multiline: true}
		if p, err = (opts.Marshal(outputProto)); err != nil {
			return err
		}
	}

	log.Infof("Marshaled result proto has %d bytes", len(p))

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if ft.isGZipped {
		writer := gzip.NewWriter(f)
		if _, err := writer.Write(p); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
	} else if _, err := f.Write(p); err != nil {
		return err
	}
	return nil
}

// ScanResultToProto converts a ScanResult go struct into the equivalent proto.
func ScanResultToProto(r *scalibr.ScanResult) (*spb.ScanResult, error) {
	pluginStatus := make([]*spb.PluginStatus, 0, len(r.PluginStatus))
	for _, s := range r.PluginStatus {
		pluginStatus = append(pluginStatus, pluginStatusToProto(s))
	}

	packages := make([]*spb.Package, 0, len(r.Inventory.Packages))
	for _, p := range r.Inventory.Packages {
		p := packageToProto(p)
		packages = append(packages, p)
	}

	findings := make([]*spb.Finding, 0, len(r.Inventory.Findings))
	for _, f := range r.Inventory.Findings {
		p, err := findingToProto(f)
		if err != nil {
			return nil, err
		}
		findings = append(findings, p)
	}

	return &spb.ScanResult{
		Version:      r.Version,
		StartTime:    timestamppb.New(r.StartTime),
		EndTime:      timestamppb.New(r.EndTime),
		Status:       scanStatusToProto(r.Status),
		PluginStatus: pluginStatus,
		// TODO(b/400910349): Stop setting the deprecated fields
		// once integrators no longer read them.
		InventoriesDeprecated: packages,
		FindingsDeprecated:    findings,
		Inventory: &spb.Inventory{
			Packages: packages,
			Findings: findings,
		},
	}, nil
}

func scanStatusToProto(s *plugin.ScanStatus) *spb.ScanStatus {
	var e spb.ScanStatus_ScanStatusEnum
	switch s.Status {
	case plugin.ScanStatusSucceeded:
		e = spb.ScanStatus_SUCCEEDED
	case plugin.ScanStatusPartiallySucceeded:
		e = spb.ScanStatus_PARTIALLY_SUCCEEDED
	case plugin.ScanStatusFailed:
		e = spb.ScanStatus_FAILED
	default:
		e = spb.ScanStatus_UNSPECIFIED
	}
	return &spb.ScanStatus{Status: e, FailureReason: s.FailureReason}
}

func pluginStatusToProto(s *plugin.Status) *spb.PluginStatus {
	return &spb.PluginStatus{
		Name:    s.Name,
		Version: int32(s.Version),
		Status:  scanStatusToProto(s.Status),
	}
}

func packageToProto(pkg *extractor.Package) *spb.Package {
	if pkg == nil {
		return nil
	}
	p := converter.ToPURL(pkg)
	packageProto := &spb.Package{
		Name:         pkg.Name,
		Version:      pkg.Version,
		SourceCode:   sourceCodeIdentifierToProto(pkg.SourceCode),
		Purl:         purlToProto(p),
		Ecosystem:    pkg.Ecosystem(),
		Locations:    pkg.Locations,
		Extractor:    pkg.Extractor.Name(),
		Annotations:  annotationsToProto(pkg.Annotations),
		LayerDetails: layerDetailsToProto(pkg.LayerDetails),
	}
	setProtoMetadata(pkg.Metadata, packageProto)
	return packageProto
}

func setProtoMetadata(meta any, p *spb.Package) {
	switch m := meta.(type) {
	case *wheelegg.PythonPackageMetadata:
		p.Metadata = &spb.Package_PythonMetadata{
			PythonMetadata: &spb.PythonPackageMetadata{
				Author:      m.Author,
				AuthorEmail: m.AuthorEmail,
			},
		}
	case *packagejson.JavascriptPackageJSONMetadata:
		p.Metadata = &spb.Package_JavascriptMetadata{
			JavascriptMetadata: &spb.JavascriptPackageJSONMetadata{
				Author:       m.Author.PersonString(),
				Contributors: personsToProto(m.Contributors),
				Maintainers:  personsToProto(m.Maintainers),
			},
		}
	case *depsjson.Metadata:
		p.Metadata = &spb.Package_DepsjsonMetadata{
			DepsjsonMetadata: &spb.DEPSJSONMetadata{
				PackageName:    m.PackageName,
				PackageVersion: m.PackageVersion,
				Type:           m.Type,
			},
		}
	case *apkmeta.Metadata:
		p.Metadata = &spb.Package_ApkMetadata{
			ApkMetadata: &spb.APKPackageMetadata{
				PackageName:  m.PackageName,
				OriginName:   m.OriginName,
				OsId:         m.OSID,
				OsVersionId:  m.OSVersionID,
				Maintainer:   m.Maintainer,
				Architecture: m.Architecture,
				License:      m.License,
			},
		}
	case *dpkgmeta.Metadata:
		p.Metadata = &spb.Package_DpkgMetadata{
			DpkgMetadata: &spb.DPKGPackageMetadata{
				PackageName:       m.PackageName,
				SourceName:        m.SourceName,
				Status:            m.Status,
				SourceVersion:     m.SourceVersion,
				PackageVersion:    m.PackageVersion,
				OsId:              m.OSID,
				OsVersionCodename: m.OSVersionCodename,
				OsVersionId:       m.OSVersionID,
				Maintainer:        m.Maintainer,
				Architecture:      m.Architecture,
			},
		}
	case *snapmeta.Metadata:
		p.Metadata = &spb.Package_SnapMetadata{
			SnapMetadata: &spb.SNAPPackageMetadata{
				Name:              m.Name,
				Version:           m.Version,
				Grade:             m.Grade,
				Type:              m.Type,
				Architectures:     m.Architectures,
				OsId:              m.OSID,
				OsVersionCodename: m.OSVersionCodename,
				OsVersionId:       m.OSVersionID,
			},
		}
	case *rpmmeta.Metadata:
		p.Metadata = &spb.Package_RpmMetadata{
			RpmMetadata: &spb.RPMPackageMetadata{
				PackageName:  m.PackageName,
				SourceRpm:    m.SourceRPM,
				Epoch:        int32(m.Epoch),
				OsName:       m.OSName,
				OsId:         m.OSID,
				OsVersionId:  m.OSVersionID,
				OsBuildId:    m.OSBuildID,
				Vendor:       m.Vendor,
				Architecture: m.Architecture,
				License:      m.License,
			},
		}
	case *cosmeta.Metadata:
		p.Metadata = &spb.Package_CosMetadata{
			CosMetadata: &spb.COSPackageMetadata{
				Name:        m.Name,
				Version:     m.Version,
				Category:    m.Category,
				OsVersion:   m.OSVersion,
				OsVersionId: m.OSVersionID,
			},
		}
	case *pacmanmeta.Metadata:
		p.Metadata = &spb.Package_PacmanMetadata{
			PacmanMetadata: &spb.PACMANPackageMetadata{
				PackageName:         m.PackageName,
				PackageVersion:      m.PackageVersion,
				OsId:                m.OSID,
				OsVersionId:         m.OSVersionID,
				PackageDependencies: m.PackageDependencies,
			},
		}
	case *portagemeta.Metadata:
		p.Metadata = &spb.Package_PortageMetadata{
			PortageMetadata: &spb.PortagePackageMetadata{
				PackageName:    m.PackageName,
				PackageVersion: m.PackageVersion,
				OsId:           m.OSID,
				OsVersionId:    m.OSVersionID,
			},
		}
	case *flatpakmeta.Metadata:
		p.Metadata = &spb.Package_FlatpakMetadata{
			FlatpakMetadata: &spb.FlatpakPackageMetadata{
				PackageName:    m.PackageName,
				PackageId:      m.PackageID,
				PackageVersion: m.PackageVersion,
				ReleaseDate:    m.ReleaseDate,
				OsName:         m.OSName,
				OsId:           m.OSID,
				OsVersionId:    m.OSVersionID,
				OsBuildId:      m.OSBuildID,
				Developer:      m.Developer,
			},
		}
	case *nixmeta.Metadata:
		p.Metadata = &spb.Package_NixMetadata{
			NixMetadata: &spb.NixPackageMetadata{
				PackageName:       m.PackageName,
				PackageVersion:    m.PackageVersion,
				PackageHash:       m.PackageHash,
				PackageOutput:     m.PackageOutput,
				OsId:              m.OSID,
				OsVersionCodename: m.OSVersionCodename,
				OsVersionId:       m.OSVersionID,
			},
		}
	case *macapps.Metadata:
		p.Metadata = &spb.Package_MacAppsMetadata{
			MacAppsMetadata: &spb.MacAppsMetadata{
				BundleDisplayName:        m.CFBundleDisplayName,
				BundleIdentifier:         m.CFBundleIdentifier,
				BundleShortVersionString: m.CFBundleShortVersionString,
				BundleExecutable:         m.CFBundleExecutable,
				BundleName:               m.CFBundleName,
				BundlePackageType:        m.CFBundlePackageType,
				BundleSignature:          m.CFBundleSignature,
				BundleVersion:            m.CFBundleVersion,
				ProductId:                m.KSProductID,
				UpdateUrl:                m.KSUpdateURL,
			},
		}
	case *homebrew.Metadata:
		p.Metadata = &spb.Package_HomebrewMetadata{
			HomebrewMetadata: &spb.HomebrewPackageMetadata{},
		}
	case *modulemeta.Metadata:
		p.Metadata = &spb.Package_KernelModuleMetadata{
			KernelModuleMetadata: &spb.KernelModuleMetadata{
				PackageName:                    m.PackageName,
				PackageVersion:                 m.PackageVersion,
				PackageVermagic:                m.PackageVermagic,
				PackageSourceVersionIdentifier: m.PackageSourceVersionIdentifier,
				OsId:                           m.OSID,
				OsVersionCodename:              m.OSVersionCodename,
				OsVersionId:                    m.OSVersionID,
				PackageAuthor:                  m.PackageAuthor},
		}
	case *vmlinuzmeta.Metadata:
		p.Metadata = &spb.Package_VmlinuzMetadata{
			VmlinuzMetadata: &spb.VmlinuzMetadata{
				Name:              m.Name,
				Version:           m.Version,
				Architecture:      m.Architecture,
				ExtendedVersion:   m.ExtendedVersion,
				Format:            m.Format,
				SwapDevice:        m.SwapDevice,
				RootDevice:        m.RootDevice,
				VideoMode:         m.VideoMode,
				OsId:              m.OSID,
				OsVersionCodename: m.OSVersionCodename,
				OsVersionId:       m.OSVersionID,
				RwRootFs:          m.RWRootFS,
			},
		}
	case *ctrdfs.Metadata:
		p.Metadata = &spb.Package_ContainerdContainerMetadata{
			ContainerdContainerMetadata: &spb.ContainerdContainerMetadata{
				NamespaceName: m.Namespace,
				ImageName:     m.ImageName,
				ImageDigest:   m.ImageDigest,
				Runtime:       m.Runtime,
				Id:            m.ID,
				PodName:       m.PodName,
				PodNamespace:  m.PodNamespace,
				Pid:           int32(m.PID),
				Snapshotter:   m.Snapshotter,
				SnapshotKey:   m.SnapshotKey,
				LowerDir:      m.LowerDir,
				UpperDir:      m.UpperDir,
				WorkDir:       m.WorkDir,
			},
		}
	case *ctrdruntime.Metadata:
		p.Metadata = &spb.Package_ContainerdRuntimeContainerMetadata{
			ContainerdRuntimeContainerMetadata: &spb.ContainerdRuntimeContainerMetadata{
				NamespaceName: m.Namespace,
				ImageName:     m.ImageName,
				ImageDigest:   m.ImageDigest,
				Runtime:       m.Runtime,
				Id:            m.ID,
				Pid:           int32(m.PID),
				RootfsPath:    m.RootFS,
			},
		}
	case *spdxmeta.Metadata:
		p.Metadata = &spb.Package_SpdxMetadata{
			SpdxMetadata: &spb.SPDXPackageMetadata{
				Purl: purlToProto(m.PURL),
				Cpes: m.CPEs,
			},
		}
	case *cdxmeta.Metadata:
		p.Metadata = &spb.Package_CdxMetadata{
			CdxMetadata: &spb.CDXPackageMetadata{
				Purl: purlToProto(m.PURL),
				Cpes: m.CPEs,
			},
		}
	case *archivemeta.Metadata:
		p.Metadata = &spb.Package_JavaArchiveMetadata{
			JavaArchiveMetadata: &spb.JavaArchiveMetadata{
				ArtifactId: m.ArtifactID,
				GroupId:    m.GroupID,
				Sha1:       m.SHA1,
			},
		}
	case *javalockfile.Metadata:
		p.Metadata = &spb.Package_JavaLockfileMetadata{
			JavaLockfileMetadata: &spb.JavaLockfileMetadata{
				ArtifactId:   m.ArtifactID,
				GroupId:      m.GroupID,
				IsTransitive: m.IsTransitive,
			},
		}
	case *osv.Metadata:
		p.Metadata = &spb.Package_OsvMetadata{
			OsvMetadata: &spb.OSVPackageMetadata{
				PurlType:  m.PURLType,
				Commit:    m.Commit,
				Ecosystem: m.Ecosystem,
				CompareAs: m.CompareAs,
			},
		}
	case *requirements.Metadata:
		p.Metadata = &spb.Package_PythonRequirementsMetadata{
			PythonRequirementsMetadata: &spb.PythonRequirementsMetadata{
				HashCheckingModeValues: m.HashCheckingModeValues,
				VersionComparator:      m.VersionComparator,
				Requirement:            m.Requirement,
			},
		}
	case *setup.Metadata:
		p.Metadata = &spb.Package_PythonSetupMetadata{
			PythonSetupMetadata: &spb.PythonSetupMetadata{
				VersionComparator: m.VersionComparator,
			},
		}
	case *winmetadata.OSVersion:
		p.Metadata = &spb.Package_WindowsOsVersionMetadata{
			WindowsOsVersionMetadata: &spb.WindowsOSVersion{
				Product:     m.Product,
				FullVersion: m.FullVersion,
			},
		}
	case *chromeextensions.Metadata:
		p.Metadata = &spb.Package_ChromeExtensionsMetadata{
			ChromeExtensionsMetadata: &spb.ChromeExtensionsMetadata{
				Name:                 m.Name,
				Description:          m.Description,
				AuthorEmail:          m.AuthorEmail,
				HostPermissions:      m.HostPermissions,
				ManifestVersion:      int32(m.ManifestVersion),
				MinimumChromeVersion: m.MinimumChromeVersion,
				Permissions:          m.Permissions,
				UpdateUrl:            m.UpdateURL,
			},
		}
	case *vscodeextensions.Metadata:
		p.Metadata = &spb.Package_VscodeExtensionsMetadata{
			VscodeExtensionsMetadata: &spb.VSCodeExtensionsMetadata{
				Id:                   m.ID,
				PublisherId:          m.PublisherID,
				PublisherDisplayName: m.PublisherDisplayName,
				TargetPlatform:       m.TargetPlatform,
				Updated:              m.Updated,
				IsPreReleaseVersion:  m.IsPreReleaseVersion,
				InstalledTimestamp:   m.InstalledTimestamp,
			},
		}
	case *podman.Metadata:
		exposedPorts := map[uint32]*spb.Protocol{}
		for p, protocols := range m.ExposedPorts {
			exposedPorts[uint32(p)] = &spb.Protocol{Names: protocols}
		}
		p.Metadata = &spb.Package_PodmanMetadata{
			PodmanMetadata: &spb.PodmanMetadata{
				ExposedPorts:  exposedPorts,
				Pid:           int32(m.PID),
				NamespaceName: m.NameSpace,
				StartedTime:   timestamppb.New(m.StartedTime),
				FinishedTime:  timestamppb.New(m.FinishedTime),
				Status:        m.Status,
				ExitCode:      m.ExitCode,
				Exited:        m.Exited,
			},
		}
	case *docker.Metadata:
		ports := make([]*spb.DockerPort, 0, len(m.Ports))
		for _, p := range m.Ports {
			ports = append(ports, &spb.DockerPort{
				Ip:          p.IP,
				PrivatePort: uint32(p.PrivatePort),
				PublicPort:  uint32(p.PublicPort),
				Type:        p.Type,
			})
		}
		p.Metadata = &spb.Package_DockerContainersMetadata{
			DockerContainersMetadata: &spb.DockerContainersMetadata{
				ImageName:   m.ImageName,
				ImageDigest: m.ImageDigest,
				Id:          m.ID,
				Ports:       ports,
			},
		}
	}
}

func personsToProto(persons []*packagejson.Person) []string {
	var personStrings []string
	for _, p := range persons {
		personStrings = append(personStrings, p.PersonString())
	}
	return personStrings
}

func purlToProto(p *purl.PackageURL) *spb.Purl {
	if p == nil {
		return nil
	}
	return &spb.Purl{
		Purl:       p.String(),
		Type:       p.Type,
		Namespace:  p.Namespace,
		Name:       p.Name,
		Version:    p.Version,
		Qualifiers: qualifiersToProto(p.Qualifiers),
		Subpath:    p.Subpath,
	}
}

func annotationsToProto(as []extractor.Annotation) []spb.Package_AnnotationEnum {
	if as == nil {
		return nil
	}
	ps := []spb.Package_AnnotationEnum{}
	for _, a := range as {
		ps = append(ps, annotationToProto(a))
	}
	return ps
}

func annotationToProto(s extractor.Annotation) spb.Package_AnnotationEnum {
	var e spb.Package_AnnotationEnum
	switch s {
	case extractor.Transitional:
		e = spb.Package_TRANSITIONAL
	case extractor.InsideOSPackage:
		e = spb.Package_INSIDE_OS_PACKAGE
	case extractor.InsideCacheDir:
		e = spb.Package_INSIDE_CACHE_DIR
	default:
		e = spb.Package_UNSPECIFIED
	}
	return e
}

func layerDetailsToProto(ld *extractor.LayerDetails) *spb.LayerDetails {
	if ld == nil {
		return nil
	}
	return &spb.LayerDetails{
		Index:       int32(ld.Index),
		DiffId:      ld.DiffID,
		ChainId:     ld.ChainID,
		Command:     ld.Command,
		InBaseImage: ld.InBaseImage,
	}
}

func sourceCodeIdentifierToProto(s *extractor.SourceCodeIdentifier) *spb.SourceCodeIdentifier {
	if s == nil {
		return nil
	}
	return &spb.SourceCodeIdentifier{
		Repo:   s.Repo,
		Commit: s.Commit,
	}
}

func qualifiersToProto(qs purl.Qualifiers) []*spb.Qualifier {
	result := make([]*spb.Qualifier, 0, len(qs))
	for _, q := range qs {
		result = append(result, &spb.Qualifier{Key: q.Key, Value: q.Value})
	}
	return result
}

// ErrAdvisoryMissing will be returned if the Advisory is not set on a finding.
var ErrAdvisoryMissing = errors.New("advisory missing in finding")

// ErrAdvisoryIDMissing will be returned if the Advisory ID is not set on a finding.
var ErrAdvisoryIDMissing = errors.New("advisory ID missing in finding")

func findingToProto(f *detector.Finding) (*spb.Finding, error) {
	if f.Adv == nil {
		return nil, ErrAdvisoryMissing
	}
	var target *spb.TargetDetails
	if f.Target != nil {
		p := packageToProto(f.Target.Package)
		target = &spb.TargetDetails{
			Location: f.Target.Location,
			Package:  p,
		}
	}
	if f.Adv.ID == nil {
		return nil, ErrAdvisoryIDMissing
	}
	return &spb.Finding{
		Adv: &spb.Advisory{
			Id: &spb.AdvisoryId{
				Publisher: f.Adv.ID.Publisher,
				Reference: f.Adv.ID.Reference,
			},
			Type:           typeEnumToProto(f.Adv.Type),
			Title:          f.Adv.Title,
			Description:    f.Adv.Description,
			Recommendation: f.Adv.Recommendation,
			Sev:            severityToProto(f.Adv.Sev),
		},
		Target: target,
		Extra:  f.Extra,
	}, nil
}

func typeEnumToProto(e detector.TypeEnum) spb.Advisory_TypeEnum {
	switch e {
	case detector.TypeVulnerability:
		return spb.Advisory_VULNERABILITY
	case detector.TypeCISFinding:
		return spb.Advisory_CIS_FINDING
	default:
		return spb.Advisory_UNKNOWN
	}
}

func severityToProto(s *detector.Severity) *spb.Severity {
	r := &spb.Severity{}
	switch s.Severity {
	case detector.SeverityMinimal:
		r.Severity = spb.Severity_MINIMAL
	case detector.SeverityLow:
		r.Severity = spb.Severity_LOW
	case detector.SeverityMedium:
		r.Severity = spb.Severity_MEDIUM
	case detector.SeverityHigh:
		r.Severity = spb.Severity_HIGH
	case detector.SeverityCritical:
		r.Severity = spb.Severity_CRITICAL
	default:
		r.Severity = spb.Severity_UNSPECIFIED
	}
	if s.CVSSV2 != nil {
		r.CvssV2 = cvssToProto(s.CVSSV2)
	}
	if s.CVSSV3 != nil {
		r.CvssV3 = cvssToProto(s.CVSSV3)
	}
	return r
}

func cvssToProto(c *detector.CVSS) *spb.CVSS {
	return &spb.CVSS{
		BaseScore:          c.BaseScore,
		TemporalScore:      c.TemporalScore,
		EnvironmentalScore: c.EnvironmentalScore,
	}
}
