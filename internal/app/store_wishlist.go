package app

import "github.com/ApolloF/Seaglass/internal/store/discovery"

// wishlistState keeps the Store's wishlist (work package 5).
type wishlistState struct{ c *Core }

func newWishlistState(c *Core) *wishlistState { return &wishlistState{c: c} }

// observe compares saved games' releases with their baselines.
func (w *wishlistState) observe(d *discoveryState) {}

// title is a saved game's title ("" when it isn't saved).
func (w *wishlistState) title(key string) string { return "" }

// keys are the saved games' keys.
func (w *wishlistState) keys() []string { return nil }

// rekey follows a game whose key changed after a Steam correction.
func (w *wishlistState) rekey(old, key, name string, appID int) {}

// wishlistAnnotator marks saved games and their unread activity.
func (c *Core) wishlistAnnotator() discovery.Annotate { return nil }

// enrichAnnotator adds the chart rank and cached review scores.
func (c *Core) enrichAnnotator() discovery.Annotate { return nil }

// popularState is the state of Steam's chart for the Popular shelf.
func (c *Core) popularState() string { return discovery.StateUnavailable }
