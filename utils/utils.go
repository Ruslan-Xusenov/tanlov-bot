package utils

import "fmt"

// BuildReferralLink returns a t.me deep-link with the user's ID as referral code
func BuildReferralLink(botUsername string, userID int64) string {
	return fmt.Sprintf("https://t.me/%s?start=ref_%d", botUsername, userID)
}
