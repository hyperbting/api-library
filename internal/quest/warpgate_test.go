package quest

import (
	"context"
	"testing"
)

func TestWarpUnlocked(t *testing.T) {
	prog := func(p *Progress) *QuestsDoc {
		d := NewQuestsDoc(1)
		d.Quests[WarpQuestID] = p
		return d
	}
	cases := []struct {
		name string
		doc  *QuestsDoc
		want bool
	}{
		{"no document", nil, false},
		{"no quest", NewQuestsDoc(1), false},
		{"quest before the step", prog(&Progress{QuestID: WarpQuestID, CompletedSteps: []string{"earn_level", "level_first", "earn_decor", "buy_decor"}}), false},
		{"step done", prog(&Progress{QuestID: WarpQuestID, CompletedSteps: []string{"buy_decor", WarpStepID}}), true},
		{"quest finished", prog(&Progress{QuestID: WarpQuestID, Completed: true}), true},
		{"another quest's step of the same name", func() *QuestsDoc {
			d := NewQuestsDoc(1)
			d.Quests["tut_prologue"] = &Progress{QuestID: "tut_prologue", CompletedSteps: []string{WarpStepID}}
			return d
		}(), false},
	}
	for _, c := range cases {
		if got := WarpUnlocked(c.doc); got != c.want {
			t.Errorf("%s: WarpUnlocked = %v, want %v", c.name, got, c.want)
		}
	}
}

// The gate names a quest and a step by ID: a catalog edit that renames them must fail here, not silently lock
// every player out of the shop.
func TestWarpGateNamesAStepOfTheShippedCatalog(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := c.Get(WarpQuestID)
	if !ok {
		t.Fatalf("catalog has no quest %q", WarpQuestID)
	}
	for _, s := range def.Steps {
		if s.ID == WarpStepID {
			return
		}
	}
	t.Errorf("quest %q has no step %q", WarpQuestID, WarpStepID)
}

type getRepo struct {
	Repository
	doc *QuestsDoc
	err error
}

func (r getRepo) Get(context.Context, string) (*QuestsDoc, error) { return r.doc, r.err }

func TestNewWarpGateReadsTheQuestDocument(t *testing.T) {
	d := NewQuestsDoc(1)
	d.Quests[WarpQuestID] = &Progress{QuestID: WarpQuestID, CompletedSteps: []string{WarpStepID}}

	ok, err := NewWarpGate(getRepo{doc: d})(context.Background(), "u1")
	if err != nil || !ok {
		t.Errorf("player with the step: ok=%v err=%v", ok, err)
	}
	ok, err = NewWarpGate(getRepo{})(context.Background(), "u1")
	if err != nil || ok {
		t.Errorf("player without a document: ok=%v err=%v", ok, err)
	}
	if _, err = NewWarpGate(getRepo{err: context.DeadlineExceeded})(context.Background(), "u1"); err == nil {
		t.Error("a read error must come back as an error")
	}
}
