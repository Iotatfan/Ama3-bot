package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	aiHandler "github.com/iotatfan/sora-go/internal/ai"
	"github.com/iotatfan/sora-go/internal/commands"
	"github.com/iotatfan/sora-go/internal/config"
	urlParser "github.com/iotatfan/sora-go/internal/parser"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	ctx := context.Background()

	if err := config.LoadConfig(); err != nil {
		fmt.Println("config load error:", err)
		return
	}

	discord, err := discordgo.New("Bot " + config.GetConfig().Auth.DiscordToken)
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	aiClient := openai.NewClient(
		option.WithAPIKey(config.GetConfig().Auth.OpenAIKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		aiHandler.ParseMessage(s, m, &aiClient, ctx)
	})
	discord.AddHandler(urlParser.ParseUrl)
	commands.RegisterCommands(discord)

	if err := discord.Open(); err != nil {
		fmt.Println("discord open error:", err)
		return
	}
	defer discord.Close()

	fmt.Println("Started")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
