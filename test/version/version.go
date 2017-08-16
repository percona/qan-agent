package version

import (
	"github.com/Masterminds/semver"
)

func Constraint(constraint, version string) (bool, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, err
	}

	constraints, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, err
	}
	return constraints.Check(v), nil

}
