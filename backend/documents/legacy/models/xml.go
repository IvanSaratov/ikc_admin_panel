package models

import "time"

type Worker struct {
	DocumentID     int
	LastName       string `xml:"LastName"`
	FirstName      string `xml:"FirstName"`
	MiddleName     string `xml:"MiddleName"`
	Snils          string `xml:"Snils"`
	Email          string
	IsForeignSnils string `xml:"IsForeignSnils"`
	ForeignSnils   string `xml:"ForeignSnils"`
	Citizenship    string `xml:"Citizenship"`
	Position       string `xml:"Position"`
	EmployerInn    string `xml:"EmployerInn"`
	EmployerTitle  string `xml:"EmployerTitle"`
}

type Organization struct {
	Inn   string `xml:"Inn"`
	Title string `xml:"Title"`
}

type Test struct {
	IsPassedAttr       string    `xml:"isPassed,attr"`
	LearnProgramIdAttr string    `xml:"learnProgramId,attr"`
	Date               time.Time `xml:"Date"`
	ProtocolNumber     string    `xml:"ProtocolNumber"`
	LearnProgramTitle  string    `xml:"LearnProgramTitle"`
	EducationStart     time.Time `xml:"EducationStart"`
	EducationEnd       time.Time `xml:"EducationEnd"`
}

type RegistryRecord struct {
	OuterIdAttr  string        `xml:"outerId,attr,omitempty"`
	Worker       *Worker       `xml:"Worker"`
	Organization *Organization `xml:"Organization"`
	Test         *Test         `xml:"Test"`
}

type RegistrySet struct {
	RegistryRecord []*RegistryRecord `xml:"RegistryRecord"`
}
