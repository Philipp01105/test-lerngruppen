package store

import (
	"database/sql"
	"fmt"
	"time"

	"test-lerngruppen/internal/domain"

	"github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

func New(dbURL string) (*Store, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS groups (
		id SERIAL PRIMARY KEY,
		guild_id TEXT NOT NULL,
		name TEXT NOT NULL,
		owner_user_id TEXT NOT NULL,
		forum_thread_id TEXT UNIQUE NOT NULL,
		private_channel_id TEXT UNIQUE NOT NULL,
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS memberships (
		group_id INTEGER NOT NULL,
		user_id TEXT NOT NULL,
		joined_at TIMESTAMP NOT NULL,
		PRIMARY KEY (group_id, user_id),
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS events (
		id SERIAL PRIMARY KEY,
		group_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		starts_at TIMESTAMP NOT NULL,
		duration_min INTEGER DEFAULT 0,
		notes TEXT,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Add tags column if it doesn't exist
	alterSchema := `
	DO $$ 
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name='groups' AND column_name='tags'
		) THEN
			ALTER TABLE groups ADD COLUMN tags TEXT[] DEFAULT ARRAY[]::TEXT[];
		END IF;
	END $$;
	`

	_, err := s.db.Exec(alterSchema)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateGroup(group *domain.Group) error {
	err := s.db.QueryRow(
		`INSERT INTO groups (guild_id, name, owner_user_id, forum_thread_id, private_channel_id, tags, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		group.GuildID, group.Name, group.OwnerUserID, group.ForumThreadID, group.PrivateChannelID, pq.Array(group.Tags), group.CreatedAt,
	).Scan(&group.ID)
	return err
}

func (s *Store) GetGroupByID(id int64) (*domain.Group, error) {
	group := &domain.Group{}
	err := s.db.QueryRow(
		`SELECT id, guild_id, name, owner_user_id, forum_thread_id, private_channel_id, tags, created_at
		 FROM groups WHERE id = $1`,
		id,
	).Scan(&group.ID, &group.GuildID, &group.Name, &group.OwnerUserID, &group.ForumThreadID, &group.PrivateChannelID, pq.Array(&group.Tags), &group.CreatedAt)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Store) GetGroupByForumThreadID(forumThreadID string) (*domain.Group, error) {
	group := &domain.Group{}
	err := s.db.QueryRow(
		`SELECT id, guild_id, name, owner_user_id, forum_thread_id, private_channel_id, tags, created_at
		 FROM groups WHERE forum_thread_id = $1`,
		forumThreadID,
	).Scan(&group.ID, &group.GuildID, &group.Name, &group.OwnerUserID, &group.ForumThreadID, &group.PrivateChannelID, pq.Array(&group.Tags), &group.CreatedAt)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Store) GetGroupsByOwner(ownerUserID string) ([]*domain.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, guild_id, name, owner_user_id, forum_thread_id, private_channel_id, tags, created_at
		 FROM groups WHERE owner_user_id = $1`,
		ownerUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*domain.Group
	for rows.Next() {
		group := &domain.Group{}
		if err := rows.Scan(&group.ID, &group.GuildID, &group.Name, &group.OwnerUserID, &group.ForumThreadID, &group.PrivateChannelID, pq.Array(&group.Tags), &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) GetAllGroups(guildID string) ([]*domain.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, guild_id, name, owner_user_id, forum_thread_id, private_channel_id, tags, created_at
		 FROM groups WHERE guild_id = $1`,
		guildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*domain.Group
	for rows.Next() {
		group := &domain.Group{}
		if err := rows.Scan(&group.ID, &group.GuildID, &group.Name, &group.OwnerUserID, &group.ForumThreadID, &group.PrivateChannelID, pq.Array(&group.Tags), &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) DeleteGroup(id int64) error {
	_, err := s.db.Exec(`DELETE FROM groups WHERE id = $1`, id)
	return err
}

func (s *Store) AddMember(groupID int64, userID string) error {
	_, err := s.db.Exec(
		`INSERT INTO memberships (group_id, user_id, joined_at)
		 VALUES ($1, $2, $3)`,
		groupID, userID, time.Now(),
	)
	return err
}

func (s *Store) RemoveMember(groupID int64, userID string) error {
	_, err := s.db.Exec(
		`DELETE FROM memberships WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	)
	return err
}

func (s *Store) IsMember(groupID int64, userID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memberships WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	).Scan(&count)
	return count > 0, err
}

func (s *Store) GetMembers(groupID int64) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT user_id FROM memberships WHERE group_id = $1`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		members = append(members, userID)
	}
	return members, rows.Err()
}

func (s *Store) GetUserGroups(userID string) ([]*domain.Group, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.guild_id, g.name, g.owner_user_id, g.forum_thread_id, g.private_channel_id, g.tags, g.created_at
		 FROM groups g
		 INNER JOIN memberships m ON g.id = m.group_id
		 WHERE m.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*domain.Group
	for rows.Next() {
		group := &domain.Group{}
		if err := rows.Scan(&group.ID, &group.GuildID, &group.Name, &group.OwnerUserID, &group.ForumThreadID, &group.PrivateChannelID, pq.Array(&group.Tags), &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) CreateEvent(event *domain.Event) error {
	err := s.db.QueryRow(
		`INSERT INTO events (group_id, title, starts_at, duration_min, notes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		event.GroupID, event.Title, event.StartsAt, event.DurationMin, event.Notes, event.CreatedAt,
	).Scan(&event.ID)
	return err
}

func (s *Store) GetEventsByGroupID(groupID int64) ([]*domain.Event, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, title, starts_at, duration_min, notes, created_at
		 FROM events WHERE group_id = $1
		 ORDER BY starts_at ASC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.Event
	for rows.Next() {
		event := &domain.Event{}
		if err := rows.Scan(&event.ID, &event.GroupID, &event.Title, &event.StartsAt, &event.DurationMin, &event.Notes, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) DeleteEvent(eventID int64) error {
	_, err := s.db.Exec(`DELETE FROM events WHERE id = $1`, eventID)
	return err
}
