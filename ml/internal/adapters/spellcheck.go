package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/fivecode/plotty/internal/infrastructure/languagetool"
	"github.com/fivecode/plotty/ml/internal/models"
)

type LanguageToolAdapter struct {
	client *languagetool.Client
}

func NewLanguageToolAdapter(client *languagetool.Client) *LanguageToolAdapter {
	return &LanguageToolAdapter{
		client: client,
	}
}

func (a *LanguageToolAdapter) CheckText(ctx context.Context, text string, allowedWords []string) (models.SpellcheckResult, error) {
	resp, err := a.client.Check(ctx, text)
	if err != nil {
		return models.SpellcheckResult{}, err
	}

	allowedMap := make(map[string]struct{}, len(allowedWords))
	for _, w := range allowedWords {
		cleanWord := strings.ToLower(strings.TrimSpace(w))
		if cleanWord != "" {
			parts := strings.Fields(cleanWord)
			for _, p := range parts {
				allowedMap[p] = struct{}{}
			}
		}
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
		if _, ok := allowedMap[cleanFrag]; ok {
			continue
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
