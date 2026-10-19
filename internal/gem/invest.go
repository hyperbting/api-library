package gem

// Investment (GDD 5.3.1, decided 2026-10-20; the zh-Hant name is 投資): a permanent upgrade, sold in the Shop for
// gems, of how many payout hours one rewarded warp ad gives. The level is server-owned in the gems document. The
// day's warp allowance (MaxWarpHoursPerDay) is shared with the gem warps, so the upgrade does not raise the daily
// maximum, it lowers the number of ads needed to reach it.

// ProductInvest raises the Investment level by one. Its price depends on the level, so it is not in Offers.
const ProductInvest Product = "invest_up"

// CodeInvestMax: the level is already the highest one.
const CodeInvestMax = "invest_max"

// InvestHours is the payout hours per warp ad at each level (index = level). Level 0 is the 1 hour of the first
// build. Working numbers for testing (user, 2026-10-20: +0.5 h per level, level 3 is the cap for now).
var InvestHours = []float64{1.0, 1.5, 2.0, 2.5}

// InvestPrices is the gems the next level costs (index = the level being left). 25, 50, 75 (user, 2026-10-20).
var InvestPrices = []int64{25, 50, 75}

// MaxInvestLevel is the highest level.
func MaxInvestLevel() int { return len(InvestHours) - 1 }

// investLevel is the stored level held inside the valid range, so a bad document never indexes out of bounds.
func (d *Doc) investLevel() int {
	switch {
	case d.InvestLevel < 0:
		return 0
	case d.InvestLevel > MaxInvestLevel():
		return MaxInvestLevel()
	}
	return d.InvestLevel
}

// InvestAdHours is the payout hours one warp ad gives this player now.
func (d *Doc) InvestAdHours() float64 { return InvestHours[d.investLevel()] }

// InvestNextPrice is what the next level costs, 0 at the top.
func (d *Doc) InvestNextPrice() int64 {
	if l := d.investLevel(); l < len(InvestPrices) {
		return InvestPrices[l]
	}
	return 0
}

// InvestNextAdHours is the payout hours per ad at the next level, 0 at the top.
func (d *Doc) InvestNextAdHours() float64 {
	if l := d.investLevel(); l < MaxInvestLevel() {
		return InvestHours[l+1]
	}
	return 0
}

// spendInvest buys the next level. A bought level is permanent; the grant ledger is untouched.
func (s *service) spendInvest(d *Doc, req SpendRequest, res *SpendResponse, refuse func(string) (bool, bool, error)) (bool, bool, error) {
	price := d.InvestNextPrice()
	if price == 0 {
		return refuse(CodeInvestMax)
	}
	if d.Balance < price {
		return refuse(CodeInsufficientGems)
	}
	now := s.now()
	d.InvestLevel = d.investLevel() + 1
	d.Balance -= price
	d.remember(req.RequestID, &SpendRecord{Product: ProductInvest, Price: price, At: now, BalanceAfter: d.Balance})
	res.OK = true
	res.State = s.state(d, now)
	return true, false, nil
}
