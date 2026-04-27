package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
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
}

func RegisterCommands(s *discordgo.Session) {
	defaultCommandsHandler.RegisterCommands(s)
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

	h.registrations = []commandRegistration{
		{
			command: &discordgo.ApplicationCommand{
				Name:        "help",
				Description: "Get help about how to use me",
			},
			handler: h.handleHelp,
		},
		{
			command: &discordgo.ApplicationCommand{
				Name:        "say",
				Description: "*Warning. Send message as Ama3",
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
		},
		{
			command: &discordgo.ApplicationCommand{
				Name:        "nick",
				Description: "*Warning. Change bot's nickname",
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
		},
		{
			command: &discordgo.ApplicationCommand{
				Name:        "mon3tr_release",
				Description: "AMA3: Meltdown Sequence Authorized",
			},
			handler: h.handleReleaseMon3tr,
		},
	}

	return h
}

func (h *CommandsHandler) RegisterCommands(s *discordgo.Session) {
	fmt.Println("Registering commands")

	cfg := h.getConfig()
	if cfg == nil || cfg.App.BotID == "" {
		fmt.Println("Cannot register commands: missing bot ID in config")
		return
	}

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
			handler(s, i)
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
	h.respondText(s, i, "Please @Me to chat with me! I can also respond to messages that reply to my messages. Sometimes I will change the link in Twitter and Instagram messages to something else ehe~. I'm a Goldfish so I may forget the context of our conversation anytime soon.")
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

	h.respondText(s, i, "https://tenor.com/view/arknights-mon3tr-wahh-cute-roar-gif-18351888906315697493")
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
