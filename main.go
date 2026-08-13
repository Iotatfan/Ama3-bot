package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	aiHandler "github.com/iotatfan/sora-go/internal/ai"
	"github.com/iotatfan/sora-go/internal/commands"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/iotatfan/sora-go/internal/errorhandler"
	"github.com/iotatfan/sora-go/internal/models"
	"github.com/iotatfan/sora-go/internal/repository"
	urlReplaceHandler "github.com/iotatfan/sora-go/internal/url_replace"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	ctx := context.Background()
	errors := errorhandler.New(slog.Default())

	if err := config.LoadConfig(); err != nil {
		errors.Error("config.load", err)
		return
	}

	cfg := config.GetConfig()
	dsn := cfg.Database.DSN
	var userRepo repository.UserRepository
	if dsn == "" {
		slog.Warn("database disabled", "reason", "empty DSN")
	} else {
		db, gormErr := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if gormErr != nil {
			errors.Error("database.connect", gormErr)
			slog.Warn("proceeding without database support")
		} else {

			if err := db.AutoMigrate(&models.UserProfile{}); err != nil {
				errors.Error("database.migrate", err)
			}
			userRepo = repository.NewUserRepository(db)
		}
	}
	discord, err := discordgo.New("Bot " + cfg.Auth.DiscordToken)
	if err != nil {
		errors.Error("discord.session.create", err)
		return
	}

	aiClient := openai.NewClient(
		option.WithAPIKey(cfg.Auth.OpenAIKey),
	)
	handler := aiHandler.NewAIHandler(cfg, &aiClient, userRepo)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		errors.Run("discord.message", func() { handler.ParseMessage(s, m, ctx) })
	})
	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		errors.Run("discord.url_replace", func() { urlReplaceHandler.ParseUrl(s, m) })
	})

	if cfg.App.EnableCommands {
		commandsHandler := commands.NewCommandsHandler()
		commandsHandler.RegisterCommandsWithErrorHandler(discord, errors)
	}

	if err := discord.Open(); err != nil {
		errors.Error("discord.open", err)
		return
	}
	defer discord.Close()

	slog.Info("started")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
