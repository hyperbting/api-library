package gacha

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"api-library/internal/gem"
	"api-library/internal/grant"
)

// loadFiveTierCatalog is the pool the rate, pity and pool tests were written against (1 to 5 stars, a x10 floor of
// 3 stars, hard pity at 4 and 5 stars). The shipped pool tops out at 3 stars (TestShippedPool).
func loadFiveTierCatalog() (*Catalog, error) {
	data, err := os.ReadFile("testdata/pool_five_tier.json")
	if err != nil {
		return nil, err
	}
	return ParseCatalog(data)
}

// memRepo mimics the Firestore repository: Run works on copies and keeps them only when fn asks.
type memRepo struct {
	gacha  map[string]*Doc
	gems   map[string]*gem.Doc
	grants map[string]*grant.Doc
}

func newMemRepo() *memRepo {
	return &memRepo{gacha: map[string]*Doc{}, gems: map[string]*gem.Doc{}, grants: map[string]*grant.Doc{}}
}

func (r *memRepo) Run(_ context.Context, uid string, fn func(*Doc, *gem.Doc, *grant.Doc) (bool, bool, bool, error)) error {
	d := NewDoc()
	if src := r.gacha[uid]; src != nil {
		v := *src
		v.Rows = map[string]*RowState{}
		for k, rs := range src.Rows {
			c := *rs
			c.Pity = map[string]int{}
			for pk, pv := range rs.Pity {
				c.Pity[pk] = pv
			}
			v.Rows[k] = &c
		}
		v.Accessories = map[string]time.Time{}
		for k, t := range src.Accessories {
			v.Accessories[k] = t
		}
		v.Credited = map[string]time.Time{}
		for k, t := range src.Credited {
			v.Credited[k] = t
		}
		v.Draws = map[string]*DrawRecord{}
		for k, rec := range src.Draws {
			c := *rec
			v.Draws[k] = &c
		}
		d = &v
	}
	gems := gem.NewDoc()
	if src := r.gems[uid]; src != nil {
		v := *src
		v.Credited = map[string]time.Time{}
		for k, t := range src.Credited {
			v.Credited[k] = t
		}
		gems = &v
	}
	grants := grant.NewDoc()
	if src := r.grants[uid]; src != nil {
		v := *src
		v.Grants = map[string]*grant.Grant{}
		for k, g := range src.Grants {
			c := *g
			c.Rewards = append([]grant.Reward(nil), g.Rewards...)
			v.Grants[k] = &c
		}
		grants = &v
	}
	wd, wg, wgr, err := fn(d, gems, grants)
	if err != nil {
		return err
	}
	if wd {
		r.gacha[uid] = d
	}
	if wg {
		r.gems[uid] = gems
	}
	if wgr {
		r.grants[uid] = grants
	}
	return nil
}

var clock = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

type fixture struct {
	srv  *service
	repo *memRepo
	cat  *Catalog
	ctx  context.Context
	now  time.Time
	seq  []int // scripted rolls; empty = always 0
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cat, err := loadFiveTierCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cat.Rows[RowCharacter].FirstDraw = nil // the tests script their own first rolls; TestFirstDraw turns it on
	for i := range cat.Pets {
		cat.Pets[i].Pool = 1 // every pet is open: the tests roll every tier; TestPetPools uses the real pools
	}
	f := &fixture{repo: newMemRepo(), cat: cat, ctx: context.Background(), now: clock}
	f.srv = NewService(f.repo, cat).(*service)
	f.srv.now = func() time.Time { return f.now }
	f.srv.intn = func(n int) int {
		if len(f.seq) == 0 {
			return 0
		}
		v := f.seq[0]
		f.seq = f.seq[1:]
		return v % n
	}
	return f
}

func (f *fixture) giveGems(uid, id string, n float64) {
	f.giveReward(uid, id, grant.Reward{Type: grant.RewardGem, Amount: n})
}

func (f *fixture) giveReward(uid, id string, r grant.Reward) {
	doc := f.repo.grants[uid]
	if doc == nil {
		doc = grant.NewDoc()
		f.repo.grants[uid] = doc
	}
	doc.Grants[id] = &grant.Grant{ID: id, Source: grant.SourceAdmin, Rewards: []grant.Reward{r}, CreatedAt: f.now}
}

