package discord

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"test-lerngruppen/internal/config"
	"test-lerngruppen/internal/domain"
	"test-lerngruppen/internal/store"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session *discordgo.Session
	config  *config.Config
	store   *store.Store
}

func NewBot(cfg *config.Config, st *store.Store) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	bot := &Bot{
		session: session,
		config:  cfg,
		store:   st,
	}

	session.AddHandler(bot.interactionCreate)

	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	return bot, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	log.Printf("%s is Running\n", b.session.State.User.Username)

	if err := b.registerCommands(); err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}

	log.Println("Commands registered successfully")
	return nil
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) registerCommands() error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "group",
			Description: "Lerngruppen verwalten",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "create",
					Description: "Neue Lerngruppe erstellen",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "delete",
					Description: "Lerngruppe löschen",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "thread_id",
							Description: "Forum-Thread-ID der Gruppe",
							Required:    true,
						},
					},
				},
			},
		},
		{
			Name:        "event",
			Description: "Lerngruppen-Events verwalten",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "create",
					Description: "Neues Event erstellen (nur für Besitzer)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "thread_id",
							Description: "Forum-Thread-ID der Gruppe",
							Required:    true,
						},
					},
				},
			},
		},
	}

	guildID := b.config.GuildID
	for _, cmd := range commands {
		if _, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, guildID, cmd); err != nil {
			return fmt.Errorf("failed to create command %s: %w", cmd.Name, err)
		}
	}

	return nil
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleCommand(s, i)
	case discordgo.InteractionModalSubmit:
		b.handleModalSubmit(s, i)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, i)
	}
}

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	switch data.Name {
	case "group":
		b.handleGroupCommand(s, i)
	case "event":
		b.handleEventCommand(s, i)
	}
}

func (b *Bot) handleGroupCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}

	subcommand := data.Options[0]

	switch subcommand.Name {
	case "create":
		b.handleGroupCreateStart(s, i)
	case "delete":
		b.handleGroupDelete(s, i, subcommand.Options)
	}
}

func (b *Bot) handleGroupCreateStart(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "group_create_modal",
			Title:    "Neue Lerngruppe erstellen",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "group_name",
							Label:       "Gruppenname",
							Style:       discordgo.TextInputShort,
							Required:    true,
							MaxLength:   100,
							Placeholder: "z.B. Mathe-Lerngruppe",
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("Failed to show group create modal: %v", err)
	}
}

func (b *Bot) handleGroupDelete(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	threadID := options[0].StringValue()

	// Get group from database
	group, err := b.store.GetGroupByForumThreadID(threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			b.respondError(s, i.Interaction, "Gruppe nicht gefunden")
		} else {
			b.respondError(s, i.Interaction, fmt.Sprintf("Fehler beim Abrufen der Gruppe: %v", err))
		}
		return
	}

	userID := i.Member.User.ID

	// Check if user is owner
	if group.OwnerUserID != userID {
		b.respondError(s, i.Interaction, "Nur der Gruppenbesitzer kann die Gruppe löschen")
		return
	}

	// Acknowledge the interaction
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	// Delete the private channel
	_, err = s.ChannelDelete(group.PrivateChannelID)
	if err != nil {
		log.Printf("Failed to delete private channel: %v", err)
	}

	// Delete the forum thread
	_, err = s.ChannelDelete(group.ForumThreadID)
	if err != nil {
		log.Printf("Failed to delete forum thread: %v", err)
	}

	// Delete from database (this will cascade delete memberships and events)
	if err := b.store.DeleteGroup(group.ID); err != nil {
		b.followUpError(s, i.Interaction, fmt.Sprintf("Fehler beim Löschen der Gruppe: %v", err))
		return
	}

	// Follow up with success message
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: strPtr(fmt.Sprintf("✅ Lerngruppe **%s** wurde erfolgreich gelöscht.", group.Name)),
	})
	if err != nil {
		log.Printf("Failed to edit interaction response: %v", err)
	}
}

func (b *Bot) handleEventCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}

	subcommand := data.Options[0]

	switch subcommand.Name {
	case "create":
		b.handleEventCreate(s, i, subcommand.Options)
	}
}

