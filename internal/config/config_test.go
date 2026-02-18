package config

import (
	"os"
	"testing"
)

func TestLoad_MissingToken(t *testing.T) {
	os.Unsetenv("DISCORD_TOKEN")
	_, err := Load()
	if err == nil {
		t.Error("expected error when DISCORD_TOKEN is not set")
	}
}

func TestLoad_MissingForumChannelID(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "test-token")
	t.Setenv("FORUM_CHANNEL_ID", "")
	_, err := Load()
	if err == nil {
		t.Error("expected error when FORUM_CHANNEL_ID is not set")
	}
}

func TestLoad_MissingPrivateCategoryID(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "test-token")
	t.Setenv("FORUM_CHANNEL_ID", "forum-123")
	t.Setenv("PRIVATE_CATEGORY_ID", "")
	_, err := Load()
	if err == nil {
		t.Error("expected error when PRIVATE_CATEGORY_ID is not set")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "test-token")
	t.Setenv("FORUM_CHANNEL_ID", "forum-123")
	t.Setenv("PRIVATE_CATEGORY_ID", "cat-456")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("GUILD_ID", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DiscordToken != "test-token" {
		t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "test-token")
	}
	if cfg.ForumChannelID != "forum-123" {
		t.Errorf("ForumChannelID = %q, want %q", cfg.ForumChannelID, "forum-123")
	}
	if cfg.PrivateCategoryID != "cat-456" {
		t.Errorf("PrivateCategoryID = %q, want %q", cfg.PrivateCategoryID, "cat-456")
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "INFO")
	}
	if cfg.DatabaseURL == "" {
		t.Error("expected DatabaseURL to be populated with defaults")
	}
}

func TestLoad_ExplicitDatabaseURL(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "test-token")
	t.Setenv("FORUM_CHANNEL_ID", "forum-123")
	t.Setenv("PRIVATE_CATEGORY_ID", "cat-456")
	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://user:pass@host:5432/db" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://user:pass@host:5432/db")
	}
}
