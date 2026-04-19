package auth

import "github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

// AccountIDFromResult returns stable MSAL home account ID for future silent refresh.
func AccountIDFromResult(res public.AuthResult) string {
	return res.Account.HomeAccountID
}

// PreferredUsernameFromResult returns displayable account hint from MSAL result.
func PreferredUsernameFromResult(res public.AuthResult) string {
	return res.Account.PreferredUsername
}