func (b *Bot) handleEventCreate(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	threadID := options[0].StringValue()

	// Get group from database
	group, err := b.store.GetGroupByForumThreadID(threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			b.respondError(s, i.Interaction, "Gruppe nicht gefunden")
		} else {
			b.respondError(s, i.Interaction, fmt.Sprintf("Fehler beim Abrufen der Gruppe: %v", err))
		}
		return
	}

	userID := i.Member.User.ID

	// Check if user is owner
	if group.OwnerUserID != userID {
		b.respondError(s, i.Interaction, "Nur der Gruppenbesitzer kann Events erstellen")
		return
	}

	// Show modal for event creation
	b.showEventModal(s, i, group.ID)
}

func (b *Bot) showEventModal(s *discordgo.Session, i *discordgo.InteractionCreate, groupID int64) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: fmt.Sprintf("event_create_modal_%d", groupID),
			Title:    "Event erstellen",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "event_title",
							Label:       "Event-Titel",
							Style:       discordgo.TextInputShort,
							Required:    true,
							MaxLength:   100,
							Placeholder: "Study session",
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "event_date",
							Label:       "Datum (JJJJ-MM-TT)",
							Style:       discordgo.TextInputShort,
							Required:    true,
							Placeholder: "2024-12-25",
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "event_time",
							Label:       "Zeit (HH:MM, optional)",
							Style:       discordgo.TextInputShort,
							Required:    false,
							Placeholder: "14:30",
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "event_duration",
							Label:       "Dauer (Minuten, optional)",
							Style:       discordgo.TextInputShort,
							Required:    false,
							Placeholder: "60",
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "event_notes",
							Label:       "Notizen (optional)",
							Style:       discordgo.TextInputParagraph,
							Required:    false,
							MaxLength:   500,
							Placeholder: "Additional information...",
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("Failed to show event modal: %v", err)
	}
}

func (b *Bot) handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	if data.CustomID == "group_create_modal" {
		b.handleGroupCreateModal(s, i)
	} else if strings.HasPrefix(data.CustomID, "event_create_modal_") {
		b.handleEventCreateModal(s, i)
	}
}

func (b *Bot) handleGroupCreateModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	// Extract group name
	groupName := ""
	for _, row := range data.Components {
		for _, component := range row.(*discordgo.ActionsRow).Components {
			switch comp := component.(type) {
			case *discordgo.TextInput:
				if comp.CustomID == "group_name" {
					groupName = comp.Value
				}
			}
		}
	}

	// Validate group name
	if groupName == "" {
		b.respondError(s, i.Interaction, "Gruppenname darf nicht leer sein")
		return
	}

	// Define available tags
	tagOptions := []discordgo.SelectMenuOption{
		{Label: "Abschlussprüfung 1", Value: "Abschlussprüfung 1"},
		{Label: "Abschlussprüfung 2", Value: "Abschlussprüfung 2"},
		{Label: "FIAE", Value: "FIAE"},
		{Label: "FISI", Value: "FISI"},
		{Label: "FIDP", Value: "FIDP"},
		{Label: "FIDV", Value: "FIDV"},
		{Label: "IT-Systemelektroniker", Value: "IT-Systemelektroniker"},
		{Label: "Kaufmann It-System-Managment", Value: "Kaufmann It-System-Managment"},
	}

	// Show tag selection with SelectMenu
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("📋 **Lerngruppe '%s'** wird erstellt. Bitte wähle Tags aus (mehrere möglich):", groupName),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("group_tags_%s", groupName),
							Placeholder: "Wähle Tags (optional)",
							MinValues:   intPtr(0),
							MaxValues:   len(tagOptions),
							Options:     tagOptions,
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Failed to show tag selection: %v", err)
	}
}

