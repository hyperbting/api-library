package gacha

import (
	"context"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"api-library/internal/gem"
	"api-library/internal/grant"
	"api-library/internal/pet"
)

type Service interface {
	// State credits gems and tickets granted since the last read, brings the free draw timers up to date and
	// reports both rows, the accessory inventory and the owned pets.
	State(ctx context.Context, uid string) (*State, error)
	// Draw pays (a stored free draw, gems or tickets) and rolls in one transaction. A refusal is ok=false with an
	// error code; a malformed request is ErrInvalidRequest.
	Draw(ctx context.Context, uid string, req DrawRequest) (*DrawResponse, error)
}

type service struct {
	repo Repository
	cat  *Catalog
	now  func() time.Time
	intn func(n int) int // injectable for tests
}

func NewService(repo Repository, cat *Catalog) Service {
	return &service{repo: repo, cat: cat, now: time.Now, intn: rand.IntN}
}

// touch settles credits and brings both rows up to date. It reports whether the gacha document changed.
func (s *service) touch(d *Doc, g *gem.Doc, grants *grant.Doc, now time.Time) (gachaChanged, gemsChanged bool) {
	if s.settleGems(g, grants, now) {
		gemsChanged = true
	}
	if d.SettleTickets(grants, now) > 0 {
		gachaChanged = true
	}
	for _, row := range Rows {
		cfg := s.cat.Rows[row]
		rs, created := d.openRow(cfg, row, now)
		if created || rs.accrue(cfg, now) {
			gachaChanged = true
		}
	}
	if gachaChanged {
		d.UpdatedAt = now
	}
	return gachaChanged, gemsChanged
}

func (s *service) settleGems(g *gem.Doc, grants *grant.Doc, now time.Time) bool {
	added := g.Settle(grants, now)
	if added > 0 {
		g.UpdatedAt = now
	}
	return added > 0
}

func (s *service) state(d *Doc, g *gem.Doc, grants *grant.Doc, now time.Time) State {
	st := State{
		PoolVersion: s.cat.Version,
		ServerNowMs: now.UnixMilli(),
		Gems:        g.Balance,
		Tickets:     d.Tickets,
		Rows:        []RowView{},
		Accessories: []OwnedAccessory{},
		Pets:        []OwnedPet{},
	}
	openPets := s.cat.openPetsByTier(grants)
	for _, row := range Rows {
		cfg := s.cat.Rows[row]
		// The tiers that have something to give now: the character row shows only the stars of the open pools.
		has := allTiers
		if row == RowCharacter {
			has = func(tier int) bool { return len(openPets[tier]) > 0 }
		}
		maxTier := cfg.maxAvailable(has)
		rs := d.Rows[string(row)]
		if rs == nil {
			rs = &RowState{Stored: 1}
		}
		v := RowView{
			Row: row, FreeStored: rs.Stored, FreeMax: cfg.FreeMax, FreeIntervalSecs: cfg.FreeIntervalSeconds,
			GemPrice1: cfg.GemPrice1, GemPrice10: cfg.GemPrice10, TicketCost1: cfg.TicketCost1,
			TenMinTier: cfg.Pity.TenMinTier, Pity: []PityView{}, Tiers: []TierView{},
		}
		if v.TenMinTier > maxTier {
			v.TenMinTier = maxTier
		}
		if !rs.NextFreeAt.IsZero() && rs.Stored < cfg.FreeMax {
			t := rs.NextFreeAt
			v.NextFreeAt = &t
			v.NextFreeInSeconds = int64(math.Max(0, math.Ceil(t.Sub(now).Seconds())))
		}
		for _, h := range cfg.Pity.Hard {
			if h.MinTier > maxTier {
				continue
			}
			since := rs.Pity[strconv.Itoa(h.MinTier)]
			left := h.Every - since
			if left < 1 {
				left = 1
			}
			v.Pity = append(v.Pity, PityView{MinTier: h.MinTier, Every: h.Every, Since: since, Left: left})
		}
		openRate := 0
		for _, t := range cfg.Tiers {
			if has(t.Tier) {
				openRate += t.Rate
			}
		}
		for _, t := range cfg.Tiers {
			if !has(t.Tier) {
				continue
			}
			v.Tiers = append(v.Tiers, TierView{Tier: t.Tier, Name: t.Name, RatePct: float64(t.Rate) / float64(openRate) * 100, DupMinutes: t.DupMinutes})
		}
		st.Rows = append(st.Rows, v)
	}

	ids := make([]string, 0, len(d.Accessories))
	for id := range d.Accessories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if a := s.cat.accs[id]; a != nil {
			st.Accessories = append(st.Accessories, OwnedAccessory{ID: a.ID, Slot: a.Slot, Rarity: a.Rarity, Fits: a.Fits})
		}
	}
	for _, addr := range pet.Owned(grants) {
		stars := 1 // pets outside the pool (the starter Tortoise) are 1-star
		for _, p := range s.cat.Pets {
			if p.Address == addr {
				stars = p.Stars
				break
			}
		}
		st.Pets = append(st.Pets, OwnedPet{ID: addr, Stars: stars, MaxAccessories: stars})
	}
	return st
}