func (f *fixture) draw(uid string, row Row, mode Mode, count int, rid string) *DrawResponse {
	f.srv.now = func() time.Time { return f.now }
	res, err := f.srv.Draw(f.ctx, uid, DrawRequest{Row: row, Mode: mode, Count: count, RequestID: rid})
	if err != nil {
		panic(err)
	}
	return res
}

func (f *fixture) state(uid string) *State {
	st, err := f.srv.State(f.ctx, uid)
	if err != nil {
		panic(err)
	}
	return st
}

func rowView(st *State, row Row) RowView {
	for _, r := range st.Rows {
		if r.Row == row {
			return r
		}
	}
	panic("row " + string(row))
}

func TestCatalogValid(t *testing.T) {
	cat, err := loadFiveTierCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Version < 1 || len(cat.Pets) == 0 || len(cat.Accessories) == 0 {
		t.Fatalf("empty pool: %+v", cat.Version)
	}
}

func TestCatalogRejects(t *testing.T) {
	tests := map[string]func(c *Catalog){
		"rates not 100%":           func(c *Catalog) { c.Rows[RowCharacter].Tiers[0].Rate++ },
		"duplicate pet id":         func(c *Catalog) { c.Pets[1].ID = c.Pets[0].ID },
		"unknown pet address":      func(c *Catalog) { c.Pets[0].Address = "Pet_Nope" },
		"starter in the pool":      func(c *Catalog) { c.Pets[0].Address = "Pet_Tortoise" },
		"empty tier":               func(c *Catalog) { c.Pets = c.Pets[:len(c.Pets)-2] },
		"unknown rarity":           func(c *Catalog) { c.Accessories[0].Rarity = "Mythic" },
		"unknown slot":             func(c *Catalog) { c.Accessories[0].Slot = "Foot" },
		"no fits":                  func(c *Catalog) { c.Accessories[0].Fits = nil },
		"duplicate beats price":    func(c *Catalog) { c.Rows[RowAccessory].Tiers[3].DupMinutes = 30 * MinutesPerGem },
		"x10 dearer than 10 x x1":  func(c *Catalog) { c.Rows[RowAccessory].GemPrice10 = 301 },
		"hard pity below tier two": func(c *Catalog) { c.Rows[RowCharacter].Pity.Hard[0].MinTier = 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c, err := loadFiveTierCatalog()
			if err != nil {
				t.Fatal(err)
			}
			mutate(c)
			if err := c.validate(); err == nil {
				t.Fatal("invalid pool accepted")
			}
		})
	}
}

func TestRatesCoverTheTableExactly(t *testing.T) {
	cat, _ := loadFiveTierCatalog()
	want := map[Row][]int{
		RowCharacter: {4500, 3000, 1700, 650, 150},
		RowAccessory: {6000, 2800, 1000, 200},
	}
	for row, rates := range want {
		cfg := cat.Rows[row]
		counts := make([]int, len(cfg.Tiers))
		// Every possible roll value once: the share of each tier is its rate.
		for n := 0; n < RateScale; n++ {
			rs := &RowState{Pity: map[string]int{}}
			tier, _ := cfg.rollTier(rs, false, func(int) int { return n })
			counts[tier-1]++
		}
		for i, w := range rates {
			if counts[i] != w {
				t.Errorf("%s tier %d: %d of %d, want %d", row, i+1, counts[i], RateScale, w)
			}
		}
	}
}

func TestHardPity(t *testing.T) {
	cat, _ := loadFiveTierCatalog()
	tests := []struct {
		name  string
		row   Row
		since map[string]int
		min   int // forced minimum tier of the next draw
	}{
		{"character 4 star at the 20th", RowCharacter, map[string]int{"4": 19, "5": 5}, 4},
		{"character 5 star at the 80th", RowCharacter, map[string]int{"4": 3, "5": 79}, 5},
		{"character 19 misses is not enough", RowCharacter, map[string]int{"4": 18, "5": 0}, 1},
		{"accessory legendary at the 60th", RowAccessory, map[string]int{"4": 59}, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cat.Rows[tc.row]
			// The lowest roll value gives the lowest tier the floor allows.
			rs := &RowState{Pity: tc.since}
			tier, _ := cfg.rollTier(rs, false, func(int) int { return 0 })
			if tier != tc.min {
				t.Fatalf("tier %d, want %d", tier, tc.min)
			}
		})
	}
}

