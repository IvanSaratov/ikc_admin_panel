package models

type MUser struct {
	ID         int    `json:"id"`
	Username   string `json:"username"`
	Password   string `json:"-"`
	EMail      string `json:"email"`
	FirstName  string `json:"firstname"`
	LastName   string `json:"lastname"`
	MiddleName string `json:"middlename"`
	SpecField  string `json:"idnumber,omitempty"`
	CreateDate string `json:"department,omitempty"`
}

type UsersResponse struct {
	Users []MUser `json:"users"`
}

type MCourse struct {
	ID        int    `json:"id"`
	ShortName string `json:"shortname"`
}

type MoodleErrorResponse struct {
	Exception string `json:"exception"`
	ErrorCode string `json:"errorcode"`
	Message   string `json:"message"`
}
