package gacha

import "strconv"

// rollTier picks the tier of one draw and updates the row's pity counters.
//
// A hard pity (MinTier, Every) forces a result of MinTier or better when Every-1 draws in a row fell short; the
// guarantee of a x10 draw (forceTen) is the same rule with the tier Pity.TenMinTier. The roll then runs over the
// tiers at or above the floor with their normal rates, so a forced draw is not always the lowest tier of the
// guarantee. Free, gem and ticket draws all go through here, so they share the counters.
func (c *RowConfig) rollTier(rs *RowState, forceTen bool, intn func(int) int) (tier int, pity bool) {
	floor := 1
	for _, h := range c.Pity.Hard {
		if rs.Pity[strconv.Itoa(h.MinTier)]+1 >= h.Every && h.MinTier > floor {
			floor = h.MinTier
		}
	}
	if forceTen && c.Pity.TenMinTier > floor {
		floor = c.Pity.TenMinTier
	}

	total := 0
	for _, t := range c.Tiers[floor-1:] {
		total += t.Rate
	}
	n := intn(total)
	tier = len(c.Tiers)
	for _, t := range c.Tiers[floor-1:] {
		if n < t.Rate {
			tier = t.Tier
			break
		}
		n -= t.Rate
	}

	// For the reveal: a pity rule was in force for this draw (the floor was raised above the plain table).
	pity = floor > 1
	for _, h := range c.Pity.Hard {
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