func (b *Bot) handleEventCreateModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	// Parse group ID from custom ID
	groupIDStr := strings.TrimPrefix(data.CustomID, "event_create_modal_")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		b.respondError(s, i.Interaction, "Ungültige Gruppen-ID")
		return
	}

	// Get group from database
	group, err := b.store.GetGroupByID(groupID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Abrufen der Gruppe")
		return
	}

	// Extract form values
	title := ""
	dateStr := ""
	timeStr := ""
	durationStr := ""
	notes := ""

	for _, row := range data.Components {
		for _, component := range row.(*discordgo.ActionsRow).Components {
			input := component.(*discordgo.TextInput)
			switch input.CustomID {
			case "event_title":
				title = input.Value
			case "event_date":
				dateStr = input.Value
			case "event_time":
				timeStr = input.Value
			case "event_duration":
				durationStr = input.Value
			case "event_notes":
				notes = input.Value
			}
		}
	}

	if title == "" {
		b.respondError(s, i.Interaction, "Event-Titel darf nicht leer sein")
		return
	}

	// Parse date and time
	dateTimeStr := dateStr
	if timeStr != "" {
		dateTimeStr += " " + timeStr
	} else {
		dateTimeStr += " 00:00"
	}

	startsAt, err := time.Parse("2006-01-02 15:04", dateTimeStr)
	if err != nil {
		b.respondError(s, i.Interaction, "Ungültiges Datums- oder Zeitformat. Bitte verwende JJJJ-MM-TT und HH:MM")
		return
	}

	// Parse duration
	var duration int
	if durationStr != "" {
		duration, err = strconv.Atoi(strings.TrimSpace(durationStr))
		if err != nil || duration < 0 {
			b.respondError(s, i.Interaction, "Ungültige Dauer. Bitte gib eine positive Zahl in Minuten an")
			return
		}
	}

	// Create event
	event := &domain.Event{
		GroupID:     groupID,
		Title:       title,
		StartsAt:    startsAt,
		DurationMin: duration,
		Notes:       notes,
		CreatedAt:   time.Now(),
	}

	if err := b.store.CreateEvent(event); err != nil {
		b.respondError(s, i.Interaction, fmt.Sprintf("Fehler beim Erstellen des Events: %v", err))
		return
	}

	// Update planner embed
	if err := b.updatePlannerEmbed(s, group); err != nil {
		log.Printf("Failed to update planner embed: %v", err)
	}

	b.respondSuccess(s, i.Interaction, fmt.Sprintf("✅ Event **%s** erfolgreich erstellt!", title))
}

func (b *Bot) handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if strings.HasPrefix(data.CustomID, "join_") {
		b.handleJoinButton(s, i)
	} else if strings.HasPrefix(data.CustomID, "leave_") {
		b.handleLeaveButton(s, i)
	} else if strings.HasPrefix(data.CustomID, "add_event_") {
		b.handleAddEventButton(s, i)
	} else if strings.HasPrefix(data.CustomID, "group_tags_") {
		b.handleGroupTagsSelect(s, i)
	}
}

func (b *Bot) handleJoinButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	groupID, err := parseGroupID(data.CustomID, "join_")
	if err != nil {
		b.respondError(s, i.Interaction, "Ungültige Gruppen-ID")
		return
	}

	group, err := b.store.GetGroupByID(groupID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Abrufen der Gruppe")
		return
	}

	userID := i.Member.User.ID

	// Check if already a member
	isMember, err := b.store.IsMember(groupID, userID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Überprüfen der Mitgliedschaft")
		return
	}
	if isMember {
		b.respondError(s, i.Interaction, "Du bist bereits Mitglied dieser Gruppe")
		return
	}

	// Add to database
	if err := b.store.AddMember(groupID, userID); err != nil {
		b.respondError(s, i.Interaction, fmt.Sprintf("Fehler beim Hinzufügen des Mitglieds: %v", err))
		return
	}

	// Update channel permissions
	err = s.ChannelPermissionSet(group.PrivateChannelID, userID, discordgo.PermissionOverwriteTypeMember,
		discordgo.PermissionViewChannel|discordgo.PermissionSendMessages, 0)
	if err != nil {
		log.Printf("Failed to update channel permissions: %v", err)
	}

	b.respondSuccess(s, i.Interaction, fmt.Sprintf("✅ Du bist der Gruppe **%s** beigetreten! Schau dir <#%s> an", group.Name, group.PrivateChannelID))
}

