package models

type User struct {
	ID                int    `json:"id"`
	Email             string `json:"email"`
	PasswordHash      string `json:"-"`
	Verified          bool   `json:"verified"`
	VerificationToken string `json:"-"`
}

type GuessInput struct {
	WrestlerID int    `json:"wrestler_id"`
	GuessDate  string `json:"guess_date"`
	GuessOrder int    `json:"guess_order"`
}

type Guess struct {
	Name          string `json:"name"`
	WeightClass   string `json:"weight_class"`
	Year          string `json:"year"`
	Team          string `json:"team"`
	Conference    string `json:"conference"`
	WinPercentage string `json:"win_percentage"`
	NcaaFinish    string `json:"ncaa_finish"`
	GuessOrder    int    `json:"guess_order"`
}
