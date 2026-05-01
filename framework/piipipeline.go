package framework

import (
	"fmt"
	"os"
	"strings"

	"github.com/karldane/go-presidio/presidio"
	"github.com/karldane/go-presidio/recognizers"
)

// toPresidioHints converts framework ColumnHints to the presidio type.
// This is the only place in the framework that crosses this boundary.
func toPresidioHints(hints map[string]ColumnHint) map[string]presidio.ColumnHint {
	if hints == nil {
		return nil
	}
	out := make(map[string]presidio.ColumnHint, len(hints))
	for col, h := range hints {
		out[col] = presidio.ColumnHint{
			ScanPolicy: presidio.ScanPolicy(h.ScanPolicy),
			MaxLength:  h.MaxLength,
			OracleType: h.EntityType,
		}
	}
	return out
}

// fromPresidioReports converts presidio ColumnReports to the framework type.
func fromPresidioReports(reports []presidio.ColumnReport) []ColumnReport {
	out := make([]ColumnReport, len(reports))
	for i, r := range reports {
		entityStrs := make([]string, len(r.PIIEntities))
		for j, e := range r.PIIEntities {
			entityStrs[j] = string(e)
		}
		out[i] = ColumnReport{
			ColumnName:     r.ColumnName,
			PIIDetected:    r.PIIDetected,
			EntityTypes:    entityStrs,
			Treatment:      string(r.Treatment),
			RowsScanned:    0, // not exposed by presidio
			RowsTreated:    0, // not exposed by presidio
			OriginalLength: r.OriginalLength,
			TruncatedAt:    r.TruncatedAt,
		}
	}
	return out
}

type PIIPipeline struct {
	analyzer        *presidio.AnalyzerEngine
	anonymizer      *presidio.AnonymizerEngine
	structured      *presidio.StructuredAnalyzer
	minConfidence   float64
	defaultOperator presidio.Operator
	entityOperators map[presidio.EntityType]presidio.Operator
	hmacKey         []byte
}

type PIIPipelineConfig struct {
	HMACKeyEnv      string
	MinConfidence   float64
	DefaultOperator string
	EntityOperators map[string]string
	SampleSize      int
}

func buildRegistry() *presidio.RecognizerRegistry {
	registry := presidio.NewRecognizerRegistry()
	registry.Add(recognizers.NewEmailRecognizer())
	registry.Add(recognizers.NewPhoneRecognizer())
	registry.Add(recognizers.NewCreditCardRecognizer())
	registry.Add(recognizers.NewIPv4Recognizer())
	registry.Add(recognizers.NewIPv6Recognizer())
	registry.Add(recognizers.NewPersonRecognizer())
	registry.Add(recognizers.NewUkPostcodeRecognizer())
	registry.Add(recognizers.NewUkNinoRecognizer())
	registry.Add(recognizers.NewUKNHSRecognizer())
	registry.Add(recognizers.NewIbanRecognizer())
	registry.Add(recognizers.NewDobRecognizer())
	registry.Add(recognizers.NewUsSsnRecognizer())
	registry.Add(recognizers.NewPassportRecognizer())
	return registry
}

func NewPIIPipeline(cfg *PIIPipelineConfig) *PIIPipeline {
	p := &PIIPipeline{
		minConfidence:   0.5,
		defaultOperator: &presidio.RedactOperator{},
		entityOperators: make(map[presidio.EntityType]presidio.Operator),
		hmacKey:         []byte(os.Getenv("PRESIDIO_HMAC_KEY")),
	}

	sampleSize := 20 // default

	if cfg != nil {
		if cfg.HMACKeyEnv != "" {
			p.hmacKey = []byte(os.Getenv(cfg.HMACKeyEnv))
		}
		if cfg.MinConfidence > 0 {
			p.minConfidence = cfg.MinConfidence
		}
		if cfg.SampleSize > 0 {
			sampleSize = cfg.SampleSize
		}
		p.applyConfigOperators(cfg)
	}

	registry := buildRegistry()

	p.analyzer = presidio.NewAnalyzerEngine(presidio.AnalyzerConfig{
		Registry:     registry,
		MinScore:     p.minConfidence,
		Entities:     nil,
		ContextBoost: true,
	})

	p.anonymizer = presidio.NewAnonymizerEngine(presidio.AnonymizerConfig{
		Operators: p.buildOperatorMap(),
	})

	p.structured = presidio.NewStructuredAnalyzer(presidio.StructuredConfig{
		Analyzer:   p.analyzer,
		Anonymizer: p.anonymizer,
		Policies:   make(map[string]presidio.ColumnPolicy),
		SampleSize: sampleSize,
	})

	return p
}