func TestPityCountersResetAndShare(t *testing.T) {
	cat, _ := loadFiveTierCatalog()
	cfg := cat.Rows[RowCharacter]
	rs := &RowState{Pity: map[string]int{}}
	// 5-star roll value (top of the table) resets both counters.
	for i := 0; i < 3; i++ {
		cfg.rollTier(rs, false, func(int) int { return 0 })
	}
	if rs.Pity["4"] != 3 || rs.Pity["5"] != 3 {
		t.Fatalf("counters %v", rs.Pity)
	}
	if tier, _ := cfg.rollTier(rs, false, func(n int) int { return n - 1 }); tier != 5 {
		t.Fatalf("top roll gave tier %d", tier)
	}
	if rs.Pity["4"] != 0 || rs.Pity["5"] != 0 {
		t.Fatalf("a 5 star resets both: %v", rs.Pity)
	}
	// A 4 star resets only the 4 star counter.
	cfg.rollTier(rs, false, func(int) int { return 0 })
	cfg.rollTier(rs, false, func(int) int { return 9500 }) // 4-star band
	if rs.Pity["4"] != 0 || rs.Pity["5"] != 2 {
		t.Fatalf("a 4 star resets only its own counter: %v", rs.Pity)
	}
}

func TestTenDrawHoldsMinimumTier(t *testing.T) {
	for _, tc := range []struct {
		row  Row
		min  int
		cost int64
	}{{RowCharacter, 3, 1350}, {RowAccessory, 2, 270}} {
		t.Run(string(tc.row), func(t *testing.T) {
			f := newFixture(t)
			f.giveGems("u", "g1", 5000)
			// Every roll is the lowest value: tier 1 each time, so only the guarantee can lift one draw.
			res := f.draw("u", tc.row, ModeGem, 10, "req-ten-0001")
			if !res.OK || len(res.Results) != 10 {
				t.Fatalf("draw: %+v", res)
			}
			best := 0
			for _, r := range res.Results {
				if r.Tier > best {
					best = r.Tier
				}
			}
			if best < tc.min {
				t.Fatalf("best tier %d, want >= %d", best, tc.min)
			}
			if res.State.Gems != 5000-tc.cost {
				t.Fatalf("gems %d", res.State.Gems)
			}
		})
	}
}

func TestTenDrawNoForcedUpgradeWhenAlreadyMet(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 5000)
	// First roll lands in the 3-star band (7500..9199), all others in the 1-star band.
	f.seq = []int{7600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	res := f.draw("u", RowCharacter, ModeGem, 10, "req-ten-0002")
	for i, r := range res.Results {
		if i > 0 && r.Tier != 1 {
			t.Fatalf("draw %d was lifted to tier %d though the guarantee was met", i, r.Tier)
		}
	}
}

func TestPriceTable(t *testing.T) {
	tests := []struct {
		row   Row
		count int
		price int64
	}{
		{RowCharacter, 1, 150}, {RowCharacter, 10, 1350}, {RowAccessory, 1, 30}, {RowAccessory, 10, 270},
	}
	for _, tc := range tests {
		t.Run(string(tc.row)+"x"+strconv.Itoa(tc.count), func(t *testing.T) {
			f := newFixture(t)
			f.giveGems("u", "g1", 2000)
			res := f.draw("u", tc.row, ModeGem, tc.count, "req-price-01")
			if !res.OK || res.State.Gems != 2000-tc.price {
				t.Fatalf("ok=%v gems=%d want %d", res.OK, res.State.Gems, 2000-tc.price)
			}
		})
	}
}

func TestGemDrawRefusedWithoutGemsChargesNothing(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 149)
	res := f.draw("u", RowCharacter, ModeGem, 1, "req-poor-001")
	if res.OK || res.ErrorCode != CodeInsufficientGems || res.State.Gems != 149 {
		t.Fatalf("%+v", res)
	}
	if len(f.repo.gacha["u"].Draws) != 0 || len(f.repo.grants["u"].Grants) != 1 {
		t.Fatal("a refused draw left a record or a grant")
	}
}

