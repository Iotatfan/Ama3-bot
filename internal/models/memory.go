package models

import "time"

type UserMemory struct {
	ID              string  `gorm:"column:id;type:uuid;primaryKey"`
	DiscordUID      string  `gorm:"column:discord_uid;type:varchar(64);index"`
	Content         string  `gorm:"column:content;type:text"`
	Category        string  `gorm:"column:category;type:varchar(32)"`
	Confidence      float64 `gorm:"column:confidence"`
	Importance      float64 `gorm:"column:importance"`
	Embedding       string  `gorm:"column:embedding;type:vector"`
	SourceMessageID string  `gorm:"column:source_message_id;type:varchar(64)"`
	SourceGuildID   string  `gorm:"column:source_guild_id;type:varchar(64)"`
	SourceChannelID string  `gorm:"column:source_channel_id;type:varchar(64)"`
	Active          bool    `gorm:"column:active;default:true;index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type SelfMemory struct {
	ID                                              string `gorm:"column:id;type:uuid;primaryKey"`
	Content                                         string `gorm:"column:content;type:text"`
	Category                                        string `gorm:"column:category;type:varchar(64)"`
	Confidence, Importance                          float64
	SourceMessageID, SourceGuildID, SourceChannelID string
	Active                                          bool
	time.Time
}
