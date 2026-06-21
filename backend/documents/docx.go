package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy/models"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// protocolTemplateFS embeds the legacy DOCX template so the binary asset
// ships with the binary. The template lives in
// backend/documents/templates/protocol.docx.
//
//go:embed templates/protocol.docx
var protocolTemplateFS embed.FS

// protocolTemplateOnce + cached bytes so we read the embedded file once.
// Any read error is sticky: subsequent calls return it until the binary
// is rebuilt with a corrected asset.
var (
	protocolTemplateOnce    bool
	protocolTemplateCached  []byte
	protocolTemplateReadErr error
)

// ProtocolTemplate returns the legacy DOCX template bytes.
func ProtocolTemplate() ([]byte, error) {
	if protocolTemplateOnce {
		return protocolTemplateCached, protocolTemplateReadErr
	}
	data, err := protocolTemplateFS.ReadFile("templates/protocol.docx")
	protocolTemplateOnce = true
	protocolTemplateCached = data
	protocolTemplateReadErr = err
	return data, err
}

// GenerateDOCX produces the DOCX archive (ZIP of one .docx per program
// group) for a fixed protocol. Returns the bytes + generation_runs row.
//
// Frozen signature per core MVP plan §0.2:
//
//	func GenerateDOCX(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
//
// Behaviour parallels GenerateXML:
//   - Status must be >= fixed.
//   - legacy.CreateDocx is called with the adapter's RegistrySet + the
//     template bytes + a time-type key derived from the first program.
//   - The slice of DOCX byte streams is wrapped in a single ZIP archive
//     because that is what the browser downloads (one file, not many).
//   - generation_runs row recorded as 'success' or 'failed'.
func GenerateDOCX(ctx context.Context, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error) {
	svc := currentService()
	if svc == nil {
		return nil, nil, errors.New("documents: default service not initialised (call SetDefaultService in main)")
	}

	svc.recordAudit(ctx, "documents.generate.requested", protocolID, map[string]any{
		"type": "docx",
	})

	registrySet, err := renderRegistrySet(ctx, q, protocolID)
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "docx", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "docx",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("render registry: %w", err)
	}

	template, err := ProtocolTemplate()
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "docx", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "docx",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("load protocol template: %w", err)
	}

	// Determine the time-type key (А/Б/В/П/С) — we always use 'А' for
	// now because the adapter doesn't expose program hours. Future
	// enhancement: lift the program hours into the RegistryRecord so the
	// mapping can be per-program.
	timeType := "А"

	parts, err := legacyCreateDocx(registrySet, template, timeType)
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "docx", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "docx",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("legacy create docx: %w", err)
	}

	// Wrap the slice of DOCX streams in a single ZIP archive. Filenames
	// are derived from the program code on the matching RegistryRecord
	// so multiple programs in one protocol produce distinct, human-
	// readable archive entries.
	zipped, err := zipDocs(registrySet, parts)
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "docx", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "docx",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("zip docx parts: %w", err)
	}

	fileName := docxFileName(protocolID)
	run, err := svc.recordGenerationRun(ctx, protocolID, "docx", "success", fileName, "")
	if err != nil {
		svc.log.Error("insert generation_runs row after docx success", "protocol_id", protocolID, "err", err)
		return zipped, nil, nil
	}
	svc.recordAudit(ctx, "documents.generate.completed", protocolID, map[string]any{
		"type":      "docx",
		"bytes":     len(zipped),
		"run_id":    run.ID,
		"file_name": fileName,
	})
	return zipped, &run, nil
}

// zipDocs wraps each DOCX byte stream in a single ZIP archive. The
// adapter's RegistrySet groups records by LearnProgramIdAttr; we use
// the first record's program code as the entry name and fall back to
// "protocol_N.docx" when no records are present.
//
// Each entry is stored uncompressed — DOCX is already a ZIP, so the
// outer deflate overhead is wasted CPU. legacy.CreateDocx emits one
// DOCX per program code; ZIPping them together is what operators expect
// to download.
func zipDocs(registrySet *models.RegistrySet, parts [][]byte) ([]byte, error) {
	if len(parts) == 0 {
		return nil, errors.New("zipDocs: no DOCX parts to zip")
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Build a sorted list of program codes that appear in the registry
	// set so entry names match the order legacy.CreateDocx used to
	// produce the parts slice. The legacy pipeline sorts the keys
	// alphabetically; we mirror that here so archive entries line up
	// with their DOCX payload.
	programKeys := sortedProgramKeys(registrySet)

	for i, part := range parts {
		name := fmt.Sprintf("protocol_%d.docx", i+1)
		if i < len(programKeys) && programKeys[i] != "" {
			name = fmt.Sprintf("protocol_%s.docx", programKeys[i])
		}
		fw, err := w.Create(name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := fw.Write(part); err != nil {
			return nil, fmt.Errorf("write zip entry %s: %w", name, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// sortedProgramKeys returns the unique LearnProgramIdAttr values from
// the registry set, sorted alphabetically. The result matches the order
// legacy.CreateDocx uses when iterating groups, so the i-th DOCX part
// corresponds to the i-th key.
func sortedProgramKeys(registrySet *models.RegistrySet) []string {
	seen := map[string]bool{}
	var keys []string
	if registrySet == nil {
		return keys
	}
	for _, rec := range registrySet.RegistryRecord {
		if rec == nil || rec.Test == nil {
			continue
		}
		key := rec.Test.LearnProgramIdAttr
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	// In-place sort (small N; this is deterministic because the legacy
	// pipeline does the same).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
