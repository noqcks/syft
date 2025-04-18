package dotnet

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/anchore/syft/internal/log"
	"github.com/anchore/syft/syft/artifact"
	"github.com/anchore/syft/syft/file"
	"github.com/anchore/syft/syft/pkg"
	"github.com/anchore/syft/syft/pkg/cataloger/generic"
)

var _ generic.Parser = parseDotnetDeps

type dotnetRuntimeTarget struct {
	Name string `json:"name"`
}

type dotnetDepsTarget struct {
	Dependencies map[string]string   `json:"dependencies"`
	Runtime      map[string]struct{} `json:"runtime"`
}
type dotnetDeps struct {
	RuntimeTarget dotnetRuntimeTarget                    `json:"runtimeTarget"`
	Targets       map[string]map[string]dotnetDepsTarget `json:"targets"`
	Libraries     map[string]dotnetDepsLibrary           `json:"libraries"`
}

type dotnetDepsLibrary struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Sha512   string `json:"sha512"`
	HashPath string `json:"hashPath"`
}

//nolint:funlen
func parseDotnetDeps(_ file.Resolver, _ *generic.Environment, reader file.LocationReadCloser) ([]pkg.Package, []artifact.Relationship, error) {
	var pkgs []pkg.Package
	var pkgMap = make(map[string]pkg.Package)
	var relationships []artifact.Relationship

	dec := json.NewDecoder(reader)

	var p dotnetDeps
	if err := dec.Decode(&p); err != nil {
		return nil, nil, fmt.Errorf("failed to parse deps.json file: %w", err)
	}

	rootName := getDepsJSONFilePrefix(reader.AccessPath())
	if rootName == "" {
		return nil, nil, fmt.Errorf("unable to determine root package name from deps.json file: %s", reader.AccessPath())
	}
	var rootpkg *pkg.Package
	for nameVersion, lib := range p.Libraries {
		name, _ := extractNameAndVersion(nameVersion)
		if lib.Type == "project" && name == rootName {
			rootpkg = newDotnetDepsPackage(
				nameVersion,
				lib,
				pkg.ComponentTypeApplication,
				reader.Location.WithAnnotation(pkg.EvidenceAnnotationKey, pkg.PrimaryEvidenceAnnotation),
			)
		}
	}
	if rootpkg == nil {
		return nil, nil, fmt.Errorf("unable to determine root package from deps.json file: %s", reader.AccessPath())
	}
	pkgs = append(pkgs, *rootpkg)
	pkgMap[createNameAndVersion(rootpkg.Name, rootpkg.Version)] = *rootpkg

	var names []string
	for nameVersion := range p.Libraries {
		names = append(names, nameVersion)
	}
	// sort the names so that the order of the packages is deterministic
	sort.Strings(names)

	for _, nameVersion := range names {
		// skip the root package
		name, version := extractNameAndVersion(nameVersion)
		if name == rootpkg.Name && version == rootpkg.Version {
			continue
		}

		lib := p.Libraries[nameVersion]
		dotnetPkg := newDotnetDepsPackage(
			nameVersion,
			lib,
			pkg.ComponentTypeLibrary,
			reader.Location.WithAnnotation(pkg.EvidenceAnnotationKey, pkg.PrimaryEvidenceAnnotation),
		)

		if dotnetPkg != nil {
			pkgs = append(pkgs, *dotnetPkg)
			pkgMap[nameVersion] = *dotnetPkg
		}
	}

	for pkgNameVersion, target := range p.Targets[p.RuntimeTarget.Name] {
		for depName, depVersion := range target.Dependencies {
			depNameVersion := createNameAndVersion(depName, depVersion)
			depPkg, ok := pkgMap[depNameVersion]
			if !ok {
				log.Debug("unable to find package in map", depNameVersion)
				continue
			}
			pkg, ok := pkgMap[pkgNameVersion]
			if !ok {
				log.Debug("unable to find package in map", pkgNameVersion)
				continue
			}
			rel := artifact.Relationship{
				From: depPkg,
				To:   pkg,
				Type: artifact.DependencyOfRelationship,
			}
			relationships = append(relationships, rel)
		}
	}

	pkg.Sort(pkgs)
	pkg.SortRelationships(relationships)
	return pkgs, relationships, nil
}
