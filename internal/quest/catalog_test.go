package quest

import (
	"errors"
	"testing"
)

func TestEmbeddedCatalogLoads(t *testing.T) {
	if _, err := LoadCatalog(); err != nil {
		t.Fatalf("embedded catalog is invalid: %v", err)
	}
}

const validCatalog = `{
  "version": 3,
  "quests": [
    {"id": "tut_prologue", "type": "Tutorial", "initial": true, "followUp": "ch1",
     "steps": [
       {"id": "intro", "checkpoint": true, "condition": {"eventType": "ConversationEnded", "key": "Prologue Intro", "target": 1}, "rewards": []},
       {"id": "jar", "condition": {"eventType": "ActionDone", "key": "ui.incomejar.tap", "target": 1},
        "rewards": [{"type": "Cash", "key": "", "amount": 100}]}
     ]},
    {"id": "ch1", "type": "Chapter",
     "steps": [{"id": "retail_t1", "condition": {"eventType": "StateChanged", "key": "store.retail.tier", "target": 1}, "rewards": []}]}
  ]
}`

func TestParseCatalog(t *testing.T) {
	c, err := ParseCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.Get("ch1"); !ok {
		t.Fatal("ch1 not indexed")
	}

	none := func(string) bool { return false }
	done := func(id string) bool { return id == "tut_prologue" }
	if !c.IsUnlockedBy("tut_prologue", none) {
		t.Error("initial quest should be unlocked")
	}
	if c.IsUnlockedBy("ch1", none) {
		t.Error("ch1 should be locked before the prologue completes")
	}
	if !c.IsUnlockedBy("ch1", done) {
		t.Error("ch1 should unlock after the prologue completes")
	}
}

func TestParseCatalogRejects(t *testing.T) {
	cases := map[string]string{
		"duplicate quest": `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]},{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"no steps":        `{"quests":[{"id":"a","type":"Goal","steps":[]}]}`,
		"duplicate step":  `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}},{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad type":        `{"quests":[{"id":"a","type":"Daily","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad event":       `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"Tapped"}}]}]}`,
		"bad reward":      `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"},"rewards":[{"type":"Gems"}]}]}]}`,
		"missing follow":  `{"quests":[{"id":"a","type":"Goal","followUp":"b","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCatalog([]byte(data)); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("want ErrInvalidCatalog, got %v", err)
			}
		})
	}
}
