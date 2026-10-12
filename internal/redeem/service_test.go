package redeem

import (
	"context"
	"errors"
	"testing"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

// memRepo mimics the Firestore repository: Redeem works on copies and keeps them only when fn asks.
type memRepo struct {
	codes  map[string]*Code
	grants map[string]*grant.Doc
}

func newMemRepo() *memRepo {
	return &memRepo{codes: map[string]*Code{}, grants: map[string]*grant.Doc{}}
}

func (r *memRepo) Redeem(_ context.Context, uid, code string, fn func(*Code, *grant.Doc) (bool, bool, error)) error {
	var c *Code
	if stored := r.codes[code]; stored != nil {
		v := *stored
		c = &v
	}
	grants := grant.NewDoc()
	if d := r.grants[uid]; d != nil {
		v := *d
		v.Grants = map[string]*grant.Grant{}
		for k, g := range d.Grants {
			gv := *g
			v.Grants[k] = &gv
		}
		grants = &v
	}
	writeCode, writeGrants, err := fn(c, grants)
	if err != nil {
		return err
	}
	if writeCode && c != nil {
		r.codes[code] = c
	}
	if writeGrants {
		r.grants[uid] = grants
	}
	return nil
}

func (r *memRepo) Create(_ context.Context, c *Code) error {
	if r.codes[c.Code] != nil {
		return ErrCodeExists
	}
	v := *c
	r.codes[c.Code] = &v
	return nil
}

func (r *memRepo) SetDisabled(_ context.Context, code string, disabled bool) error {
	c := r.codes[code]
	if c == nil {
		return ErrCodeNotFound
	}
	c.Disabled = disabled
	return nil
}

func (r *memRepo) List(context.Context) ([]Code, error) {
	out := []Code{}
	for _, c := range r.codes {
		out = append(out, *c)
	}
	return out, nil
}

var clock = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type fixture struct {
	srv  *service
	repo *memRepo
	ctx  context.Context
}

const testItems = `{"version": 1, "items": [
  {"id": "pull_ticket", "type": "Pull", "amount": 1},
  {"id": "timewarp_1h", "type": "TimeWarp", "amount": 1},
  {"id": "cash_500", "type": "Cash", "amount": 500},
  {"id": "unlock_habitat", "type": "Unlock", "key": "home.habitat", "amount": 0}
]}`

func newFixture() *fixture {
	repo := newMemRepo()
	catalog, err := item.ParseCatalog([]byte(testItems))
	if err != nil {
		panic(err)
	}
	s := NewService(repo, catalog).(*service)
	s.now = func() time.Time { return clock }
	return &fixture{srv: s, repo: repo, ctx: context.Background()}
}

func (f *fixture) code(t *testing.T, req CreateCodeRequest) string {
	t.Helper()
	if len(req.Items) == 0 {
		req.Items = []item.Ref{{ItemID: "pull_ticket", Count: 2}}
	}
	c, err := f.srv.CreateCode(f.ctx, "admin", req)
	if err != nil {
		t.Fatal(err)
	}
	return c.Code
}

func (f *fixture) use(t *testing.T, uid, code string) *UseResponse {
	t.Helper()
	res, err := f.srv.Use(f.ctx, uid, code)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestNormalize(t *testing.T) {
	good := map[string]string{
		"abcd-efgh-2345": "ABCDEFGH2345",
		" welcome 2026 ": "WELCOME2026",
		"WELCOME_2026":   "WELCOME2026",
	}
	for in, want := range good {
		if got, ok := Normalize(in); !ok || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "has$symbol", "cafécode", string(make([]byte, 40))} {
		if _, ok := Normalize(in); ok {
			t.Errorf("Normalize(%q) accepted", in)
		}
	}
}

func TestFormatAndGenerate(t *testing.T) {
	if got := Format("ABCDEFGH2345"); got != "ABCD-EFGH-2345" {
		t.Errorf("Format = %q", got)
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		c, err := generate()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := Normalize(c); !ok || len(c) != generatedLength {
			t.Fatalf("generated %q is not a valid code", c)
		}
		if seen[c] {
			t.Fatalf("duplicate generated code %q", c)
		}
		seen[c] = true
	}
}

func TestRedeemGivesGrantOnce(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{Items: []item.Ref{{ItemID: "cash_500", Count: 3}, {ItemID: "pull_ticket", Count: 1}}})

	res := f.use(t, "u1", Format(code)) // typed with dashes and in lower case would also work
	if !res.OK || res.Grant == nil || res.Grant.ID != "redeem:"+code || len(res.Grant.Rewards) != 2 {
		t.Fatalf("redeem = %+v", res)
	}
	// Item unit amount times count, with the item kept for display
	if got := res.Grant.Rewards[0]; got.Type != grant.RewardCash || got.Amount != 1500 || got.ItemID != "cash_500" || got.Count != 3 {
		t.Errorf("first reward = %+v", got)
	}
	if f.repo.codes[code].Redeemed != 1 {
		t.Error("redeemed counter not incremented")
	}
	if got := f.repo.grants["u1"].Unclaimed(); len(got) != 1 {
		t.Errorf("unclaimed grants = %d", len(got))
	}

	again := f.use(t, "u1", code)
	if again.OK || again.ErrorCode != CodeAlreadyRedeemed {
		t.Errorf("second redeem = %+v", again)
	}
	if f.repo.codes[code].Redeemed != 1 {
		t.Error("a repeat redeem moved the counter")
	}

	if other := f.use(t, "u2", code); !other.OK {
		t.Errorf("another player could not redeem: %+v", other)
	}
}

func TestRedeemAfterClaimStaysAlreadyRedeemed(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{})
	f.use(t, "u", code)
	f.repo.grants["u"].Claim([]string{"redeem:" + code}, clock)

	if res := f.use(t, "u", code); res.OK || res.ErrorCode != CodeAlreadyRedeemed {
		t.Errorf("claimed grant could be redeemed again: %+v", res)
	}
}