func (p *PIIPipeline) applyConfigOperators(cfg *PIIPipelineConfig) {
	if cfg.DefaultOperator != "" {
		switch strings.ToLower(cfg.DefaultOperator) {
		case "redact":
			p.defaultOperator = &presidio.RedactOperator{}
		case "hash":
			if len(p.hmacKey) > 0 {
				p.defaultOperator = &presidio.HashOperator{}
			}
		case "mask":
			p.defaultOperator = &presidio.MaskOperator{}
		case "pseudonymise":
			if len(p.hmacKey) == 0 {
				break
			}
			if len(p.hmacKey) != 32 && len(p.hmacKey) != 64 {
				panic(fmt.Sprintf(
					"pseudonymise operator requires a 32 or 64 byte key (AES-128-SIV or AES-256-SIV); %s is %d bytes",
					cfg.HMACKeyEnv, len(p.hmacKey),
				))
			}
			p.defaultOperator = &presidio.PseudonymiseOperator{Key: p.hmacKey}
		}
	}

	for entity, op := range cfg.EntityOperators {
		entityType := presidio.EntityType(entity)
		switch strings.ToLower(op) {
		case "redact":
			p.entityOperators[entityType] = &presidio.RedactOperator{}
		case "hash":
			if len(p.hmacKey) > 0 {
				p.entityOperators[entityType] = &presidio.HashOperator{}
			}
		case "mask":
			p.entityOperators[entityType] = &presidio.MaskOperator{}
		case "pseudonymise":
			if len(p.hmacKey) == 0 {
				break
			}
			if len(p.hmacKey) != 32 && len(p.hmacKey) != 64 {
				panic(fmt.Sprintf(
					"pseudonymise operator requires a 32 or 64 byte key (AES-128-SIV or AES-256-SIV); %s is %d bytes",
					cfg.HMACKeyEnv, len(p.hmacKey),
				))
			}
			p.entityOperators[entityType] = &presidio.PseudonymiseOperator{Key: p.hmacKey}
		}
	}
}

func (p *PIIPipeline) buildOperatorMap() map[presidio.EntityType]presidio.Operator {
	operators := make(map[presidio.EntityType]presidio.Operator)

	entities := []presidio.EntityType{
		presidio.EntityEmailAddress,
		presidio.EntityPhoneNumber,
		presidio.EntityUkPostcode,
		presidio.EntityUkNino,
		presidio.EntityUkNhsNumber,
		presidio.EntityCreditCard,
		presidio.EntityIban,
		presidio.EntityIPv4,
		presidio.EntityIPv6,
		presidio.EntityDateOfBirth,
		presidio.EntityPerson,
		presidio.EntityUsSsn,
		presidio.EntityPassportNumber,
		presidio.EntityDriverLicence,
	}

	for _, entity := range entities {
		if op, ok := p.entityOperators[entity]; ok {
			operators[entity] = op
		} else {
			operators[entity] = p.defaultOperator
		}
	}

	return operators
}

// Resolve scans args for PII tokens and decrypts them in-place.
// A PII token is any string value with the prefix "pii:".
// Non-string values and non-token strings are returned unchanged.
// If the pipeline is not configured with a PseudonymiseOperator, all args
// pass through unchanged — this is correct, since no tokens will have been produced.
// Returns an error if any token fails AES-SIV authentication.
func (p *PIIPipeline) Resolve(args map[string]interface{}) (map[string]interface{}, error) {
	if len(args) == 0 {
		return args, nil
	}

	// Only PseudonymiseOperator produces reversible tokens.
	// If the default operator is not pseudonymise, pass through unchanged.
	op, isPseudonymise := p.defaultOperator.(*presidio.PseudonymiseOperator)
	if !isPseudonymise {
		return args, nil
	}

	resolved := make(map[string]interface{}, len(args))
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if strings.HasPrefix(val, "pii:") {
				plaintext, err := op.Decrypt(val)
				if err != nil {
					return nil, fmt.Errorf("pii resolve: arg %q: %w", k, err)
				}
				resolved[k] = plaintext
			} else {
				resolved[k] = val
			}
		case map[string]interface{}:
			// Recurse one level for nested param maps (e.g., params: {surname: "pii:..."})
			inner, err := p.Resolve(val)
			if err != nil {
				return nil, fmt.Errorf("pii resolve: nested arg %q: %w", k, err)
			}
			resolved[k] = inner
		default:
			resolved[k] = v
		}
	}
	return resolved, nil
}

