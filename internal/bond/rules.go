package bond

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"api-library/internal/grant"
	"api-library/internal/pet"
)

// validateReport checks the request shape. Unknown pets and seen ids are not errors: they are listed in Rejected.
func validateReport(req ReportRequest) error {
	if n := len(req.BatchID); n < MinBatchIDLength || n > MaxBatchIDLength {
		return fmt.Errorf("%w: batchId must be %d..%d characters", ErrInvalidRequest, MinBatchIDLength, MaxBatchIDLength)
	}
	if len(req.Entries) == 0 || len(req.Entries) > MaxEntries {
		return fmt.Errorf("%w: need 1..%d entries", ErrInvalidRequest, MaxEntries)
	}
	for _, e := range req.Entries {
		if !strings.HasPrefix(e.Pet, "Pet_") || len(e.Pet) > MaxIDLength {
			return fmt.Errorf("%w: bad pet %q", ErrInvalidRequest, e.Pet)
		}
		if len(e.StoreID) > MaxIDLength {
			return fmt.Errorf("%w: storeId too long", ErrInvalidRequest)
		}
		if e.Seconds < 0 || e.Seconds > MaxSecondsPerEntry {
			return fmt.Errorf("%w: seconds must be 0..%d", ErrInvalidRequest, MaxSecondsPerEntry)
		}
		if len(e.Seen) > MaxSeenPerEntry {
			return fmt.Errorf("%w: at most %d seen ids per entry", ErrInvalidRequest, MaxSeenPerEntry)
		}
		for _, id := range e.Seen {
			if len(id) > MaxIDLength {
				return fmt.Errorf("%w: seen id too long", ErrInvalidRequest)
			}
		}
	}
	return nil
}

// ownedSet is pet.Owned as a set: starter pets plus every Pet grant in the ledger, claimed or not (a grant is marked
// claimed only on the client's next save sync, while the client already shows the pet).
func ownedSet(grants *grant.Doc) map[string]bool {
	out := map[string]bool{}
	for _, id := range pet.Owned(grants) {
		out[id] = true
	}
	return out
}

// grantTimes is, per pet, the createdAt of its earliest Pet grant in the ledger.
func grantTimes(grants *grant.Doc) map[string]time.Time {
	out := map[string]time.Time{}
	if grants == nil {
		return out
	}
	for _, g := range grants.Grants {
		if g == nil {
			continue
		}
		for _, r := range g.Rewards {
			if r.Type != grant.RewardPet {
				continue
			}
			if t, ok := out[r.Key]; !ok || g.CreatedAt.Before(t) {
				out[r.Key] = g.CreatedAt
			}
		}
	}
	return out
}

// applyReport applies one batch to doc at server time now. It reports the items it did not accept, and whether the
// batch was a resend of one already applied (then doc is unchanged). Rules (design, "Rules applied on every report"):
//
//  1. Only owned pets count; an unknown pet is rejected and its record is not touched.
//  2. A pet's seconds, summed over its entries, are clamped once to the time since its last report plus ClampSlack;
//     a first report to FirstReportWindow, and never to time before the pet's grant.
//  3. Seconds that arrive during the Cooldown after the last heart are dropped (not rejected: the client is not
//     meant to send them, and must not retry them).
//  4. At HeartSeconds after the cooldown, one heart is awarded. At most one heart per pet per report.
//  5. Seen ids must match seenPattern; seen only grows.
func applyReport(doc *Doc, owned map[string]bool, grantedAt map[string]time.Time, req ReportRequest, now time.Time) (rejected []Rejected, duplicate bool) {
	for _, id := range doc.Batches {
		if id == req.BatchID {
			return nil, true
		}
	}
	if doc.Pets == nil {
		doc.Pets = map[string]*PetBond{}
	}

	type sum struct {
		seconds int
		seen    []string
	}
	perPet := map[string]*sum{}
	var order []string
	for _, e := range req.Entries {
		s := perPet[e.Pet]
		if s == nil {
			s = &sum{}
			perPet[e.Pet] = s
			order = append(order, e.Pet)
		}
		s.seconds += e.Seconds
		s.seen = append(s.seen, e.Seen...)
	}
	sort.Strings(order)

	for _, id := range order {
		s := perPet[id]
		if !owned[id] {
			rejected = append(rejected, Rejected{Pet: id, Item: id, Reason: ReasonUnknownPet})
			continue
		}
		pb := doc.Pets[id]
		if pb == nil {
			pb = &PetBond{}
			doc.Pets[id] = pb
		}

		for _, sid := range s.seen {
			if !seenPattern.MatchString(sid) {
				rejected = append(rejected, Rejected{Pet: id, Item: sid, Reason: ReasonUnknownSeen})
				continue
			}
			pb.addSeen(sid)
		}

		if s.seconds <= 0 {
			continue
		}
		secs := clampSeconds(pb, grantedAt[id], s.seconds, now)
		t := now
		pb.LastReportAt = &t
		if inCooldown(pb, now) {
			continue
		}
		pb.SecondsSinceLastBond += secs
		if pb.SecondsSinceLastBond >= HeartSeconds {
			pb.BondDays++
			b := now
			pb.LastBondAt = &b
			pb.SecondsSinceLastBond = 0
		}
	}

	doc.Batches = append(doc.Batches, req.BatchID)
	if len(doc.Batches) > MaxBatchIDs {
		doc.Batches = doc.Batches[len(doc.Batches)-MaxBatchIDs:]
	}
	doc.UpdatedAt = now
	return rejected, false
}

