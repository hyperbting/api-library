package gacha

import (
	"context"
	"log"
	"time"
)

// The store tutorial's character draw lessons (gacha_shop, gacha_open, gacha_character) teach the free draw button.
// A row that already spent its free draw (a developer who reset the quests and plays the tutorial again, an old row)
// would leave the lesson with nothing to press. While the lessons are on, the character row therefore always holds at
// least one stored free draw. The gate reads the player's quest progress, so it cannot be used to skip the free-draw
// timer outside the lesson, and it adds at most one draw at a time.

// LessonGate says whether the player's quest progress is on the character draw lessons right now.
type LessonGate interface {
	CharacterDrawLessonOn(ctx context.Context, uid string) (bool, error)
}

// LessonGateFunc adapts a function to LessonGate.
type LessonGateFunc func(ctx context.Context, uid string) (bool, error)

func (f LessonGateFunc) CharacterDrawLessonOn(ctx context.Context, uid string) (bool, error) {
	return f(ctx, uid)
}

// Option configures NewService.
type Option func(*service)

// WithLessonGate makes the character row hold a free draw while the gate says the draw lessons are on.
func WithLessonGate(g LessonGate) Option {
	return func(s *service) { s.lesson = g }
}

// lessonOn asks the gate. The read is not part of the gacha transaction: a stale answer can only mean "not yet" or
// "one more draw ready" for a moment. A gate error counts as off (the draw rules stay as they are).
func (s *service) lessonOn(ctx context.Context, uid string) bool {
	if s.lesson == nil {
		return false
	}
	on, err := s.lesson.CharacterDrawLessonOn(ctx, uid)
	if err != nil {
		log.Printf("[gacha] lesson gate uid=%s: %v", uid, err)
		return false
	}
	return on
}

// ensureLessonDraw raises the character row to one stored free draw when it has none. It reports whether it changed.
func (s *service) ensureLessonDraw(d *Doc, now time.Time) bool {
	cfg := s.cat.Rows[RowCharacter]
	if cfg == nil {
		return false
	}
	rs, _ := d.openRow(cfg, RowCharacter, now)
	if rs.Stored >= 1 {
		return false
	}
	rs.Stored = 1
	if rs.Stored >= cfg.FreeMax {
		rs.NextFreeAt = time.Time{}
	} else if rs.NextFreeAt.IsZero() || rs.NextFreeAt.Before(now) {
		rs.NextFreeAt = now.Add(cfg.interval())
	}
	d.UpdatedAt = now
	return true
}
