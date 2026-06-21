package legacy

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy/models"

	docx_tpl "github.com/fumiama/go-docx"
	"github.com/goodsign/monday"
	"github.com/lukasjarosch/go-docx"
)

const (
	// docxProtocolTableIndex — индекс (0-based) таблицы участников в шаблоне protocol.docx.
	// Шаблон содержит 3 таблицы: [0] шапка документа, [1] комиссия, [2] список участников.
	docxProtocolTableIndex = 2
	// docxTableColumnCount — количество колонок в таблице участников.
	// Порядок: №, Организация, ФИО, Должность, Результат, Дата, Рег.номер, Подпись.
	docxTableColumnCount = 8
	// testResultText — фиксированный результат для всех участников.
	// Неудовлетворительные результаты в реестр не попадают — такие строки не передаются в функцию.
	testResultText = "удовл"
)

// CreateDocx генерирует набор DOCX-протоколов — по одному на каждую программу обучения.
// Записи группируются по LearnProgramIdAttr; ключи сортируются для детерминированного
// порядка файлов в итоговом ZIP-архиве (порядок итерации по map в Go не определён).
func CreateDocx(data *models.RegistrySet, templatePath, timeType string, log FieldLogger) ([][]byte, error) {
	groups := make(map[string][]*models.RegistryRecord)
	for _, record := range data.RegistryRecord {
		groups[record.Test.LearnProgramIdAttr] = append(groups[record.Test.LearnProgramIdAttr], record)
	}

	// Сортируем ключи для стабильного порядка файлов в архиве.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	log.Infof("Генерация DOCX: групп программ=%d", len(keys))

	var result [][]byte
	for _, key := range keys {
		log.Infof("Создание протокола для программы %q (%d участников)", key, len(groups[key]))
		doc, err := internalCreateDocx(&models.RegistrySet{RegistryRecord: groups[key]}, templatePath, timeType, log)
		if err != nil {
			return nil, err
		}
		result = append(result, doc)
	}

	return result, nil
}

// internalCreateDocx генерирует один DOCX-протокол для одной программы обучения.
//
// Используется двухэтапный pipeline из двух разных DOCX-библиотек, поскольку ни одна
// из доступных библиотек Go не умеет делать оба действия одновременно:
//
//  1. lukasjarosch/go-docx  — подстановка текстовых плейсхолдеров ({placeholder}) в шаблон.
//     Библиотека работает с XML напрямую и хорошо справляется с заменой, но не поддерживает
//     добавление строк в таблицы.
//
//  2. fumiama/go-docx — манипуляции со структурой документа (добавление строк в таблицу).
//     Библиотека понимает структуру OOXML, но не имеет механизма замены плейсхолдеров.
//
// Результат первого этапа записывается в bytes.Buffer и передаётся парсеру второго этапа.
func internalCreateDocx(data *models.RegistrySet, templatePath, timeType string, log FieldLogger) ([]byte, error) {
	// Этап 1: подстановка плейсхолдеров через lukasjarosch/go-docx.
	doc, err := docx.OpenBytes([]byte(templatePath))
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	replaceMap := docx.PlaceholderMap{
		"people_count":    len(data.RegistryRecord),
		"user_program":    data.RegistryRecord[0].Test.LearnProgramTitle,
		"program_time":    models.TIME_TO_TYPE[timeType],
		"protocol_number": data.RegistryRecord[0].Test.ProtocolNumber,
		"protocol_date":   monday.Format(data.RegistryRecord[0].Test.Date, monday.LongFormatsByLocale[monday.LocaleRuRU], monday.LocaleRuRU),
		"education_start": monday.Format(data.RegistryRecord[0].Test.EducationStart, monday.LongFormatsByLocale[monday.LocaleRuRU], monday.LocaleRuRU),
		"education_end":   monday.Format(data.RegistryRecord[0].Test.EducationEnd, monday.LongFormatsByLocale[monday.LocaleRuRU], monday.LocaleRuRU),
	}

	if err := doc.ReplaceAll(replaceMap); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := doc.Write(buf); err != nil {
		return nil, err
	}

	log.Debugf("Плейсхолдеры подставлены, размер документа после замены: %d байт", buf.Len())

	// Этап 2: добавление строк в таблицу участников через fumiama/go-docx.
	reader := bytes.NewReader(buf.Bytes())
	doc_tpl, err := docx_tpl.Parse(reader, int64(buf.Len()))
	if err != nil {
		return nil, err
	}

	// Ищем таблицу участников по её порядковому индексу в документе.
	// tableCount считает только элементы типа *Table — другие типы элементов тела (параграфы и т.д.)
	// счётчик не увеличивают, поэтому счётчик обновляется только в ветке case *docx_tpl.Table.
	tableCount := 0
	for _, item := range doc_tpl.Document.Body.Items {
		switch item := item.(type) {
		case *docx_tpl.Table:
			if tableCount == docxProtocolTableIndex {
				// Сначала добавляем пустые строки (по одной на участника),
				// затем заполняем их — это избегает проблем с индексами при одновременном
				// добавлении и записи.
				for range len(data.RegistryRecord) {
					cells := make([]*docx_tpl.WTableCell, docxTableColumnCount)
					for i := range cells {
						cells[i] = &docx_tpl.WTableCell{}
					}
					item.TableRows = append(
						item.TableRows,
						&docx_tpl.WTableRow{TableCells: cells},
					)
				}

				// index+1: строка [0] — заголовок таблицы из шаблона, данные начинаются с [1].
				for index := range len(data.RegistryRecord) {
					row := item.TableRows[index+1]
					row.TableCells[0].AddParagraph().AddText(strconv.Itoa(index+1)).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                                                       // Порядковый номер
					row.TableCells[1].AddParagraph().AddText(data.RegistryRecord[index].Worker.EmployerTitle).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                             // Наименование организации
					row.TableCells[2].AddParagraph().AddText(data.RegistryRecord[index].Worker.LastName+" "+data.RegistryRecord[index].Worker.FirstName+" "+data.RegistryRecord[index].Worker.MiddleName).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman") // ФИО
					row.TableCells[3].AddParagraph().AddText(data.RegistryRecord[index].Worker.Position).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                                  // Должность
					row.TableCells[4].AddParagraph().AddText(testResultText).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                                                              // Результат
					row.TableCells[5].AddParagraph().AddText(data.RegistryRecord[index].Test.Date.Format("02.01.2006")).Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                   // Дата проверки
					row.TableCells[6].AddParagraph().AddText("").Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                                                                          // Регистрационный номер (заполняется вручную)
					row.TableCells[7].AddParagraph().AddText("").Font("Times New Roman", "Times New Roman", "Times New Roman", "Times New Roman")                                                                                                                                          // Подпись (заполняется вручную)
				}

				log.Debugf("Таблица участников заполнена: %d строк добавлено", len(data.RegistryRecord))
			}
			tableCount++
		}
	}

	// Записываем итоговый документ.
	// bufio.Writer используется потому, что fumiama/go-docx пишет множество мелких кусков;
	// буферизация снижает число аллокаций.
	newBuf := new(bytes.Buffer)
	writer := bufio.NewWriter(newBuf)
	if _, err := doc_tpl.WriteTo(writer); err != nil {
		return nil, err
	}
	// Flush обязателен: bufio.Writer буферизует запись, без Flush последние байты
	// (в том числе центральный каталог ZIP) могут не попасть в newBuf.
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	// fumiama/go-docx парсит <w:sectPr> частично: из <w:pgSz> сохраняет только w/h,
	// теряя w:orient="landscape" и все остальные sub-элементы (pgMar, cols, docGrid).
	// Восстанавливаем оригинальный sectPr из шаблона прямым патчингом ZIP.
	patched, err := restoreSectPr(newBuf.Bytes(), []byte(templatePath))
	if err != nil {
		log.Warnf("Не удалось восстановить sectPr из шаблона: %v", err)
		return newBuf.Bytes(), nil
	}

	return patched, nil
}