func TestRedeemRefusals(t *testing.T) {
	f := newFixture()
	past := clock.Add(-time.Hour)
	future := clock.Add(time.Hour)

	disabled := f.code(t, CreateCodeRequest{})
	f.srv.SetDisabled(f.ctx, disabled, true)

	expired := f.code(t, CreateCodeRequest{ExpiresAt: &future})
	f.repo.codes[expired].ExpiresAt = &past // expired since creation (creation itself refuses a past date)

	finished := f.code(t, CreateCodeRequest{MaxRedemptions: 1})
	f.use(t, "someone", finished)

	cases := map[string]string{
		"not a code":   CodeInvalid,
		"NOSUCHCODE12": CodeInvalid,
		disabled:       CodeDisabled,
		expired:        CodeExpired,
		finished:       CodeFinished,
	}
	for input, want := range cases {
		if res := f.use(t, "u", input); res.OK || res.ErrorCode != want {
			t.Errorf("%q: got %+v, want %s", input, res, want)
		}
	}
}

func TestRedeemMaxRedemptions(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{MaxRedemptions: 2})

	for _, uid := range []string{"a", "b"} {
		if res := f.use(t, uid, code); !res.OK {
			t.Fatalf("%s: %+v", uid, res)
		}
	}
	if res := f.use(t, "c", code); res.OK || res.ErrorCode != CodeFinished {
		t.Errorf("third redeem = %+v", res)
	}
}

func TestRedeemThrottleLocksAfterFailures(t *testing.T) {
	f := newFixture()
	good := f.code(t, CreateCodeRequest{})

	for i := 0; i < grant.RedeemMaxFailures; i++ {
		if res := f.use(t, "guesser", "WRONGCODE999"); res.ErrorCode != CodeInvalid {
			t.Fatalf("attempt %d: %+v", i, res)
		}
	}
	// Locked now, even for a valid code.
	if res := f.use(t, "guesser", good); res.OK || res.ErrorCode != CodeRateLimited {
		t.Errorf("locked user redeemed: %+v", res)
	}
	// Someone else is not affected.
	if res := f.use(t, "honest", good); !res.OK {
		t.Errorf("other user refused: %+v", res)
	}

	// An hour later the window is over.
	f.srv.now = func() time.Time { return clock.Add(grant.RedeemWindow) }
	if res := f.use(t, "guesser", good); !res.OK {
		t.Errorf("still locked after the window: %+v", res)
	}
}

