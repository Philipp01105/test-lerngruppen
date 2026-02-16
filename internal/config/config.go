package config

import (
	"fmt"
	"net/url"
	"os"
)

type Config struct {
	DiscordToken      string
	GuildID           string
	ForumChannelID    string
	PrivateCategoryID string
	LogLevel          string
	DatabaseURL       string
}

func Load() (*Config, error) {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN environment variable is required")
	}

	forumChannelID := os.Getenv("FORUM_CHANNEL_ID")
	if forumChannelID == "" {
		return nil, fmt.Errorf("FORUM_CHANNEL_ID environment variable is required")
	}

	privateCategoryID := os.Getenv("PRIVATE_CATEGORY_ID")
	if privateCategoryID == "" {
		return nil, fmt.Errorf("PRIVATE_CATEGORY_ID environment variable is required")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		dbUser := os.Getenv("DB_USER")
		if dbUser == "" {
			dbUser = "postgres"
		}
		dbPassword := os.Getenv("DB_PASSWORD")
		dbHost := os.Getenv("DB_HOST")
		if dbHost == "" {
			dbHost = "localhost"
		}
		dbPort := os.Getenv("DB_PORT")
		if dbPort == "" {
			dbPort = "5432"
		}
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "test_lerngruppen"
		}

		userinfo := url.User(dbUser)
		if dbPassword != "" {
			userinfo = url.UserPassword(dbUser, dbPassword)
		}

		u := &url.URL{
			Scheme:   "postgres",
			User:     userinfo,
			Host:     fmt.Sprintf("%s:%s", dbHost, dbPort),
			Path:     fmt.Sprintf("/%s", dbName),
			RawQuery: "sslmode=disable",
		}
		databaseURL = u.String()
	}

	return &Config{
		DiscordToken:      token,
		GuildID:           os.Getenv("GUILD_ID"),
		ForumChannelID:    forumChannelID,
		PrivateCategoryID: privateCategoryID,
		LogLevel:          logLevel,
		DatabaseURL:       databaseURL,
	}, nil
}
