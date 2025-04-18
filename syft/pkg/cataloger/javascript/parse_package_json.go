package javascript

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/mitchellh/mapstructure"

	"github.com/anchore/syft/internal"
	"github.com/anchore/syft/internal/log"
	"github.com/anchore/syft/syft/artifact"
	"github.com/anchore/syft/syft/file"
	"github.com/anchore/syft/syft/pkg"
	"github.com/anchore/syft/syft/pkg/cataloger/generic"
)

// integrity check
var _ generic.Parser = parsePackageJSON

type packageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Author               author            `json:"author"`
	License              json.RawMessage   `json:"license"`
	Licenses             json.RawMessage   `json:"licenses"`
	Homepage             string            `json:"homepage"`
	Private              bool              `json:"private"`
	Description          string            `json:"description"`
	Develop              bool              `json:"dev"` // lock v3
	Repository           repository        `json:"repository"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	PeerDependenciesMeta map[string]struct {
		Optional bool `json:"optional"`
	} `json:"peerDependenciesMeta"`
	File string `json:"-"`
}

type author struct {
	Name  string `json:"name" mapstruct:"name"`
	Email string `json:"email" mapstruct:"email"`
	URL   string `json:"url" mapstruct:"url"`
}

type repository struct {
	Type string `json:"type" mapstructure:"type"`
	URL  string `json:"url" mapstructure:"url"`
}

// match example: "author": "Isaac Z. Schlueter <i@izs.me> (http://blog.izs.me)"
// ---> name: "Isaac Z. Schlueter" email: "i@izs.me" url: "http://blog.izs.me"
var authorPattern = regexp.MustCompile(`^\s*(?P<name>[^<(]*)(\s+<(?P<email>.*)>)?(\s\((?P<url>.*)\))?\s*$`)

func parsePackageJSON(resolver file.Resolver, e *generic.Environment, reader file.LocationReadCloser) ([]pkg.Package, []artifact.Relationship, error) {
	pkgjson, err := parsePackageJSONFile(resolver, e, reader)
	if err != nil {
		return nil, nil, err
	}

	if !pkgjson.hasNameAndVersionValues() {
		log.Debugf("encountered package.json file without a name and/or version field, ignoring (path=%q)", reader.Location.AccessPath())
		return nil, nil, nil
	}

	rootPkg := newPackageJSONRootPackage(*pkgjson, reader.Location)
	return []pkg.Package{rootPkg}, nil, nil
}

// parsePackageJSON parses a package.json and returns the discovered JavaScript packages.
func parsePackageJSONFile(_ file.Resolver, _ *generic.Environment, reader file.LocationReadCloser) (*packageJSON, error) {
	var js *packageJSON
	decoder := json.NewDecoder(reader)
	err := decoder.Decode(&js)
	if err != nil {
		return nil, err
	}

	return js, nil
}

func (a *author) UnmarshalJSON(b []byte) error {
	var authorStr string
	var fields map[string]string
	var auth author

	if err := json.Unmarshal(b, &authorStr); err != nil {
		// string parsing did not work, assume a map was given
		// for more information: https://docs.npmjs.com/files/package.json#people-fields-author-contributors
		if err := json.Unmarshal(b, &fields); err != nil {
			return fmt.Errorf("unable to parse package.json author: %w", err)
		}
	} else {
		// parse out "name <email> (url)" into an author struct
		fields = internal.MatchNamedCaptureGroups(authorPattern, authorStr)
	}

	// translate the map into a structure
	if err := mapstructure.Decode(fields, &auth); err != nil {
		return fmt.Errorf("unable to decode package.json author: %w", err)
	}

	*a = auth

	return nil
}

func (a *author) AuthorString() string {
	result := a.Name
	if a.Email != "" {
		result += fmt.Sprintf(" <%s>", a.Email)
	}
	if a.URL != "" {
		result += fmt.Sprintf(" (%s)", a.URL)
	}
	return result
}

func (r *repository) UnmarshalJSON(b []byte) error {
	var repositoryStr string
	var fields map[string]string
	var repo repository

	if err := json.Unmarshal(b, &repositoryStr); err != nil {
		// string parsing did not work, assume a map was given
		// for more information: https://docs.npmjs.com/files/package.json#people-fields-author-contributors
		if err := json.Unmarshal(b, &fields); err != nil {
			return fmt.Errorf("unable to parse package.json author: %w", err)
		}
		// translate the map into a structure
		if err := mapstructure.Decode(fields, &repo); err != nil {
			return fmt.Errorf("unable to decode package.json author: %w", err)
		}

		*r = repo
	} else {
		r.URL = repositoryStr
	}

	return nil
}

type npmPackageLicense struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func licenseFromJSON(b []byte) (string, error) {
	// first try as string
	var licenseString string
	err := json.Unmarshal(b, &licenseString)
	if err == nil {
		return licenseString, nil
	}

	// then try as object (this format is deprecated)
	var licenseObject npmPackageLicense
	err = json.Unmarshal(b, &licenseObject)
	if err == nil {
		return licenseObject.Type, nil
	}

	return "", errors.New("unable to unmarshal license field as either string or object")
}

func (p packageJSON) licensesFromJSON() ([]string, error) {
	if p.License == nil && p.Licenses == nil {
		// This package.json doesn't specify any licenses whatsoever
		return []string{}, nil
	}

	singleLicense, err := licenseFromJSON(p.License)
	if err == nil {
		return []string{singleLicense}, nil
	}

	multiLicense, err := licensesFromJSON(p.Licenses)

	// The "licenses" field is deprecated. It should be inspected as a last resort.
	if multiLicense != nil && err == nil {
		mapLicenses := func(licenses []npmPackageLicense) []string {
			mappedLicenses := make([]string, len(licenses))
			for i, l := range licenses {
				mappedLicenses[i] = l.Type
			}
			return mappedLicenses
		}

		return mapLicenses(multiLicense), nil
	}

	return nil, err
}

func licensesFromJSON(b []byte) ([]npmPackageLicense, error) {
	var licenseObject []npmPackageLicense
	err := json.Unmarshal(b, &licenseObject)
	if err == nil {
		return licenseObject, nil
	}

	return nil, errors.New("unmarshal failed")
}

func (p packageJSON) hasNameAndVersionValues() bool {
	return p.Name != "" && p.Version != ""
}

// this supports both windows and unix paths
var filepathSeparator = regexp.MustCompile(`[\\/]`)

func pathContainsNodeModulesDirectory(p string) bool {
	for _, subPath := range filepathSeparator.Split(p, -1) {
		if subPath == "node_modules" {
			return true
		}
	}
	return false
}
