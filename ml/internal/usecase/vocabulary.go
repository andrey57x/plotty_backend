package usecase

import (
	"context"
	"sync"

	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/fivecode/plotty/ml/internal/stemmer"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type VocabularyManager struct {
	repo repository.MLRepository

	fandomMutex sync.RWMutex
	fandomStems map[string]map[string]struct{}

	storyMutex sync.RWMutex
	storyStems map[uuid.UUID]map[string]struct{}
}

func NewVocabularyManager(repo repository.MLRepository) *VocabularyManager {
	return &VocabularyManager{
		repo:        repo,
		fandomStems: make(map[string]map[string]struct{}),
		storyStems:  make(map[uuid.UUID]map[string]struct{}),
	}
}

// GetFandomVocabulary лениво загружает канонические имена и тексты фактов фэндома
func (vm *VocabularyManager) GetFandomVocabulary(ctx context.Context, fandomSlug string) map[string]struct{} {
	if fandomSlug == "" {
		return nil
	}

	vm.fandomMutex.RLock()
	vocab, exists := vm.fandomStems[fandomSlug]
	vm.fandomMutex.RUnlock()
	if exists {
		return vocab
	}

	vm.fandomMutex.Lock()
	defer vm.fandomMutex.Unlock()

	// Double-check после захвата блокировки
	if vocab, exists = vm.fandomStems[fandomSlug]; exists {
		return vocab
	}

	log.Info().Str("fandom_slug", fandomSlug).Msg("Инициализация in-memory словаря исключений для фэндома")
	vocab = make(map[string]struct{})

	// 1. Достаем имена каноничных сущностей и стеммим их
	canonNames, err := vm.repo.GetCanonEntityNames(ctx, fandomSlug)
	if err == nil {
		for _, name := range canonNames {
			for stem := range stemmer.ExtractStemsFromText(name) {
				vocab[stem] = struct{}{}
			}
		}
	}

	// 2. Достаем тексты каноничных фактов и стеммим их
	factTexts, err := vm.repo.GetFandomFactTexts(ctx, fandomSlug)
	if err == nil {
		for _, text := range factTexts {
			for stem := range stemmer.ExtractStemsFromText(text) {
				vocab[stem] = struct{}{}
			}
		}
	}

	vm.fandomStems[fandomSlug] = vocab
	return vocab
}

// GetStoryVocabulary лениво загружает уникальный словарь персонажей и объектов фанфика
func (vm *VocabularyManager) GetStoryVocabulary(ctx context.Context, storyID uuid.UUID) map[string]struct{} {
	if storyID == uuid.Nil {
		return nil
	}

	vm.storyMutex.RLock()
	vocab, exists := vm.storyStems[storyID]
	vm.storyMutex.RUnlock()
	if exists {
		return vocab
	}

	vm.storyMutex.Lock()
	defer vm.storyMutex.Unlock()

	// Double-check
	if vocab, exists = vm.storyStems[storyID]; exists {
		return vocab
	}

	log.Info().Str("story_id", storyID.String()).Msg("Инициализация in-memory словаря исключений для фанфика")
	vocab = make(map[string]struct{})

	storyNames, err := vm.repo.GetStoryLoreNames(ctx, storyID)
	if err == nil {
		for _, name := range storyNames {
			for stem := range stemmer.ExtractStemsFromText(name) {
				vocab[stem] = struct{}{}
			}
		}
	}

	vm.storyStems[storyID] = vocab
	return vocab
}

// InvalidateStoryVocabulary полностью очищает кеш истории в памяти.
// Вызывается после извлечения нового лора в extract_lore.
func (vm *VocabularyManager) InvalidateStoryVocabulary(storyID uuid.UUID) {
	vm.storyMutex.Lock()
	defer vm.storyMutex.Unlock()
	delete(vm.storyStems, storyID)
	log.Debug().Str("story_id", storyID.String()).Msg("Сброшен инмемори кеш словаря фанфика")
}