func (b *Bot) handleLeaveButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	groupID, err := parseGroupID(data.CustomID, "leave_")
	if err != nil {
		b.respondError(s, i.Interaction, "Ungültige Gruppen-ID")
		return
	}

	group, err := b.store.GetGroupByID(groupID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Abrufen der Gruppe")
		return
	}

	userID := i.Member.User.ID

	// Check if owner
	if group.OwnerUserID == userID {
		b.respondError(s, i.Interaction, "Du kannst deine eigene Gruppe nicht verlassen")
		return
	}

	// Check if member
	isMember, err := b.store.IsMember(groupID, userID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Überprüfen der Mitgliedschaft")
		return
	}
	if !isMember {
		b.respondError(s, i.Interaction, "Du bist kein Mitglied dieser Gruppe")
		return
	}

	// Remove from database
	if err := b.store.RemoveMember(groupID, userID); err != nil {
		b.respondError(s, i.Interaction, fmt.Sprintf("Fehler beim Entfernen des Mitglieds: %v", err))
		return
	}

	// Remove channel permissions
	err = s.ChannelPermissionDelete(group.PrivateChannelID, userID)
	if err != nil {
		log.Printf("Failed to remove channel permissions: %v", err)
	}

	b.respondSuccess(s, i.Interaction, fmt.Sprintf("✅ Du hast die Gruppe **%s** verlassen.", group.Name))
}

func (b *Bot) handleAddEventButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	groupID, err := parseGroupID(data.CustomID, "add_event_")
	if err != nil {
		b.respondError(s, i.Interaction, "Ungültige Gruppen-ID")
		return
	}

	group, err := b.store.GetGroupByID(groupID)
	if err != nil {
		b.respondError(s, i.Interaction, "Fehler beim Abrufen der Gruppe")
		return
	}

	userID := i.Member.User.ID

	// Check if user is owner
	if group.OwnerUserID != userID {
		b.respondError(s, i.Interaction, "Nur der Gruppenbesitzer kann Events erstellen")
		return
	}

	// Show modal for event creation
	b.showEventModal(s, i, group.ID)
}

