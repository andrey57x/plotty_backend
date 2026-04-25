package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/ml"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/slug"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
)

type Usecase struct {
	stories  *storyrepo.Repository
	tags     *tagrepo.Repository
	chapters *chapterrepo.Repository
	mlClient *ml.Client
}

func New(stories *storyrepo.Repository, tags *tagrepo.Repository, chapters *chapterrepo.Repository, mlClient *ml.Client) *Usecase {
	return &Usecase{stories: stories, tags: tags, chapters: chapters, mlClient: mlClient}
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
	log := logger.FromContext(ctx)

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

	var ids []uuid.UUID
	var total int
	var err error

	words := strings.Fields(q)
	isSemantic := len(words) > 2

	if isSemantic {
		semanticIDs, mlErr := u.mlClient.SearchSemantic(ctx, q)
		if mlErr == nil && len(semanticIDs) > 0 {
			total, err = u.stories.CountSemanticIDs(ctx, semanticIDs, tagSlugs)
			if err == nil {
				ids, err = u.stories.ListSemanticIDs(ctx, semanticIDs, tagSlugs, pageSize, offset)
			}
		} else {
			isSemantic = false
		}
	}

	if !isSemantic {
		total, err = u.stories.CountList(ctx, q, tagSlugs)
		if err != nil {
			log.Error().Err(err).Msg("story_uc: CountList failed")
			return nil, 0, fmt.Errorf("story_uc.List count: %w", err)
		}
		ids, err = u.stories.ListIDs(ctx, q, tagSlugs, pageSize, offset)
		if err != nil {
			log.Error().Err(err).Msg("story_uc: ListIDs failed")
			return nil, 0, fmt.Errorf("story_uc.List ids: %w", err)
		}
	}

	if len(ids) == 0 {
		return []models.StoryListItem{}, total, nil
	}

	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: LoadStoriesByIDs failed")
		return nil, 0, fmt.Errorf("story_uc.List load: %w", err)
	}
	tagsMap, err := u.stories.TagsForStories(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: TagsForStories failed")
		return nil, 0, fmt.Errorf("story_uc.List tags: %w", err)
	}
	counts, err := u.stories.ChapterCountsPublished(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: ChapterCountsPublished failed")
		return nil, 0, fmt.Errorf("story_uc.List chapter counts: %w", err)
	}
	likes, err := u.stories.LikeCounts(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: LikeCounts failed")
		return nil, 0, fmt.Errorf("story_uc.List likes: %w", err)
	}
	authors, err := u.stories.AuthorsForStories(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: AuthorsForStories failed")
		return nil, 0, fmt.Errorf("story_uc.List authors: %w", err)
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

	log.Info().Int("total", total).Int("returned", len(items)).Bool("semantic", isSemantic).Msg("story_uc: list ok")
	return items, total, nil
}

func (u *Usecase) ListMy(ctx context.Context, q string, tagSlugs []string, page, pageSize int) ([]models.StoryListItem, int, error) {
	log := logger.FromContext(ctx)

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		log.Warn().Msg("story_uc: list_my without auth")
		return nil, 0, named_errors.ErrNoAccess
	}

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

	total, err := u.stories.CountMyList(ctx, userID, q, tagSlugs)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: CountMyList failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy count: %w", err)
	}
	ids, err := u.stories.ListMyIDs(ctx, userID, q, tagSlugs, pageSize, offset)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: ListMyIDs failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy ids: %w", err)
	}
	if len(ids) == 0 {
		return []models.StoryListItem{}, total, nil
	}

	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: LoadStoriesByIDs failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy load: %w", err)
	}
	tagsMap, err := u.stories.TagsForStories(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: TagsForStories failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy tags: %w", err)
	}
	counts, err := u.stories.ChapterCounts(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: ChapterCounts failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy chapter counts: %w", err)
	}
	likes, err := u.stories.LikeCounts(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: LikeCounts failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy likes: %w", err)
	}
	authors, err := u.stories.AuthorsForStories(ctx, ids)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: AuthorsForStories failed")
		return nil, 0, fmt.Errorf("story_uc.ListMy authors: %w", err)
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

	log.Info().Uint64("user_id", userID).Int("total", total).Int("returned", len(items)).Msg("story_uc: list_my ok")
	return items, total, nil
}

func (u *Usecase) ListPublishedByAuthor(ctx context.Context, authorID uint64, q string, tagSlugs []string, page, pageSize int) ([]models.StoryListItem, int, error) {
	log := logger.FromContext(ctx)

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

	total, err := u.stories.CountPublishedByAuthor(ctx, authorID, q, tagSlugs)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: CountPublishedByAuthor failed")
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor count: %w", err)
	}
	ids, err := u.stories.ListPublishedIDsByAuthor(ctx, authorID, q, tagSlugs, pageSize, offset)
	if err != nil {
		log.Error().Err(err).Msg("story_uc: ListPublishedIDsByAuthor failed")
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor ids: %w", err)
	}
	if len(ids) == 0 {
		return []models.StoryListItem{}, total, nil
	}

	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor load: %w", err)
	}
	tagsMap, err := u.stories.TagsForStories(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor tags: %w", err)
	}
	counts, err := u.stories.ChapterCountsPublished(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor chapter counts: %w", err)
	}
	likes, err := u.stories.LikeCounts(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor likes: %w", err)
	}
	authors, err := u.stories.AuthorsForStories(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("story_uc.ListPublishedByAuthor authors: %w", err)
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
	log := logger.FromContext(ctx)

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		log.Warn().Msg("story_uc: create without auth")
		return nil, named_errors.ErrNoAccess
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, named_errors.ErrInvalidInput
	}
	tagIDs = dedupeUUIDs(tagIDs)
	if err := u.tags.ValidateAllExist(ctx, tagIDs); err != nil {
		log.Warn().Err(err).Msg("story_uc: invalid tag ids")
		return nil, fmt.Errorf("story_uc.Create validate tags: %w", err)
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
			log.Info().Stringer("story_id", created.ID).Uint64("author_id", userID).Msg("story_uc: created")
			return created, nil
		}
		if errors.Is(err, named_errors.ErrConflict) {
			continue
		}
		return nil, fmt.Errorf("story_uc.Create: %w", err)
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
		return fmt.Errorf("story_uc.checkAuthor get story: %w", err)
	}
	if story.AuthorID == nil || *story.AuthorID != userID {
		logger.Ctx(ctx).Warn().
			Uint64("user_id", userID).
			Stringer("story_id", storyID).
			Msg("story_uc: access denied, not author")
		return named_errors.ErrNoAccess
	}
	return nil
}

