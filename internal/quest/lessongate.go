package quest

import "context"

// The character draw lessons of the store tutorial. The gacha service asks NewDrawLessonGate whether the player is on
// them, to keep one free draw ready (gacha.LessonGate).
const (
	DrawLessonQuestID   = "tut_ch1_store"
	DrawLessonFirstStep = "gacha_shop"
	DrawLessonLastStep  = "gacha_character"
)

// DrawLessonOn reports whether the tutorial's current step is one of the draw lessons (gacha_shop to gacha_character
// in the catalog's order). A missing document or quest, a finished quest and a catalog without those steps are off.
func DrawLessonOn(catalog *Catalog, doc *QuestsDoc) bool {
	if catalog == nil || doc == nil {
		return false
	}
	def, ok := catalog.Get(DrawLessonQuestID)
	if !ok {
		return false
	}
	first, last := -1, -1
	for i, s := range def.Steps {
		switch s.ID {
		case DrawLessonFirstStep:
			first = i
		case DrawLessonLastStep:
			last = i
		}
	}
	p := doc.Quests[DrawLessonQuestID]
	if p == nil || p.Completed || first < 0 || last < first {
		return false
	}
	return p.StepIndex >= first && p.StepIndex <= last
}

// NewDrawLessonGate reads the player's quest document. The read is not part of the gacha transaction.
func NewDrawLessonGate(repo Repository, catalog *Catalog) func(ctx context.Context, uid string) (bool, error) {
	return func(ctx context.Context, uid string) (bool, error) {
		doc, err := repo.Get(ctx, uid)
		if err != nil {
			return false, err
		}
		return DrawLessonOn(catalog, doc), nil
	}
}
