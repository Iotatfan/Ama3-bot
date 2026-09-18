package commands

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/iotatfan/sora-go/internal/errorhandler"
	"github.com/iotatfan/sora-go/internal/repository"
)

var defaultCommandsHandler = NewCommandsHandler()

type commandHandler func(s *discordgo.Session, i *discordgo.InteractionCreate)

type commandRegistration struct {
	command *discordgo.ApplicationCommand
	handler commandHandler
}

type CommandsHandler struct {
	getConfig     func() *config.Config
	registrations []commandRegistration
	memoryRepo    repository.MemoryRepository
	embed         func(context.Context, string) ([]float64, error)
}

func (h *CommandsHandler) NewWithMemoryRepository(repo repository.MemoryRepository) *CommandsHandler {
	h.memoryRepo = repo
	return h
}
func NewCommandsHandlerWithMemoryRepository(repo repository.MemoryRepository) *CommandsHandler {
	h := NewCommandsHandler()
	h.memoryRepo = repo
	return h
}

func NewCommandsHandlerWithMemoryRepositoryAndEmbedder(repo repository.MemoryRepository, embed func(context.Context, string) ([]float64, error)) *CommandsHandler {
	h := NewCommandsHandlerWithMemoryRepository(repo)
	h.embed = embed
	return h
}

func RegisterCommands(s *discordgo.Session) {
	defaultCommandsHandler.RegisterCommandsWithErrorHandler(s, errorhandler.New(nil))
}

func NewCommandsHandler() *CommandsHandler {
	return NewCommandsHandlerWithConfig(config.GetConfig)
}

func NewCommandsHandlerWithConfig(getConfig func() *config.Config) *CommandsHandler {
	if getConfig == nil {
		getConfig = config.GetConfig
	}

	h := &CommandsHandler{
		getConfig: getConfig,
	}

	return h
}

func (h *CommandsHandler) buildRegistrations(cfg *config.Config) []commandRegistration {
	command := func(key string) config.Command {
		item := cfg.Commands.Items[key]
		if item.Name == "" {
			item.Name = key
		}
		return item
	}

	help := command("help")
	say := command("say")
	nick := command("nick")
	release := command("mon3tr_release")

	registrations := []commandRegistration{
		{command: &discordgo.ApplicationCommand{Name: help.Name, Description: help.Description}, handler: h.handleHelp},
		{command: &discordgo.ApplicationCommand{Name: say.Name, Description: say.Description, Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "message", Description: "The message to echo.", Required: true}}}, handler: h.handleSay},
		{command: &discordgo.ApplicationCommand{Name: nick.Name, Description: nick.Description, Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "nick", Description: "New nickname", Required: true}}}, handler: h.handleNick},
		{command: &discordgo.ApplicationCommand{Name: release.Name, Description: release.Description}, handler: h.handleReleaseMon3tr},
	}
	if h.memoryRepo != nil && cfg.AI.Memory.Enabled {
		registrations = append(registrations, commandRegistration{command: &discordgo.ApplicationCommand{Name: "memory", Description: "Manage protected bot memory.", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "self-list", Description: "List bot self-memory"}, {Type: discordgo.ApplicationCommandOptionSubCommand, Name: "conflicts", Description: "List unresolved conflicts"}, {Type: discordgo.ApplicationCommandOptionSubCommand, Name: "resolve", Description: "Resolve a conflict", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Conflict ID", Required: true}, {Type: discordgo.ApplicationCommandOptionString, Name: "action", Description: "Resolution action", Required: true}}}, {Type: discordgo.ApplicationCommandOptionSubCommand, Name: "self-forget", Description: "Deactivate self-memory", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Memory ID", Required: true}}}}}, handler: h.handleMemory})
	}
	/* legacy registrations replaced above */
	/* registrations = append(registrations, commandRegistration{
		command: &discordgo.ApplicationCommand{
			Name:        help.Name,
			Description: help.Description,
		},
		handler: h.handleHelp,
	}, commandRegistration{
		command: &discordgo.ApplicationCommand{
			command: &discordgo.ApplicationCommand{
				Name:        say.Name,
				Description: say.Description,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "message",
						Description: "The message to echo.",
						Required:    true,
					},
				},
			},
			handler: h.handleSay,
		}, handler: h.handleSay,
	}, commandRegistration{
			command: &discordgo.ApplicationCommand{
				Name:        nick.Name,
				Description: nick.Description,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "nick",
						Description: "New nickname",
						Required:    true,
					},
				},
			},
			handler: h.handleNick,
		}, handler: h.handleNick,
	}, commandRegistration{
			command: &discordgo.ApplicationCommand{
				Name:        release.Name,
				Description: release.Description,
			},
			handler: h.handleReleaseMon3tr,
		}, handler: h.handleReleaseMon3tr,
	}) */
	return registrations
}