func TestFirstLookGivesOneFreeDrawPerRow(t *testing.T) {
	f := newFixture(t)
	st := f.state("u")
	for _, row := range Rows {
		v := rowView(st, row)
		if v.FreeStored != 1 || v.NextFreeAt == nil {
			t.Fatalf("%s: %+v", row, v)
		}
	}
	if got := rowView(st, RowCharacter).NextFreeInSeconds; got != 4*3600 {
		t.Fatalf("character timer %d", got)
	}
	if got := rowView(st, RowAccessory).NextFreeInSeconds; got != 3600 {
		t.Fatalf("accessory timer %d", got)
	}
}

func TestStoredFreeDrawsAndCap(t *testing.T) {
	tests := []struct {
		name        string
		row         Row
		elapsed     time.Duration
		wantStored  int
		wantTimerOn bool
	}{
		{"character before the first interval", RowCharacter, 3*time.Hour + 59*time.Minute, 1, true},
		{"character one interval", RowCharacter, 4 * time.Hour, 2, false},
		{"character stops at 2", RowCharacter, 40 * time.Hour, 2, false},
		{"accessory one interval", RowAccessory, time.Hour, 2, true},
		{"accessory two intervals", RowAccessory, 2 * time.Hour, 3, false},
		{"accessory stops at 3", RowAccessory, 500 * time.Hour, 3, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.state("u")
			f.now = f.now.Add(tc.elapsed)
			v := rowView(f.state("u"), tc.row)
			if v.FreeStored != tc.wantStored || (v.NextFreeAt != nil) != tc.wantTimerOn {
				t.Fatalf("stored=%d timer=%v, want %d / %v", v.FreeStored, v.NextFreeAt, tc.wantStored, tc.wantTimerOn)
			}
		})
	}
}

func TestTimerKeepsRhythmAndRestartsAfterCap(t *testing.T) {
	f := newFixture(t)
	f.state("u")
	// 2 h in: one accessory draw stored, timer for the next at +1 h from the first arrival (3 h after start... 2 total).
	f.now = f.now.Add(90 * time.Minute)
	v := rowView(f.state("u"), RowAccessory)
	if v.FreeStored != 2 || v.NextFreeInSeconds != 30*60 {
		t.Fatalf("stored=%d next in %ds", v.FreeStored, v.NextFreeInSeconds)
	}
	// Use one at 90 min: the timer is running, so it is not restarted.
	if res := f.draw("u", RowAccessory, ModeFree, 1, "req-free-001"); !res.OK {
		t.Fatalf("%+v", res)
	}
	v = rowView(f.state("u"), RowAccessory)
	if v.FreeStored != 1 || v.NextFreeInSeconds != 30*60 {
		t.Fatalf("after use: stored=%d next in %ds", v.FreeStored, v.NextFreeInSeconds)
	}
	// At the cap the timer is off; using one starts it from the moment of use.
	f.now = f.now.Add(24 * time.Hour)
	if v = rowView(f.state("u"), RowAccessory); v.FreeStored != 3 || v.NextFreeAt != nil {
		t.Fatalf("at cap: %+v", v)
	}
	f.draw("u", RowAccessory, ModeFree, 1, "req-free-002")
	v = rowView(f.state("u"), RowAccessory)
	if v.FreeStored != 2 || v.NextFreeInSeconds != 3600 {
		t.Fatalf("restart: stored=%d next in %ds", v.FreeStored, v.NextFreeInSeconds)
	}
}

func TestFreeDrawNeedsAStoredDraw(t *testing.T) {
	f := newFixture(t)
	if res := f.draw("u", RowCharacter, ModeFree, 1, "req-free-0a"); !res.OK {
		t.Fatalf("%+v", res)
	}
	res := f.draw("u", RowCharacter, ModeFree, 1, "req-free-0b")
	if res.OK || res.ErrorCode != CodeNoFreeDraw {
		t.Fatalf("second free draw: %+v", res)
	}
	// An earlier device clock changes nothing: the server clock rules.
	f.now = f.now.Add(-48 * time.Hour)
	if res = f.draw("u", RowCharacter, ModeFree, 1, "req-free-0c"); res.OK {
		t.Fatal("a clock set back gave a free draw")
	}
}

