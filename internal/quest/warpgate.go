package quest

import "context"

// Time warps (gem and ad) work once the store tutorial's third decoration is done (GDD 5.2.1: the third
// decoration carries the warp). The gem service asks this through gem.WarpGate; see there for what it proves.
const (
	WarpQuestID = "tut_ch1_store"
	WarpStepID  = "third_deco"
)

// WarpUnlocked reports whether the player's quest progress has reached the warp-enabling step. A finished quest
// counts even if its step list was edited later; a missing document or quest does not.
func WarpUnlocked(doc *QuestsDoc) bool {
	if doc == nil {
		return false
	}
	p := doc.Quests[WarpQuestID]
	if p == nil {
		return false
	}
	return p.Completed || p.HasCompletedStep(WarpStepID)
}

// NewWarpGate reads the player's quest document for the gem service. The read is not part of the gem
// transaction: the step is one way, so a stale answer can only be "not yet" for a moment.
func NewWarpGate(repo Repository) func(ctx context.Context, uid string) (bool, error) {
	return func(ctx context.Context, uid string) (bool, error) {
		doc, err := repo.Get(ctx, uid)
		if err != nil {
			return false, err
		}
		return WarpUnlocked(doc), nil
	}
}
