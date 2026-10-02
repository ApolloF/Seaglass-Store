// Package edition says which Seaglass this is. The Store Edition is
// Seaglass with the experimental store, released from its own repository
// with its own release key. It keeps Seaglass's install folder and data,
// so it installs over Seaglass, and Seaglass installs back over it.
//
// Everything that differs from Seaglass hangs off these constants, so
// merging Seaglass's main branch in rarely touches anything else.
package edition

const (
	// Name is the product's full name.
	Name = "Seaglass Store Edition"
	// Short is what the interface shows next to "Seaglass".
	Short = "Store Edition"
	// Repo is the GitHub repository releases, updates and issues come from.
	Repo = "ApolloF/Seaglass-Store"
	// Suffix marks the edition's own release number in a version:
	// v1.9.0-store.2 is the second Store Edition release on Seaglass 1.9.0.
	Suffix = "-store."
)
