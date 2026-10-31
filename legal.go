// Package vogter holds the licence texts, which go:embed can only reach from
// the module root. A bare binary carries them, so it can be shared alone.
package vogter

import _ "embed"

// LicenceFiles are the texts `vaegter licences` prints, in order.
var LicenceFiles = []string{license, notice, thirdParty}

var (
	//go:embed LICENSE
	license string
	//go:embed NOTICE
	notice string
	//go:embed THIRD_PARTY_NOTICES.txt
	thirdParty string
)
