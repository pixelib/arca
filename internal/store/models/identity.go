package models

// Identity ties an account to the subject an OpenID Connect issuer knows it by,
// which stays put when the user's email changes at the provider.
type Identity struct {
	Issuer  string `gorm:"primaryKey"`
	Subject string `gorm:"primaryKey"`
	UserID  string `gorm:"index;not null"`
}

func (Identity) TableName() string { return "user_identities" }
