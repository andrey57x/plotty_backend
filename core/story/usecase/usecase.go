package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/slug"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	"github.com/google/uuid"
)

type Usecase struct {
	stories  *storyrepo.Repository
	tags     *tagrepo.Repository
	chapters *chapterrepo.Repository
}

func New(stories *storyrepo.Repository, tags *tagrepo.Repository, chapters *chapterrepo.Repository) *Usecase {
	return &Usecase{stories: stories, tags: tags, chapters: chapters}
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	var out []uuid.UUID
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (u *Usecase) List(ctx context.Context, q string, tagSlugs []string, page, pageSize int) ([]models.StoryListItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	total, err := u.stories.CountList(ctx, q, tagSlugs)
	if err != nil {
		return nil, 0, err
	}
	ids, err := u.stories.ListIDs(ctx, q, tagSlugs, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []models.StoryListItem{}, total, nil
	}
	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	tagsMap, err := u.stories.TagsForStories(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	counts, err := u.stories.ChapterCounts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	likes, err := u.stories.LikeCounts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	authors, err := u.stories.AuthorsForStories(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	items := make([]models.StoryListItem, 0, len(ids))
	for _, id := range ids {
		s, ok := byID[id]
		if !ok {
			continue
		}
		items = append(items, models.StoryListItem{
			Story:         s,
			Tags:          tagsMap[id],
			ChaptersCount: counts[id],
			LikesCount:    likes[id],
			Author:        authors[id],
		})
	}
	return items, total, nil
}

func (u *Usecase) Create(ctx context.Context, title string, tagIDs []uuid.UUID) (*models.Story, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, named_errors.ErrInvalidInput
	}
	tagIDs = dedupeUUIDs(tagIDs)
	if err := u.tags.ValidateAllExist(ctx, tagIDs); err != nil {
		return nil, err
	}
	base := slug.FromTitle(title)
	s := models.Story{
		ID:       uuid.New(),
		Slug:     base,
		Title:    title,
		AuthorID: &userID,
	}
	for i := 0; i < 12; i++ {
		if i > 0 {
			s.Slug = fmt.Sprintf("%s-%d", base, i)
		}
		created, err := u.stories.Create(ctx, s, tagIDs)
		if err == nil {
			return created, nil
		}
		if errors.Is(err, named_errors.ErrConflict) {
			continue
		}
		return nil, err
	}
	return nil, named_errors.ErrConflict
}

func (u *Usecase) checkAuthor(ctx context.Context, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	story, err := u.stories.GetByID(ctx, storyID)
	if err != nil {
		return err
	}
	if story.AuthorID == nil || *story.AuthorID != userID {
		return named_errors.ErrNoAccess
	}
	return nil
}

func (u *Usecase) CheckAuthorByChapter(ctx context.Context, chapterID uuid.UUID) error {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return err
	}
	return u.checkAuthor(ctx, ch.StoryID)
}

func (u *Usecase) CheckAuthorByStory(ctx context.Context, storyID uuid.UUID) error {
	return u.checkAuthor(ctx, storyID)
}

func (u *Usecase) Update(ctx context.Context, id uuid.UUID, title *string, tagIDs *[]uuid.UUID) (*models.Story, error) {
	if err := u.checkAuthor(ctx, id); err != nil {
		return nil, err
	}
	if tagIDs != nil {
		*tagIDs = dedupeUUIDs(*tagIDs)
		if err := u.tags.ValidateAllExist(ctx, *tagIDs); err != nil {
			return nil, err
		}
	}
	return u.stories.Update(ctx, id, title, tagIDs)
}

func (u *Usecase) GetBySlug(ctx context.Context, storySlug string) (*models.StoryDetail, error) {
	s, err := u.stories.GetBySlug(ctx, storySlug)
	if err != nil {
		return nil, err
	}
	tags, err := u.stories.TagsForStory(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	chs, err := u.chapters.ListBriefByStory(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	likesCount, err := u.stories.LikeCount(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	author, err := u.stories.GetAuthorForStory(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	var likedByMe bool
	if uid, ok := middleware.GetUserID(ctx); ok {
		likedByMe, _ = u.stories.IsLikedByUser(ctx, s.ID, uid)
	}
	return &models.StoryDetail{
		Story:      *s,
		Tags:       tags,
		Chapters:   chs,
		LikesCount: likesCount,
		LikedByMe:  likedByMe,
		Author:     author,
	}, nil
}

func (u *Usecase) Delete(ctx context.Context, id uuid.UUID) error {
	if err := u.checkAuthor(ctx, id); err != nil {
		return err
	}
	return u.stories.Delete(ctx, id)
}
