package version

import (
	"strings"

	"github.com/Masterminds/semver"
)

func Constraint(constraint, version string) (bool, error) {
	// Drop everything after first dash.
	// Version with dash is considered a pre-release
	// but some MongoDB builds add additional information after dash
	// even though it's not considered a pre-release but a release.
	s := strings.SplitN(version, "-", 2)
	version = s[0]

	// Create new version
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, err
	}

	// Check if version matches constraint
	constraints, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, err
	}
	return constraints.Check(v), nil
}