func (b *Bot) handleGroupTagsSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()

	// Extract group name from custom ID
	groupName := strings.TrimPrefix(data.CustomID, "group_tags_")

	// Get selected tags
	selectedTags := data.Values

	// Acknowledge the interaction with deferred response
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	// Create private channel
	guildID := i.GuildID
	channelName := strings.ToLower(strings.ReplaceAll(groupName, " ", "-"))

	privateChannel, err := s.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     channelName,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: b.config.PrivateCategoryID,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{
			{
				ID:   guildID, // @everyone
				Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionViewChannel,
			},
			{
				ID:    i.Member.User.ID,
				Type:  discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages,
			},
		},
	})
	if err != nil {
		b.followUpError(s, i.Interaction, fmt.Sprintf("Fehler beim Erstellen des privaten Kanals: %v", err))
		return
	}

	// Build initial embed with tags
	tagsDisplay := "Keine"
	if len(selectedTags) > 0 {
		tagsDisplay = strings.Join(selectedTags, ", ")
	}

	initialEmbed := &discordgo.MessageEmbed{
		Title: "📚 Lerngruppen Planer",
		Description: fmt.Sprintf("**Gruppe:** %s\n**Privater Kanal:** <#%s>\n**Tags:** %s\n\n**Anstehende Events:**\nNoch keine Events geplant",
			groupName, privateChannel.ID, tagsDisplay),
		Color:     0x5865F2,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Create buttons for owner only (will be updated with correct IDs after DB insert)
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Event hinzufügen",
					Style:    discordgo.PrimaryButton,
					CustomID: "add_event_temp",
					Emoji: &discordgo.ComponentEmoji{
						Name: "📅",
					},
				},
			},
		},
	}

	// Resolve selected tags to forum channel tag IDs (ensures tags exist on the forum channel)
	appliedTagIDs, err := b.resolveForumTagIDs(s, selectedTags)
	if err != nil {
		log.Printf("Failed to resolve forum tags: %v", err)
	}

	// Create forum thread with proper embed and applied tags
	thread, err := s.ForumThreadStartComplex(b.config.ForumChannelID,
		&discordgo.ThreadStart{
			Name:        groupName,
			AppliedTags: appliedTagIDs,
		},
		&discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{initialEmbed},
			Components: components,
		})
	if err != nil {
		b.followUpError(s, i.Interaction, fmt.Sprintf("Fehler beim Erstellen des Forum-Threads: %v", err))
		return
	}

	// Get the planner message ID from the thread's starter message
	plannerMessageID := ""
	if len(thread.Messages) > 0 {
		plannerMessageID = thread.Messages[0].ID
	}

	// Save to database with selected tags
	group := &domain.Group{
		GuildID:          guildID,
		Name:             groupName,
		OwnerUserID:      i.Member.User.ID,
		ForumThreadID:    thread.ID,
		PrivateChannelID: privateChannel.ID,
		PlannerMessageID: plannerMessageID,
		Tags:             selectedTags,
		CreatedAt:        time.Now(),
	}

	if err := b.store.CreateGroup(group); err != nil {
		b.followUpError(s, i.Interaction, fmt.Sprintf("Fehler beim Speichern der Gruppe: %v", err))
		return
	}

	// Add owner as member
	if err := b.store.AddMember(group.ID, i.Member.User.ID); err != nil {
		log.Printf("Failed to add owner as member: %v", err)
	}

	// Update forum thread with correct button IDs
	if err := b.updatePlannerEmbed(s, group); err != nil {
		log.Printf("Failed to update planner embed: %v", err)
	}

	// Follow up with success message
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: strPtr(fmt.Sprintf("✅ Lerngruppe **%s** erfolgreich erstellt!\n\n📋 Forum-Post: <#%s>\n🔒 Privater Kanal: <#%s>\n🏷️ Tags: %s",
			groupName, thread.ID, privateChannel.ID, tagsDisplay)),
	})
	if err != nil {
		log.Printf("Failed to edit interaction response: %v", err)
	}
}

func (b *Bot) updatePlannerEmbed(s *discordgo.Session, group *domain.Group) error {
	// Get events for this group
	events, err := b.store.GetEventsByGroupID(group.ID)
	if err != nil {
		return err
	}

	// Get member count
	members, err := b.store.GetMembers(group.ID)
	if err != nil {
		return err
	}

	// Build event list, filtering out past events
	now := time.Now()
	eventList := ""
	upcomingCount := 0
	for _, event := range events {
		if event.StartsAt.Before(now) {
			continue
		}
		upcomingCount++
		dateStr := event.StartsAt.Format("2006-01-02 15:04")
		eventList += fmt.Sprintf("• **%s** - %s", event.Title, dateStr)
		if event.DurationMin > 0 {
			eventList += fmt.Sprintf(" (%d min)", event.DurationMin)
		}
		if event.Notes != "" {
			eventList += fmt.Sprintf("\n  _%s_", event.Notes)
		}
		eventList += "\n"
	}
	if upcomingCount == 0 {
		eventList = "Noch keine Events geplant"
	}

	// Build tags display
	tagsDisplay := "Keine"
	if len(group.Tags) > 0 {
		tagsDisplay = strings.Join(group.Tags, ", ")
	}

	embed := &discordgo.MessageEmbed{
		Title: "📚 Lerngruppen Planer",
		Description: fmt.Sprintf("**Gruppe:** %s\n**Privater Kanal:** <#%s>\n**Tags:** %s\n**Mitglieder:** %d\n\n**Anstehende Events:**\n%s",
			group.Name, group.PrivateChannelID, tagsDisplay, len(members), eventList),
		Color:     0x5865F2,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Show buttons for joining, leaving, and adding events
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Beitreten",
					Style:    discordgo.SuccessButton,
					CustomID: fmt.Sprintf("join_%d", group.ID),
					Emoji: &discordgo.ComponentEmoji{
						Name: "➕",
					},
				},
				discordgo.Button{
					Label:    "Verlassen",
					Style:    discordgo.DangerButton,
					CustomID: fmt.Sprintf("leave_%d", group.ID),
					Emoji: &discordgo.ComponentEmoji{
						Name: "➖",
					},
				},
				discordgo.Button{
					Label:    "Event hinzufügen",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("add_event_%d", group.ID),
					Emoji: &discordgo.ComponentEmoji{
						Name: "📅",
					},
				},
			},
		},
	}

	// Use stored planner message ID if available, otherwise fall back to fetching
	messageID := group.PlannerMessageID
	if messageID == "" {
		messages, err := s.ChannelMessages(group.ForumThreadID, 1, "", "", "")
		if err != nil || len(messages) == 0 {
			return fmt.Errorf("failed to get thread messages: %w", err)
		}
		messageID = messages[0].ID

		// Store the message ID for future use
		if err := b.store.UpdatePlannerMessageID(group.ID, messageID); err != nil {
			log.Printf("Failed to store planner message ID: %v", err)
		}
		group.PlannerMessageID = messageID
	}

	_, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    group.ForumThreadID,
		ID:         messageID,
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})

	return err
}

