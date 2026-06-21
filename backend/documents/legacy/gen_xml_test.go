package legacy

import (
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy/models"
)

func TestGenerateXML_Empty(t *testing.T) {
	data := &models.RegistrySet{}
	out, err := GenerateXML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	xml := string(out)
	if !strings.Contains(xml, "RegistrySet") {
		t.Fatalf("expected <RegistrySet> in output, got: %s", xml)
	}
}

func TestGenerateXML_SingleRecord(t *testing.T) {
	date := time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC)
	data := &models.RegistrySet{
		RegistryRecord: []*models.RegistryRecord{
			{
				Worker: &models.Worker{
					LastName:      "Иванов",
					FirstName:     "Иван",
					MiddleName:    "Иванович",
					Snils:         "123-456-789 00",
					Position:      "Инженер",
					EmployerInn:   "1234567890",
					EmployerTitle: "ООО Ромашка",
				},
				Organization: &models.Organization{
					Inn:   "9876543210",
					Title: "МинТруд",
				},
				Test: &models.Test{
					Date:               date,
					ProtocolNumber:     "42",
					LearnProgramTitle:  "Охрана труда",
					LearnProgramIdAttr: "OT-01",
				},
			},
		},
	}

	out, err := GenerateXML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	xml := string(out)

	checks := []string{
		"<RegistryRecord>",
		"<Worker>",
		"<LastName>Иванов</LastName>",
		"<FirstName>Иван</FirstName>",
		"<MiddleName>Иванович</MiddleName>",
		"<Snils>123-456-789 00</Snils>",
		"<Position>Инженер</Position>",
		"<EmployerInn>1234567890</EmployerInn>",
		"<EmployerTitle>ООО Ромашка</EmployerTitle>",
		"<Organization>",
		"<Inn>9876543210</Inn>",
		"<Title>МинТруд</Title>",
		`isPassed="true"`,
		`learnProgramId="OT-01"`,
		"<Date>2024-05-15</Date>",
	}

	for _, want := range checks {
		if !strings.Contains(xml, want) {
			t.Errorf("expected XML to contain %q\nfull output:\n%s", want, xml)
		}
	}
}

func TestGenerateXML_MultipleRecords(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	makeRecord := func(last string) *models.RegistryRecord {
		return &models.RegistryRecord{
			Worker: &models.Worker{
				LastName:      last,
				FirstName:     "Имя",
				MiddleName:    "Отчество",
				Snils:         "000-000-000 00",
				Position:      "Должность",
				EmployerInn:   "0000000000",
				EmployerTitle: "Организация",
			},
			Organization: &models.Organization{Inn: "1111111111", Title: "МинТруд"},
			Test: &models.Test{
				Date:               date,
				ProtocolNumber:     "1",
				LearnProgramTitle:  "Программа",
				LearnProgramIdAttr: "P-01",
			},
		}
	}

	data := &models.RegistrySet{
		RegistryRecord: []*models.RegistryRecord{
			makeRecord("Петров"),
			makeRecord("Сидоров"),
		},
	}

	out, err := GenerateXML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	xml := string(out)

	count := strings.Count(xml, "<RegistryRecord>")
	if count != 2 {
		t.Fatalf("expected 2 <RegistryRecord> elements, found %d\nfull output:\n%s", count, xml)
	}
}
