package integration

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anchore/syft/internal"
	"github.com/anchore/syft/syft/pkg"
)

func TestNpmPackageLockDirectory(t *testing.T) {
	sbom, _ := catalogDirectory(t, "test-fixtures/npm-lock")

	foundPackages := internal.NewStringSet()
	// root pkg
	foundPackages.Add("npm-lock")

	for actualPkg := range sbom.Artifacts.Packages.Enumerate(pkg.NpmPkg) {
		for _, actualLocation := range actualPkg.Locations.ToSlice() {
			if strings.Contains(actualLocation.RealPath, "node_modules") {
				t.Errorf("found packages from package-lock.json in node_modules: %s", actualLocation)
			}
		}
		foundPackages.Add(actualPkg.Name)
	}

	// ensure that integration test commonTestCases stay in sync with the available catalogers
	const expectedPackageCount = 7
	if len(foundPackages) != expectedPackageCount {
		t.Errorf("found the wrong set of npm package-lock.json packages (expected: %d, actual: %d)", expectedPackageCount, len(foundPackages))
	}
}

func TestYarnPackageLockDirectory(t *testing.T) {
	sbom, _ := catalogDirectory(t, "test-fixtures/yarn-lock")

	foundPackages := internal.NewStringSet()
	expectedPackages := internal.NewStringSet("async@0.9.2", "async@3.2.3", "merge-objects@1.0.5", "should-type@1.3.0", "@4lolo/resize-observer-polyfill@1.5.2", "yarn-lock@1.0.0")

	for actualPkg := range sbom.Artifacts.Packages.Enumerate(pkg.NpmPkg) {
		for _, actualLocation := range actualPkg.Locations.ToSlice() {
			if strings.Contains(actualLocation.RealPath, "node_modules") {
				t.Errorf("found packages from yarn.lock in node_modules: %s", actualLocation)
			}
		}
		foundPackages.Add(actualPkg.Name + "@" + actualPkg.Version)
	}

	// ensure that integration test commonTestCases stay in sync with the available catalogers
	if len(foundPackages) != len(expectedPackages) {
		t.Errorf("found the wrong set of yarn.lock packages (expected: %d, actual: %d)", len(expectedPackages), len(foundPackages))
	} else if !reflect.DeepEqual(foundPackages, expectedPackages) {
		t.Errorf("found the wrong set of yarn.lock packages (expected: %+q, actual: %+q)", expectedPackages.ToSlice(), foundPackages.ToSlice())
	}
}
