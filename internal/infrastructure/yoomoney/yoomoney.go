package yoomoney

import (
	"crypto/sha1"
	"fmt"
	"net/url"
)

const quickPayURL = "https://yoomoney.ru/quickpay/confirm.xml"

func BuildPayURL(wallet, label, returnURL string, amountKopecks int) string {
	amount := fmt.Sprintf("%.2f", float64(amountKopecks)/100.0)
	params := url.Values{}
	params.Set("receiver", wallet)
	params.Set("quickpay-form", "button")
	params.Set("targets", "Покупка AI-кредитов")
	params.Set("paymentType", "AC")
	params.Set("sum", amount)
	params.Set("label", label)
	params.Set("successURL", returnURL)
	return quickPayURL + "?" + params.Encode()
}

func VerifyIPN(secret, notificationType, operationID, amount, currency, datetime, sender, codepro, label, receivedHash string) bool {
	data := notificationType + "&" + operationID + "&" + amount + "&" + currency + "&" + datetime + "&" + sender + "&" + codepro + "&" + secret + "&" + label
	h := sha1.Sum([]byte(data))
	return fmt.Sprintf("%x", h[:]) == receivedHash
}
