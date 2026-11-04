package quest

import "testing"

func TestDrawLessonOn(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := catalog.Get(DrawLessonQuestID)
	if !ok {
		t.Fatalf("no %s", DrawLessonQuestID)
	}
	index := func(id string) int {
		for i, s := range def.Steps {
			if s.ID == id {
				return i
			}
		}
		t.Fatalf("step %s missing", id)
		return -1
	}
	doc := func(p *Progress) *QuestsDoc {
		d := NewQuestsDoc(catalog.Version)
		d.Quests[DrawLessonQuestID] = p
		return d
	}
	at := func(id string) *QuestsDoc { return doc(&Progress{QuestID: DrawLessonQuestID, StepIndex: index(id)}) }

	cases := []struct {
		name string
		doc  *QuestsDoc
		want bool
	}{
		{"no document", nil, false},
		{"no quest", NewQuestsDoc(1), false},
		{"turtle leaves", at("turtle_leaves"), false},
		{"gacha_shop", at("gacha_shop"), true},
		{"gacha_open", at("gacha_open"), true},
		{"gacha_character", at("gacha_character"), true},
		{"earn_level", at("earn_level"), false},
		{"finished", doc(&Progress{QuestID: DrawLessonQuestID, StepIndex: index("gacha_open"), Completed: true}), false},
	}
	for _, c := range cases {
		if got := DrawLessonOn(catalog, c.doc); got != c.want {
			t.Errorf("%s: DrawLessonOn = %v, want %v", c.name, got, c.want)
		}
	}
	if DrawLessonOn(nil, at("gacha_open")) {
		t.Error("nil catalog counts as off")
	}
}
