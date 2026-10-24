package gacha

import "strconv"

// allTiers: every tier of the row has something to give.
func allTiers(int) bool { return true }

// maxAvailable is the highest tier that has something to give (0 when none).
func (c *RowConfig) maxAvailable(has func(int) bool) int {
	max := 0
	for _, t := range c.Tiers {
		if has(t.Tier) && t.Tier > max {
			max = t.Tier
		}
	}
	return max
}

// noteTier counts a draw that did not go through rollTier (the first draw's guaranteed pet) in the pity counters,
// as a result of that tier.
func (c *RowConfig) noteTier(rs *RowState, tier int, has func(int) bool) {
	maxTier := c.maxAvailable(has)
	for _, h := range c.Pity.Hard {
		if h.MinTier > maxTier {
			continue // nothing of that tier is in the pool yet: its counter waits
		}
		key := strconv.Itoa(h.MinTier)
		if tier >= h.MinTier {
			rs.Pity[key] = 0
		} else {
			rs.Pity[key]++
		}
	}
	rs.Total++
}

// rollTier picks the tier of one draw and updates the row's pity counters, over every tier of the row.
func (c *RowConfig) rollTier(rs *RowState, forceTen bool, intn func(int) int) (tier int, pity bool) {
	return c.rollTierAvail(rs, forceTen, intn, allTiers)
}

// rollTierAvail is rollTier over the tiers that have something to give (`has`): the character row opens its
// 4- and 5-star pets with a later pool, so until then those tiers are not rolled, their rates go to the others and
// their pity rules wait (GDD 5.2.2).
//
// A hard pity (MinTier, Every) forces a result of MinTier or better when Every-1 draws in a row fell short; the
// guarantee of a x10 draw (forceTen) is the same rule with the tier Pity.TenMinTier. The roll then runs over the
// tiers at or above the floor with their normal rates, so a forced draw is not always the lowest tier of the
// guarantee. Free, gem and ticket draws all go through here, so they share the counters.
func (c *RowConfig) rollTierAvail(rs *RowState, forceTen bool, intn func(int) int, has func(int) bool) (tier int, pity bool) {
	maxTier := c.maxAvailable(has)
	floor := 1
	for _, h := range c.Pity.Hard {
		if h.MinTier <= maxTier && rs.Pity[strconv.Itoa(h.MinTier)]+1 >= h.Every && h.MinTier > floor {
			floor = h.MinTier
		}
	}
	if forceTen && c.Pity.TenMinTier <= maxTier && c.Pity.TenMinTier > floor {
		floor = c.Pity.TenMinTier
	}

	total := 0
	for _, t := range c.Tiers[floor-1:] {
		if has(t.Tier) {
			total += t.Rate
		}
	}
	n := intn(total)
	tier = maxTier
	for _, t := range c.Tiers[floor-1:] {
		if !has(t.Tier) {
			continue
		}
		if n < t.Rate {
			tier = t.Tier
			break
		}
		n -= t.Rate
	}

	// For the reveal: a pity rule was in force for this draw (the floor was raised above the plain table).
	pity = floor > 1
	for _, h := range c.Pity.Hard {
		if h.MinTier > maxTier {
			continue
		}
		key := strconv.Itoa(h.MinTier)
		if tier >= h.MinTier {
			rs.Pity[key] = 0
		} else {
			rs.Pity[key]++
		}
	}
	rs.Total++
	return tier, pity
}
