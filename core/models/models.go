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
	AuthorID  *uint64   `json:"authorId,omitempty"`
	AiSummary *string   `json:"aiHint,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StoryAuthor struct {
	ID        uint64  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

type StoryListItem struct {
	Story
	Tags                []Tag        `json:"tags"`
	ChaptersCount       int          `json:"chaptersCount"`
	LikesCount          int          `json:"likesCount"`
	Author              *StoryAuthor `json:"author,omitempty"`
	CoverURL            *string      `json:"coverUrl,omitempty"`
	ReadChapterNumber   *int         `json:"readChapterNumber"`
}

type StoryDetail struct {
	Story
	Tags       []Tag          `json:"tags"`
	Chapters   []ChapterBrief `json:"chapters"`
	LikesCount int            `json:"likesCount"`
	LikedByMe  bool           `json:"likedByMe"`
	Author     *StoryAuthor   `json:"author,omitempty"`
}

type Comment struct {
	ID        uuid.UUID `json:"id"`
	ChapterID uuid.UUID `json:"chapterId"`
	UserID    uint64    `json:"userId"`
	Username  string    `json:"username"`
	AvatarURL *string   `json:"avatarUrl,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type ChapterBrief struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Chapter struct {
	ID           uuid.UUID `json:"id"`
	StoryID      uuid.UUID `json:"storyId"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	DraftTitle   string    `json:"draftTitle"`
	DraftContent string    `json:"draftContent"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
	ContentHash   *string
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
	AvatarURL *string    `json:"avatarUrl,omitempty"`
	Bio       *string    `json:"bio,omitempty"`
	Credits   int        `json:"credits"`
	IsAdmin   bool       `json:"isAdmin"`
	Password  string     `json:"-"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type SuggestedFandom struct {
	ID          uuid.UUID `json:"id"`
	UserID      uint64    `json:"userId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PublicUserProfile struct {
	ID        uint64  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
	Bio       *string `json:"bio,omitempty"`
}

type UserCollection struct {
	ID          uuid.UUID `json:"id"`
	UserID      uint64    `json:"userId"`
	Title       string    `json:"title"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UserCollectionSummary struct {
	UserCollection
	StoriesCount int `json:"storiesCount"`
}

type UserCollectionDetail struct {
	UserCollection
	Stories []StoryListItem `json:"stories"`
}

type ReaderShelf string

const (
	ReaderShelfReading  ReaderShelf = "reading"
	ReaderShelfPlanned  ReaderShelf = "planned"
	ReaderShelfRead     ReaderShelf = "read"
	ReaderShelfDropped  ReaderShelf = "dropped"
	ReaderShelfFavorite ReaderShelf = "favorite"
)

type ReaderShelfEntry struct {
	StoryID   uuid.UUID     `json:"storyId"`
	Shelf     ReaderShelf   `json:"shelf"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Story     StoryListItem `json:"story"`
}

type ChapterAnalytics struct {
	ChapterID uuid.UUID `json:"chapterId"`
	Title     string    `json:"title"`
	Views     int       `json:"views"`
}

type ChapterViewed struct {
	ChapterID uuid.UUID `json:"chapterId"`
	Title     string    `json:"title"`
	Viewed    bool      `json:"viewed"`
}

type CreditTransaction struct {
	ID           uuid.UUID `json:"id"`
	UserID       uint64    `json:"userId"`
	Amount       int       `json:"amount"`
	Type         string    `json:"type"`
	Description  *string   `json:"description,omitempty"`
	PaymentLabel *string   `json:"paymentLabel,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}