func (u *Usecase) CheckAuthorByChapter(ctx context.Context, chapterID uuid.UUID) error {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return fmt.Errorf("story_uc.CheckAuthorByChapter: %w", err)
	}
	return u.checkAuthor(ctx, ch.StoryID)
}

func (u *Usecase) CheckAuthorByStory(ctx context.Context, storyID uuid.UUID) error {
	return u.checkAuthor(ctx, storyID)
}

func (u *Usecase) Update(ctx context.Context, id uuid.UUID, title *string, tagIDs *[]uuid.UUID) (*models.Story, error) {
	log := logger.FromContext(ctx)

	if err := u.checkAuthor(ctx, id); err != nil {
		return nil, err
	}
	if tagIDs != nil {
		*tagIDs = dedupeUUIDs(*tagIDs)
		if err := u.tags.ValidateAllExist(ctx, *tagIDs); err != nil {
			log.Warn().Err(err).Stringer("story_id", id).Msg("story_uc: invalid tag ids on update")
			return nil, fmt.Errorf("story_uc.Update validate tags: %w", err)
		}
	}
	s, err := u.stories.Update(ctx, id, title, tagIDs)
	if err != nil {
		return nil, fmt.Errorf("story_uc.Update: %w", err)
	}

	log.Info().Stringer("story_id", id).Msg("story_uc: updated")
	return s, nil
}

func (u *Usecase) GetBySlug(ctx context.Context, storySlug string) (*models.StoryDetail, error) {
	log := logger.FromContext(ctx)

	s, err := u.stories.GetBySlug(ctx, storySlug)
	if err != nil {
		return nil, fmt.Errorf("story_uc.GetBySlug story: %w", err)
	}
	if s.Status != "published" {
		if err := u.checkAuthor(ctx, s.ID); err != nil {
			return nil, err
		}
	}
	tags, err := u.stories.TagsForStory(ctx, s.ID)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", s.ID).Msg("story_uc: TagsForStory failed")
		return nil, fmt.Errorf("story_uc.GetBySlug tags: %w", err)
	}
	chs, err := u.chapters.ListBriefByStory(ctx, s.ID)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", s.ID).Msg("story_uc: ListBriefByStory failed")
		return nil, fmt.Errorf("story_uc.GetBySlug chapters: %w", err)
	}
	likesCount, err := u.stories.LikeCount(ctx, s.ID)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", s.ID).Msg("story_uc: LikeCount failed")
		return nil, fmt.Errorf("story_uc.GetBySlug likes: %w", err)
	}
	author, err := u.stories.GetAuthorForStory(ctx, s.ID)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", s.ID).Msg("story_uc: GetAuthorForStory failed")
		return nil, fmt.Errorf("story_uc.GetBySlug author: %w", err)
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
	if err := u.stories.Delete(ctx, id); err != nil {
		return fmt.Errorf("story_uc.Delete: %w", err)
	}
	logger.Ctx(ctx).Info().Stringer("story_id", id).Msg("story_uc: deleted")

	_ = rabbitmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(),
		Type:    "delete_story_lore",
		Metadata: map[string]string{
			"story_id": id.String(),
		},
	}

	// TODO: delete chapters

	return nil
}

func (u *Usecase) GetSimilar(ctx context.Context, storyID uuid.UUID) ([]models.StoryListItem, error) {
	log := logger.FromContext(ctx)

	ids, err := u.mlClient.GetSimilarStories(ctx, storyID)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", storyID).Msg("story_uc: ml GetSimilarStories failed")
		return nil, fmt.Errorf("story_uc.GetSimilar ml: %w", err)
	}

	if len(ids) == 0 {
		return []models.StoryListItem{}, nil
	}

	byID, err := u.stories.LoadStoriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tagsMap, _ := u.stories.TagsForStories(ctx, ids)
	counts, _ := u.stories.ChapterCountsPublished(ctx, ids)
	likes, _ := u.stories.LikeCounts(ctx, ids)
	authors, _ := u.stories.AuthorsForStories(ctx, ids)

	items := make([]models.StoryListItem, 0, len(ids))
	for _, id := range ids {
		s, ok := byID[id]
		if !ok || s.Status != "published" {
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

	return items, nil
}

func (u *Usecase) GetAnalytics(ctx context.Context, storyID uuid.UUID) ([]models.ChapterAnalytics, error) {
	if err := u.checkAuthor(ctx, storyID); err != nil {
		return nil, err
	}
	return u.chapters.GetStoryAnalytics(ctx, storyID)
}