func (p *PIIPipeline) Process(result ToolResult) ToolResult {
	if result.Meta.PIIScanApplied {
		return result
	}

	result.Meta.PIIScanApplied = true

	if result.Data != nil {
		return p.processStructuredData(result)
	}

	if result.RawText != "" {
		return p.processRawText(result)
	}

	return result
}

func (p *PIIPipeline) processRawText(result ToolResult) ToolResult {
	results := p.analyzer.AnalyseText(result.RawText)

	if len(results) == 0 {
		result.Meta.SafetyNote = "no pii detected"
		return result
	}

	var detectedEntities []presidio.EntityType
	for _, r := range results {
		detectedEntities = append(detectedEntities, r.EntityType)
	}

	result.Meta.SafetyNote = "pii detected and treated"

	treated := result.RawText
	for _, entity := range detectedEntities {
		operator := p.defaultOperator
		if op, ok := p.entityOperators[entity]; ok {
			operator = op
		}
		treated = operator.Anonymise(treated, 0, len(treated), entity)
	}

	result.RawText = treated

	return result
}

func (p *PIIPipeline) processStructuredData(result ToolResult) ToolResult {
	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		result.Meta.SafetyNote = "data not processable as rows"
		return result
	}
	if len(rows) == 0 {
		return result
	}

	processedRows, presidioReports := p.structured.ProcessRows(rows, toPresidioHints(result.ColumnHints))
	result.Data = processedRows

	columnReports := make([]ColumnReport, 0, len(presidioReports))
	for _, pr := range presidioReports {
		cr := ColumnReport{
			ColumnName:     pr.ColumnName,
			PIIDetected:    pr.PIIDetected,
			Treatment:      string(pr.Treatment),
			OriginalLength: pr.OriginalLength,
			TruncatedAt:    pr.TruncatedAt,
		}

		for _, et := range pr.PIIEntities {
			cr.EntityTypes = append(cr.EntityTypes, string(et))
		}
		if cr.EntityTypes == nil {
			cr.EntityTypes = []string{}
		}

		if hint, ok := result.ColumnHints[pr.ColumnName]; ok {
			cr.DataType = hint.DataType
			cr.ScanPolicyName = scanPolicyName(int(hint.ScanPolicy))
		}

		if cr.PIIDetected {
			cr.RowsScanned = len(rows)
			cr.RowsTreated = len(rows)
		}

		columnReports = append(columnReports, cr)
	}

	reportedCols := make(map[string]bool, len(columnReports))
	for _, cr := range columnReports {
		reportedCols[cr.ColumnName] = true
	}
	for colName, hint := range result.ColumnHints {
		if !reportedCols[colName] {
			columnReports = append(columnReports, ColumnReport{
				ColumnName:     colName,
				PIIDetected:    false,
				EntityTypes:    []string{},
				DataType:       hint.DataType,
				ScanPolicyName: scanPolicyName(int(hint.ScanPolicy)),
			})
		}
	}

	result.Meta.ColumnReports = columnReports

	var piiColumns []string
	var truncatedColumns []TruncationNote
	for _, cr := range columnReports {
		if cr.PIIDetected {
			piiColumns = append(piiColumns, cr.ColumnName)
		}
		if cr.TruncatedAt > 0 {
			truncatedColumns = append(truncatedColumns, TruncationNote{
				Column:         cr.ColumnName,
				OriginalLength: cr.OriginalLength,
				TruncatedAt:    cr.TruncatedAt,
			})
		}
	}
	result.Meta.Truncations = truncatedColumns
	result.Meta.PIIScanApplied = true

	if len(piiColumns) == 0 {
		result.Meta.SafetyNote = "no pii detected in structured data"
	} else {
		result.Meta.SafetyNote = "pii detected in columns: " + strings.Join(piiColumns, ", ")
	}

	return result
}

// scanPolicyName returns the human-readable name for a ScanPolicy constant.
func scanPolicyName(policy int) string {
	switch ScanPolicy(policy) {
	case ScanPolicySafe:
		return "safe"
	case ScanPolicyFull:
		return "full"
	case ScanPolicyNameOnly:
		return "name_only"
	case ScanPolicyTruncateThenScan:
		return "truncate_then_scan"
	case ScanPolicyStrip:
		return "strip"
	default:
		return "default"
	}
}
