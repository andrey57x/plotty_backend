package adapters

import (
	"context"
	"fmt"
	"unicode"

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

func isStartOfSentence(text []rune, offset int) bool {
	for i := offset - 1; i >= 0; i-- {
		r := text[i]
		if unicode.IsSpace(r) || r == '"' || r == '«' || r == '»' || r == '\'' || r == '-' || r == '—' {
			continue
		}
		if r == '.' || r == '!' || r == '?' || r == '\n' || r == '…' {
			return true
		}
		return false
	}
	return true
}

func (a *LanguageToolAdapter) CheckText(ctx context.Context, text string) (models.SpellcheckResult, error) {
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

		if len(fragment) > 0 {
			firstRune := []rune(fragment)[0]
			if unicode.IsUpper(firstRune) && (m.Rule.ID == "MORFOLOGIK_RULE_RU_RU" || m.Rule.ID == "HUNSPELL_RULE" || m.Rule.ID == "HUNSPELL_NO_SUGGEST_RULE") && !isStartOfSentence(runeText, m.Offset) {
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