func (b *Bot) respondError(s *discordgo.Session, i *discordgo.Interaction, message string) {
	err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "❌ " + message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}

func (b *Bot) respondSuccess(s *discordgo.Session, i *discordgo.Interaction, message string) {
	err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Failed to send success response: %v", err)
	}
}

func (b *Bot) followUpError(s *discordgo.Session, i *discordgo.Interaction, message string) {
	_, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content: strPtr("❌ " + message),
	})
	if err != nil {
		log.Printf("Failed to send follow-up error: %v", err)
	}
}

// resolveForumTagIDs ensures the selected tag names exist as available tags on the
// forum channel and returns their IDs so they can be applied to a new thread.
func (b *Bot) resolveForumTagIDs(s *discordgo.Session, selectedTags []string) ([]string, error) {
	if len(selectedTags) == 0 {
		return nil, nil
	}

	// Fetch the forum channel to get existing available tags
	forumChannel, err := s.Channel(b.config.ForumChannelID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forum channel: %w", err)
	}

	// Determine which selected tags are missing from the forum channel
	existingTags := buildTagNameToIDMap(forumChannel.AvailableTags)
	missingTags := findMissingTags(selectedTags, existingTags)

	// If there are missing tags, add them to the forum channel
	if len(missingTags) > 0 {
		updatedTags := make([]discordgo.ForumTag, len(forumChannel.AvailableTags), len(forumChannel.AvailableTags)+len(missingTags))
		copy(updatedTags, forumChannel.AvailableTags)
		for _, tagName := range missingTags {
			updatedTags = append(updatedTags, discordgo.ForumTag{Name: tagName})
		}

		editedChannel, err := s.ChannelEditComplex(b.config.ForumChannelID, &discordgo.ChannelEdit{
			AvailableTags: &updatedTags,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update forum channel tags: %w", err)
		}

		// Refresh the tag name-to-ID map from the updated channel
		existingTags = buildTagNameToIDMap(editedChannel.AvailableTags)
	}

	// Collect the IDs for all selected tags
	return collectTagIDs(selectedTags, existingTags), nil
}

// buildTagNameToIDMap creates a map from tag name to tag ID.
func buildTagNameToIDMap(tags []discordgo.ForumTag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, tag := range tags {
		m[tag.Name] = tag.ID
	}
	return m
}

// findMissingTags returns tag names from selectedTags that don't exist in existingTags.
func findMissingTags(selectedTags []string, existingTags map[string]string) []string {
	var missing []string
	for _, tagName := range selectedTags {
		if _, exists := existingTags[tagName]; !exists {
			missing = append(missing, tagName)
		}
	}
	return missing
}

// collectTagIDs returns the tag IDs for the given tag names from the map.
func collectTagIDs(selectedTags []string, tagMap map[string]string) []string {
	var ids []string
	for _, tagName := range selectedTags {
		if id, ok := tagMap[tagName]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseGroupID(customID, prefix string) (int64, error) {
	idStr := strings.TrimPrefix(customID, prefix)
	return strconv.ParseInt(idStr, 10, 64)
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
