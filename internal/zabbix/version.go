package zabbix

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a Zabbix API version as reported by apiinfo.version.
type Version struct {
	Major, Minor, Patch int
	Raw                 string
}

// ParseVersion reads "7.0.12" style versions; anything after the patch
// number (such as "rc3" in "8.0.0rc3") is kept in Raw only.
func ParseVersion(s string) (Version, error) {
	v := Version{Raw: s}
	parts := strings.SplitN(strings.TrimSpace(s), ".", 3)
	if len(parts) < 2 {
		return v, fmt.Errorf("unrecognised Zabbix version %q", s)
	}
	nums := []*int{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		end := strings.IndexFunc(p, func(r rune) bool { return r < '0' || r > '9' })
		if end == 0 {
			return v, fmt.Errorf("unrecognised Zabbix version %q", s)
		}
		if end > 0 {
			p = p[:end]
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, fmt.Errorf("unrecognised Zabbix version %q", s)
		}
		*nums[i] = n
	}
	return v, nil
}

func (v Version) String() string { return v.Raw }

// AtLeast reports whether v is major.minor or newer.
func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

// Support describes how well Vogter supports a server version.
type Support int

const (
	// Supported is 7.0 LTS or newer, the versions Vogter is checked against.
	Supported Support = iota
	// Untested is 6.4: the API has every feature Vogter uses, but nobody
	// has run Vogter against it.
	Untested
	// Unsupported is older than 6.4, which lacks the Bearer header,
	// selectHostGroups and active_available.
	Unsupported
)

// SupportFor classifies v.
func SupportFor(v Version) Support {
	switch {
	case v.AtLeast(7, 0):
		return Supported
	case v.AtLeast(6, 4):
		return Untested
	}
	return Unsupported
}

// Describe returns a one-line note for versions that are not fully
// supported, and "" for supported ones.
func (s Support) Describe(v Version) string {
	switch s {
	case Untested:
		return fmt.Sprintf("Zabbix %s is untested; Vogter targets 7.0 LTS and newer", v)
	case Unsupported:
		return fmt.Sprintf("Zabbix %s is not supported: Vogter needs 6.4 or newer (7.0 LTS recommended)", v)
	}
	return ""
}
