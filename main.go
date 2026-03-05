package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/commands"
	"github.com/iotatfan/sora-go/internal/config"
	chatGpt "github.com/iotatfan/sora-go/internal/gpt"
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

	discord, err := discordgo.New("Bot " + config.GetConfig().Token)
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	gptClient := openai.NewClient(
		option.WithAPIKey(config.GetConfig().OpenAIKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		chatGpt.ParseGptMessage(s, m, &gptClient, ctx)
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
