package repository

import (
	"github.com/iotatfan/sora-go/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRepository interface {
	UpsertUserSummary(uid, summary string) error
	GetUserSummary(uid string) (string, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) UpsertUserSummary(uid, summary string) error {
	if r.db == nil {
		return nil
	}

	userProfile := models.UserProfile{
		DiscordUID: uid,
		Summary:    summary,
	}

	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "discord_uid"}},
		DoUpdates: clause.AssignmentColumns([]string{"summary", "last_updated"}),
	}).Create(&userProfile).Error
}

func (r *userRepository) GetUserSummary(uid string) (string, error) {
	if r.db == nil {
		return "", nil
	}

	var userProfile models.UserProfile
	result := r.db.Where("discord_uid = ?", uid).First(&userProfile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", result.Error
	}

	return userProfile.Summary, nil
}
