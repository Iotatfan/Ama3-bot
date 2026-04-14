package models

import "time"

type UserProfile struct {
	DiscordUID  string    `gorm:"primaryKey;column:discord_uid;type:varchar(64)"`
	Username    string    `gorm:"column:username;type:varchar(255)"`
	Summary     string    `gorm:"column:summary;type:text;default:'No prior record.'"`
	LastUpdated time.Time `gorm:"column:last_updated;autoUpdateTime"`
}
