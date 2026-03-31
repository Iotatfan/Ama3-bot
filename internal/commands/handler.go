package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
)

var (
	// integerOptionMinValue          = 1.0
	// dmPermission                   = false
	// defaultMemberPermissions int64 = discordgo.PermissionManageServer

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "help",
			Description: "Get help about how to use me",
		},
		{
			Name:        "say",
			Description: "Send message as Ama3",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "The message to echo.",
				},
			},
		},
		{
			Name:        "nick",
			Description: "*Warning. Change bot's nickname",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "nick",
					Description: "New nickname",
					Required:    true, // Make the text input required
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"help": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Please @Me to chat with me! I can also respond to messages that reply to my messages. Sometimes I will change the link in Twitter and Instagram messages to something else ehe~. I'm a Goldfish so I may forget the context of our conversation anytime soon.",
				},
			})
		},
		"say": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			var inputString string
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "message" {
					inputString = opt.StringValue() // Retrieve the string value
					break
				}
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "",
				},
			})

			_, err := s.ChannelMessageSend(i.ChannelID, inputString)
			if err != nil {
				fmt.Println(err)
			}

			s.InteractionResponseDelete(i.Interaction)
		},
		"nick": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if i.Member.User.ID != config.GetConfig().App.OwnerID {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Ga boleh",
					},
				})

				return
			}

			var inputString string
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "nick" {
					inputString = opt.StringValue() // Retrieve the string value
					break
				}
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "",
				},
			})

			err := s.GuildMemberNickname(i.GuildID, "@me", inputString)
			if err != nil {
				fmt.Println(err)
			}

			s.InteractionResponseDelete(i.Interaction)
		},
	}
)

func RegisterCommands(s *discordgo.Session) {
	fmt.Println("Registering commands")

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(config.GetConfig().App.BotID, "", v)
		if err != nil {
			fmt.Println("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}