func TestFreeAndGemDrawsShareThePity(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	f.draw("u", RowCharacter, ModeFree, 1, "req-share-01")
	f.draw("u", RowCharacter, ModeGem, 1, "req-share-02")
	for _, p := range rowView(f.state("u"), RowCharacter).Pity {
		if p.Since != 2 {
			t.Fatalf("pity %+v after one free and one gem draw", p)
		}
	}
}

func TestIdempotentRequest(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 1500)
	f.seq = []int{9000, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	first := f.draw("u", RowCharacter, ModeGem, 10, "req-idem-001")
	if !first.OK || first.State.Gems != 150 {
		t.Fatalf("%+v", first)
	}
	grantsBefore := len(f.repo.grants["u"].Grants)

	f.now = f.now.Add(time.Minute)
	f.seq = []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5} // a re-roll would differ
	again := f.draw("u", RowCharacter, ModeGem, 10, "req-idem-001")
	if !again.OK || !again.Repeated || again.State.Gems != 150 {
		t.Fatalf("repeat: %+v", again)
	}
	if len(again.Results) != len(first.Results) {
		t.Fatal("different result count")
	}
	for i := range first.Results {
		if first.Results[i] != again.Results[i] {
			t.Fatalf("result %d changed: %+v vs %+v", i, first.Results[i], again.Results[i])
		}
	}
	if len(again.Grants) != len(first.Grants) || len(f.repo.grants["u"].Grants) != grantsBefore {
		t.Fatal("a repeat created grants")
	}
	if rowView(&again.State, RowCharacter).Pity[0].Since != rowView(&first.State, RowCharacter).Pity[0].Since {
		t.Fatal("a repeat moved the pity")
	}

	// The same id for another draw is refused, and charges nothing.
	other := f.draw("u", RowAccessory, ModeGem, 1, "req-idem-001")
	if other.OK || other.ErrorCode != CodeRequestMismatch || other.State.Gems != 150 {
		t.Fatalf("mismatch: %+v", other)
	}
}

func TestIdempotentFreeDrawUsesOneStoredDraw(t *testing.T) {
	f := newFixture(t)
	f.state("u")
	f.now = f.now.Add(4 * time.Hour) // character: 2 stored
	f.draw("u", RowCharacter, ModeFree, 1, "req-free-idem")
	f.draw("u", RowCharacter, ModeFree, 1, "req-free-idem")
	if v := rowView(f.state("u"), RowCharacter); v.FreeStored != 1 {
		t.Fatalf("stored %d after a draw and its repeat, want 1", v.FreeStored)
	}
}

func TestDuplicatePetBecomesCash(t *testing.T) {
	cat, _ := loadFiveTierCatalog()
	// Tier 1 pet at pool index 0 = Pet_Hamster; two 1-star rolls in a row: new, then duplicate.
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	first := f.draw("u", RowCharacter, ModeGem, 1, "req-dup-0001")
	if !first.Results[0].New || first.Results[0].ID != "Pet_Hamster" || first.CashHours != 0 {
		t.Fatalf("first: %+v", first)
	}
	if len(first.Grants) != 1 || first.Grants[0].Rewards[0].Type != grant.RewardPet || first.Grants[0].Rewards[0].Key != "Pet_Hamster" {
		t.Fatalf("new pet grant: %+v", first.Grants)
	}
	second := f.draw("u", RowCharacter, ModeGem, 1, "req-dup-0002")
	r := second.Results[0]
	if r.New || r.CashMinutes != cat.Rows[RowCharacter].Tiers[0].DupMinutes {
		t.Fatalf("duplicate: %+v", r)
	}
	if len(second.Grants) != 1 {
		t.Fatalf("grants: %+v", second.Grants)
	}
	rw := second.Grants[0].Rewards[0]
	if rw.Type != grant.RewardTimeWarp || rw.Amount != 2.0/60 {
		t.Fatalf("cash reward %+v", rw)
	}
	for _, g := range second.Grants {
		for _, rw := range g.Rewards {
			if rw.Type == grant.RewardGem {
				t.Fatal("a duplicate paid gems")
			}
		}
	}
	// The pet is owned once.
	pets := 0
	for _, p := range second.State.Pets {
		if p.ID == "Pet_Hamster" {
			pets++
			if p.Stars != 1 || p.MaxAccessories != 1 {
				t.Fatalf("%+v", p)
			}
		}
	}
	if pets != 1 {
		t.Fatalf("Pet_Hamster owned %d times", pets)
	}
}