// clampSeconds limits reported seconds to the real time they could cover.
func clampSeconds(pb *PetBond, grantedAt time.Time, seconds int, now time.Time) int {
	var from time.Time
	if pb.LastReportAt != nil {
		from = *pb.LastReportAt
	} else {
		from = now.Add(-FirstReportWindow)
		if grantedAt.After(from) {
			from = grantedAt
		}
	}
	allowed := now.Sub(from) + ClampSlack
	if allowed < 0 {
		return 0
	}
	if max := int(allowed / time.Second); seconds > max {
		return max
	}
	return seconds
}

func inCooldown(pb *PetBond, now time.Time) bool {
	return pb.LastBondAt != nil && now.Before(pb.LastBondAt.Add(Cooldown))
}

func (pb *PetBond) addSeen(id string) {
	for _, s := range pb.Seen {
		if s == id {
			return
		}
	}
	pb.Seen = append(pb.Seen, id)
}

// LevelFor is the heart level reached at days hearts (0 before the first).
func LevelFor(days int) int {
	level := 0
	for _, n := range Levels {
		if days >= n {
			level++
		}
	}
	return level
}

// nextHeartIn is the whole seconds until the pet's cooldown ends (0 when a heart can fill now).
func nextHeartIn(pb *PetBond, now time.Time) int {
	if pb == nil || pb.LastBondAt == nil {
		return 0
	}
	left := pb.LastBondAt.Add(Cooldown).Sub(now)
	if left <= 0 {
		return 0
	}
	return int(math.Ceil(left.Seconds()))
}

// buildState is the reply: every owned pet, plus any pet with a record (a pet the player no longer has in the ledger
// is impossible today, but its record is still shown rather than hidden).
func buildState(doc *Doc, grants *grant.Doc, now time.Time) StateResponse {
	res := StateResponse{Pets: map[string]PetView{}}
	granted := grantTimes(grants)
	ids := pet.Owned(grants)
	if doc != nil {
		for id := range doc.Pets {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if _, done := res.Pets[id]; done {
			continue
		}
		var pb *PetBond
		if doc != nil {
			pb = doc.Pets[id]
		}
		v := PetView{Seen: []string{}, NextHeartInSeconds: nextHeartIn(pb, now)}
		if pb != nil {
			v.BondDays = pb.BondDays
			v.Level = LevelFor(pb.BondDays)
			v.SecondsSinceLastBond = pb.SecondsSinceLastBond
			v.LastBondAt = pb.LastBondAt
			v.Seen = append(v.Seen, pb.Seen...)
		}
		if t, ok := granted[id]; ok {
			t := t
			v.FirstMetAt = &t
		} else if doc != nil && isStarter(id) {
			t := doc.CreatedAt
			v.FirstMetAt = &t
		}
		res.Pets[id] = v
	}
	return res
}

func isStarter(id string) bool {
	for _, s := range grant.StarterPets {
		if s == id {
			return true
		}
	}
	return false
}
