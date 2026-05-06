package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/fivecode/plotty/core/constants"
	credrepo "github.com/fivecode/plotty/core/credits/repository"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/internal/infrastructure/yoomoney"
)

type Usecase struct {
	repo        *credrepo.Repository
	wallet      string
	secret      string
	frontendURL string
}

func New(repo *credrepo.Repository, wallet, secret, frontendURL string) *Usecase {
	return &Usecase{repo: repo, wallet: wallet, secret: secret, frontendURL: frontendURL}
}

func (u *Usecase) GetBalance(ctx context.Context, userID uint64) (int, error) {
	return u.repo.GetBalance(ctx, userID)
}

func (u *Usecase) GetTransactions(ctx context.Context, userID uint64) ([]models.CreditTransaction, error) {
	return u.repo.GetTransactions(ctx, userID)
}

func (u *Usecase) GetPackages() []constants.CreditPackage {
	return constants.CreditPackages
}

func (u *Usecase) InitiatePurchase(userID uint64, packageID int) (string, error) {
	var pkg *constants.CreditPackage
	for _, p := range constants.CreditPackages {
		if p.ID == packageID {
			pkg = &p
			break
		}
	}
	if pkg == nil {
		return "", named_errors.ErrNotFound
	}

	nonce := make([]byte, 8)
	rand.Read(nonce)
	label := fmt.Sprintf("%d:%d:%s", userID, packageID, hex.EncodeToString(nonce))

	returnURL := u.frontendURL + "/credits"
	payURL := yoomoney.BuildPayURL(u.wallet, label, returnURL, pkg.PriceKopecks)
	return payURL, nil
}

func (u *Usecase) HandleIPN(ctx context.Context, notificationType, operationID, amount, currency, datetime, sender, codepro, label, sha1Hash string) error {
	if !yoomoney.VerifyIPN(u.secret, notificationType, operationID, amount, currency, datetime, sender, codepro, label, sha1Hash) {
		return named_errors.ErrInvalidInput
	}

	parts := strings.SplitN(label, ":", 3)
	if len(parts) < 2 {
		return named_errors.ErrInvalidInput
	}
	userID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return named_errors.ErrInvalidInput
	}
	packageID, err := strconv.Atoi(parts[1])
	if err != nil {
		return named_errors.ErrInvalidInput
	}

	var pkg *constants.CreditPackage
	for _, p := range constants.CreditPackages {
		if p.ID == packageID {
			pkg = &p
			break
		}
	}
	if pkg == nil {
		return named_errors.ErrNotFound
	}

	return u.repo.AddCredits(ctx, userID, pkg.Credits, label)
}