func TestStarterPetIsOwnedAndPaysCash(t *testing.T) {
	// A pet the account already owns (here through the ledger) pays cash even on the first draw.
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	f.giveReward("u", "gift", grant.Reward{Type: grant.RewardPet, Key: "Pet_Hamster", Amount: 1})
	res := f.draw("u", RowCharacter, ModeGem, 1, "req-dup-0003")
	if res.Results[0].New || res.CashHours <= 0 {
		t.Fatalf("%+v", res)
	}
}

func TestDuplicatesInsideOneTenDraw(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 5000)
	// Ten 1-star rolls (the last lifted to 3 star by the guarantee) all pick index 0 of their tier.
	res := f.draw("u", RowCharacter, ModeGem, 10, "req-dup-ten-1")
	news, dups := 0, 0
	var minutes float64
	for _, r := range res.Results {
		if r.New {
			news++
		} else {
			dups++
			minutes += r.CashMinutes
		}
	}
	if news != 2 || dups != 8 {
		t.Fatalf("new %d dup %d", news, dups)
	}
	if res.CashHours != minutes/60 {
		t.Fatalf("cash hours %v want %v", res.CashHours, minutes/60)
	}
	// One pet grant for the two new pets, one cash grant for the eight duplicates.
	if len(res.Grants) != 2 || len(res.Grants[0].Rewards) != 2 || res.Grants[1].Rewards[0].Count != 8 {
		t.Fatalf("grants %+v", res.Grants)
	}
}

func TestAccessoryInventoryAndDuplicate(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	a := f.draw("u", RowAccessory, ModeGem, 1, "req-acc-0001")
	if !a.Results[0].New || a.Results[0].Kind != "accessory" || len(a.Grants) != 0 {
		t.Fatalf("%+v", a)
	}
	if len(a.State.Accessories) != 1 || a.State.Accessories[0].ID != a.Results[0].ID {
		t.Fatalf("inventory %+v", a.State.Accessories)
	}
	b := f.draw("u", RowAccessory, ModeGem, 1, "req-acc-0002")
	if b.Results[0].New || b.Results[0].CashMinutes != 2 || len(b.State.Accessories) != 1 {
		t.Fatalf("%+v", b)
	}
}

func TestAccessoryFitsAreReported(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	// Legendary band (top 2%), index 0 of the tier = the Hare's aviators.
	f.seq = []int{9990, 0}
	res := f.draw("u", RowAccessory, ModeGem, 1, "req-acc-fit-1")
	got := res.State.Accessories[0]
	if got.ID != "acc_face_shades" || len(got.Fits) != 1 || got.Fits[0] != "Hare" || got.Slot != "Face" {
		t.Fatalf("%+v", got)
	}
}

