package quest

import "testing"

// tut_j: one step that puts 20 coins in the Home Income Jar.
const jarCatalog = `{
  "version": 12,
  "quests": [
    {"id": "tut_j", "type": "Tutorial", "initial": true, "steps": [
      {"id": "shelf", "condition": {"eventType": "ConversationEnded", "key": "Prologue/TrophyShelf", "target": 1},
       "rewards": [{"type": "Jar", "key": "", "amount": 20}]}
    ]}
  ]
}`

func TestJarRewardIsValidAndListedForTheClient(t *testing.T) {
	c, err := ParseCatalog([]byte(jarCatalog))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(newMemRepo(), c)

	res := complete(t, s, "tut_j", "shelf", 0)
	if !res.Accepted {
		t.Fatalf("not accepted: %+v", res)
	}
	// Like Cash, the client applies a Jar reward from `granted`; it needs no grant.
	if len(res.Granted) != 1 || res.Granted[0].Type != RewardJar || res.Granted[0].Amount != 20 {
		t.Errorf("granted = %+v, want the 20 jar coins", res.Granted)
	}
}