func (h *CommandsHandler) handleMemory(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.requireOwner(s, i) {
		return
	}
	if h.memoryRepo == nil {
		h.respondText(s, i, "Memory storage is unavailable.")
		return
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return
	}
	sub := opts[0]
	switch sub.Name {
	case "self-list":
		xs, e := h.memoryRepo.ListSelfMemories()
		if e != nil {
			h.respondText(s, i, "Unable to read self-memory.")
			return
		}
		out := ""
		for _, x := range xs {
			out += x.ID + " — " + x.Content + "\n"
		}
		if out == "" {
			out = "No self-memory records."
		}
		h.respondText(s, i, out)
	case "conflicts":
		xs, e := h.memoryRepo.ListConflicts()
		if e != nil {
			h.respondText(s, i, "Unable to read conflicts.")
			return
		}
		out := ""
		for _, x := range xs {
			out += x.ID + " — " + x.ProposedContent + "\n"
		}
		if out == "" {
			out = "No unresolved conflicts."
		}
		h.respondText(s, i, out)
	case "resolve":
		id := sub.Options[0].StringValue()
		action := sub.Options[1].StringValue()
		var embedding []float64
		if action == "accept_new" {
			conflict, e := h.memoryRepo.GetConflict(id)
			if e != nil {
				h.respondText(s, i, "Unable to read conflict: "+e.Error())
				return
			}
			cfg := h.getConfig()
			if cfg == nil || cfg.AI.Memory.EnableEmbeddings {
				if h.embed == nil {
					h.respondText(s, i, "Embedding service is unavailable.")
					return
				}
				embedding, e = h.embed(context.Background(), conflict.ProposedContent)
				if e != nil {
					h.respondText(s, i, "Unable to create embedding: "+e.Error())
					return
				}
			}
		}
		if e := h.memoryRepo.ResolveConflict(id, action, h.ownerID(i), embedding); e != nil {
			h.respondText(s, i, "Resolution failed: "+e.Error())
			return
		}
		h.respondText(s, i, "Conflict resolved.")
	case "self-forget":
		if e := h.memoryRepo.DeleteSelfMemory(sub.Options[0].StringValue()); e != nil {
			h.respondText(s, i, "Unable to forget memory.")
			return
		}
		h.respondText(s, i, "Self-memory deactivated.")
	}
}
func (h *CommandsHandler) ownerID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func (h *CommandsHandler) RegisterCommands(s *discordgo.Session) {
	h.RegisterCommandsWithErrorHandler(s, errorhandler.New(nil))
}

// RegisterCommandsWithErrorHandler registers commands and protects command
// callbacks with the application's shared error boundary.
func (h *CommandsHandler) RegisterCommandsWithErrorHandler(s *discordgo.Session, errors *errorhandler.Handler) {
	if errors == nil {
		errors = errorhandler.New(nil)
	}

	fmt.Println("Registering commands")

	cfg := h.getConfig()
	if cfg == nil || cfg.App.BotID == "" {
		fmt.Println("Cannot register commands: missing bot ID in config")
		return
	}
	h.registrations = h.buildRegistrations(cfg)

	commandHandlers := make(map[string]commandHandler, len(h.registrations))

	// Register local handlers first so they are immediately available
	for _, registration := range h.registrations {
		commandHandlers[registration.command.Name] = registration.handler
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		if handler, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			errors.Run("discord.interaction."+i.ApplicationCommandData().Name, func() {
				handler(s, i)
			})
		}
	})

	// Register commands with Discord in the background to avoid blocking
	// on network errors or rate limits.
	go func() {
		registeredCount := 0
		for _, registration := range h.registrations {
			_, err := s.ApplicationCommandCreate(cfg.App.BotID, "", registration.command)
			if err != nil {
				fmt.Printf("Cannot create '%v' command: %v\n", registration.command.Name, err)
				continue
			}
			registeredCount++
		}

		if registeredCount == 0 {
			fmt.Println("No commands were registered")
		} else {
			fmt.Printf("Successfully registered %d commands\n", registeredCount)
		}
	}()
}

func (h *CommandsHandler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.respondText(s, i, h.getConfig().Commands.Items["help"].Content)
}

func (h *CommandsHandler) handleSay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.requireOwner(s, i) {
		return
	}

	inputString, ok := h.getStringOption(i, "message")
	if !ok {
		h.respondText(s, i, "Missing required option: message")
		return
	}

	if !h.respondDeferred(s, i) {
		return
	}

	if _, err := s.ChannelMessageSend(i.ChannelID, inputString); err != nil {
		fmt.Println(err)
	}

	h.deleteResponse(s, i)
}

func (h *CommandsHandler) handleNick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.requireOwner(s, i) {
		return
	}

	inputString, ok := h.getStringOption(i, "nick")
	if !ok {
		h.respondText(s, i, "Missing required option: nick")
		return
	}

	if !h.respondDeferred(s, i) {
		return
	}

	if err := s.GuildMemberNickname(i.GuildID, "@me", inputString); err != nil {
		fmt.Println(err)
	}

	h.deleteResponse(s, i)
}

func (h *CommandsHandler) handleReleaseMon3tr(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.requireOwner(s, i) {
		return
	}

	h.respondText(s, i, h.getConfig().Commands.Items["mon3tr_release"].Content)
}

func (h *CommandsHandler) getStringOption(i *discordgo.InteractionCreate, optionName string) (string, bool) {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == optionName {
			return opt.StringValue(), true
		}
	}

	return "", false
}

func (h *CommandsHandler) requireOwner(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if h.isOwnerInteraction(i) {
		return true
	}

	h.respondText(s, i, "Access denied. Only those with the necessary clearance may interface with this system. Retrace your steps before you cause an irreversible error")
	return false
}

func (h *CommandsHandler) respondText(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	}); err != nil {
		fmt.Println(err)
	}
}

func (h *CommandsHandler) respondDeferred(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (h *CommandsHandler) deleteResponse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionResponseDelete(i.Interaction); err != nil {
		fmt.Println(err)
	}
}

func (h *CommandsHandler) isOwnerInteraction(i *discordgo.InteractionCreate) bool {
	cfg := h.getConfig()
	if cfg == nil {
		return false
	}

	ownerID := cfg.App.OwnerID
	switch {
	case i.Member != nil && i.Member.User != nil:
		return i.Member.User.ID == ownerID
	case i.User != nil:
		return i.User.ID == ownerID
	default:
		return false
	}
}
