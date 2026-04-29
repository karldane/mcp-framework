package framework

import (
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

	if cfg != nil {
		if cfg.HMACKeyEnv != "" {
			p.hmacKey = []byte(os.Getenv(cfg.HMACKeyEnv))
		}
		if cfg.MinConfidence > 0 {
			p.minConfidence = cfg.MinConfidence
		}
		p.applyConfigOperators(cfg)

		if cfg.SampleSize > 0 {
			p.structured = presidio.NewStructuredAnalyzer(presidio.StructuredConfig{
				SampleSize: cfg.SampleSize,
			})
		}
	}

	registry := buildRegistry()

	aCfg := presidio.AnalyzerConfig{
		Registry:     registry,
		MinScore:     p.minConfidence,
		Entities:     nil,
		ContextBoost: true,
	}
	p.analyzer = presidio.NewAnalyzerEngine(aCfg)

	anonymizerCfg := presidio.AnonymizerConfig{
		Operators: p.buildOperatorMap(),
	}
	p.anonymizer = presidio.NewAnonymizerEngine(anonymizerCfg)

	if p.structured == nil {
		policy := make(map[string]presidio.ColumnPolicy)
		p.structured = presidio.NewStructuredAnalyzer(presidio.StructuredConfig{
			Analyzer:   p.analyzer,
			Anonymizer: p.anonymizer,
			Policies:   policy,
			SampleSize: 20,
		})
	} else {
		p.structured = presidio.NewStructuredAnalyzer(presidio.StructuredConfig{
			Analyzer:   p.analyzer,
			Anonymizer: p.anonymizer,
			Policies:   make(map[string]presidio.ColumnPolicy),
			SampleSize: 20,
		})
	}

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
			if len(p.hmacKey) > 0 {
				p.defaultOperator = &presidio.PseudonymiseOperator{}
			}
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
			if len(p.hmacKey) > 0 {
				p.entityOperators[entityType] = &presidio.PseudonymiseOperator{}
			}
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
	result.Meta.ColumnReports = fromPresidioReports(presidioReports)

	var piiColumns []string
	var truncatedColumns []TruncationNote

	for _, report := range result.Meta.ColumnReports {
		if report.PIIDetected {
			piiColumns = append(piiColumns, report.ColumnName)
		}
		if report.Treatment == string(presidio.TreatmentTruncated) {
			truncatedColumns = append(truncatedColumns, TruncationNote{
				Column:         report.ColumnName,
				OriginalLength: report.OriginalLength,
				TruncatedAt:    report.TruncatedAt,
			})
		}
	}

	result.Meta.Truncations = truncatedColumns

	if len(piiColumns) == 0 {
		result.Meta.SafetyNote = "no pii detected in structured data"
	} else {
		result.Meta.SafetyNote = "pii detected in columns: " + strings.Join(piiColumns, ", ")
	}

	return result
}
