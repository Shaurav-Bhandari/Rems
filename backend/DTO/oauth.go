package DTO

// GoogleUserInfo represents the user info returned by Google's userinfo endpoint.
type GoogleUserInfo struct {
	Sub           string `json:"sub"`            // Google's unique user ID
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}
