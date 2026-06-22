package models

type UserResponse struct {
	ID           int    `json:"id"`
	FIO          string `json:"fio"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	IsError      bool   `json:"iserror"`
	ErrorMessage string `json:"errormessage,omitempty"`
}

type DownloadFile struct {
	FileName string
	FileType string
	Content  []byte
}
