package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/fivecode/plotty/internal/infrastructure/languagetool"
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/stemmer"
)

type LanguageToolAdapter struct {
	client *languagetool.Client
}

func NewLanguageToolAdapter(client *languagetool.Client) *LanguageToolAdapter {
	return &LanguageToolAdapter{
		client: client,
	}
}

func (a *LanguageToolAdapter) CheckText(ctx context.Context, text string, allowedStems map[string]struct{}) (models.SpellcheckResult, error) {
	resp, err := a.client.Check(ctx, text)
	if err != nil {
		return models.SpellcheckResult{}, err
	}

	res := models.SpellcheckResult{
		Items: make([]models.SpellcheckItem, 0),
	}

	runeText := []rune(text)

	for _, m := range resp.Matches {
		fragment := ""
		if m.Offset+m.Length <= len(runeText) {
			fragment = string(runeText[m.Offset : m.Offset+m.Length])
		}

		cleanFrag := strings.ToLower(strings.TrimSpace(fragment))

		words := stemmer.WordRegex.FindAllString(cleanFrag, -1)
		if len(words) > 0 {
			allMatch := true
			for _, w := range words {
				stem := stemmer.CleanAndStemToken(w)
				if stem == "" {
					continue // Игнорируем предлоги и союзы во фрагменте
				}
				if _, ok := allowedStems[stem]; !ok {
					allMatch = false
					break
				}
			}
			// Если все составные части слова совпали с нашими исключениями — не считаем это ошибкой!
			if allMatch {
				continue
			}
		}

		suggestion := ""
		if len(m.Replacements) > 0 {
			suggestion = m.Replacements[0].Value
		}

		res.Items = append(res.Items, models.SpellcheckItem{
			FragmentText: fragment,
			StartOffset:  m.Offset,
			EndOffset:    m.Offset + m.Length,
			Message:      m.Message,
			Suggestion:   suggestion,
		})
	}

	res.Summary = fmt.Sprintf("Найдено возможных ошибок: %d", len(res.Items))

	return res, nil
}