func TestStarsSetAccessorySlots(t *testing.T) {
	f := newFixture(t)
	f.giveGems("u", "g1", 5000)
	for i, addr := range []string{"Pet_Sloth", "Pet_Doberman"} {
		f.giveReward("u", "gift"+strconv.Itoa(i), grant.Reward{Type: grant.RewardPet, Key: addr, Amount: 1})
	}
	want := map[string]int{"Pet_Tortoise": 1, "Pet_Sloth": 3, "Pet_Doberman": 5}
	for _, p := range f.state("u").Pets {
		if w, ok := want[p.ID]; ok {
			if p.MaxAccessories != w || p.Stars != w {
				t.Fatalf("%s: %+v", p.ID, p)
			}
			delete(want, p.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing pets %v", want)
	}
}

func TestTicketPaysOneCharacterDraw(t *testing.T) {
	f := newFixture(t)
	f.giveReward("u", "t1", grant.Reward{Type: grant.RewardPull, Amount: 2})
	if st := f.state("u"); st.Tickets != 2 {
		t.Fatalf("tickets %d", st.Tickets)
	}
	res := f.draw("u", RowCharacter, ModeTicket, 1, "req-tick-001")
	if !res.OK || res.State.Tickets != 1 {
		t.Fatalf("%+v", res)
	}
	// The same grant is not counted twice on the next read.
	if st := f.state("u"); st.Tickets != 1 {
		t.Fatalf("tickets %d", st.Tickets)
	}
	f.draw("u", RowCharacter, ModeTicket, 1, "req-tick-002")
	res = f.draw("u", RowCharacter, ModeTicket, 1, "req-tick-003")
	if res.OK || res.ErrorCode != CodeInsufficientTicket {
		t.Fatalf("%+v", res)
	}
}

func TestInvalidRequests(t *testing.T) {
	f := newFixture(t)
	tests := []DrawRequest{
		{Row: RowAccessory, Mode: ModeTicket, Count: 1, RequestID: "req-bad-0001"},  // the accessory row takes no tickets
		{Row: RowCharacter, Mode: ModeTicket, Count: 10, RequestID: "req-bad-0002"}, // a ticket pays one x1 draw
		{Row: RowCharacter, Mode: ModeFree, Count: 10, RequestID: "req-bad-0003"},   // free draws are single
		{Row: RowCharacter, Mode: ModeGem, Count: 5, RequestID: "req-bad-0004"},
		{Row: "pets", Mode: ModeGem, Count: 1, RequestID: "req-bad-0005"},
		{Row: RowCharacter, Mode: "ad", Count: 1, RequestID: "req-bad-0006"}, // an ad never pays or skips a draw
	}
	for _, req := range tests {
		if _, err := f.srv.Draw(f.ctx, "u", req); err != ErrInvalidRequest {
			t.Errorf("%+v: err %v", req, err)
		}
	}
}

func TestGemSpendIsAtomicWithTheGrantWrite(t *testing.T) {
	// A full ledger fails the draw: the gems must not be taken.
	f := newFixture(t)
	f.giveGems("u", "g1", 1000)
	for i := 0; i < grant.MaxGrants-1; i++ {
		f.repo.grants["u"].Grants["fill"+strconv.Itoa(i)] = &grant.Grant{ID: "fill" + strconv.Itoa(i), Source: grant.SourceAdmin, Rewards: []grant.Reward{{Type: grant.RewardCash, Amount: 1}}}
	}
	// Duplicate pets need two grants (not here): a new pet needs one more slot than is left after the fill.
	f.repo.grants["u"].Grants["fillx"] = &grant.Grant{ID: "fillx", Source: grant.SourceAdmin, Rewards: []grant.Reward{{Type: grant.RewardCash, Amount: 1}}}
	_, err := f.srv.Draw(f.ctx, "u", DrawRequest{Row: RowCharacter, Mode: ModeGem, Count: 1, RequestID: "req-full-001"})
	if err == nil {
		t.Fatal("a draw succeeded with a full ledger")
	}
	if b := f.repo.gems["u"]; b != nil && b.Balance != 0 && b.Balance != 1000 {
		t.Fatalf("balance %d", b.Balance)
	}
	if d := f.repo.gacha["u"]; d != nil && len(d.Draws) != 0 {
		t.Fatal("a failed draw left a record")
	}
}

func TestPruneClaimedCashMakesRoom(t *testing.T) {
	grants := grant.NewDoc()
	now := time.Now()
	for i := 0; i < grant.MaxGrants-5; i++ {
		id := "gacha:r" + strconv.Itoa(i) + ":cash"
		grants.Grants[id] = &grant.Grant{ID: id, Source: grant.SourceGacha, ClaimedAt: &now}
	}
	grants.Grants["gacha:r1:pets"] = &grant.Grant{ID: "gacha:r1:pets", Source: grant.SourceGacha, ClaimedAt: &now}
	grants.Grants["unclaimed:cash"] = &grant.Grant{ID: "unclaimed:cash", Source: grant.SourceGacha}
	pruneClaimedCash(grants)
	if len(grants.Grants) != 2 || grants.Grants["gacha:r1:pets"] == nil || grants.Grants["unclaimed:cash"] == nil {
		t.Fatalf("%d grants left", len(grants.Grants))
	}
}

// The first ever character draw is a pet the player does not own, from the row's FirstDraw list (the tutorial's
// second pet); after it the row rolls normally, and pity counts it as a result of its stars.
func TestFirstDraw(t *testing.T) {
	f := newFixture(t)
	f.cat.Rows[RowCharacter].FirstDraw = []string{"Pet_Hamster", "Pet_Squirrel"}
	f.seq = []int{1, 7600} // the first pick is the second FirstDraw pet; the next draw rolls tier 3
	first := f.draw("u", RowCharacter, ModeFree, 1, "req-first-1")
	if !first.OK || len(first.Results) != 1 {
		t.Fatalf("%+v", first)
	}
	r := first.Results[0]
	if r.ID != "Pet_Squirrel" || !r.New || r.Tier != 1 || len(first.Grants) != 1 {
		t.Fatalf("first draw: %+v grants=%d", r, len(first.Grants))
	}
	f.now = f.now.Add(5 * time.Hour)
	second := f.draw("u", RowCharacter, ModeGem, 1, "req-first-2")
	if second.OK && second.Results[0].Tier != 3 {
		t.Fatalf("the second draw rolls normally: %+v", second)
	}
}

// Pool 2 pets (the 4- and 5-star ones) are not drawn until a grant carries the Unlock reward "gacha_pool_2"; until then
// their tiers are not rolled, the odds shown add up over the open tiers, and the hard pity of a closed tier waits.
func TestPetPools(t *testing.T) {
	f := newFixture(t)
	cat := f.cat
	for i := range cat.Pets {
		if cat.Pets[i].Stars >= 4 {
			cat.Pets[i].Pool = 2
		}
	}
	f.giveGems("u", "g1", 100000)
	// A roll at the top of the open table lands on the highest open tier: 3-star, not 5.
	f.seq = []int{9199, 0}
	res := f.draw("u", RowCharacter, ModeGem, 1, "req-pool-1")
	if !res.OK || res.Results[0].Tier != 3 {
		t.Fatalf("pool 1 only: %+v", res.Results)
	}
	var sum float64
	var tiers []int
	for _, row := range res.State.Rows {
		if row.Row != RowCharacter {
			continue
		}
		for _, tv := range row.Tiers {
			sum += tv.RatePct
			tiers = append(tiers, tv.Tier)
		}
		if row.TenMinTier != 3 || len(row.Pity) != 0 {
			t.Fatalf("closed tiers show no pity: ten=%d pity=%v", row.TenMinTier, row.Pity)
		}
	}
	if len(tiers) != 3 || sum < 99.99 || sum > 100.01 {
		t.Fatalf("odds of the open tiers: %v sum=%v", tiers, sum)
	}
	f.giveReward("u", "unlock-pool-2", grant.Reward{Type: grant.RewardUnlock, Key: "gacha_pool_2"})
	f.seq = []int{9999, 0}
	res = f.draw("u", RowCharacter, ModeGem, 1, "req-pool-2")
	if !res.OK || res.Results[0].Tier != 5 {
		t.Fatalf("pool 2 open: %+v", res.Results)
	}
}

// The pool that ships: draws top out at 3 stars (user, 2026-10-24); a pet may be able to upgrade its stars later
// (MaxStars above Stars), a fixed one has MaxStars equal to Stars.
func TestShippedPool(t *testing.T) {
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	chars := cat.Rows[RowCharacter]
	if len(chars.Tiers) != 3 {
		t.Fatalf("character tiers %d, want 3 (1 to 3 stars)", len(chars.Tiers))
	}
	for _, p := range cat.Pets {
		if p.Stars < 1 || p.Stars > 3 {
			t.Errorf("%s: %d stars, draws top out at 3", p.ID, p.Stars)
		}
		if p.MaxStars < p.Stars || p.MaxStars > 5 {
			t.Errorf("%s: maxStars %d (stars %d)", p.ID, p.MaxStars, p.Stars)
		}
	}
}
