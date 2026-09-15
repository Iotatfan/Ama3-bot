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
	slog.Info("memory configuration",
		"enabled", cfg.AI.Memory.Enabled,
		"write_enabled", cfg.AI.Memory.WriteEnabled,
		"retrieval_mode", cfg.AI.Memory.RetrievalMode,
		"embedding_model", cfg.AI.Memory.EmbeddingModel,
		"encryption_enabled", cfg.AI.Memory.EncryptContent,
	)
	dsn := cfg.Database.DSN
	var userRepo repository.UserRepository
	var memoryRepo repository.MemoryRepository
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
			if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
				errors.Error("database.vector_extension", err)
			}
			if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
				errors.Error("database.pgcrypto_extension", err)
			}
			if err := db.Exec(`CREATE TABLE IF NOT EXISTS user_memories (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), discord_uid varchar(64) NOT NULL, content text NOT NULL, category varchar(128) NOT NULL, confidence double precision NOT NULL, importance double precision NOT NULL, embedding vector(1536) NOT NULL, source_message_id varchar(64), source_guild_id varchar(64), source_channel_id varchar(64), active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL)`).Error; err != nil {
				errors.Error("database.memory_migrate", err)
			}
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_user_memories_uid_active ON user_memories(discord_uid, active)").Error; err != nil {
				errors.Error("database.memory_index", err)
			}
			db.Exec(`CREATE TABLE IF NOT EXISTS self_memories (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), content text NOT NULL, category varchar(64) NOT NULL, confidence double precision NOT NULL, importance double precision NOT NULL, source_message_id varchar(64), source_guild_id varchar(64), source_channel_id varchar(64), active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL)`)
			db.Exec("ALTER TABLE self_memories ADD COLUMN IF NOT EXISTS embedding vector(1536)")
			db.Exec(`CREATE TABLE IF NOT EXISTS self_memory_conflicts (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), self_memory_id uuid NOT NULL REFERENCES self_memories(id), proposed_content text NOT NULL, source_message_id varchar(64), status varchar(16) NOT NULL DEFAULT 'unresolved', resolved_by varchar(64), resolved_at timestamptz, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL)`)
			db.Exec("CREATE INDEX IF NOT EXISTS idx_self_memories_active ON self_memories(active)")
			db.Exec("CREATE INDEX IF NOT EXISTS idx_self_memory_conflicts_status ON self_memory_conflicts(status)")
			if err := db.AutoMigrate(&models.UserProfile{}); err != nil {
				errors.Error("database.migrate", err)
			}
			userRepo = repository.NewUserRepository(db)
			memoryRepo = repository.NewMemoryRepository(db, cfg.AI.Memory.EncryptContent)
		}
	}
	discord, err := discordgo.New("Bot " + cfg.Auth.DiscordToken)
	if err != nil {
		errors.Error("discord.session.create", err)
		return
	}

	aiClient := openai.NewClient(
		option.WithAPIKey(cfg.Auth.OpenAIKey),
		option.WithMaxRetries(0),
	)
	handler := aiHandler.NewAIHandler(cfg, &aiClient, userRepo, memoryRepo)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		errors.Run("discord.message", func() { handler.ParseMessage(s, m, ctx) })
	})
	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		errors.Run("discord.url_replace", func() { urlReplaceHandler.ParseUrl(s, m) })
	})

	if cfg.Commands.Enabled {
		commandsHandler := commands.NewCommandsHandlerWithMemoryRepositoryAndEmbedder(memoryRepo, handler.EmbedText)
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
	handler.FlushMemoryQueues()
}
