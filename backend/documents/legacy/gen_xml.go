// Package legacy is a vendored, lightly-patched copy of the
// mintrud_generator XML/DOCX generator code. It exists so the D3 slice can
// produce Mintrud-compliant XML and DOCX outputs without taking on the
// legacy repo's CLI/Moodle/MSI/server surface.
//
// Behaviour MUST stay aligned with the original: any change here should be
// mirrored upstream. The package is intentionally leaky about model types
// because the adapter layer (see backend/documents/adapter_legacy.go) is
// the single translation point between our domain and the legacy
// RegistrySet / RegistryRecord types.
package legacy

import (
	"bytes"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy/models"
	"github.com/shabbyrobe/xmlwriter"
)

// GenerateXML формирует XML-реестр для подачи в систему МинТруда.
// Структура выходного файла соответствует схеме RegistrySet с вложенными RegistryRecord.
//
// ErrCollector используется вместо пошаговой проверки каждой операции записи:
// xmlwriter накапливает ошибки внутри и останавливает запись после первой из них,
// что позволяет читать построение документа линейно, без вложенных if-err блоков.
func GenerateXML(data *models.RegistrySet) ([]byte, error) {
	b := &bytes.Buffer{}
	w := xmlwriter.Open(b, xmlwriter.WithIndentString("    "))
	ec := &xmlwriter.ErrCollector{}

	records := []xmlwriter.Writable{}
	for _, elem := range data.RegistryRecord {
		records = append(records, generateRegistryElem(elem))
	}
	ec.Do(
		w.StartDoc(xmlwriter.Doc{}.WithStandalone(true)),
		w.Start(xmlwriter.Elem{
			Name:    "RegistrySet",
			Content: records,
		}),
		w.EndDoc(),
		w.EndAllFlush(),
	)
	if ec.Err != nil {
		return nil, ec.Err
	}

	return b.Bytes(), nil
}

// generateRegistryElem строит XML-элемент RegistryRecord для одного сотрудника.
//
// Full: true на каждом элементе принудительно генерирует закрывающий тег (<Tag></Tag>)
// вместо самозакрывающегося (<Tag/>). Валидатор реестра МинТруда не принимает
// самозакрывающиеся теги для пустых полей (например, пустое отчество).
//
// isPassed всегда true: незачёты не попадают в реестр на уровне входных данных
// (такие строки не включаются в XLSX, переданный пользователем).
func generateRegistryElem(data *models.RegistryRecord) xmlwriter.Elem {
	result := xmlwriter.Elem{
		Name: "RegistryRecord",
		Content: []xmlwriter.Writable{
			xmlwriter.Elem{
				Name: "Worker",
				Full: true,
				Content: []xmlwriter.Writable{
					xmlwriter.Elem{
						Name:    "LastName",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.LastName)},
					},
					xmlwriter.Elem{
						Name:    "FirstName",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.FirstName)},
					},
					xmlwriter.Elem{
						Name:    "MiddleName",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.MiddleName)},
					},
					xmlwriter.Elem{
						Name:    "Snils",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.Snils)},
					},
					xmlwriter.Elem{
						Name:    "Position",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.Position)},
					},
					xmlwriter.Elem{
						Name:    "EmployerInn",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.EmployerInn)},
					},
					xmlwriter.Elem{
						Name:    "EmployerTitle",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Worker.EmployerTitle)},
					},
				},
			},
			xmlwriter.Elem{
				Name: "Organization",
				Full: true,
				Content: []xmlwriter.Writable{
					xmlwriter.Elem{
						Name:    "Inn",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Organization.Inn)},
					},
					xmlwriter.Elem{
						Name:    "Title",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Organization.Title)},
					},
				},
			},
			xmlwriter.Elem{
				Name: "Test",
				Full: true,
				Content: []xmlwriter.Writable{
					xmlwriter.Elem{
						Name:    "Date",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Test.Date.Format("2006-01-02"))},
					},
					xmlwriter.Elem{
						Name:    "ProtocolNumber",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Test.ProtocolNumber)},
					},
					xmlwriter.Elem{
						Name:    "LearnProgramTitle",
						Full:    true,
						Content: []xmlwriter.Writable{xmlwriter.Text(data.Test.LearnProgramTitle)},
					},
				},
				Attrs: []xmlwriter.Attr{
					xmlwriter.Attr{Name: "isPassed"}.Bool(true),
					{Name: "learnProgramId", Value: data.Test.LearnProgramIdAttr},
				},
			},
		},
	}

	return result
}
