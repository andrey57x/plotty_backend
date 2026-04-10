package models

import (
	"time"

	"github.com/google/uuid"
)

type Tag struct {
	ID       uuid.UUID `json:"id"`
	Category string    `json:"category"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
}

type Story struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	AiSummary *string   `json:"aiHint,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StoryListItem struct {
	Story
	Tags          []Tag `json:"tags"`
	ChaptersCount int   `json:"chaptersCount"`
}

type StoryDetail struct {
	Story
	Tags     []Tag          `json:"tags"`
	Chapters []ChapterBrief `json:"chapters"`
}

type ChapterBrief struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Chapter struct {
	ID        uuid.UUID `json:"id"`
	StoryID   uuid.UUID `json:"storyId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AIJob struct {
	ID            uuid.UUID
	Type          string
	Status        string
	StoryID       *uuid.UUID
	ChapterID     *uuid.UUID
	InputPayload  []byte
	ResultPayload []byte
	ErrorMessage  *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type GeneratedImage struct {
	ID        uuid.UUID
	JobID     uuid.UUID
	ChapterID *uuid.UUID
	Prompt    string
	ImageURL  string
	CreatedAt time.Time
}

type User struct {
	ID        uint64     `json:"id"`
	Email     string     `json:"email"`
	Username  string     `json:"username"`
	AvatarURL *string    `json:"avatar_url,omitempty"`
	Password  string     `json:"-"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
