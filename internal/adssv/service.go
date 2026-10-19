package adssv

import (
	"context"
	"time"
)

// RecordTTL is how long a verified callback is kept. The reward route asks for it within minutes of the ad; the
// ExpireAt field lets a Firestore TTL policy on it delete old documents (optional, set up in the console).
const RecordTTL = 14 * 24 * time.Hour

// Record is one stored, verified callback. The document ID is the transaction ID, so a replayed callback is
// stored once.
type Record struct {
	TransactionID string    `firestore:"transactionId"`
	UID           string    `firestore:"uid"`
	RequestID     string    `firestore:"requestId"` // the callback's custom_data
	AdUnit        string    `firestore:"adUnit"`
	RewardItem    string    `firestore:"rewardItem,omitempty"`
	RewardAmount  int       `firestore:"rewardAmount,omitempty"`
	At            time.Time `firestore:"at"` // when the ad was watched, from the callback
	ReceivedAt    time.Time `firestore:"receivedAt"`
	ExpireAt      time.Time `firestore:"expireAt"`
}

type Repository interface {
	// Save stores the record unless one with that transaction ID exists (a replay), which is not an error.
	Save(ctx context.Context, r Record) error
	// Has reports whether a verified callback for this player and request ID exists.
	Has(ctx context.Context, uid, requestID string) (bool, error)
}

type Service struct {
	verifier *Verifier
	repo     Repository
	now      func() time.Time
}

func NewService(v *Verifier, repo Repository) *Service {
	return &Service{verifier: v, repo: repo, now: time.Now}
}

// Handle verifies a callback's raw query string and stores it.
func (s *Service) Handle(ctx context.Context, rawQuery string) (*Callback, error) {
	cb, err := s.verifier.Verify(ctx, rawQuery)
	if err != nil {
		return nil, err
	}
	now := s.now()
	err = s.repo.Save(ctx, Record{
		TransactionID: cb.TransactionID,
		UID:           cb.UserID,
		RequestID:     cb.CustomData,
		AdUnit:        cb.AdUnit,
		RewardItem:    cb.RewardItem,
		RewardAmount:  cb.RewardAmount,
		At:            cb.At,
		ReceivedAt:    now,
		ExpireAt:      now.Add(RecordTTL),
	})
	if err != nil {
		return nil, err
	}
	return cb, nil
}

// Verified implements gem.AdVerifier.
func (s *Service) Verified(ctx context.Context, uid, requestID string) (bool, error) {
	return s.repo.Has(ctx, uid, requestID)
}
