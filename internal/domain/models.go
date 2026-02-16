package domain

import "time"

type Group struct {
	ID               int64
	GuildID          string
	Name             string
	OwnerUserID      string
	ForumThreadID    string
	PrivateChannelID string
	Tags             []string
	CreatedAt        time.Time
}

type Membership struct {
	GroupID  int64
	UserID   string
	JoinedAt time.Time
}

type Event struct {
	ID          int64
	GroupID     int64
	Title       string
	StartsAt    time.Time
	DurationMin int
	Notes       string
	CreatedAt   time.Time
}