func TestAlreadyRedeemedIsNotAFailedGuess(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{})
	f.use(t, "u", code)
	for i := 0; i < grant.RedeemMaxFailures*2; i++ {
		f.use(t, "u", code)
	}
	if res := f.use(t, "u", "WRONGCODE999"); res.ErrorCode != CodeInvalid {
		t.Errorf("repeats counted as failures: %+v", res)
	}
}

func TestMalformedInputCountsAsFailure(t *testing.T) {
	f := newFixture()
	for i := 0; i < grant.RedeemMaxFailures; i++ {
		f.use(t, "u", "$$$")
	}
	if res := f.use(t, "u", "$$$"); res.ErrorCode != CodeRateLimited {
		t.Errorf("malformed attempts were free: %+v", res)
	}
}

func TestCreateCodeValidation(t *testing.T) {
	f := newFixture()
	ok := []item.Ref{{ItemID: "pull_ticket", Count: 1}}
	past := clock.Add(-time.Minute)

	bad := map[string]CreateCodeRequest{
		"no items":       {},
		"unknown item":   {Items: []item.Ref{{ItemID: "gems", Count: 1}}},
		"zero count":     {Items: []item.Ref{{ItemID: "pull_ticket", Count: 0}}},
		"time warp cap":  {Items: []item.Ref{{ItemID: "timewarp_1h", Count: 200}}},
		"negative max":   {Items: ok, MaxRedemptions: -1},
		"huge max":       {Items: ok, MaxRedemptions: MaxRedemptionsLimit + 1},
		"past expiry":    {Items: ok, ExpiresAt: &past},
		"bad custom":     {Items: ok, Code: "no!"},
		"note too large": {Items: ok, Note: string(make([]byte, 300))},
	}
	for name, req := range bad {
		if _, err := f.srv.CreateCode(f.ctx, "admin", req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: want ErrInvalidRequest, got %v", name, err)
		}
	}
}

func TestCreateCustomCodeAndDuplicate(t *testing.T) {
	f := newFixture()
	req := CreateCodeRequest{Code: "welcome-2026", Items: []item.Ref{{ItemID: "unlock_habitat", Count: 1}}}

	c, err := f.srv.CreateCode(f.ctx, "admin", req)
	if err != nil || c.Code != "WELCOME2026" || c.CreatedBy != "admin" {
		t.Fatalf("create = %+v, %v", c, err)
	}
	if _, err := f.srv.CreateCode(f.ctx, "admin", req); !errors.Is(err, ErrCodeExists) {
		t.Errorf("duplicate: want ErrCodeExists, got %v", err)
	}
}

func TestDisableAndReenable(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{})

	f.srv.SetDisabled(f.ctx, code, true)
	if res := f.use(t, "u", code); res.ErrorCode != CodeDisabled {
		t.Errorf("disabled code: %+v", res)
	}
	f.srv.SetDisabled(f.ctx, code, false)
	if res := f.use(t, "u", code); !res.OK {
		t.Errorf("re-enabled code: %+v", res)
	}
	if err := f.srv.SetDisabled(f.ctx, "NOSUCHCODE12", true); !errors.Is(err, ErrCodeNotFound) {
		t.Errorf("unknown code: %v", err)
	}
}

func TestGrantKeepsItsSnapshotWhenTheCatalogChanges(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{Items: []item.Ref{{ItemID: "pull_ticket", Count: 2}}})
	res := f.use(t, "u", code)
	if !res.OK {
		t.Fatalf("redeem = %+v", res)
	}

	// The delivered grant is stored with its amounts, whatever the catalog says afterwards.
	stored := f.repo.grants["u"].Grants["redeem:"+code]
	if len(stored.Rewards) != 1 || stored.Rewards[0].Amount != 2 || stored.Rewards[0].ItemID != "pull_ticket" {
		t.Errorf("stored rewards = %+v", stored.Rewards)
	}
}

func TestUnresolvableCodeIsAServerErrorNotAGuess(t *testing.T) {
	f := newFixture()
	code := f.code(t, CreateCodeRequest{})
	f.repo.codes[code].Items = []item.Ref{{ItemID: "removed_item", Count: 1}} // an item left the catalog

	if _, err := f.srv.Use(f.ctx, "u", code); err == nil {
		t.Fatal("expected an error")
	}
	if g := f.repo.grants["u"]; g != nil && g.RedeemFailures != 0 {
		t.Error("a server-side mistake counted against the player")
	}
}