// extractSectPr извлекает сырой XML-блок <w:sectPr>…</w:sectPr> из word/document.xml в DOCX-файле.
func extractSectPr(docxBytes []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return nil, err
	}
	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		start := bytes.Index(content, []byte("<w:sectPr"))
		if start == -1 {
			return nil, fmt.Errorf("sectPr не найден в word/document.xml")
		}
		if end := bytes.Index(content[start:], []byte("</w:sectPr>")); end != -1 {
			return content[start : start+end+len("</w:sectPr>")], nil
		}
		if end := bytes.Index(content[start:], []byte("/>")); end != -1 {
			return content[start : start+end+2], nil
		}
		return nil, fmt.Errorf("закрывающий тег sectPr не найден")
	}
	return nil, fmt.Errorf("word/document.xml не найден в DOCX")
}

// injectSectPr заменяет sectPr в XML документа на указанный.
// Если sectPr в docXML отсутствует — вставляет перед </w:body>.
func injectSectPr(docXML, sectPr []byte) []byte {
	start := bytes.Index(docXML, []byte("<w:sectPr"))
	if start == -1 {
		bodyEnd := bytes.Index(docXML, []byte("</w:body>"))
		if bodyEnd == -1 {
			return docXML
		}
		result := make([]byte, 0, len(docXML)+len(sectPr))
		result = append(result, docXML[:bodyEnd]...)
		result = append(result, sectPr...)
		return append(result, docXML[bodyEnd:]...)
	}

	var sectEnd int
	if end := bytes.Index(docXML[start:], []byte("</w:sectPr>")); end != -1 {
		sectEnd = start + end + len("</w:sectPr>")
	} else if end := bytes.Index(docXML[start:], []byte("/>")); end != -1 {
		sectEnd = start + end + 2
	} else {
		return docXML
	}

	result := make([]byte, 0, len(docXML)+len(sectPr))
	result = append(result, docXML[:start]...)
	result = append(result, sectPr...)
	return append(result, docXML[sectEnd:]...)
}

// restoreSectPr патчит сгенерированный DOCX: заменяет обрезанный sectPr
// оригинальным из шаблона (fumiama/go-docx теряет orient и pgMar при парсинге).
func restoreSectPr(generatedBytes, templateBytes []byte) ([]byte, error) {
	originalSectPr, err := extractSectPr(templateBytes)
	if err != nil {
		return nil, fmt.Errorf("extractSectPr из шаблона: %w", err)
	}

	r, err := zip.NewReader(bytes.NewReader(generatedBytes), int64(len(generatedBytes)))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for _, f := range r.File {
		fw, err := w.Create(f.Name)
		if err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if f.Name == "word/document.xml" {
			content = injectSectPr(content, originalSectPr)
		}
		if _, err = fw.Write(content); err != nil {
			return nil, err
		}
	}

	if err = w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