func (s *service) State(ctx context.Context, uid string) (*State, error) {
	var out State
	err := s.repo.Run(ctx, uid, func(d *Doc, g *gem.Doc, grants *grant.Doc) (bool, bool, bool, error) {
		now := s.now().UTC()
		wd, wg := s.touch(d, g, grants, now)
		out = s.state(d, g, grants, now)
		return wd, wg, false, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) Draw(ctx context.Context, uid string, req DrawRequest) (*DrawResponse, error) {
	cfg := s.cat.Rows[req.Row]
	switch {
	case cfg == nil, req.Count != 1 && req.Count != 10, req.RequestID == "":
		return nil, ErrInvalidRequest
	case req.Mode == ModeFree && req.Count != 1:
		return nil, ErrInvalidRequest
	case req.Mode == ModeTicket && (req.Count != 1 || cfg.TicketCost1 == 0):
		return nil, ErrInvalidRequest
	case req.Mode != ModeFree && req.Mode != ModeGem && req.Mode != ModeTicket:
		return nil, ErrInvalidRequest
	}

	var res *DrawResponse
	err := s.repo.Run(ctx, uid, func(d *Doc, g *gem.Doc, grants *grant.Doc) (bool, bool, bool, error) {
		// fn may be retried by the transaction: build everything from scratch each time.
		now := s.now().UTC()
		res = &DrawResponse{}
		s.touch(d, g, grants, now)

		refuse := func(code string) (bool, bool, bool, error) {
			res.ErrorCode = code
			res.State = s.state(d, g, grants, now)
			// The credits and the timer updates are real changes: keep them.
			return true, true, false, nil
		}

		// A retried request: answer with what happened, charge nothing.
		if rec := d.Draws[req.RequestID]; rec != nil {
			if rec.Row != req.Row || rec.Mode != req.Mode || rec.Count != req.Count {
				return refuse(CodeRequestMismatch)
			}
			res.OK, res.Repeated, res.Results = true, true, rec.Results
			for _, id := range rec.GrantIDs {
				if gr := grants.Grants[id]; gr != nil {
					res.Grants = append(res.Grants, *gr)
				}
			}
			res.CashHours = cashHours(rec.Results)
			res.State = s.state(d, g, grants, now)
			return true, true, false, nil
		}

		rs := d.Rows[string(req.Row)]
		var gems, tickets int64
		switch req.Mode {
		case ModeFree:
			if rs.Stored < 1 {
				return refuse(CodeNoFreeDraw)
			}
		case ModeGem:
			gems = cfg.GemPrice1
			if req.Count == 10 {
				gems = cfg.GemPrice10
			}
			if g.Balance < gems {
				return refuse(CodeInsufficientGems)
			}
		case ModeTicket:
			tickets = cfg.TicketCost1
			if d.Tickets < tickets {
				return refuse(CodeInsufficientTicket)
			}
		}

		results, newPets := s.roll(req, cfg, rs, d, grants, now)

		// Grants: one for the new pets, one for the duplicates' cash. Old claimed cash grants make room first.
		rec := &DrawRecord{Row: req.Row, Mode: req.Mode, Count: req.Count, At: now, Results: results, Gems: gems, Tickets: tickets}
		pruneClaimedCash(grants)
		var added []string
		if len(newPets) > 0 {
			rewards := make([]grant.Reward, len(newPets))
			for i, addr := range newPets {
				rewards[i] = grant.Reward{Type: grant.RewardPet, Key: addr, Amount: 1}
			}
			added = append(added, "gacha:"+req.RequestID+":pets")
			if _, err := grants.Add(grant.Grant{ID: added[len(added)-1], Source: grant.SourceGacha, SourceRef: string(req.Row), Rewards: rewards, CreatedAt: now}); err != nil {
				return false, false, false, err
			}
		}
		if hours := cashHours(results); hours > 0 {
			dups := 0
			for _, r := range results {
				if !r.New {
					dups++
				}
			}
			id := "gacha:" + req.RequestID + ":cash"
			added = append(added, id)
			// A TimeWarp reward is hours of the player's current income; the client turns it into coins.
			rw := grant.Reward{Type: grant.RewardTimeWarp, Amount: hours, ItemID: "gacha_duplicate", Count: dups}
			if _, err := grants.Add(grant.Grant{ID: id, Source: grant.SourceGacha, SourceRef: string(req.Row), Rewards: []grant.Reward{rw}, CreatedAt: now}); err != nil {
				return false, false, false, err
			}
		}
		rec.GrantIDs = added

		switch req.Mode {
		case ModeFree:
			rs.useFree(cfg, now)
		case ModeGem:
			g.Balance -= gems
			g.UpdatedAt = now
		case ModeTicket:
			d.Tickets -= tickets
		}
		d.remember(req.RequestID, rec)
		d.UpdatedAt = now

		res.OK, res.Results, res.CashHours = true, results, cashHours(results)
		for _, id := range added {
			res.Grants = append(res.Grants, *grants.Grants[id])
		}
		res.State = s.state(d, g, grants, now)
		return true, true, len(added) > 0, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// roll draws req.Count results for the row, updating its pity counters and the accessory inventory. It returns
// the results and the addresses of the pets that are new to the player.
func (s *service) roll(req DrawRequest, cfg *RowConfig, rs *RowState, d *Doc, grants *grant.Doc, now time.Time) ([]Result, []string) {
	ownedPets := map[string]bool{}
	for _, addr := range pet.Owned(grants) {
		ownedPets[addr] = true
	}
	// The character row draws from the pet pools that are open for this player; a tier with no pet there is not rolled.
	has := allTiers
	var openPets map[int][]*PetDef
	if req.Row == RowCharacter {
		openPets = s.cat.openPetsByTier(grants)
		has = func(tier int) bool { return len(openPets[tier]) > 0 }
	}
	var results []Result
	var newPets []string
	reachedTen := false
	for i := 0; i < req.Count; i++ {
		// The first ever draw of the character row gives a pet the player does not own (the tutorial's second pet).
		if first := s.firstDrawPet(req, cfg, rs, i, ownedPets, grants); first != nil {
			cfg.noteTier(rs, first.Stars, has)
			if first.Stars >= cfg.Pity.TenMinTier {
				reachedTen = true
			}
			ownedPets[first.Address] = true
			newPets = append(newPets, first.Address)
			results = append(results, Result{Kind: "pet", ID: first.ID, Tier: first.Stars, TierName: cfg.tier(first.Stars).Name, New: true})
			continue
		}
		// The guarantee of a x10 draw falls on its last roll when no earlier one reached the tier.
		forceTen := req.Count == 10 && i == req.Count-1 && !reachedTen
		tier, pity := cfg.rollTierAvail(rs, forceTen, s.intn, has)
		if tier >= cfg.Pity.TenMinTier {
			reachedTen = true
		}
		t := cfg.tier(tier)
		r := Result{Tier: tier, TierName: t.Name, Pity: pity, New: true}
		if req.Row == RowCharacter {
			pool := openPets[tier]
			p := pool[s.intn(len(pool))]
			r.Kind, r.ID = "pet", p.ID
			if ownedPets[p.Address] {
				r.New = false
			} else {
				ownedPets[p.Address] = true
				newPets = append(newPets, p.Address)
			}
		} else {
			pool := s.cat.accsByTier[tier]
			a := pool[s.intn(len(pool))]
			r.Kind, r.ID = "accessory", a.ID
			if _, have := d.Accessories[a.ID]; have {
				r.New = false
			} else {
				d.Accessories[a.ID] = now
			}
		}
		if !r.New {
			r.CashMinutes = t.DupMinutes
		}
		results = append(results, r)
	}
	return results, newPets
}

// firstDrawPet is the pet of the row's first ever draw (rs.Total == 0, first roll of the request), picked at random
// among the FirstDraw pets the player does not own; nil when this is not that draw or they are all owned.
func (s *service) firstDrawPet(req DrawRequest, cfg *RowConfig, rs *RowState, i int, owned map[string]bool, grants *grant.Doc) *PetDef {
	if req.Row != RowCharacter || i != 0 || rs.Total != 0 || len(cfg.FirstDraw) == 0 {
		return nil
	}
	var free []*PetDef
	for _, id := range cfg.FirstDraw {
		if p := s.cat.pets[id]; p != nil && !owned[p.Address] && poolUnlocked(grants, p.Pool) {
			free = append(free, p)
		}
	}
	if len(free) == 0 {
		return nil
	}
	return free[s.intn(len(free))]
}

// cashHours is the duplicates' payout in hours of current income.
func cashHours(results []Result) float64 {
	var minutes float64
	for _, r := range results {
		minutes += r.CashMinutes
	}
	return minutes / 60
}

// pruneClaimedCash drops claimed duplicate-cash grants of old draws when the ledger is nearly full, so a long
// run of free draws never fills the 500 grants. (Pet grants stay: they are the ownership record.)
func pruneClaimedCash(grants *grant.Doc) {
	if len(grants.Grants) < grant.MaxGrants-12 {
		return
	}
	for id, g := range grants.Grants {
		if g.Source == grant.SourceGacha && g.ClaimedAt != nil && strings.HasSuffix(id, ":cash") {
			delete(grants.Grants, id)
		}
	}
}
