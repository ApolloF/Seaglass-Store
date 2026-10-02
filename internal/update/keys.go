package update

// ReleaseKeys verify release signatures (see signature.go). The Store
// Edition has its own key, apart from Seaglass's: it's on the
// maintainer's PC (tools/release) and, as the maintainer chose, in the
// repository's Actions secrets so CI signs tagged releases. To rotate, add
// the new key here in a release signed with the old one, and remove the
// old one a release later.
var ReleaseKeys = mustKeys(
	"GLXHtZaV3EAdcU+AjdHl0DHuk4Ctem90Dmlq7HRIyBs=", // made 2026-10-02 (Seaglass Store Edition)
)
