package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

var (
	// integerOptionMinValue          = 1.0
	// dmPermission                   = false
	// defaultMemberPermissions int64 = discordgo.PermissionManageServer

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "say",
			Description: "Send message as Sora",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "The message to echo.",
					Required:    true, // Make the text input required
				},
			},
			// DefaultMemberPermissions: &defaultMemberPermissions,
			// DMPermission:             &dmPermission,
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"say": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			var inputString string
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "message" {
					inputString = opt.StringValue() // Retrieve the string value
					break
				}
			}

			_, err := s.ChannelMessageSend(i.ChannelID, inputString)
			if err != nil {
				fmt.Println(err)
			}

			// s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			// 	Type: discordgo.InteractionResponseChannelMessageWithSource,
			// 	Data: &discordgo.InteractionResponseData{
			// 		Content: inputString,
			// 	},
			// })
		},
	}
)

func RegisterCommands(s *discordgo.Session) {
	fmt.Println("Registering commands")

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(viper.GetString("BOT_ID"), "", v)
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
