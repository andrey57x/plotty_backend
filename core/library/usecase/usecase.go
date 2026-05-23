package usecase

import (
	"context"
	"strings"
	"time"

	librepo "github.com/fivecode/plotty/core/library/repository"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/google/uuid"
)

type Usecase struct {
	lib     *librepo.Repository
	stories *storyrepo.Repository
}

func New(lib *librepo.Repository, stories *storyrepo.Repository) *Usecase {
	return &Usecase{lib: lib, stories: stories}
}

var allowedShelves = map[string]struct{}{
	"reading":  {},
	"planned":  {},
	"read":     {},
	"dropped":  {},
	"favorite": {},
}

func (u *Usecase) SetShelf(ctx context.Context, storyID uuid.UUID, shelf string) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	shelf = strings.TrimSpace(strings.ToLower(shelf))
	if _, ok := allowedShelves[shelf]; !ok {
		return named_errors.ErrInvalidInput
	}
	st, err := u.stories.GetByID(ctx, storyID)
	if err != nil {
		return err
	}
	if st.Status != "published" {
		return named_errors.ErrInvalidInput
	}
	return u.lib.UpsertShelf(ctx, userID, storyID, shelf)
}

func (u *Usecase) RemoveShelf(ctx context.Context, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	return u.lib.RemoveShelf(ctx, userID, storyID)
}

func (u *Usecase) ListShelf(ctx context.Context, shelfFilter *string) ([]models.ReaderShelfEntry, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}
	if shelfFilter != nil {
		s := strings.TrimSpace(strings.ToLower(*shelfFilter))
		if s != "" {
			if _, ok := allowedShelves[s]; !ok {
				return nil, named_errors.ErrInvalidInput
			}
			shelfFilter = &s
		} else {
			shelfFilter = nil
		}
	}
	rows, err := u.lib.ListShelfRows(ctx, userID, shelfFilter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []models.ReaderShelfEntry{}, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.StoryID)
	}
	items, err := u.buildListItems(ctx, ids, true)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]models.StoryListItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	out := make([]models.ReaderShelfEntry, 0, len(rows))
	for _, row := range rows {
		it, ok := byID[row.StoryID]
		if !ok {
			continue
		}
		out = append(out, models.ReaderShelfEntry{
			StoryID:   row.StoryID,
			Shelf:     models.ReaderShelf(row.Shelf),
			UpdatedAt: row.UpdatedAt,
			Story:     it,
		})
	}
	return out, nil
}

func (u *Usecase) buildListItems(ctx context.Context, ids []uuid.UUID, publishedCounts bool) ([]models.StoryListItem, error) {
	if len(ids) == 0 {
		return []models.StoryListItem{}, nil
	}
	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tagsMap, err := u.stories.TagsForStories(ctx, ids)
	if err != nil {
		return nil, err
	}
	var counts map[uuid.UUID]int
	if publishedCounts {
		counts, err = u.stories.ChapterCountsPublished(ctx, ids)
	} else {
		counts, err = u.stories.ChapterCounts(ctx, ids)
	}
	if err != nil {
		return nil, err
	}
	likes, err := u.stories.LikeCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	authors, err := u.stories.AuthorsForStories(ctx, ids)
	if err != nil {
		return nil, err
	}
	covers, _ := u.stories.FirstChapterCoverURLs(ctx, ids)
	var uid *uint64
	if id, ok := middleware.GetUserID(ctx); ok {
		uid = &id
	}
	readNums, _ := u.stories.ReadChapterNumbers(ctx, ids, uid)
	items := make([]models.StoryListItem, 0, len(ids))
	for _, id := range ids {
		s, ok := byID[id]
		if !ok {
			continue
		}
		item := models.StoryListItem{
			Story:             s,
			Tags:              tagsMap[id],
			ChaptersCount:     counts[id],
			LikesCount:        likes[id],
			Author:            authors[id],
			ReadChapterNumber: readNums[id],
		}
		if url := covers[id]; url != "" {
			item.CoverURL = &url
		}
		items = append(items, item)
	}
	return items, nil
}

func (u *Usecase) CreateCollection(ctx context.Context, title string, description *string) (*models.UserCollection, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 200 {
		return nil, named_errors.ErrInvalidInput
	}
	var desc *string
	if description != nil {
		d := strings.TrimSpace(*description)
		if d != "" {
			if len(d) > 5000 {
				return nil, named_errors.ErrInvalidInput
			}
			desc = &d
		}
	}
	now := time.Now().UTC()
	c := models.UserCollection{
		ID:          uuid.New(),
		UserID:      userID,
		Title:       title,
		Description: desc,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := u.lib.InsertCollection(ctx, c); err != nil {
		return nil, err
	}
	return u.lib.GetCollectionByID(ctx, c.ID)
}

func (u *Usecase) UpdateCollection(ctx context.Context, id uuid.UUID, title *string, description *string) (*models.UserCollection, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}
	return u.lib.UpdateCollection(ctx, userID, id, title, description)
}

func (u *Usecase) DeleteCollection(ctx context.Context, id uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	return u.lib.DeleteCollection(ctx, userID, id)
}

func (u *Usecase) ListCollectionsForUser(ctx context.Context, userID uint64) ([]models.UserCollectionSummary, error) {
	return u.lib.ListCollectionSummaries(ctx, userID)
}

func (u *Usecase) GetCollectionDetail(ctx context.Context, collectionID uuid.UUID) (*models.UserCollectionDetail, error) {
	c, err := u.lib.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	return u.collectionDetail(ctx, c)
}

func (u *Usecase) collectionDetail(ctx context.Context, c *models.UserCollection) (*models.UserCollectionDetail, error) {
	ids, err := u.lib.ListCollectionStoryIDs(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &models.UserCollectionDetail{UserCollection: *c, Stories: []models.StoryListItem{}}, nil
	}
	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok && s.Status == "published" {
			filtered = append(filtered, id)
		}
	}
	stories, err := u.buildListItems(ctx, filtered, true)
	if err != nil {
		return nil, err
	}
	return &models.UserCollectionDetail{UserCollection: *c, Stories: stories}, nil
}

func (u *Usecase) AddStoryToCollection(ctx context.Context, collectionID, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	c, err := u.lib.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return err
	}
	if c.UserID != userID {
		return named_errors.ErrNoAccess
	}
	st, err := u.stories.GetByID(ctx, storyID)
	if err != nil {
		return err
	}
	if st.Status != "published" {
		return named_errors.ErrInvalidInput
	}
	return u.lib.AddCollectionStory(ctx, collectionID, storyID)
}

func (u *Usecase) RemoveStoryFromCollection(ctx context.Context, collectionID, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	c, err := u.lib.GetCollectionByID(ctx, collectionID)
	if err != nil {
		return err
	}
	if c.UserID != userID {
		return named_errors.ErrNoAccess
	}
	return u.lib.RemoveCollectionStory(ctx, collectionID, storyID)
}
