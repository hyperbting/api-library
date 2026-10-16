package pet

import (
	"context"
	"reflect"
	"testing"
	"time"

	"api-library/internal/grant"
)

func docWith(rewards ...[]grant.Reward) *grant.Doc {
	d := grant.NewDoc()
	now := time.Now()
	for i, r := range rewards {
		id := string(rune('a' + i))
		claimed := now
		g := grant.Grant{ID: id, Source: grant.SourceQuest, Rewards: r, CreatedAt: now}
		if i%2 == 1 {
			g.ClaimedAt = &claimed // claimed and unclaimed grants both count
		}
		if _, err := d.Add(g); err != nil {
			panic(err)
		}
	}
	return d
}

func TestOwnedStartsWithTheStarterPet(t *testing.T) {
	if got, want := Owned(nil), []string{"Pet_Tortoise"}; !reflect.DeepEqual(got, want) {
		t.Errorf("no grants: %v, want %v", got, want)
	}
	if got, want := Owned(grant.NewDoc()), []string{"Pet_Tortoise"}; !reflect.DeepEqual(got, want) {
		t.Errorf("empty ledger: %v, want %v", got, want)
	}
}

func TestOwnedIsBuiltFromPetGrantsOnly(t *testing.T) {
	d := docWith(
		[]grant.Reward{{Type: grant.RewardPet, Key: "Pet_Turtle", Amount: 1}},                                    // unclaimed
		[]grant.Reward{{Type: grant.RewardPet, Key: "Pet_Wani", Amount: 1}, {Type: grant.RewardCash, Amount: 5}}, // claimed
		[]grant.Reward{{Type: grant.RewardPet, Key: "Pet_Turtle", Amount: 1}},                                    // the same pet again
		[]grant.Reward{{Type: grant.RewardPet, Key: "Pet_Tortoise", Amount: 1}},                                  // a starter: not listed twice
		[]grant.Reward{{Type: grant.RewardCash, Amount: 10}},                                                     // not a pet
	)
	got := Owned(d)
	want := []string{"Pet_Tortoise", "Pet_Turtle", "Pet_Wani"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("owned = %v, want %v", got, want)
	}
}

type fakeRepo struct{ doc *grant.Doc }

func (f fakeRepo) Grants(context.Context, string) (*grant.Doc, error) { return f.doc, nil }

func TestServiceOwned(t *testing.T) {
	s := NewService(fakeRepo{doc: docWith([]grant.Reward{{Type: grant.RewardPet, Key: "Pet_Squirrel", Amount: 1}})})
	res, err := s.Owned(context.Background(), "uid")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Pet_Tortoise", "Pet_Squirrel"}; !reflect.DeepEqual(res.Owned, want) {
		t.Errorf("owned = %v, want %v", res.Owned, want)
	}
}
