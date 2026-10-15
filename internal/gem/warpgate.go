package gem

import "context"

// WarpGate says whether a player has reached the point where time warps work at all (GDD 5.2.1: the store's
// warp-enabled decoration, the third one). The economy and the decorations live on the client, so the request's
// warpEnabled flag alone is only a claim. The gate adds what the server can see: the player's own quest progress,
// which is ordered and one way (a step is recorded once, in sequence, and an admin reset is the only way back).
//
// What it does not do: a client that posts the quest steps itself still passes, because the server cannot see the
// decorations until they are server-owned (quest.Service.CompleteStep has the same TODO). It stops the cheap
// cases: a flag flipped on a fresh account, or one request that was never preceded by the tutorial's progress.
//
// A nil gate (tests, a deployment without quests) leaves the client's flag as the only check.
type WarpGate interface {
	WarpUnlocked(ctx context.Context, uid string) (bool, error)
}

// WarpGateFunc adapts a function to WarpGate.
type WarpGateFunc func(ctx context.Context, uid string) (bool, error)

func (f WarpGateFunc) WarpUnlocked(ctx context.Context, uid string) (bool, error) { return f(ctx, uid) }

// Option configures NewService.
type Option func(*service)

// WithWarpGate makes warps (gem and ad) require the gate on top of the request's warpEnabled flag.
func WithWarpGate(g WarpGate) Option {
	return func(s *service) { s.gate = g }
}

// warpAllowed reports whether a warp request may go ahead as far as the enabling rule is concerned. A request
// that does not claim the flag is refused without asking the gate (no read). A gate error is an error, never a
// pass: nothing is charged or granted when the quest state cannot be read.
func (s *service) warpAllowed(ctx context.Context, uid string, claimed bool) (bool, error) {
	if !claimed {
		return false, nil
	}
	if s.gate == nil {
		return true, nil
	}
	return s.gate.WarpUnlocked(ctx, uid)
}
